package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type recordingVideoResultPublisher struct {
	returnedURL string
	err         error
	owner       TemporaryAssetOwner
	baseURL     string
	upstreamURL string
}

func (p *recordingVideoResultPublisher) PublishGeneratedVideo(
	_ context.Context,
	owner TemporaryAssetOwner,
	fallbackPublicBaseURL string,
	upstreamURL string,
) (string, error) {
	p.owner = owner
	p.baseURL = fallbackPublicBaseURL
	p.upstreamURL = upstreamURL
	if p.err != nil {
		return "", p.err
	}
	return p.returnedURL, nil
}

func TestVideoServicePublishesUpstreamResultWithoutFallback(t *testing.T) {
	groupID := int64(33)
	apiKey := &APIKey{ID: 22, User: &User{ID: 11}, Group: &Group{ID: groupID}}
	publisher := &recordingVideoResultPublisher{returnedURL: "https://sub2api.example.com/media/result/asset.mp4"}
	service := &VideoService{videoResultPublisher: publisher}
	input := VideoTaskLifecycleInput{
		APIKey:              apiKey,
		ResultPublicBaseURL: "https://sub2api.example.com",
	}

	resultURL, err := service.publishVideoResult(context.Background(), input, "https://supplier.example.com/original.mp4?token=secret")
	require.NoError(t, err)
	require.Equal(t, publisher.returnedURL, resultURL)
	require.Equal(t, TemporaryAssetOwner{UserID: 11, APIKeyID: 22, GroupID: 33}, publisher.owner)
	require.Equal(t, input.ResultPublicBaseURL, publisher.baseURL)
	require.Equal(t, "https://supplier.example.com/original.mp4?token=secret", publisher.upstreamURL)
	require.NotContains(t, resultURL, "supplier.example.com")

	publisher.err = errors.New("storage failed")
	resultURL, err = service.publishVideoResult(context.Background(), input, publisher.upstreamURL)
	require.Error(t, err)
	require.Empty(t, resultURL)
}

func TestNormalizeVideoCreateRequestBuildsSeedancePayloadAndBillingSeconds(t *testing.T) {
	generateAudio := true
	req := &VideoCreateRequest{
		Model:         VideoModelSeedance20Fast,
		Prompt:        "a cinematic shot",
		Duration:      8,
		Resolution:    VideoResolution480P,
		AspectRatio:   "16:9",
		GenerateAudio: &generateAudio,
		Content: []VideoContent{
			{
				Type:     "text",
				Text:     "a cinematic shot",
				Role:     "",
				ImageURL: nil,
			},
			{
				Type: "image_url",
				Role: "reference_image",
				ImageURL: &VideoContentURL{
					URL: "https://cdn.example.com/ref.png",
				},
			},
			{
				Type:            "video_url",
				Role:            "reference_video",
				VideoURL:        &VideoContentURL{URL: "https://cdn.example.com/ref.mp4"},
				DurationSeconds: float64PtrForVideoTest(5.2),
			},
			{
				Type:            "video_url",
				Role:            "reference_video",
				VideoURL:        &VideoContentURL{URL: "https://cdn.example.com/ref2.mp4"},
				DurationSeconds: float64PtrForVideoTest(7),
			},
		},
	}

	normalized, err := normalizeVideoCreateRequest(req)
	require.NoError(t, err)
	require.Equal(t, videoAbilityReferenceToVideo, normalized.AbilityCode)
	require.Equal(t, 8, normalized.GeneratedSeconds)
	require.Equal(t, 13, normalized.ReferenceVideoSeconds)
	require.Equal(t, 21, normalized.BillableSeconds)

	body := normalized.UpstreamBody(SeedanceUpstreamModel(normalized.Model, normalized.Resolution))
	require.Equal(t, "seedance-2.0-fast-480p", body["model"])
	require.Equal(t, "a cinematic shot", body["prompt"])
	require.Equal(t, "16:9", body["ratio"])
	require.Equal(t, 8, body["duration"])
	require.Equal(t, true, body["generate_audio"])

	content, ok := body["content"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, content, 4)
	require.Equal(t, map[string]any{"type": "text", "text": "a cinematic shot"}, content[0])
	require.Equal(t, "reference_image", content[1]["role"])
	require.Equal(t, "reference_video", content[2]["role"])
	require.NotContains(t, body, "resolution")
	require.NotContains(t, body, "ability_code")
}

func TestJingyuUpstreamBodyMapsSeedanceRequest(t *testing.T) {
	generateAudio := true
	req := &VideoCreateRequest{
		Model:         VideoModelSeedance20,
		Prompt:        "runway shot",
		Duration:      5,
		Resolution:    VideoResolution720P,
		AspectRatio:   "9:16",
		GenerateAudio: &generateAudio,
		Content: []VideoContent{
			{
				Type:     "image_url",
				Role:     "reference_image",
				ImageURL: &VideoContentURL{URL: "https://cdn.example.com/fashion.png"},
			},
			{
				Type:     "audio_url",
				Role:     "reference_audio",
				AudioURL: &VideoContentURL{URL: "https://cdn.example.com/music.mp3"},
			},
		},
		Raw: map[string]any{"seed": float64(12345), "extra": map[string]any{"must_not": "forward"}},
	}

	normalized, err := normalizeVideoCreateRequest(req)
	require.NoError(t, err)
	body := normalized.JingyuUpstreamBody(videoJingyuSeedance20Model)

	require.Equal(t, "yu-video-2-pro", body["model"])
	require.Equal(t, "runway shot", body["prompt"])
	require.Equal(t, 5, body["duration"])
	require.Equal(t, "9:16", body["aspect_ratio"])
	require.Equal(t, VideoResolution720P, body["resolution"])
	require.Equal(t, true, body["generate_audio"])
	require.Equal(t, float64(12345), body["seed"])
	require.NotContains(t, body, "content")
	require.NotContains(t, body, "ratio")

	refs, ok := body["references"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, refs, 2)
	require.Equal(t, map[string]any{
		"type": "image",
		"role": "reference_image",
		"url":  "https://cdn.example.com/fashion.png",
	}, refs[0])
	require.NotContains(t, refs[0], "subject_type")
	require.Equal(t, "audio", refs[1]["type"])
	require.NotContains(t, body, "extra")
}

func TestJingyuUpstreamBodyForwardsSupportedResolutions(t *testing.T) {
	adapter := jingyuVideoProviderAdapter{}
	for _, resolution := range []string{
		VideoResolution480P,
		VideoResolution720P,
		VideoResolution1080P,
		VideoResolution4K,
	} {
		t.Run(resolution, func(t *testing.T) {
			require.True(t, adapter.Compatible(VideoModelSeedance20, resolution))
			normalized, err := normalizeVideoCreateRequest(&VideoCreateRequest{
				Model:      VideoModelSeedance20,
				Prompt:     "runway shot",
				Duration:   5,
				Resolution: resolution,
			})
			require.NoError(t, err)

			body := normalized.JingyuUpstreamBody(videoJingyuSeedance20Model)
			require.Equal(t, strings.ToLower(resolution), body["resolution"])
			require.Equal(t, "yu-video-2-pro", body["model"])
		})
	}
	require.False(t, adapter.Compatible(VideoModelSeedance20Fast, VideoResolution720P))
}

func TestNormalizeVideoCreateRequestRequiresReferenceVideoDuration(t *testing.T) {
	req := &VideoCreateRequest{
		Model:      VideoModelSeedance20,
		Prompt:     "a cinematic shot",
		Duration:   8,
		Resolution: VideoResolution720P,
		Content: []VideoContent{
			{
				Type:     "video_url",
				Role:     "reference_video",
				VideoURL: &VideoContentURL{URL: "https://cdn.example.com/ref.mp4"},
			},
		},
	}

	_, err := normalizeVideoCreateRequest(req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "reference_video_duration_required")
}

func TestNormalizeVideoCreateRequestRejectsUnsupportedDuration(t *testing.T) {
	for _, tc := range []struct {
		name     string
		duration float64
	}{
		{name: "too short", duration: 3},
		{name: "too long", duration: 16},
		{name: "fractional", duration: 8.5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &VideoCreateRequest{
				Model:      VideoModelSeedance20,
				Prompt:     "a cinematic shot",
				Duration:   tc.duration,
				Resolution: VideoResolution720P,
			}

			_, err := normalizeVideoCreateRequest(req)
			require.Error(t, err)
			require.Contains(t, err.Error(), "invalid_video_duration")
		})
	}
}

