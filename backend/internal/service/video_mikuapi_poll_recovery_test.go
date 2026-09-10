package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMikuapiPollTemporaryResponses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		retry  bool
	}{
		{"400 without code", 400, `{"message":"Video task is temporarily unavailable, please retry later."}`, true},
		{"400 query_failed", 400, `{"code":"query_failed","message":"Video task is temporarily unavailable,please retry later."}`, true},
		{"200 nested error", 200, `{"error":{"message":"Video task is temporarily unavailable, please retry later."}}`, true},
		{"200 failed query", 200, `{"status":"failed","error":{"code":"unknown","message":"Video task is temporarily unavailable, please retry later."}}`, true},
		{"200 data error", 200, `{"data":{"status":"failed","error":{"message":"Video task is temporarily unavailable, please retry later."}}}`, true},
		{"200 malformed JSON", 200, `<html>upstream unavailable</html>`, true},
		{"404 propagation", 404, `{}`, true},
		{"408 timeout", 408, `{}`, true},
		{"429 limit", 429, `{}`, true},
		{"502 proxy", 502, `<html>Bad gateway</html>`, true},
		{"401 credentials", 401, `{"message":"temporarily unavailable"}`, false},
		{"403 credentials", 403, `{"message":"temporarily unavailable"}`, false},
		{"400 permanent", 400, `{"error":{"message":"Invalid task ID"}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, "/v1/videos/original-task", r.URL.Path)
				require.Equal(t, "Bearer sk-miku-test", r.Header.Get("Authorization"))
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			result, err := (&VideoService{}).pollUpstreamTask(context.Background(), newMikuapiTestAccount(server.URL), "original-task")
			require.Error(t, err)
			require.Nil(t, result)
			require.Equal(t, tc.retry, mapVideoUpstreamError(err, true).Retryable)
			require.False(t, mapVideoUpstreamError(err, false).Retryable, "query classification must never retry task creation")
		})
	}
}

func TestMikuapiPollDefinitiveFailureAndNestedCompletion(t *testing.T) {
	for _, tc := range []struct{ body, status string }{
		{`{"status":"failed","error":{"message":"Content rejected"}}`, VideoTaskStatusFailed},
		{`{"status":"failed","error":{"message":"Video generation failed. Please retry later."}}`, VideoTaskStatusFailed},
		{`{"status":"cancelled"}`, VideoTaskStatusCancelled},
		{`{"data":{"status":"completed","video_url":"https://cdn.example/result.mp4"}}`, VideoTaskStatusCompleted},
	} {
		server := newMikuapiPollServer(t, tc.body)
		result, err := (&VideoService{}).pollUpstreamTask(context.Background(), newMikuapiTestAccount(server.URL), "original-task")
		require.NoError(t, err)
		require.Equal(t, tc.status, result.Status)
		if tc.status == VideoTaskStatusCompleted {
			require.Equal(t, "https://cdn.example/result.mp4", result.VideoURL)
		}
	}
}

func TestMikuapiPollingClassificationDoesNotChangeOtherProviders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"message":"Video task is temporarily unavailable, please retry later."}`))
	}))
	defer server.Close()
	account := newMikuapiTestAccount(server.URL)
	account.Extra["video_provider"] = videoProviderAigod
	_, err := (&VideoService{}).pollUpstreamTask(context.Background(), account, "original-task")
	require.Error(t, err)
	require.False(t, mapVideoUpstreamError(err, true).Retryable)
}

func TestMikuapiPollRetryBackoff(t *testing.T) {
	for i, expected := range []time.Duration{5, 10, 20, 30, 30, 30} {
		require.Equal(t, expected*time.Second, mikuapiPollRetryDelay(5*time.Second, i+1))
	}
}

func TestMikuapiPollLifecycleRecoversWithoutFailureOrRecreation(t *testing.T) {
	repo := newVideoTaskMemoryRepo()
	_, err := repo.Create(context.Background(), &VideoTaskCreateInput{PublicID: "video-recovery", Status: VideoTaskStatusProcessing})
	require.NoError(t, err)
	var queries atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method, "must not recreate an upstream task")
		require.Equal(t, "/v1/videos/original-task", r.URL.Path)
		task, readErr := repo.GetByPublicID(context.Background(), "video-recovery")
		require.NoError(t, readErr)
		require.Equal(t, VideoTaskStatusProcessing, task.Status)
		require.Nil(t, task.ErrorJSON)
		require.Nil(t, task.RefundedAt)
		if queries.Add(1) <= 8 {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"message":"Video task is temporarily unavailable, please retry later."}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"completed","video_url":"https://cdn.example/result.mp4"}`))
	}))
	defer server.Close()
	account := newMikuapiTestAccount(server.URL)
	account.Extra["poll_interval_ms"] = 1
	account.Extra["poll_timeout_ms"] = 5000
	service := &VideoService{taskRepo: repo, videoResultPublisher: &recordingVideoResultPublisher{returnedURL: "https://relay.example/result.mp4"}}
	service.pollLifecycle(VideoTaskLifecycleInput{PublicID: "video-recovery", Account: account, APIKey: &APIKey{ID: 1, User: &User{ID: 1}, Group: &Group{ID: 1}}}, "original-task")
	task, err := repo.GetByPublicID(context.Background(), "video-recovery")
	require.NoError(t, err)
	require.Equal(t, int64(9), queries.Load())
	require.Equal(t, VideoTaskStatusCompleted, task.Status)
	require.Nil(t, task.ErrorJSON)
	require.Nil(t, task.RefundedAt)
	require.Equal(t, "https://relay.example/result.mp4", *task.ResultVideoURL)
}

func TestMikuapiPollLifecycleOutageRespectsDeadline(t *testing.T) {
	repo := newVideoTaskMemoryRepo()
	_, err := repo.Create(context.Background(), &VideoTaskCreateInput{PublicID: "video-timeout", Status: VideoTaskStatusProcessing})
	require.NoError(t, err)
	var queries atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries.Add(1)
		w.WriteHeader(400)
		_, _ = fmt.Fprint(w, `{"message":"Video task is temporarily unavailable, please retry later."}`)
	}))
	defer server.Close()
	account := newMikuapiTestAccount(server.URL)
	account.Extra["poll_interval_ms"] = 1
	account.Extra["poll_timeout_ms"] = 40
	started := time.Now()
	(&VideoService{taskRepo: repo}).pollLifecycle(VideoTaskLifecycleInput{PublicID: "video-timeout", Account: account}, "original-task")
	require.GreaterOrEqual(t, time.Since(started), 40*time.Millisecond)
	require.Less(t, time.Since(started), time.Second)
	require.Greater(t, queries.Load(), int64(1))
	task, err := repo.GetByPublicID(context.Background(), "video-timeout")
	require.NoError(t, err)
	require.Equal(t, VideoTaskStatusFailed, task.Status)
}
