package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func validSeedanceReferencePreflight() AgentMediaPreflightRequest {
	return AgentMediaPreflightRequest{
		Model:           "seedance-2.0",
		Mode:            videoAbilityReferenceToVideo,
		Prompt:          "角色走进房间，镜头缓慢推进。",
		DurationSeconds: 5,
		Ratio:           "16:9",
		Resolution:      "1080P",
		References: []AgentMediaMetadata{
			{
				AssetID:   "asset_1",
				Role:      "reference_image",
				MediaType: "image",
				MIMEType:  "image/png",
				SizeBytes: 1024,
				Width:     1920,
				Height:    1080,
			},
		},
	}
}

func TestPreflightAgentMediaValidSeedanceReference(t *testing.T) {
	result := PreflightAgentMedia(validSeedanceReferencePreflight())
	require.True(t, result.Valid)
	require.Empty(t, result.Errors)
	require.Empty(t, result.Warnings)
	require.Equal(t, AgentCapabilityVersion, result.CapabilityVersion)
	require.Equal(t, "zh", result.PromptMetrics.Language)
	imageBudget, ok := result.ReferenceBudget["images"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, 1, imageBudget["used"])
}

func TestPreflightAgentMediaPromptGuidanceIsWarningOnly(t *testing.T) {
	input := validSeedanceReferencePreflight()
	input.Prompt = strings.Repeat("镜", 501)
	result := PreflightAgentMedia(input)
	require.True(t, result.Valid)
	require.Empty(t, result.Errors)
	require.Len(t, result.Warnings, 1)
	require.Equal(t, "prompt_quality_length", result.Warnings[0].Code)
}

func TestPreflightAgentMediaDoesNotCountURLReferenceBytesAsJSONBody(t *testing.T) {
	input := validSeedanceReferencePreflight()
	input.RequestBytes = 2048
	input.References[0].SizeBytes = 30 << 20
	result := PreflightAgentMedia(input)
	require.True(t, result.Valid)
	requestBudget, ok := result.ReferenceBudget["request_bytes"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, int64(2048), requestBudget["used"])
	referenceBudget, ok := result.ReferenceBudget["reference_bytes"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, int64(30<<20), referenceBudget["used"])
}

func TestPreflightAgentMediaReportsFieldLevelSeedanceErrors(t *testing.T) {
	input := validSeedanceReferencePreflight()
	input.RequestBytes = 65 << 20
	input.References = []AgentMediaMetadata{
		{
			Role:      "first_frame",
			MediaType: "image",
			MIMEType:  "image/svg+xml",
			SizeBytes: 31 << 20,
			Width:     100,
			Height:    7000,
		},
		{
			Role:            "reference_video",
			MediaType:       "video",
			Container:       "avi",
			SizeBytes:       1024,
			Width:           1920,
			Height:          1080,
			DurationSeconds: 16,
			FPS:             12,
			VideoCodec:      "vp9",
			AudioCodec:      "opus",
		},
	}
	result := PreflightAgentMedia(input)
	require.False(t, result.Valid)
	codes := make(map[string]bool)
	for _, issue := range result.Errors {
		codes[issue.Code] = true
		require.NotEmpty(t, issue.Path)
		require.NotEmpty(t, issue.Suggestion)
	}
	for _, expected := range []string{
		"unsupported_image_format",
		"invalid_media_size",
		"invalid_media_width",
		"invalid_media_height",
		"unsupported_video_container",
		"unsupported_video_codec",
		"unsupported_audio_codec",
		"invalid_video_fps",
		"invalid_reference_duration",
		"request_too_large",
		"mode_conflict",
	} {
		require.True(t, codes[expected], "missing error code %s", expected)
	}
}

func TestPreflightAgentMediaRejectsAudioOnlyReference(t *testing.T) {
	input := validSeedanceReferencePreflight()
	input.References = []AgentMediaMetadata{{
		Role:            "reference_audio",
		MediaType:       "audio",
		MIMEType:        "audio/wav",
		SizeBytes:       1024,
		DurationSeconds: 3,
	}}
	result := PreflightAgentMedia(input)
	require.False(t, result.Valid)
	require.Contains(t, validationCodes(result.Errors), "reference_visual_required")
}

func TestPreflightAgentMediaRejectsUnknownModelAndMode(t *testing.T) {
	unknown := PreflightAgentMedia(AgentMediaPreflightRequest{Model: "unknown", Mode: "generation"})
	require.False(t, unknown.Valid)
	require.Equal(t, "model_not_found", unknown.Errors[0].Code)

	unsupported := validSeedanceReferencePreflight()
	unsupported.Mode = "not-a-mode"
	result := PreflightAgentMedia(unsupported)
	require.False(t, result.Valid)
	require.Contains(t, validationCodes(result.Errors), "unsupported_mode")
}

func validationCodes(issues []AgentValidationIssue) []string {
	codes := make([]string, 0, len(issues))
	for _, issue := range issues {
		codes = append(codes, issue.Code)
	}
	return codes
}