func TestNormalizeVideoCreateRequestRejectsReferenceVideoDurationOutsideOfficialLimits(t *testing.T) {
	req := &VideoCreateRequest{
		Model:      VideoModelSeedance20,
		Prompt:     "a cinematic shot",
		Duration:   8,
		Resolution: VideoResolution720P,
		Content: []VideoContent{
			{
				Type:            "video_url",
				Role:            "reference_video",
				VideoURL:        &VideoContentURL{URL: "https://cdn.example.com/ref.mp4"},
				DurationSeconds: float64PtrForVideoTest(16),
			},
		},
	}

	_, err := normalizeVideoCreateRequest(req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid_reference_video_duration")
}

func TestNormalizeVideoCreateRequestRejectsReferenceVideoTotalDurationAboveOfficialLimit(t *testing.T) {
	req := &VideoCreateRequest{
		Model:      VideoModelSeedance20,
		Prompt:     "a cinematic shot",
		Duration:   8,
		Resolution: VideoResolution720P,
		Content: []VideoContent{
			{
				Type:            "video_url",
				Role:            "reference_video",
				VideoURL:        &VideoContentURL{URL: "https://cdn.example.com/ref.mp4"},
				DurationSeconds: float64PtrForVideoTest(8),
			},
			{
				Type:            "video_url",
				Role:            "reference_video",
				VideoURL:        &VideoContentURL{URL: "https://cdn.example.com/ref2.mp4"},
				DurationSeconds: float64PtrForVideoTest(8),
			},
		},
	}

	_, err := normalizeVideoCreateRequest(req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid_reference_video_duration")
}

func TestNormalizeVideoCreateRequestSupportsSeedance1080P(t *testing.T) {
	req := &VideoCreateRequest{
		Model:      VideoModelSeedance20,
		Prompt:     "a cinematic shot",
		Duration:   8,
		Resolution: VideoResolution1080P,
	}

	normalized, err := normalizeVideoCreateRequest(req)
	require.NoError(t, err)
	require.Equal(t, VideoResolution1080P, normalized.Resolution)
	require.Equal(t, "seedance-2.0-1080p", SeedanceUpstreamModel(normalized.Model, normalized.Resolution))
}

func TestNormalizeVideoCreateRequestSupportsSeedance4K(t *testing.T) {
	req := &VideoCreateRequest{
		Model:      VideoModelSeedance20,
		Prompt:     "a cinematic shot",
		Duration:   8,
		Resolution: VideoResolution4K,
	}

	normalized, err := normalizeVideoCreateRequest(req)
	require.NoError(t, err)
	require.Equal(t, VideoResolution4K, normalized.Resolution)
	require.Equal(t, "seedance-2.0-4K", SeedanceUpstreamModel(normalized.Model, normalized.Resolution))
}

func TestNormalizeVideoCreateRequestRejectsFast1080P(t *testing.T) {
	req := &VideoCreateRequest{
		Model:      VideoModelSeedance20Fast,
		Prompt:     "a cinematic shot",
		Duration:   8,
		Resolution: VideoResolution1080P,
	}

	_, err := normalizeVideoCreateRequest(req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid_video_resolution")
}

func TestNormalizeVideoCreateRequestSupportsSeedance25ProviderContracts(t *testing.T) {
	req := &VideoCreateRequest{
		Model:       VideoModelSeedance25,
		Prompt:      "bring the portrait to life",
		Duration:    -1,
		Resolution:  VideoResolution720P,
		AspectRatio: "adaptive",
		Content: []VideoContent{
			{
				Type:     "image_url",
				Role:     "first_frame",
				ImageURL: &VideoContentURL{URL: "https://cdn.example.com/person.png"},
			},
		},
	}

	normalized, err := normalizeVideoCreateRequest(req)
	require.NoError(t, err)
	require.Equal(t, VideoModelSeedance25, normalized.Model)
	require.Equal(t, 5, normalized.GeneratedSeconds)
	require.Equal(t, float64(5), normalized.Duration)
	require.Equal(t, "auto", normalized.Ratio)
	require.Equal(t, videoAbilityImageToVideo, normalized.AbilityCode)

	adapter := aigodVideoProviderAdapter{}
	require.True(t, adapter.Compatible(VideoModelSeedance25, VideoResolution720P))
	require.False(t, adapter.Compatible(VideoModelSeedance25, VideoResolution1080P))
	require.False(t, adapter.Compatible(VideoModelSeedance25, VideoResolution4K))
	jingyuAdapter := jingyuVideoProviderAdapter{}
	require.True(t, jingyuAdapter.Compatible(VideoModelSeedance25, VideoResolution480P))
	require.True(t, jingyuAdapter.Compatible(VideoModelSeedance25, VideoResolution720P))
	require.False(t, jingyuAdapter.Compatible(VideoModelSeedance25, VideoResolution1080P))
	require.False(t, jingyuAdapter.Compatible(VideoModelSeedance25, VideoResolution4K))

	body := adapter.BuildCreateBody(normalized, adapter.UpstreamModel(nil, normalized))
	require.Equal(t, "seedance-2.5-720p", body["model"])
	require.Equal(t, 5, body["duration"])
	require.Equal(t, "auto", body["ratio"])
	content, ok := body["content"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, content, 2)
	require.Equal(t, "person", content[1]["subject_type"])

	jingyuBody := jingyuAdapter.BuildCreateBody(normalized, jingyuAdapter.UpstreamModel(nil, normalized))
	require.Equal(t, "yu-video-2.5-pro", jingyuBody["model"])
	require.Equal(t, VideoResolution720P, jingyuBody["resolution"])
	require.Equal(t, "auto", jingyuBody["aspect_ratio"])
	require.Equal(t, float64(-1), jingyuBody["duration"])
	require.NotContains(t, jingyuBody, "content")
	references, ok := jingyuBody["references"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, references, 1)
	require.Equal(t, "image", references[0]["type"])
	require.Equal(t, "first_frame", references[0]["role"])
}

func TestYCYAPIVideoProviderMapsModelsAndFiltersProviderSpecificLimits(t *testing.T) {
	adapter := ycyapiVideoProviderAdapter{}
	require.Equal(t, videoYCYAPISeedance20Model, adapter.UpstreamModel(nil, &normalizedVideoRequest{Model: VideoModelSeedance20}))
	require.Equal(t, videoYCYAPISeedance20FastModel, adapter.UpstreamModel(nil, &normalizedVideoRequest{Model: VideoModelSeedance20Fast}))
	require.Equal(t, videoYCYAPISeedance25Model, adapter.UpstreamModel(nil, &normalizedVideoRequest{Model: VideoModelSeedance25}))
	require.True(t, adapter.Compatible(VideoModelSeedance20, VideoResolution1080P))
	require.False(t, adapter.Compatible(VideoModelSeedance20, VideoResolution4K))

	standard := &normalizedVideoRequest{
		Model: VideoModelSeedance20, Resolution: VideoResolution720P,
		GeneratedSeconds: 10, Ratio: "21:9", RatioProvided: true,
	}
	require.True(t, adapter.CompatibleRequest(standard))
	standard.GeneratedSeconds = 8
	require.False(t, adapter.CompatibleRequest(standard))

	leonardo := &normalizedVideoRequest{
		Model: VideoModelSeedance25, Resolution: VideoResolution720P,
		GeneratedSeconds: 19, Ratio: "16:9", RatioProvided: true,
		Content: []VideoContent{{
			Type: "video_url", Role: "reference_video", VideoURL: &VideoContentURL{URL: "https://cdn.example.com/ref.mp4"},
			DurationSeconds: videoFloat64Ptr(5),
		}},
	}
	require.False(t, adapter.CompatibleRequest(leonardo))
	leonardo.GeneratedSeconds = 18
	require.True(t, adapter.CompatibleRequest(leonardo))
	leonardo.Content = []VideoContent{{Type: "audio_url", Role: "reference_audio", AudioURL: &VideoContentURL{URL: "https://cdn.example.com/ref.mp3"}}}
	require.False(t, adapter.CompatibleRequest(leonardo))
}

func TestYCYAPIAdapterBuildsJSONWithoutChangingCanonicalRequest(t *testing.T) {
	normalized, err := normalizeVideoCreateRequest(&VideoCreateRequest{
		Model: VideoModelSeedance20Fast, Prompt: "move", Duration: 10,
		Resolution: VideoResolution720P, AspectRatio: "9:16", GenerateAudio: boolPtr(true),
		Raw: map[string]any{"seed": float64(42), "negative_prompt": "watermark"},
	})
	require.NoError(t, err)
	adapter := ycyapiVideoProviderAdapter{}
	body := adapter.BuildCreateBody(normalized, adapter.UpstreamModel(nil, normalized))
	require.Equal(t, videoYCYAPISeedance20FastModel, body["model"])
	require.Equal(t, 10, body["duration"])
	require.Equal(t, VideoResolution720P, body["resolution"])
	require.Equal(t, "9:16", body["aspect_ratio"])
	require.Equal(t, "watermark", body["negative_prompt"])
	require.NotContains(t, body, "content")
	require.NotContains(t, body, ycyapiContentBodyKey)

	reader, contentType, done, err := (&VideoService{}).buildYCYAPICreateRequest(context.Background(), body)
	require.NoError(t, err)
	require.Nil(t, done)
	require.Equal(t, "application/json", contentType)
	raw, err := io.ReadAll(reader)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))
	require.Equal(t, videoYCYAPISeedance20FastModel, payload["model"])
	require.NotContains(t, payload, ycyapiContentBodyKey)
}

func TestYCYAPIAdapterMaterializesCanonicalMediaAsMultipart(t *testing.T) {
	assetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("\x89PNG\r\n\x1a\nreference"))
	}))
	t.Cleanup(assetServer.Close)

	normalized, err := normalizeVideoCreateRequest(&VideoCreateRequest{
		Model: VideoModelSeedance25, Prompt: "move", Duration: 10,
		Resolution: VideoResolution720P, AspectRatio: "16:9",
		Content: []VideoContent{{
			Type: "image_url", Role: "first_frame", ImageURL: &VideoContentURL{URL: assetServer.URL + "/first.png"},
		}},
	})
	require.NoError(t, err)
	adapter := ycyapiVideoProviderAdapter{}
	body := adapter.BuildCreateBody(normalized, adapter.UpstreamModel(nil, normalized))
	service := &VideoService{videoReferenceClient: assetServer.Client()}
	reader, contentType, done, err := service.buildYCYAPICreateRequest(context.Background(), body)
	require.NoError(t, err)
	raw, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, <-done)

	mediaType, params, err := mime.ParseMediaType(contentType)
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", mediaType)
	form, err := multipart.NewReader(strings.NewReader(string(raw)), params["boundary"]).ReadForm(1 << 20)
	require.NoError(t, err)
	defer form.RemoveAll()
	require.Equal(t, []string{videoYCYAPISeedance25Model}, form.Value["model"])
	require.Equal(t, []string{"10"}, form.Value["duration"])
	require.Len(t, form.File["first_frame"], 1)
	require.Equal(t, "image/png", form.File["first_frame"][0].Header.Get("Content-Type"))
	require.Empty(t, form.Value[ycyapiContentBodyKey])
}

