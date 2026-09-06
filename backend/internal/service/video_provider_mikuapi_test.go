package service

import (
	"github.com/stretchr/testify/require"
	"testing"
)

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
