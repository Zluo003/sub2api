package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func newMikuapiTestAccount(baseURL string) *Account {
	return &Account{
		ID: 91,
		Extra: map[string]any{
			"video_provider": videoProviderMikuapi,
			"base_url":       baseURL,
			"api_path":       videoDefaultAPIPath,
		},
		Credentials: map[string]any{"api_key": "sk-miku-test"},
	}
}

func newMikuapiPollServer(t *testing.T, payload string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestMikuapiPollPrefersDirectResultURL(t *testing.T) {
	server := newMikuapiPollServer(t, `{"status":"completed","video_url":"https://cdn.example/out.mp4"}`)
	account := newMikuapiTestAccount(server.URL)

	result, err := (&VideoService{}).pollUpstreamTask(context.Background(), account, "task-1")
	require.NoError(t, err)
	require.Equal(t, VideoTaskStatusCompleted, result.Status)
	require.Equal(t, "https://cdn.example/out.mp4", result.VideoURL)
	// A direct link is fetched anonymously; forwarding the account credential
	// breaks presigned URLs that already carry their own auth.
	require.False(t, videoResultURLNeedsAccountAuth(account, result.VideoURL))
}

func TestMikuapiPollResolvesRelativeResultURL(t *testing.T) {
	server := newMikuapiPollServer(t, `{"status":"completed","url":"/v1/videos/task-1/content"}`)
	account := newMikuapiTestAccount(server.URL)

	result, err := (&VideoService{}).pollUpstreamTask(context.Background(), account, "task-1")
	require.NoError(t, err)
	require.Equal(t, server.URL+"/v1/videos/task-1/content", result.VideoURL)
	require.True(t, videoResultURLNeedsAccountAuth(account, result.VideoURL))
}

func TestMikuapiPollFallsBackToContentEndpoint(t *testing.T) {
	server := newMikuapiPollServer(t, `{"status":"completed"}`)
	account := newMikuapiTestAccount(server.URL)

	result, err := (&VideoService{}).pollUpstreamTask(context.Background(), account, "task-1")
	require.NoError(t, err)
	require.Equal(t, server.URL+"/v1/videos/task-1/content", result.VideoURL)
	require.True(t, videoResultURLNeedsAccountAuth(account, result.VideoURL))
}

func TestMikuapiResolutionKeepsUppercaseKTiers(t *testing.T) {
	require.Equal(t, "4K", mikuapiResolution(VideoResolution4K))
	require.Equal(t, "4K", mikuapiResolution("4k"))
	require.Equal(t, "2K", mikuapiResolution("2K"))
	require.Equal(t, "720p", mikuapiResolution("720P"))

	body := (mikuapiVideoProviderAdapter{}).BuildCreateBody(&normalizedVideoRequest{
		Model: VideoModelSeedance20, Prompt: "move", GeneratedSeconds: 6, Resolution: VideoResolution4K,
	}, "seedance-2-pro")
	require.Equal(t, "4K", body["resolution"])
}

func TestMikuapiTaskIDParsingAcceptsNestedPayloadAndLocation(t *testing.T) {
	require.Equal(t, "nested-task", videoTaskIDFromPayload(map[string]any{
		"status": "queued", "data": map[string]any{"id": "nested-task"},
	}))
	require.Equal(t, "location-task", videoTaskIDFromLocation("https://mikuapi.org/v1/videos/location-task"))
	require.Empty(t, videoTaskIDFromLocation("https://mikuapi.org/v1/videos/location-task/content"))
}

func TestMikuapiVideoProviderMapsSeedanceModelsAnd1080p(t *testing.T) {
	adapter := mikuapiVideoProviderAdapter{}
	for _, tc := range []struct{ model, resolution, upstream string }{
		{VideoModelSeedance20, VideoResolution720P, "seedance-2-pro"},
		{VideoModelSeedance20Fast, VideoResolution720P, "seedance-2-fast"},
		{VideoModelSeedance25, VideoResolution1080P, "seedance-2.5-pro"},
	} {
		r := &normalizedVideoRequest{Model: tc.model, Resolution: tc.resolution}
		require.True(t, adapter.Compatible(tc.model, tc.resolution))
		require.Equal(t, tc.upstream, adapter.UpstreamModel(nil, r))
	}
	require.False(t, adapter.Compatible(VideoModelSeedance20Fast, VideoResolution1080P))
}

func TestMikuapiVideoProviderBuildsDocumentedBody(t *testing.T) {
	r := &normalizedVideoRequest{
		Model: VideoModelSeedance25, Prompt: "move", GeneratedSeconds: 8, Resolution: VideoResolution1080P,
		Ratio: "9:16", RatioProvided: true,
		Content: []VideoContent{
			{Type: "image_url", Role: "reference_image", ImageURL: &VideoContentURL{URL: "https://cdn.example/image.png"}},
			{Type: "video_url", Role: "reference_video", VideoURL: &VideoContentURL{URL: "https://cdn.example/video.mp4"}},
			{Type: "audio_url", Role: "reference_audio", AudioURL: &VideoContentURL{URL: "https://cdn.example/audio.mp3"}},
		},
	}
	body := (mikuapiVideoProviderAdapter{}).BuildCreateBody(r, "seedance-2.5-pro")
	require.Equal(t, 8, body["seconds"])
	require.Equal(t, "1080p", body["resolution"])
	require.Equal(t, "9:16", body["aspect_ratio"])
	require.Equal(t, []string{"https://cdn.example/image.png"}, body["reference_images"])
	require.Equal(t, []string{"https://cdn.example/video.mp4"}, body["reference_videos"])
	require.Equal(t, []string{"https://cdn.example/audio.mp3"}, body["reference_audios"])
}

func TestMikuapiVideoProviderResolutionSetsForNewModels(t *testing.T) {
	adapter := mikuapiVideoProviderAdapter{}
	for _, tc := range []struct {
		model     string
		supported []string
		rejected  []string
	}{
		{VideoModelMinimaxH3, []string{VideoResolution768P, VideoResolution2K}, []string{VideoResolution480P, VideoResolution720P, VideoResolution1080P, VideoResolution4K}},
		{VideoModelMinimaxH3Max, []string{VideoResolution480P, VideoResolution768P}, []string{VideoResolution720P, VideoResolution1080P, VideoResolution2K}},
		{VideoModelWan3, []string{VideoResolution480P, VideoResolution720P, VideoResolution1080P}, []string{VideoResolution768P, VideoResolution2K, VideoResolution4K}},
	} {
		for _, resolution := range tc.supported {
			require.True(t, adapter.Compatible(tc.model, resolution), "%s should accept %s", tc.model, resolution)
		}
		for _, resolution := range tc.rejected {
			require.False(t, adapter.Compatible(tc.model, resolution), "%s should reject %s", tc.model, resolution)
		}
	}
	// Clients may send either casing for the K tiers.
	require.True(t, adapter.Compatible(VideoModelMinimaxH3, "2k"))
	require.True(t, adapter.Compatible(VideoModelMinimaxH3Max, "768P"))
}

func TestMikuapiVideoProviderEnforcesNewModelDurations(t *testing.T) {
	adapter := mikuapiVideoProviderAdapter{}
	request := func(model, resolution string, seconds int) *normalizedVideoRequest {
		return &normalizedVideoRequest{Model: model, Prompt: "move", Resolution: resolution, GeneratedSeconds: seconds}
	}

	// wan-3 goes down to 2s and up to 30s.
	require.True(t, adapter.CompatibleRequest(request(VideoModelWan3, VideoResolution720P, 2)))
	require.True(t, adapter.CompatibleRequest(request(VideoModelWan3, VideoResolution720P, 30)))
	require.False(t, adapter.CompatibleRequest(request(VideoModelWan3, VideoResolution720P, 1)))
	require.False(t, adapter.CompatibleRequest(request(VideoModelWan3, VideoResolution720P, 31)))

	// The MiniMax pair starts at 5s, so the Seedance 4s floor must not leak in.
	for _, model := range []string{VideoModelMinimaxH3, VideoModelMinimaxH3Max} {
		require.False(t, adapter.CompatibleRequest(request(model, VideoResolution768P, 4)))
		require.True(t, adapter.CompatibleRequest(request(model, VideoResolution768P, 5)))
		require.True(t, adapter.CompatibleRequest(request(model, VideoResolution768P, 15)))
		require.False(t, adapter.CompatibleRequest(request(model, VideoResolution768P, 16)))
	}
}

func TestMikuapiVideoProviderEnforcesNewModelReferenceCaps(t *testing.T) {
	adapter := mikuapiVideoProviderAdapter{}
	withImages := func(model, resolution string, count int) *normalizedVideoRequest {
		content := make([]VideoContent, 0, count)
		for i := 0; i < count; i++ {
			content = append(content, VideoContent{Type: "image_url", Role: "reference_image", ImageURL: &VideoContentURL{URL: "https://cdn.example/ref.png"}})
		}
		return &normalizedVideoRequest{Model: model, Prompt: "move", Resolution: resolution, GeneratedSeconds: 5, Content: content}
	}

	require.True(t, adapter.CompatibleRequest(withImages(VideoModelMinimaxH3Max, VideoResolution768P, 12)))
	require.False(t, adapter.CompatibleRequest(withImages(VideoModelMinimaxH3Max, VideoResolution768P, 13)))
	require.True(t, adapter.CompatibleRequest(withImages(VideoModelMinimaxH3, VideoResolution768P, 9)))
	require.False(t, adapter.CompatibleRequest(withImages(VideoModelMinimaxH3, VideoResolution768P, 10)))
	require.True(t, adapter.CompatibleRequest(withImages(VideoModelWan3, VideoResolution720P, 10)))
	require.False(t, adapter.CompatibleRequest(withImages(VideoModelWan3, VideoResolution720P, 11)))

	// minimax-h3-max is the one model that accepts audio with no visual reference.
	audioOnly := func(model, resolution string) *normalizedVideoRequest {
		return &normalizedVideoRequest{
			Model: model, Prompt: "move", Resolution: resolution, GeneratedSeconds: 5,
			Content: []VideoContent{{Type: "audio_url", Role: "reference_audio", AudioURL: &VideoContentURL{URL: "https://cdn.example/a.mp3"}}},
		}
	}
	require.True(t, adapter.CompatibleRequest(audioOnly(VideoModelMinimaxH3Max, VideoResolution768P)))
	require.False(t, adapter.CompatibleRequest(audioOnly(VideoModelMinimaxH3, VideoResolution768P)))
	require.False(t, adapter.CompatibleRequest(audioOnly(VideoModelWan3, VideoResolution720P)))
}

func TestMikuapiVideoProviderPassesNewModelNamesThrough(t *testing.T) {
	adapter := mikuapiVideoProviderAdapter{}
	for _, model := range []string{VideoModelMinimaxH3, VideoModelMinimaxH3Max, VideoModelWan3} {
		require.Equal(t, model, adapter.UpstreamModel(nil, &normalizedVideoRequest{Model: model}))
	}
}

func TestMikuapiVideoProviderKeepsMinimax2KCasing(t *testing.T) {
	body := (mikuapiVideoProviderAdapter{}).BuildCreateBody(&normalizedVideoRequest{
		Model: VideoModelMinimaxH3, Prompt: "move", GeneratedSeconds: 8, Resolution: VideoResolution2K,
	}, VideoModelMinimaxH3)
	require.Equal(t, VideoModelMinimaxH3, body["model"])
	require.Equal(t, "2K", body["resolution"])
}

// The aigod adapter is the fallback for accounts with no explicit provider, and
// it reads the shared resolution table. Without its own model guard it would
// claim the mikuapi-only models and steal their scheduling.
func TestAigodVideoProviderRejectsNonSeedanceModels(t *testing.T) {
	adapter := aigodVideoProviderAdapter{}
	require.False(t, adapter.Compatible(VideoModelMinimaxH3, VideoResolution768P))
	require.False(t, adapter.Compatible(VideoModelMinimaxH3Max, VideoResolution480P))
	require.False(t, adapter.Compatible(VideoModelWan3, VideoResolution1080P))
	require.True(t, adapter.Compatible(VideoModelSeedance20, VideoResolution1080P))
}

func TestNormalizeAgentPriceResolutionKeepsUppercaseKTiers(t *testing.T) {
	for _, input := range []string{"2k", "2K"} {
		resolution, err := normalizeAgentPriceResolution(AgentMediaTypeVideo, input)
		require.NoError(t, err)
		require.Equal(t, VideoResolution2K, resolution)
	}
	resolution, err := normalizeAgentPriceResolution(AgentMediaTypeVideo, "768P")
	require.NoError(t, err)
	require.Equal(t, VideoResolution768P, resolution)
}