func TestYCYAPIUpstreamLifecycleUsesBearerCreateAndDownloadURLPolling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer sk-ycy-test", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/videos":
			var payload map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			require.Equal(t, videoYCYAPISeedance20Model, payload["model"])
			_, _ = w.Write([]byte(`{"id":"task-ycy-1","status":"queued"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/videos/task-ycy-1":
			_, _ = w.Write([]byte(`{"id":"task-ycy-1","status":"completed","download_url":"https://ycyapi.cn/v1/videos/task-ycy-1/content"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	account := &Account{
		ID: 42,
		Extra: map[string]any{
			"video_provider": videoProviderYCYAPI,
			"base_url":       server.URL,
			"api_path":       videoDefaultAPIPath,
		},
		Credentials: map[string]any{"api_key": "sk-ycy-test"},
	}
	service := &VideoService{}
	created, err := service.createUpstreamTask(context.Background(), account, map[string]any{
		"model": videoYCYAPISeedance20Model, "prompt": "move", "duration": 5, "resolution": VideoResolution720P,
	})
	require.NoError(t, err)
	require.Equal(t, "task-ycy-1", created.ID)

	polled, err := service.pollUpstreamTask(context.Background(), account, created.ID)
	require.NoError(t, err)
	require.Equal(t, VideoTaskStatusCompleted, polled.Status)
	require.Equal(t, "https://ycyapi.cn/v1/videos/task-ycy-1/content", polled.VideoURL)
}

func TestNormalizeVideoCreateRequestSupportsSeedance25DurationAndRatioLimits(t *testing.T) {
	for _, duration := range []float64{4, 30} {
		normalized, err := normalizeVideoCreateRequest(&VideoCreateRequest{
			Model:      VideoModelSeedance25,
			Prompt:     "a cinematic shot",
			Duration:   duration,
			Resolution: VideoResolution480P,
			Ratio:      "21:9",
		})
		require.NoError(t, err)
		require.Equal(t, int(duration), normalized.GeneratedSeconds)
	}

	for _, tc := range []struct {
		name       string
		duration   float64
		resolution string
		ratio      string
	}{
		{name: "duration too short", duration: 3, resolution: VideoResolution720P, ratio: "16:9"},
		{name: "duration too long", duration: 31, resolution: VideoResolution720P, ratio: "16:9"},
		{name: "fractional duration", duration: 4.5, resolution: VideoResolution720P, ratio: "16:9"},
		{name: "unsupported 1080p", duration: 5, resolution: VideoResolution1080P, ratio: "16:9"},
		{name: "unsupported 4K", duration: 5, resolution: VideoResolution4K, ratio: "16:9"},
		{name: "unsupported ratio", duration: 5, resolution: VideoResolution720P, ratio: "2:1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := normalizeVideoCreateRequest(&VideoCreateRequest{
				Model:      VideoModelSeedance25,
				Prompt:     "a cinematic shot",
				Duration:   tc.duration,
				Resolution: tc.resolution,
				Ratio:      tc.ratio,
			})
			require.Error(t, err)
		})
	}
}

func TestNormalizeVideoCreateRequestSupportsSeedance25AudioOnlyReference(t *testing.T) {
	normalized, err := normalizeVideoCreateRequest(&VideoCreateRequest{
		Model:       VideoModelSeedance25,
		Prompt:      "follow the music rhythm",
		Duration:    8,
		Resolution:  VideoResolution480P,
		AbilityCode: videoAbilityReferenceToVideo,
		Content: []VideoContent{
			{
				Type:     "audio_url",
				Role:     "reference_audio",
				AudioURL: &VideoContentURL{URL: "https://cdn.example.com/music.mp3"},
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, videoAbilityReferenceToVideo, normalized.AbilityCode)

	_, err = normalizeVideoCreateRequest(&VideoCreateRequest{
		Model:       VideoModelSeedance25,
		Prompt:      "missing reference",
		Duration:    8,
		Resolution:  VideoResolution480P,
		AbilityCode: videoAbilityReferenceToVideo,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid_video_content")
}

func TestNormalizeVideoCreateRequestEnforcesSeedance25ReferenceLimits(t *testing.T) {
	content := make([]VideoContent, 0, 50)
	for index := 0; index < 30; index++ {
		content = append(content, VideoContent{
			Type:     "image_url",
			Role:     "reference_image",
			ImageURL: &VideoContentURL{URL: fmt.Sprintf("https://cdn.example.com/image-%d.png", index)},
		})
	}
	for index := 0; index < 10; index++ {
		content = append(content, VideoContent{
			Type:            "video_url",
			Role:            "reference_video",
			VideoURL:        &VideoContentURL{URL: fmt.Sprintf("https://cdn.example.com/video-%d.mp4", index)},
			DurationSeconds: float64PtrForVideoTest(3),
		})
	}
	for index := 0; index < 10; index++ {
		content = append(content, VideoContent{
			Type:     "audio_url",
			Role:     "reference_audio",
			AudioURL: &VideoContentURL{URL: fmt.Sprintf("https://cdn.example.com/audio-%d.mp3", index)},
		})
	}

	normalized, err := normalizeVideoCreateRequest(&VideoCreateRequest{
		Model:       VideoModelSeedance25,
		Prompt:      "use all references",
		Duration:    30,
		Resolution:  VideoResolution720P,
		AbilityCode: videoAbilityReferenceToVideo,
		Content:     content,
	})
	require.NoError(t, err)
	require.Equal(t, 30, normalized.ReferenceVideoSeconds)

	tooManyImages := append(append([]VideoContent{}, content...), VideoContent{
		Type:     "image_url",
		Role:     "reference_image",
		ImageURL: &VideoContentURL{URL: "https://cdn.example.com/image-overflow.png"},
	})
	_, err = normalizeVideoCreateRequest(&VideoCreateRequest{
		Model:       VideoModelSeedance25,
		Prompt:      "too many references",
		Duration:    8,
		Resolution:  VideoResolution720P,
		AbilityCode: videoAbilityReferenceToVideo,
		Content:     tooManyImages,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid_video_content")

	videoBody := normalized.UpstreamBody(SeedanceUpstreamModel(normalized.Model, normalized.Resolution))
	references, ok := videoBody["content"].([]map[string]any)
	require.True(t, ok)
	require.Equal(t, "person", references[31]["subject_type"])
}

func TestNormalizeVideoCreateRequestEnforcesSeedance25ReferenceVideoDuration(t *testing.T) {
	for _, tc := range []struct {
		name      string
		durations []float64
		wantError bool
	}{
		{name: "thirty seconds total", durations: []float64{15, 15}},
		{name: "over thirty seconds total", durations: []float64{15, 16}, wantError: true},
		{name: "single video over thirty seconds", durations: []float64{31}, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			content := make([]VideoContent, 0, len(tc.durations))
			for index, duration := range tc.durations {
				content = append(content, VideoContent{
					Type:            "video_url",
					Role:            "reference_video",
					VideoURL:        &VideoContentURL{URL: fmt.Sprintf("https://cdn.example.com/ref-%d.mp4", index)},
					DurationSeconds: float64PtrForVideoTest(duration),
				})
			}
			_, err := normalizeVideoCreateRequest(&VideoCreateRequest{
				Model:       VideoModelSeedance25,
				Prompt:      "follow the references",
				Duration:    8,
				Resolution:  VideoResolution720P,
				AbilityCode: videoAbilityReferenceToVideo,
				Content:     content,
			})
			if tc.wantError {
				require.Error(t, err)
				require.Contains(t, err.Error(), "invalid_reference_video_duration")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestNormalizeVideoPricingRulesAllowsOnlyBaseSeedance1080P(t *testing.T) {
	rules, err := normalizeVideoPricingRules([]VideoGroupPricingRule{
		{
			ModelCode:        VideoModelSeedance20,
			Resolution:       VideoResolution1080P,
			CreditsPerSecond: 1.5,
			Enabled:          true,
		},
	})
	require.NoError(t, err)
	require.Len(t, rules, 1)
	require.Equal(t, VideoModelSeedance20, rules[0].ModelCode)
	require.Equal(t, VideoResolution1080P, rules[0].Resolution)

	_, err = normalizeVideoPricingRules([]VideoGroupPricingRule{
		{
			ModelCode:        VideoModelSeedance20Fast,
			Resolution:       VideoResolution1080P,
			CreditsPerSecond: 1.5,
			Enabled:          true,
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid_video_resolution")
}

func TestNormalizeVideoPricingRulesSupportsSeedance25OnlyAt480PAnd720P(t *testing.T) {
	rules, err := normalizeVideoPricingRules([]VideoGroupPricingRule{
		{ModelCode: VideoModelSeedance25, Resolution: VideoResolution480P, CreditsPerSecond: 1, Enabled: true},
		{ModelCode: VideoModelSeedance25, Resolution: VideoResolution720P, CreditsPerSecond: 2, Enabled: true},
	})
	require.NoError(t, err)
	require.Len(t, rules, 2)

	_, err = normalizeVideoPricingRules([]VideoGroupPricingRule{
		{ModelCode: VideoModelSeedance25, Resolution: VideoResolution1080P, CreditsPerSecond: 3, Enabled: true},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid_video_resolution")
}

func TestVideoResponseFromTaskDoesNotExposeUpstreamFieldsOrProvider(t *testing.T) {
	now := time.Unix(1782700000, 0)
	completed := now.Add(30 * time.Second)
	resultURL := "https://cdn.example.com/output.mp4"
	task := &VideoTask{
		PublicID:             "video_local_123",
		Model:                VideoModelSeedance20,
		Status:               VideoTaskStatusCompleted,
		UpstreamTaskID:       stringPtr("aigod_task_123"),
		UpstreamResponseJSON: map[string]any{"id": "aigod_task_123", "provider": "aigod"},
		ResultVideoURL:       &resultURL,
		CreatedAt:            now,
		CompletedAt:          &completed,
	}

	resp := videoResponseFromTask(task)
	require.NotNil(t, resp)
	require.Equal(t, "video_local_123", resp.ID)
	require.Equal(t, VideoTaskStatusCompleted, resp.Status)
	require.Equal(t, VideoRefundStatusNotApplicable, resp.RefundStatus)
	require.Equal(t, resultURL, *resp.VideoURL)

	rendered := mustJSONForVideoTest(t, resp)
	require.NotContains(t, strings.ToLower(rendered), "aigod")
	require.NotContains(t, rendered, "upstream")
	require.NotContains(t, rendered, "aigod_task_123")
}

func TestVideoResponseFromFailedTaskReportsDurableRefundState(t *testing.T) {
	now := time.Unix(1782700000, 0)
	billed := now.Add(time.Second)
	base := &VideoTask{
		PublicID:   "video_local_refund",
		Model:      VideoModelSeedance20,
		Status:     VideoTaskStatusFailed,
		ActualCost: 2,
		BilledAt:   &billed,
		CreatedAt:  now,
	}
	require.Equal(t, VideoRefundStatusPending, videoResponseFromTask(base).RefundStatus)

	refunded := now.Add(2 * time.Second)
	require.Equal(t, VideoRefundStatusRefunded, videoResponseFromTask(&VideoTask{
		PublicID:   base.PublicID,
		Model:      base.Model,
		Status:     base.Status,
		ActualCost: base.ActualCost,
		BilledAt:   base.BilledAt,
		RefundedAt: &refunded,
		CreatedAt:  now,
	}).RefundStatus)

	require.Equal(t, VideoRefundStatusNotApplicable, videoResponseFromTask(&VideoTask{
		PublicID:  base.PublicID,
		Model:     base.Model,
		Status:    base.Status,
		CreatedAt: now,
	}).RefundStatus)
}

func TestVideoResponseFromFailedTaskSanitizesStoredErrorFallback(t *testing.T) {
	task := &VideoTask{
		PublicID:  "video_local_456",
		Model:     VideoModelSeedance20,
		Status:    VideoTaskStatusFailed,
		ErrorJSON: map[string]any{"code": "raw_upstream_code", "message": "api.aigod.one exploded"},
		CreatedAt: time.Unix(1782700000, 0),
	}

	resp := videoResponseFromTask(task)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Error)
	code, message := SanitizeVideoClientError(resp.Error.Code, resp.Error.Message)
	require.Equal(t, "video_service_unavailable", code)
	require.NotContains(t, strings.ToLower(message), "aigod")
	require.NotContains(t, strings.ToLower(message), "upstream")
}

func TestSanitizeVideoClientErrorHidesJingyuProvider(t *testing.T) {
	code, message := SanitizeVideoClientError("raw_jingyu_code", "api.jingyuapi.art returned an error")
	require.Equal(t, "video_service_unavailable", code)
	require.NotContains(t, strings.ToLower(message), "jingyu")
	require.NotContains(t, strings.ToLower(message), "upstream")
}

func TestSanitizeVideoClientErrorHidesYCYAPIProvider(t *testing.T) {
	code, message := SanitizeVideoClientError("raw_ycyapi_code", "ycyapi.cn returned an error")
	require.Equal(t, "video_service_unavailable", code)
	require.NotContains(t, strings.ToLower(message), "ycyapi")
}

func TestMapVideoUpstreamErrorStatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		polling    bool
		wantCode   string
		wantRetry  bool
	}{
		{name: "auth", statusCode: http.StatusUnauthorized, wantCode: "video_provider_unavailable"},
		{name: "forbidden", statusCode: http.StatusForbidden, wantCode: "video_provider_unavailable"},
		{name: "busy", statusCode: http.StatusTooManyRequests, polling: true, wantCode: "video_service_busy", wantRetry: true},
		{name: "server", statusCode: http.StatusBadGateway, polling: true, wantCode: "video_service_unavailable", wantRetry: true},
		{name: "network", statusCode: 0, polling: true, wantCode: "video_service_unavailable", wantRetry: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapVideoUpstreamError(&videoUpstreamError{StatusCode: tt.statusCode, Err: errors.New("raw aigod error")}, tt.polling)
			require.Equal(t, tt.wantCode, got.Code)
			require.Equal(t, tt.statusCode, got.StatusCode)
			require.Equal(t, tt.wantRetry, got.Retryable)
			require.NotContains(t, strings.ToLower(got.Message), "aigod")
			require.NotContains(t, strings.ToLower(got.Message), "upstream")
		})
	}
}

func TestVideoServiceCreateTaskReturnsQueuedLocalTaskAndStartsLifecycle(t *testing.T) {
	groupID := int64(20)
	apiKey := &APIKey{
		ID:      10,
		UserID:  100,
		GroupID: &groupID,
		User:    &User{ID: 100, Balance: 100},
		Group:   &Group{ID: groupID, Platform: PlatformSeedance, RateMultiplier: 1, SubscriptionType: SubscriptionTypeStandard},
	}
	account := Account{
		ID:          30,
		Platform:    PlatformSeedance,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"api_key":       "sk-test",
			"model_mapping": map[string]any{VideoModelSeedance20: VideoModelSeedance20},
		},
	}
	taskRepo := newVideoTaskMemoryRepo()
	pricingRepo := &videoPricingMemoryRepo{rule: &VideoGroupPricingRule{
		GroupID:          groupID,
		ModelCode:        VideoModelSeedance20,
		Resolution:       VideoResolution720P,
		CreditsPerSecond: 0.2,
		Enabled:          true,
	}}
	var lifecycle VideoTaskLifecycleInput
	service := NewVideoService(
		&videoAccountRepoStub{accounts: []Account{account}},
		taskRepo,
		pricingRepo,
		&videoUsageLogRepoStub{},
		&videoUsageBillingRepoStub{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	service.startLifecycleFunc = func(input VideoTaskLifecycleInput) {
		lifecycle = input
	}

	resp, err := service.CreateTask(context.Background(), &VideoCreateInput{
		APIKey:             apiKey,
		Request:            &VideoCreateRequest{Model: VideoModelSeedance20, Prompt: "move", Duration: 8, Resolution: VideoResolution720P},
		RequestPayloadHash: "hash",
	})

	require.NoError(t, err)
	require.Equal(t, "video", resp.Object)
	require.Equal(t, VideoTaskStatusQueued, resp.Status)
	require.Nil(t, resp.VideoURL)
	require.NotEmpty(t, resp.ID)
	require.Equal(t, resp.ID, lifecycle.PublicID)
	require.Equal(t, "seedance-2.0-720p", lifecycle.UpstreamBody["model"])

	task, err := taskRepo.GetByPublicID(context.Background(), resp.ID)
	require.NoError(t, err)
	require.Equal(t, VideoTaskStatusQueued, task.Status)
	require.Nil(t, task.UpstreamTaskID)
	require.NotNil(t, task.BilledAt)
	require.Nil(t, task.RefundedAt)
	require.Equal(t, 8, task.BillableSeconds)
	require.InDelta(t, 1.6, task.TotalCost, 0.0001)
	require.Empty(t, task.RequestJSON)
	require.Empty(t, task.UpstreamResponseJSON)
}

func TestVideoServiceCreateTaskIdempotencyReplaysWithoutDuplicateTaskOrBilling(t *testing.T) {
	previousCoordinator := DefaultIdempotencyCoordinator()
	SetDefaultIdempotencyCoordinator(NewIdempotencyCoordinator(newInMemoryIdempotencyRepo(), DefaultIdempotencyConfig()))
	t.Cleanup(func() { SetDefaultIdempotencyCoordinator(previousCoordinator) })

	groupID := int64(20)
	apiKey := &APIKey{
		ID:      10,
		UserID:  100,
		GroupID: &groupID,
		User:    &User{ID: 100, Balance: 100},
		Group:   &Group{ID: groupID, Platform: PlatformSeedance, RateMultiplier: 1, SubscriptionType: SubscriptionTypeStandard},
	}
	account := Account{
		ID:          30,
		Platform:    PlatformSeedance,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"api_key":       "sk-test",
			"model_mapping": map[string]any{VideoModelSeedance20: VideoModelSeedance20},
		},
	}
	taskRepo := newVideoTaskMemoryRepo()
	billingRepo := &videoUsageBillingRepoStub{}
	videoService := NewVideoService(
		&videoAccountRepoStub{accounts: []Account{account}},
		taskRepo,
		&videoPricingMemoryRepo{rule: &VideoGroupPricingRule{
			GroupID: groupID, ModelCode: VideoModelSeedance20, Resolution: VideoResolution720P,
			CreditsPerSecond: 0.2, Enabled: true,
		}},
		&videoUsageLogRepoStub{},
		billingRepo,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	lifecycleStarts := 0
	videoService.startLifecycleFunc = func(VideoTaskLifecycleInput) { lifecycleStarts++ }

	newInput := func(key, payloadHash, prompt string, keyID int64) *VideoCreateInput {
		keyCopy := *apiKey
		keyCopy.ID = keyID
		return &VideoCreateInput{
			APIKey:             &keyCopy,
			Request:            &VideoCreateRequest{Model: VideoModelSeedance20, Prompt: prompt, Duration: 8, Resolution: VideoResolution720P},
			IdempotencyKey:     key,
			RequestPayloadHash: payloadHash,
		}
	}

	firstInput := newInput("video-create-1", "payload-1", "move", apiKey.ID)
	first, err := videoService.CreateTask(context.Background(), firstInput)
	require.NoError(t, err)
	require.False(t, firstInput.IdempotencyReplayed)

	retryInput := newInput("video-create-1", "payload-1", "move", apiKey.ID)
	retry, err := videoService.CreateTask(context.Background(), retryInput)
	require.NoError(t, err)
	require.True(t, retryInput.IdempotencyReplayed)
	require.Equal(t, first.ID, retry.ID)
	require.Equal(t, int64(1), taskRepo.next)
	require.Equal(t, 1, lifecycleStarts)
	require.Len(t, billingRepo.commands, 1)

	_, err = videoService.CreateTask(context.Background(), newInput("video-create-1", "payload-2", "different", apiKey.ID))
	require.ErrorIs(t, err, ErrIdempotencyKeyConflict)
	require.Equal(t, int64(1), taskRepo.next)
	require.Len(t, billingRepo.commands, 1)

	otherKeyResponse, err := videoService.CreateTask(context.Background(), newInput("video-create-1", "payload-1", "move", 11))
	require.NoError(t, err)
	require.NotEqual(t, first.ID, otherKeyResponse.ID)
	require.Equal(t, int64(2), taskRepo.next)
	require.Equal(t, 2, lifecycleStarts)
	require.Len(t, billingRepo.commands, 2)
}

func TestVideoServiceForwardsLargeDataURLWithoutPersistingIt(t *testing.T) {
	groupID := int64(20)
	apiKey := &APIKey{
		ID:      10,
		UserID:  100,
		GroupID: &groupID,
		User:    &User{ID: 100, Balance: 100},
		Group:   &Group{ID: groupID, Platform: PlatformSeedance, RateMultiplier: 1, SubscriptionType: SubscriptionTypeStandard},
	}
	account := Account{
		ID:          30,
		Platform:    PlatformSeedance,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"api_key":       "sk-test",
			"model_mapping": map[string]any{VideoModelSeedance20: VideoModelSeedance20},
		},
	}
	taskRepo := newVideoTaskMemoryRepo()
	pricingRepo := &videoPricingMemoryRepo{rule: &VideoGroupPricingRule{
		GroupID:          groupID,
		ModelCode:        VideoModelSeedance20,
		Resolution:       VideoResolution720P,
		CreditsPerSecond: 0.2,
		Enabled:          true,
	}}
	var lifecycle VideoTaskLifecycleInput
	service := NewVideoService(
		&videoAccountRepoStub{accounts: []Account{account}},
		taskRepo,
		pricingRepo,
		&videoUsageLogRepoStub{},
		&videoUsageBillingRepoStub{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	service.startLifecycleFunc = func(input VideoTaskLifecycleInput) {
		lifecycle = input
	}

	dataURL := "data:image/png;base64," + strings.Repeat("A", 2*1024*1024)
	resp, err := service.CreateTask(context.Background(), &VideoCreateInput{
		APIKey: apiKey,
		Request: &VideoCreateRequest{
			Model:      VideoModelSeedance20,
			Prompt:     "animate the reference",
			Duration:   8,
			Resolution: VideoResolution720P,
			Content: []VideoContent{{
				Type:     "image_url",
				Role:     "reference_image",
				ImageURL: &VideoContentURL{URL: dataURL},
			}},
		},
		RequestPayloadHash:  "large-reference-hash",
		ResultPublicBaseURL: "https://sub2api.example.com",
	})

	require.NoError(t, err)
	content, ok := lifecycle.UpstreamBody["content"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, content, 2)
	require.Equal(t, map[string]any{"url": dataURL}, content[1]["image_url"])
	require.Equal(t, "https://sub2api.example.com", lifecycle.ResultPublicBaseURL)

	task, err := taskRepo.GetByPublicID(context.Background(), resp.ID)
	require.NoError(t, err)
	require.Empty(t, task.RequestJSON)
	require.Empty(t, task.UpstreamResponseJSON)
}

func TestVideoServiceCreateTaskUsesJingyuProviderPayload(t *testing.T) {
	groupID := int64(20)
	apiKey := &APIKey{
		ID:      10,
		UserID:  100,
		GroupID: &groupID,
		User:    &User{ID: 100, Balance: 100},
		Group:   &Group{ID: groupID, Platform: PlatformSeedance, RateMultiplier: 1, SubscriptionType: SubscriptionTypeStandard},
	}
	account := Account{
		ID:          30,
		Platform:    PlatformSeedance,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"video_provider": videoProviderJingyu,
		},
		Credentials: map[string]any{
			"api_key":       "sk-test",
			"model_mapping": map[string]any{VideoModelSeedance20: videoJingyuSeedance20Model},
		},
	}
	taskRepo := newVideoTaskMemoryRepo()
	pricingRepo := &videoPricingMemoryRepo{rule: &VideoGroupPricingRule{
		GroupID:          groupID,
		ModelCode:        VideoModelSeedance20,
		Resolution:       VideoResolution720P,
		CreditsPerSecond: 0.2,
		Enabled:          true,
	}}
	var lifecycle VideoTaskLifecycleInput
	service := NewVideoService(
		&videoAccountRepoStub{accounts: []Account{account}},
		taskRepo,
		pricingRepo,
		&videoUsageLogRepoStub{},
		&videoUsageBillingRepoStub{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	service.startLifecycleFunc = func(input VideoTaskLifecycleInput) {
		lifecycle = input
	}

	resp, err := service.CreateTask(context.Background(), &VideoCreateInput{
		APIKey: apiKey,
		Request: &VideoCreateRequest{
			Model:       VideoModelSeedance20,
			Prompt:      "move",
			Duration:    5,
			Resolution:  VideoResolution720P,
			AspectRatio: "9:16",
			Content: []VideoContent{
				{Type: "image_url", Role: "reference_image", ImageURL: &VideoContentURL{URL: "https://cdn.example.com/ref.png"}},
			},
			Raw: map[string]any{"extra": map[string]any{"seedance": map[string]any{"camera": "slow_push"}}},
		},
		RequestPayloadHash: "hash",
	})

	require.NoError(t, err)
	require.NotEmpty(t, resp.ID)
	require.Equal(t, videoJingyuSeedance20Model, lifecycle.UpstreamBody["model"])
	require.Equal(t, "9:16", lifecycle.UpstreamBody["aspect_ratio"])
	require.Equal(t, VideoResolution720P, lifecycle.UpstreamBody["resolution"])
	require.NotContains(t, lifecycle.UpstreamBody, "content")
	refs, ok := lifecycle.UpstreamBody["references"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, refs, 1)
	require.Equal(t, "image", refs[0]["type"])
	require.Equal(t, "reference_image", refs[0]["role"])
	require.NotContains(t, refs[0], "subject_type")
	require.Equal(t, videoDefaultJingyuAPIPath, lifecycle.UpstreamEndpoint)

	task, err := taskRepo.GetByPublicID(context.Background(), resp.ID)
	require.NoError(t, err)
	require.Equal(t, videoJingyuSeedance20Model, task.UpstreamModel)
	require.Empty(t, task.RequestJSON)
	require.Empty(t, task.UpstreamResponseJSON)
}

func TestVideoServiceCreateTaskUsesManualVideoModelMapping(t *testing.T) {
	groupID := int64(20)
	apiKey := &APIKey{
		ID:      10,
		UserID:  100,
		GroupID: &groupID,
		User:    &User{ID: 100, Balance: 100},
		Group:   &Group{ID: groupID, Platform: PlatformSeedance, RateMultiplier: 1, SubscriptionType: SubscriptionTypeStandard},
	}
	account := Account{
		ID:          30,
		Platform:    PlatformSeedance,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"api_key":       "sk-test",
			"model_mapping": map[string]any{VideoModelSeedance20: "custom-video-upstream-model"},
		},
	}
	taskRepo := newVideoTaskMemoryRepo()
	pricingRepo := &videoPricingMemoryRepo{rule: &VideoGroupPricingRule{
		GroupID:          groupID,
		ModelCode:        VideoModelSeedance20,
		Resolution:       VideoResolution720P,
		CreditsPerSecond: 0.2,
		Enabled:          true,
	}}
	var lifecycle VideoTaskLifecycleInput
	service := NewVideoService(
		&videoAccountRepoStub{accounts: []Account{account}},
		taskRepo,
		pricingRepo,
		&videoUsageLogRepoStub{},
		&videoUsageBillingRepoStub{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	service.startLifecycleFunc = func(input VideoTaskLifecycleInput) {
		lifecycle = input
	}

	resp, err := service.CreateTask(context.Background(), &VideoCreateInput{
		APIKey:             apiKey,
		Request:            &VideoCreateRequest{Model: VideoModelSeedance20, Prompt: "move", Duration: 8, Resolution: VideoResolution720P},
		RequestPayloadHash: "hash",
	})

	require.NoError(t, err)
	require.Equal(t, "custom-video-upstream-model", lifecycle.UpstreamBody["model"])
	task, err := taskRepo.GetByPublicID(context.Background(), resp.ID)
	require.NoError(t, err)
	require.Equal(t, "custom-video-upstream-model", task.UpstreamModel)
}

func TestVideoAccountCompatibilityUsesJingyuForSupportedModels(t *testing.T) {
	groupID := int64(20)
	jingyu := Account{
		ID:          30,
		Platform:    PlatformSeedance,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Priority:    1,
		Extra:       map[string]any{"video_provider": videoProviderJingyu},
		Credentials: map[string]any{
			"api_key": "sk-jingyu",
			"model_mapping": map[string]any{
				VideoModelSeedance20: videoJingyuSeedance20Model,
				VideoModelSeedance25: videoJingyuSeedance25Model,
			},
		},
	}
	aigod := Account{
		ID:          31,
		Platform:    PlatformSeedance,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Priority:    2,
		Credentials: map[string]any{
			"api_key":       "sk-aigod",
			"model_mapping": map[string]any{VideoModelSeedance20: VideoModelSeedance20},
		},
	}
	service := NewVideoService(
		&videoAccountRepoStub{accounts: []Account{jingyu, aigod}},
		newVideoTaskMemoryRepo(),
		&videoPricingMemoryRepo{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	for _, resolution := range []string{
		VideoResolution480P,
		VideoResolution720P,
		VideoResolution1080P,
		VideoResolution4K,
	} {
		selected, err := service.selectAccount(context.Background(), groupID, VideoModelSeedance20, resolution, false)
		require.NoError(t, err)
		require.Equal(t, jingyu.ID, selected.ID)
	}

	for _, resolution := range []string{VideoResolution480P, VideoResolution720P} {
		selected, err := service.selectAccount(context.Background(), groupID, VideoModelSeedance25, resolution, false)
		require.NoError(t, err)
		require.Equal(t, jingyu.ID, selected.ID)
	}

	_, err := service.selectAccount(context.Background(), groupID, VideoModelSeedance25, VideoResolution1080P, false)
	require.ErrorIs(t, err, ErrVideoAccountNotFound)

	require.False(t, isVideoAccountCompatible(&aigod, VideoModelSeedance20, VideoResolution4K))

	_, err = service.selectAccount(context.Background(), groupID, VideoModelSeedance20Fast, VideoResolution720P, false)
	require.ErrorIs(t, err, ErrVideoAccountNotFound)
}

func TestVideoAccountEndpointUsesJingyuDefaults(t *testing.T) {
	account := &Account{Extra: map[string]any{"video_provider": videoProviderJingyu}}
	endpoint, err := videoAccountEndpoint(account)
	require.NoError(t, err)
	require.Equal(t, "https://api.jingyuapi.art/v1/video/generations", endpoint)
}

func TestVideoServiceParsesJingyuTaskID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == videoDefaultJingyuAPIPath:
			_, _ = w.Write([]byte(`{"task_id":"jingyu-task-1","id":"legacy-id-must-not-win","status":"queued"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	account := &Account{
		ID:       30,
		Platform: PlatformSeedance,
		Extra: map[string]any{
			"video_provider": videoProviderJingyu,
			"base_url":       server.URL,
		},
		Credentials: map[string]any{"api_key": "sk-test"},
	}
	service := &VideoService{}

	created, err := service.createUpstreamTask(context.Background(), account, map[string]any{"model": videoJingyuSeedance20Model})
	require.NoError(t, err)
	require.Equal(t, "jingyu-task-1", created.ID)
}

func TestVideoServiceConfiguresCallbackOnlyForJingyuVideo(t *testing.T) {
	service := &VideoService{cfg: &config.Config{JWT: config.JWTConfig{Secret: "jingyu-callback-test-secret-32bytes"}}}
	jingyuBody := map[string]any{"model": videoJingyuSeedance20Model}
	input := VideoTaskLifecycleInput{
		PublicID:            "video_callback_1",
		Account:             &Account{Extra: map[string]any{"video_provider": videoProviderJingyu}},
		UpstreamBody:        jingyuBody,
		ResultPublicBaseURL: "https://sub2api.example.com",
	}
	require.NoError(t, service.configureJingyuVideoCallback(context.Background(), input))
	require.Equal(t, "https://sub2api.example.com/api/v1/webhooks/jingyu/videos/video_callback_1", jingyuBody["callback_url"])
	require.NotEmpty(t, jingyuBody["callback_secret"])

	aigodBody := map[string]any{"model": "seedance-2.0-720p"}
	require.NoError(t, service.configureJingyuVideoCallback(context.Background(), VideoTaskLifecycleInput{
		PublicID:            "video_aigod_1",
		Account:             &Account{Extra: map[string]any{"video_provider": videoProviderAigod}},
		UpstreamBody:        aigodBody,
		ResultPublicBaseURL: "https://sub2api.example.com",
	}))
	require.NotContains(t, aigodBody, "callback_url")
	require.NotContains(t, aigodBody, "callback_secret")
}

func TestJingyuVideoLifecycleSubmitsCallbackWithoutPolling(t *testing.T) {
	var getCount atomic.Int64
	postBody := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			postBody <- body
			_, _ = w.Write([]byte(`{"task_id":"jingyu-callback-task","status":"queued"}`))
		case http.MethodGet:
			getCount.Add(1)
			http.Error(w, "polling must not be used", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	groupID := int64(20)
	account := &Account{
		ID:       30,
		Platform: PlatformSeedance,
		Extra: map[string]any{
			"video_provider":  videoProviderJingyu,
			"base_url":        server.URL,
			"poll_timeout_ms": 40,
		},
		Credentials: map[string]any{"api_key": "sk-test"},
	}
	taskRepo := newVideoTaskMemoryRepo()
	_, err := taskRepo.Create(context.Background(), &VideoTaskCreateInput{
		PublicID: "video_callback_lifecycle", UserID: 11, APIKeyID: 22, GroupID: groupID,
		AccountID: account.ID, Model: VideoModelSeedance20, UpstreamModel: videoJingyuSeedance20Model,
		Resolution: VideoResolution720P, Status: VideoTaskStatusQueued,
	})
	require.NoError(t, err)
	service := &VideoService{
		taskRepo:             taskRepo,
		accountRepo:          &videoAccountRepoStub{accounts: []Account{*account}},
		cfg:                  &config.Config{JWT: config.JWTConfig{Secret: "jingyu-callback-test-secret-32bytes"}},
		videoResultPublisher: &recordingVideoResultPublisher{returnedURL: "https://sub2api.example.com/media/result/asset.mp4"},
	}
	service.startLifecycle(VideoTaskLifecycleInput{
		PublicID: "video_callback_lifecycle",
		Account:  account,
		APIKey: &APIKey{
			ID: 22, UserID: 11, User: &User{ID: 11}, GroupID: &groupID,
			Group: &Group{ID: groupID, Platform: PlatformSeedance},
		},
		UpstreamBody:        map[string]any{"model": videoJingyuSeedance20Model},
		ResultPublicBaseURL: "https://sub2api.example.com",
	})

	select {
	case body := <-postBody:
		require.Contains(t, body, "callback_url")
		require.Contains(t, body, "callback_secret")
	case <-time.After(time.Second):
		t.Fatal("Jingyu create request was not submitted")
	}
	time.Sleep(80 * time.Millisecond)
	require.Equal(t, int64(0), getCount.Load())
}

func TestHandleJingyuVideoCallbackRehostsResultAndCompletesTask(t *testing.T) {
	groupID := int64(33)
	account := Account{ID: 44, Platform: PlatformSeedance, Extra: map[string]any{"video_provider": videoProviderJingyu}}
	taskRepo := newVideoTaskMemoryRepo()
	_, err := taskRepo.Create(context.Background(), &VideoTaskCreateInput{
		PublicID: "video_callback_success", UserID: 11, APIKeyID: 22, GroupID: groupID,
		AccountID: account.ID, Model: VideoModelSeedance20, UpstreamModel: videoJingyuSeedance20Model,
		Resolution: VideoResolution720P, Status: VideoTaskStatusProcessing,
		UpstreamTaskID: stringPtr("jingyu-task-success"),
	})
	require.NoError(t, err)
	publisher := &recordingVideoResultPublisher{returnedURL: "https://sub2api.example.com/media/local/asset.mp4"}
	service := &VideoService{
		taskRepo:             taskRepo,
		accountRepo:          &videoAccountRepoStub{accounts: []Account{account}},
		cfg:                  &config.Config{JWT: config.JWTConfig{Secret: "jingyu-callback-test-secret-32bytes"}},
		videoResultPublisher: publisher,
	}
	rawBody := []byte(`{"task_id":"jingyu-task-success","status":"SUCCESS","video_url":"https://supplier.example.com/original.mp4?token=secret"}`)
	secret, err := service.jingyuVideoCallbackSecret("video_callback_success")
	require.NoError(t, err)
	signature := signJingyuVideoCallbackForTest(secret, rawBody)
	require.ErrorIs(t, service.HandleJingyuVideoCallback(
		context.Background(),
		"video_callback_success",
		"task.completed",
		strings.Repeat("0", sha256.Size*2),
		rawBody,
		"https://sub2api.example.com",
	), ErrJingyuCallbackUnauthorized)

	err = service.HandleJingyuVideoCallback(
		context.Background(),
		"video_callback_success",
		"task.completed",
		signature,
		rawBody,
		"https://sub2api.example.com",
	)
	require.NoError(t, err)
	task, err := taskRepo.GetByPublicID(context.Background(), "video_callback_success")
	require.NoError(t, err)
	require.Equal(t, VideoTaskStatusCompleted, task.Status)
	require.NotNil(t, task.ResultVideoURL)
	require.Equal(t, publisher.returnedURL, *task.ResultVideoURL)
	require.NotContains(t, *task.ResultVideoURL, "supplier.example.com")
	require.Equal(t, "https://supplier.example.com/original.mp4?token=secret", publisher.upstreamURL)
}

func TestHandleJingyuVideoCallbackRejectsAigodTask(t *testing.T) {
	account := Account{ID: 44, Platform: PlatformSeedance, Extra: map[string]any{"video_provider": videoProviderAigod}}
	taskRepo := newVideoTaskMemoryRepo()
	_, err := taskRepo.Create(context.Background(), &VideoTaskCreateInput{
		PublicID: "video_aigod_callback", UserID: 11, APIKeyID: 22, GroupID: 33,
		AccountID: account.ID, Model: VideoModelSeedance20, UpstreamModel: VideoModelSeedance20,
		Resolution: VideoResolution720P, Status: VideoTaskStatusProcessing,
		UpstreamTaskID: stringPtr("aigod-task"),
	})
	require.NoError(t, err)
	service := &VideoService{
		taskRepo:    taskRepo,
		accountRepo: &videoAccountRepoStub{accounts: []Account{account}},
		cfg:         &config.Config{JWT: config.JWTConfig{Secret: "jingyu-callback-test-secret-32bytes"}},
	}
	rawBody := []byte(`{"task_id":"aigod-task","status":"SUCCESS","video_url":"https://supplier.example.com/result.mp4"}`)
	secret, err := service.jingyuVideoCallbackSecret("video_aigod_callback")
	require.NoError(t, err)
	err = service.HandleJingyuVideoCallback(
		context.Background(), "video_aigod_callback", "task.completed",
		signJingyuVideoCallbackForTest(secret, rawBody), rawBody, "https://sub2api.example.com",
	)
	require.ErrorIs(t, err, ErrJingyuCallbackInvalid)
}

func signJingyuVideoCallbackForTest(secret string, rawBody []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(rawBody)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestJingyuContractUsesNewWauleFieldPrecedenceAndDefaults(t *testing.T) {
	account := &Account{Extra: map[string]any{"video_provider": videoProviderJingyu}}
	require.Equal(t, 5*time.Second, videoAccountDefaultDuration(account, "poll_interval_ms"))
	require.Equal(t, 30*time.Minute, videoAccountDefaultDuration(account, "poll_timeout_ms"))
	require.Equal(t, 30*time.Minute, videoAccountDefaultDuration(account, "request_timeout_ms"))
	require.Equal(t, 60*time.Second, videoAccountDefaultDuration(account, "connect_timeout_ms"))

	payload := map[string]any{
		"download_url":     "https://cdn.example.com/download.mp4",
		"result_asset_url": "https://cdn.example.com/asset.mp4",
		"url":              "https://cdn.example.com/url.mp4",
		"video_url":        "https://cdn.example.com/video.mp4",
	}
	require.Equal(t, "https://cdn.example.com/download.mp4", videoResultURLFromPayload(payload))

	status, known := normalizeJingyuVideoUpstreamStatus("running")
	require.False(t, known)
	require.Empty(t, status)
}

func TestJingyuUpstreamBodyPreservesSeedance25AutoDurationAndReferenceRoles(t *testing.T) {
	normalized, err := normalizeVideoCreateRequest(&VideoCreateRequest{
		Model:       VideoModelSeedance25,
		Prompt:      "follow the references",
		Duration:    -1,
		Resolution:  VideoResolution720P,
		AbilityCode: videoAbilityReferenceToVideo,
		Content: []VideoContent{
			{Type: "image_url", ImageURL: &VideoContentURL{URL: "https://cdn.example.com/ref.png"}},
			{Type: "video_url", VideoURL: &VideoContentURL{URL: "https://cdn.example.com/ref.mp4"}, DurationSeconds: float64PtrForVideoTest(5)},
			{Type: "audio_url", AudioURL: &VideoContentURL{URL: "https://cdn.example.com/ref.mp3"}},
		},
	})
	require.NoError(t, err)
	body := normalized.JingyuUpstreamBody(videoJingyuSeedance25Model)
	require.Equal(t, float64(-1), body["duration"])
	require.NotContains(t, body, "aspect_ratio")
	require.Equal(t, []map[string]any{
		{"type": "image", "role": "reference_image", "url": "https://cdn.example.com/ref.png"},
		{"type": "video", "role": "reference_video", "url": "https://cdn.example.com/ref.mp4"},
		{"type": "audio", "role": "reference_audio", "url": "https://cdn.example.com/ref.mp3"},
	}, body["references"])
}

func TestVideoResultURLFromPayloadAcceptsUnifiedJingyuFields(t *testing.T) {
	for _, payload := range []map[string]any{
		{"url": "https://cdn.example.com/url.mp4"},
		{"result_asset_url": "https://cdn.example.com/asset.mp4"},
		{"metadata": map[string]any{"url": "https://cdn.example.com/metadata.mp4"}},
	} {
		require.NotEmpty(t, videoResultURLFromPayload(payload))
	}
}

func TestVideoPrebillingDoesNotMarkTaskBilledWhenBillingFails(t *testing.T) {
	groupID := int64(20)
	apiKey := &APIKey{
		ID:      10,
		UserID:  100,
		GroupID: &groupID,
		User:    &User{ID: 100, Balance: 100},
		Group:   &Group{ID: groupID, Platform: PlatformSeedance, RateMultiplier: 1, SubscriptionType: SubscriptionTypeStandard},
	}
	account := Account{
		ID:          30,
		Platform:    PlatformSeedance,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"api_key":       "sk-test",
			"model_mapping": map[string]any{VideoModelSeedance20: VideoModelSeedance20},
		},
	}
	taskRepo := newVideoTaskMemoryRepo()
	pricingRepo := &videoPricingMemoryRepo{rule: &VideoGroupPricingRule{
		GroupID:          groupID,
		ModelCode:        VideoModelSeedance20,
		Resolution:       VideoResolution720P,
		CreditsPerSecond: 0.2,
		Enabled:          true,
	}}

	service := NewVideoService(
		&videoAccountRepoStub{accounts: []Account{account}},
		taskRepo,
		pricingRepo,
		&videoUsageLogRepoStub{},
		&videoUsageBillingRepoStub{err: errors.New("billing unavailable")},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	service.startLifecycleFunc = func(input VideoTaskLifecycleInput) {}

	_, err := service.CreateTask(context.Background(), &VideoCreateInput{
		APIKey:             apiKey,
		Request:            &VideoCreateRequest{Model: VideoModelSeedance20, Prompt: "move", Duration: 8, Resolution: VideoResolution720P},
		RequestPayloadHash: "payload-hash",
	})
	require.Error(t, err)

	require.Len(t, taskRepo.tasks, 1)
	for _, stored := range taskRepo.tasks {
		require.Nil(t, stored.BilledAt)
	}
}

func TestVideoServiceRefundsFailedPrebilledTaskOnce(t *testing.T) {
	groupID := int64(20)
	apiKey := &APIKey{
		ID:      10,
		UserID:  100,
		GroupID: &groupID,
		User:    &User{ID: 100, Balance: 100},
		Group:   &Group{ID: groupID, Platform: PlatformSeedance, RateMultiplier: 1, SubscriptionType: SubscriptionTypeStandard},
	}
	account := &Account{
		ID:       30,
		Platform: PlatformSeedance,
		Type:     AccountTypeAPIKey,
		Status:   StatusActive,
	}
	taskRepo := newVideoTaskMemoryRepo()
	task, err := taskRepo.Create(context.Background(), &VideoTaskCreateInput{
		PublicID:                 "video_test_refund",
		UserID:                   apiKey.User.ID,
		APIKeyID:                 apiKey.ID,
		GroupID:                  groupID,
		AccountID:                account.ID,
		Model:                    VideoModelSeedance20,
		UpstreamModel:            "seedance-2.0-720p",
		Resolution:               VideoResolution720P,
		DurationSeconds:          8,
		ReferenceDurationSeconds: 12,
		BillableSeconds:          20,
		CostPerSecond:            0.2,
		TotalCost:                4,
		ActualCost:               4,
		Status:                   VideoTaskStatusFailed,
	})
	require.NoError(t, err)
	_, err = taskRepo.MarkBilled(context.Background(), task.PublicID, time.Now().UTC())
	require.NoError(t, err)
	task, err = taskRepo.GetByPublicID(context.Background(), task.PublicID)
	require.NoError(t, err)

	usageRepo := &videoUsageLogRepoStub{}
	billingRepo := &videoUsageBillingRepoStub{}
	service := NewVideoService(
		&videoAccountRepoStub{},
		taskRepo,
		&videoPricingMemoryRepo{},
		usageRepo,
		billingRepo,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	require.NoError(t, service.refundFailedTask(context.Background(), task, apiKey, nil, account, "payload-hash", "ua", "127.0.0.1", "/v1/videos", "/videos"))
	require.NoError(t, service.refundFailedTask(context.Background(), task, apiKey, nil, account, "payload-hash", "ua", "127.0.0.1", "/v1/videos", "/videos"))

	stored, err := taskRepo.GetByPublicID(context.Background(), task.PublicID)
	require.NoError(t, err)
	require.NotNil(t, stored.RefundedAt)
	require.Len(t, usageRepo.logs, 1)
	require.Equal(t, "video:video_test_refund:refund", usageRepo.logs[0].RequestID)
	require.Equal(t, RequestTypeVideo, usageRepo.logs[0].RequestType)
	require.Equal(t, "video_duration", *usageRepo.logs[0].BillingMode)
	require.Equal(t, "/v1/videos", *usageRepo.logs[0].InboundEndpoint)
	require.Equal(t, "/v1/videos", *usageRepo.logs[0].UpstreamEndpoint)
	require.NotNil(t, usageRepo.logs[0].DurationMs)
	require.InDelta(t, -4, usageRepo.logs[0].OutputCost, 0.0001)
	require.InDelta(t, -4, usageRepo.logs[0].TotalCost, 0.0001)
	require.InDelta(t, -4, usageRepo.logs[0].ActualCost, 0.0001)
	require.Len(t, usageRepo.videoResultUpdates, 1)
	require.Equal(t, "video:video_test_refund", usageRepo.videoResultUpdates[0].requestID)
	require.Equal(t, "/v1/videos", usageRepo.videoResultUpdates[0].update.InboundEndpoint)
	require.Equal(t, "/v1/videos", usageRepo.videoResultUpdates[0].update.UpstreamEndpoint)
	require.NotNil(t, usageRepo.videoResultUpdates[0].update.DurationMs)
	require.Len(t, billingRepo.commands, 1)
	require.InDelta(t, -4, billingRepo.commands[0].BalanceCost, 0.0001)
}

func TestVideoCompletionUpdatesChargeUsageLogResultWithoutBillingAgain(t *testing.T) {
	groupID := int64(20)
	apiKey := &APIKey{
		ID:      10,
		UserID:  100,
		GroupID: &groupID,
		User:    &User{ID: 100, Balance: 100},
		Group:   &Group{ID: groupID, Platform: PlatformSeedance, RateMultiplier: 1, SubscriptionType: SubscriptionTypeStandard},
	}
	account := &Account{
		ID:       30,
		Platform: PlatformSeedance,
		Type:     AccountTypeAPIKey,
		Status:   StatusActive,
	}
	resultURL := "https://cdn.example.com/output.mp4"
	createdAt := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	completedAt := createdAt.Add(42 * time.Second)
	task := &VideoTask{
		PublicID:                 "video_test_completion",
		UserID:                   apiKey.User.ID,
		APIKeyID:                 apiKey.ID,
		GroupID:                  groupID,
		AccountID:                account.ID,
		Model:                    VideoModelSeedance20,
		UpstreamModel:            "seedance-2.0-720p",
		Resolution:               VideoResolution720P,
		DurationSeconds:          8,
		ReferenceDurationSeconds: 12,
		BillableSeconds:          20,
		CostPerSecond:            0.2,
		TotalCost:                4,
		ActualCost:               4,
		Status:                   VideoTaskStatusCompleted,
		ResultVideoURL:           &resultURL,
		CreatedAt:                createdAt,
		UpdatedAt:                completedAt,
		CompletedAt:              &completedAt,
		BilledAt:                 timePtrForVideoTest(time.Now().UTC()),
	}
	usageRepo := &videoUsageLogRepoStub{}
	billingRepo := &videoUsageBillingRepoStub{}
	service := NewVideoService(
		&videoAccountRepoStub{},
		newVideoTaskMemoryRepo(),
		&videoPricingMemoryRepo{},
		usageRepo,
		billingRepo,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	require.NoError(t, service.recordCompletedTask(context.Background(), task, apiKey, nil, account, "ua", "127.0.0.1", "/videos", "/v1/videos"))
	require.Empty(t, billingRepo.commands)
	require.Empty(t, usageRepo.logs)
	require.Len(t, usageRepo.videoResultUpdates, 1)
	require.Equal(t, "video:video_test_completion", usageRepo.videoResultUpdates[0].requestID)
	require.Equal(t, apiKey.ID, usageRepo.videoResultUpdates[0].apiKeyID)
	require.Equal(t, resultURL, usageRepo.videoResultUpdates[0].update.ResultURL)
	require.Equal(t, "/v1/videos", usageRepo.videoResultUpdates[0].update.InboundEndpoint)
	require.Equal(t, "/v1/videos", usageRepo.videoResultUpdates[0].update.UpstreamEndpoint)
	require.NotNil(t, usageRepo.videoResultUpdates[0].update.DurationMs)
	require.Equal(t, 42000, *usageRepo.videoResultUpdates[0].update.DurationMs)
}

func TestVideoEstimatedBillingIncludesReferenceVideoSeconds(t *testing.T) {
	req := &VideoCreateRequest{
		Model:      VideoModelSeedance20,
		Prompt:     "move",
		Duration:   8,
		Resolution: VideoResolution720P,
		Content: []VideoContent{
			{Type: "image_url", Role: "reference_image", ImageURL: &VideoContentURL{URL: "https://cdn.example.com/ref.png"}},
			{Type: "video_url", Role: "reference_video", VideoURL: &VideoContentURL{URL: "https://cdn.example.com/a.mp4"}, DurationSeconds: float64PtrForVideoTest(5)},
			{Type: "video_url", Role: "reference_video", VideoURL: &VideoContentURL{URL: "https://cdn.example.com/b.mp4"}, DurationSeconds: float64PtrForVideoTest(7)},
		},
	}

	normalized, err := normalizeVideoCreateRequest(req)
	require.NoError(t, err)
	require.Equal(t, 20, normalized.BillableSeconds)
	require.InDelta(t, 4, float64(normalized.BillableSeconds)*0.2, 0.0001)
}

func TestVideoCreateTaskAllowsAgentGroupToReachVideoValidation(t *testing.T) {
	service := NewVideoService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, &config.Config{})
	input := &VideoCreateInput{
		APIKey: &APIKey{
			User:  &User{ID: 1},
			Group: &Group{ID: 2, Platform: PlatformOpenAI, Kind: "agent", SystemCode: "yingzo"},
		},
		Request: &VideoCreateRequest{},
	}
	_, err := service.CreateTask(context.Background(), input)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "video_platform_required")

	input.APIKey.Group = &Group{ID: 2, Platform: PlatformOpenAI, Kind: "standard"}
	_, err = service.CreateTask(context.Background(), input)
	require.Error(t, err)
	require.Contains(t, err.Error(), "video_platform_required")
}

func float64PtrForVideoTest(v float64) *float64 { return &v }

func timePtrForVideoTest(v time.Time) *time.Time { return &v }

func mustJSONForVideoTest(t *testing.T, v any) string {
	t.Helper()
	raw, err := jsonMarshalForVideoTest(v)
	require.NoError(t, err)
	return string(raw)
}

func jsonMarshalForVideoTest(v any) ([]byte, error) {
	return videoTestJSON{}.Marshal(v)
}

type videoTestJSON struct{}

func (videoTestJSON) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

type videoTaskMemoryRepo struct {
	mu    sync.Mutex
	next  int64
	tasks map[string]*VideoTask
}

func newVideoTaskMemoryRepo() *videoTaskMemoryRepo {
	return &videoTaskMemoryRepo{tasks: make(map[string]*VideoTask)}
}

func (r *videoTaskMemoryRepo) Create(ctx context.Context, input *VideoTaskCreateInput) (*VideoTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.next++
	now := time.Now().UTC()
	task := &VideoTask{
		ID:                       r.next,
		PublicID:                 input.PublicID,
		RequestID:                input.RequestID,
		UserID:                   input.UserID,
		APIKeyID:                 input.APIKeyID,
		GroupID:                  input.GroupID,
		AccountID:                input.AccountID,
		Model:                    input.Model,
		UpstreamModel:            input.UpstreamModel,
		Resolution:               input.Resolution,
		DurationSeconds:          input.DurationSeconds,
		ReferenceDurationSeconds: input.ReferenceDurationSeconds,
		BillableSeconds:          input.BillableSeconds,
		CostPerSecond:            input.CostPerSecond,
		TotalCost:                input.TotalCost,
		ActualCost:               input.ActualCost,
		Status:                   input.Status,
		UpstreamTaskID:           input.UpstreamTaskID,
		RequestJSON:              map[string]any{},
		UpstreamResponseJSON:     map[string]any{},
		CreatedAt:                now,
		UpdatedAt:                now,
	}
	r.tasks[task.PublicID] = task
	return cloneVideoTaskForTest(task), nil
}

func (r *videoTaskMemoryRepo) GetByPublicID(ctx context.Context, publicID string) (*VideoTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[publicID]
	if !ok {
		return nil, ErrVideoTaskNotFound
	}
	return cloneVideoTaskForTest(task), nil
}

func (r *videoTaskMemoryRepo) UpdateByPublicID(ctx context.Context, publicID string, update VideoTaskUpdate) (*VideoTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[publicID]
	if !ok {
		return nil, ErrVideoTaskNotFound
	}
	if update.Status != nil {
		task.Status = *update.Status
	}
	if update.UpstreamTaskID != nil {
		task.UpstreamTaskID = update.UpstreamTaskID
	}
	if update.ErrorJSON != nil {
		task.ErrorJSON = cloneMap(update.ErrorJSON)
	}
	if update.ResultVideoURL != nil {
		task.ResultVideoURL = update.ResultVideoURL
	}
	if update.CompletedAt != nil {
		task.CompletedAt = update.CompletedAt
	}
	if update.BilledAt != nil {
		task.BilledAt = update.BilledAt
	}
	if update.RefundedAt != nil {
		task.RefundedAt = update.RefundedAt
	}
	task.UpdatedAt = time.Now().UTC()
	return cloneVideoTaskForTest(task), nil
}

func (r *videoTaskMemoryRepo) TransitionTerminalByPublicID(ctx context.Context, publicID string, update VideoTaskUpdate) (*VideoTask, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[publicID]
	if !ok {
		return nil, false, ErrVideoTaskNotFound
	}
	if task.Status != VideoTaskStatusQueued && task.Status != VideoTaskStatusProcessing {
		return cloneVideoTaskForTest(task), false, nil
	}
	if update.Status != nil {
		task.Status = *update.Status
	}
	if update.UpstreamTaskID != nil {
		task.UpstreamTaskID = update.UpstreamTaskID
	}
	if update.ErrorJSON != nil {
		task.ErrorJSON = cloneMap(update.ErrorJSON)
	}
	if update.ResultVideoURL != nil {
		task.ResultVideoURL = update.ResultVideoURL
	}
	if update.CompletedAt != nil {
		task.CompletedAt = update.CompletedAt
	}
	task.UpdatedAt = time.Now().UTC()
	return cloneVideoTaskForTest(task), true, nil
}

func (r *videoTaskMemoryRepo) MarkProcessingByPublicID(ctx context.Context, publicID string, upstreamTaskID string) (*VideoTask, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[publicID]
	if !ok {
		return nil, false, ErrVideoTaskNotFound
	}
	if task.Status != VideoTaskStatusQueued && task.Status != VideoTaskStatusProcessing {
		return cloneVideoTaskForTest(task), false, nil
	}
	task.Status = VideoTaskStatusProcessing
	task.UpstreamTaskID = &upstreamTaskID
	task.UpdatedAt = time.Now().UTC()
	return cloneVideoTaskForTest(task), true, nil
}

func (r *videoTaskMemoryRepo) MarkBilled(ctx context.Context, publicID string, billedAt time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[publicID]
	if !ok {
		return false, ErrVideoTaskNotFound
	}
	if task.BilledAt != nil {
		return false, nil
	}
	task.BilledAt = &billedAt
	return true, nil
}

func (r *videoTaskMemoryRepo) MarkRefunded(ctx context.Context, publicID string, refundedAt time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[publicID]
	if !ok {
		return false, ErrVideoTaskNotFound
	}
	if task.RefundedAt != nil {
		return false, nil
	}
	task.RefundedAt = &refundedAt
	return true, nil
}

func cloneVideoTaskForTest(task *VideoTask) *VideoTask {
	if task == nil {
		return nil
	}
	copy := *task
	copy.RequestJSON = cloneMap(task.RequestJSON)
	copy.UpstreamResponseJSON = cloneMap(task.UpstreamResponseJSON)
	copy.ErrorJSON = cloneMap(task.ErrorJSON)
	return &copy
}

type videoPricingMemoryRepo struct {
	rule *VideoGroupPricingRule
}

func (r *videoPricingMemoryRepo) ListByGroupID(ctx context.Context, groupID int64) ([]VideoGroupPricingRule, error) {
	if r.rule == nil || r.rule.GroupID != groupID {
		return nil, nil
	}
	return []VideoGroupPricingRule{*r.rule}, nil
}

func (r *videoPricingMemoryRepo) ReplaceForGroup(ctx context.Context, groupID int64, rules []VideoGroupPricingRule) error {
	if len(rules) == 0 {
		r.rule = nil
		return nil
	}
	rule := rules[0]
	rule.GroupID = groupID
	r.rule = &rule
	return nil
}

func (r *videoPricingMemoryRepo) GetEnabledRule(ctx context.Context, groupID int64, modelCode string, resolution string) (*VideoGroupPricingRule, error) {
	if r.rule == nil || r.rule.GroupID != groupID || r.rule.ModelCode != modelCode || r.rule.Resolution != resolution || !r.rule.Enabled {
		return nil, ErrVideoPricingRuleNotFound
	}
	rule := *r.rule
	return &rule, nil
}

type videoAccountRepoStub struct {
	accounts []Account
}

func (r *videoAccountRepoStub) GetByID(ctx context.Context, id int64) (*Account, error) {
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			account := r.accounts[i]
			return &account, nil
		}
	}
	return nil, ErrAccountNotFound
}

func (r *videoAccountRepoStub) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error) {
	var out []Account
	for _, account := range r.accounts {
		if account.Platform == platform && account.IsSchedulable() {
			out = append(out, account)
		}
	}
	return out, nil
}

func (r *videoAccountRepoStub) SetError(ctx context.Context, id int64, errorMsg string) error {
	return nil
}

func (r *videoAccountRepoStub) SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error {
	return nil
}

func (r *videoAccountRepoStub) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	return nil
}

func (r *videoAccountRepoStub) IncrementQuotaUsed(ctx context.Context, id int64, amount float64) error {
	return nil
}

type videoUsageBillingRepoStub struct {
	err      error
	commands []*UsageBillingCommand
}

func (r *videoUsageBillingRepoStub) Apply(ctx context.Context, cmd *UsageBillingCommand) (*UsageBillingApplyResult, error) {
	if r.err != nil {
		return nil, r.err
	}
	if cmd != nil {
		copy := *cmd
		r.commands = append(r.commands, &copy)
	}
	return &UsageBillingApplyResult{Applied: true}, nil
}

type videoUsageLogRepoStub struct {
	UsageLogRepository
	logs               []*UsageLog
	videoResultUpdates []videoResultUpdateForTest
	err                error
}

type videoResultUpdateForTest struct {
	requestID string
	apiKeyID  int64
	update    VideoUsageResultUpdate
}

func (r *videoUsageLogRepoStub) Create(ctx context.Context, log *UsageLog) (bool, error) {
	if r.err != nil {
		return false, r.err
	}
	if log != nil {
		copy := *log
		r.logs = append(r.logs, &copy)
	}
	return true, nil
}

func (r *videoUsageLogRepoStub) UpdateVideoResult(ctx context.Context, requestID string, apiKeyID int64, update VideoUsageResultUpdate) error {
	if r.err != nil {
		return r.err
	}
	r.videoResultUpdates = append(r.videoResultUpdates, videoResultUpdateForTest{
		requestID: requestID,
		apiKeyID:  apiKeyID,
		update:    update,
	})
	return nil
}

// 上游给 UsageBillingRepository 增加了批量图片余额冻结/扣减接口，
// 视频链路用不到，这里补空实现让本仓库的测试桩继续满足接口。
func (r *videoUsageBillingRepoStub) ReserveBatchImageBalance(ctx context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	return &BatchImageBalanceHoldResult{}, nil
}

func (r *videoUsageBillingRepoStub) CaptureBatchImageBalance(ctx context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	return &BatchImageBalanceHoldResult{}, nil
}

func (r *videoUsageBillingRepoStub) ReleaseBatchImageBalance(ctx context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	return &BatchImageBalanceHoldResult{}, nil
}
