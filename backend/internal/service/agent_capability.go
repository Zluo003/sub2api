package service

import (
	"fmt"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
)

const AgentCapabilityVersion = "2026-07-13"
const AgentAPIContractVersion = "2026-07-13.1"

type AgentCapability struct {
	AgentAPIContractVersion string         `json:"agent_api_contract_version"`
	Model                   string         `json:"model"`
	CapabilityVersion       string         `json:"capability_version"`
	Modes                   []string       `json:"modes"`
	InputLimits             map[string]any `json:"input_limits"`
	OutputLimits            map[string]any `json:"output_limits"`
	Pricing                 map[string]any `json:"pricing"`
	ConcurrencyHint         *int           `json:"concurrency_hint"`
	Warnings                []string       `json:"warnings"`
}

func AgentCapabilities() map[string]AgentCapability {
	imageConcurrency, videoConcurrency, analysisConcurrency := 6, 4, 4
	runtimePricing := map[string]any{
		"source":            "sub2api_group_pricing",
		"requires_estimate": true,
		"currency":          "credits",
	}
	return map[string]AgentCapability{
		"gpt-image-2": {
			AgentAPIContractVersion: AgentAPIContractVersion,
			Model:                   "gpt-image-2",
			CapabilityVersion:       AgentCapabilityVersion,
			Modes:                   []string{"generation", "edit"},
			InputLimits: map[string]any{
				"media_types":     []string{"image"},
				"max_image_bytes": 30 << 20,
			},
			OutputLimits: map[string]any{
				"resolutions": []string{"1K", "2K", "4K"},
			},
			Pricing:         runtimePricing,
			ConcurrencyHint: &imageConcurrency,
			Warnings:        []string{},
		},
		"seedance-2.0": {
			AgentAPIContractVersion: AgentAPIContractVersion,
			Model:                   "seedance-2.0",
			CapabilityVersion:       AgentCapabilityVersion,
			Modes: []string{
				videoAbilityTextToVideo,
				videoAbilityImageToVideo,
				videoAbilityStartEndToVideo,
				videoAbilityReferenceToVideo,
			},
			InputLimits: map[string]any{
				"max_images":                  9,
				"max_videos":                  3,
				"max_audio":                   3,
				"max_image_bytes":             30 << 20,
				"max_video_bytes":             200 << 20,
				"max_audio_bytes":             15 << 20,
				"min_reference_seconds":       2,
				"max_reference_seconds":       15,
				"max_reference_total_seconds": 15,
				"max_request_bytes":           64 << 20,
				"width_pixels":                []int{300, 6000},
				"height_pixels":               []int{300, 6000},
				"aspect_ratio":                []float64{0.4, 2.5},
				"video_total_pixels":          []int{409600, 8295044},
				"video_fps":                   []float64{24, 60},
				"image_mime_types": []string{
					"image/jpeg", "image/png", "image/webp", "image/bmp", "image/tiff",
					"image/gif", "image/heic", "image/heif",
				},
				"video_containers": []string{"mp4", "mov"},
				"video_codecs":     []string{"h264", "avc", "h265", "hevc"},
				"audio_codecs":     []string{"aac", "mp3"},
				"audio_mime_types": []string{"audio/wav", "audio/x-wav", "audio/mpeg"},
			},
			OutputLimits: map[string]any{
				"duration_seconds": []int{4, 15},
				"ratios":           []string{"16:9", "4:3", "1:1", "3:4", "9:16", "21:9", "adaptive"},
				"resolutions":      []string{"480P", "720P", "1080P"},
			},
			Pricing:         runtimePricing,
			ConcurrencyHint: &videoConcurrency,
			Warnings: []string{
				"Chinese prompts over 500 characters are a quality warning, not a hard error.",
				"English prompts over 1000 words are a quality warning, not a hard error.",
			},
		},
		"gemini-3.5-flash": {
			AgentAPIContractVersion: AgentAPIContractVersion,
			Model:                   "gemini-3.5-flash",
			CapabilityVersion:       AgentCapabilityVersion,
			Modes:                   []string{"video-analysis", "image-analysis", "multimodal-analysis"},
			InputLimits: map[string]any{
				"media_types":     []string{"image", "video", "audio"},
				"max_image_bytes": 30 << 20,
				"max_video_bytes": 200 << 20,
				"max_audio_bytes": 15 << 20,
			},
			OutputLimits:    map[string]any{},
			Pricing:         runtimePricing,
			ConcurrencyHint: &analysisConcurrency,
			Warnings:        []string{},
		},
	}
}

type AgentMediaMetadata struct {
	AssetID          string  `json:"asset_id,omitempty"`
	TemporaryAssetID string  `json:"temporary_asset_id,omitempty"`
	Role             string  `json:"role"`
	MediaType        string  `json:"media_type"`
	MIMEType         string  `json:"mime_type"`
	Container        string  `json:"container,omitempty"`
	SizeBytes        int64   `json:"size_bytes"`
	Width            int     `json:"width,omitempty"`
	Height           int     `json:"height,omitempty"`
	DurationSeconds  float64 `json:"duration_seconds,omitempty"`
	FPS              float64 `json:"fps,omitempty"`
	VideoCodec       string  `json:"video_codec,omitempty"`
	AudioCodec       string  `json:"audio_codec,omitempty"`
	Encoding         string  `json:"encoding,omitempty"`
}

type AgentMediaPreflightRequest struct {
	Model           string               `json:"model" binding:"required"`
	Mode            string               `json:"mode" binding:"required"`
	Prompt          string               `json:"prompt"`
	RequestBytes    int64                `json:"request_bytes,omitempty"`
	DurationSeconds int                  `json:"duration_seconds,omitempty"`
	Ratio           string               `json:"ratio,omitempty"`
	Resolution      string               `json:"resolution,omitempty"`
	References      []AgentMediaMetadata `json:"references"`
}

type AgentValidationIssue struct {
	Code       string `json:"code"`
	Path       string `json:"path"`
	Message    string `json:"message"`
	Actual     any    `json:"actual,omitempty"`
	Allowed    any    `json:"allowed,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
}

type AgentPromptMetrics struct {
	Language   string `json:"language"`
	Characters int    `json:"characters"`
	Words      int    `json:"words"`
}

type AgentMediaPreflightResult struct {
	Model             string                     `json:"model"`
	CapabilityVersion string                     `json:"capability_version"`
	Mode              string                     `json:"mode"`
	Valid             bool                       `json:"valid"`
	Errors            []AgentValidationIssue     `json:"errors"`
	Warnings          []AgentValidationIssue     `json:"warnings"`
	Normalized        AgentMediaPreflightRequest `json:"normalized"`
	ReferenceBudget   map[string]any             `json:"reference_budget"`
	PromptMetrics     AgentPromptMetrics         `json:"prompt_metrics"`
}

func PreflightAgentMedia(input AgentMediaPreflightRequest) AgentMediaPreflightResult {
	input.Model = strings.TrimSpace(input.Model)
	input.Mode = strings.TrimSpace(strings.ToLower(input.Mode))
	input.Prompt = strings.TrimSpace(input.Prompt)
	input.Ratio = strings.TrimSpace(input.Ratio)
	input.Resolution = strings.ToUpper(strings.TrimSpace(input.Resolution))
	for i := range input.References {
		ref := &input.References[i]
		ref.Role = strings.TrimSpace(strings.ToLower(ref.Role))
		ref.MediaType = strings.TrimSpace(strings.ToLower(ref.MediaType))
		ref.MIMEType = strings.TrimSpace(strings.ToLower(strings.Split(ref.MIMEType, ";")[0]))
		ref.Container = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(ref.Container)), ".")
		ref.VideoCodec = normalizeAgentCodec(ref.VideoCodec)
		ref.AudioCodec = normalizeAgentCodec(ref.AudioCodec)
		ref.Encoding = strings.TrimSpace(strings.ToLower(ref.Encoding))
	}
	result := AgentMediaPreflightResult{
		Model:             input.Model,
		CapabilityVersion: AgentCapabilityVersion,
		Mode:              input.Mode,
		Errors:            []AgentValidationIssue{},
		Warnings:          []AgentValidationIssue{},
		Normalized:        input,
		ReferenceBudget:   map[string]any{},
		PromptMetrics:     measureAgentPrompt(input.Prompt),
	}
	capability, ok := AgentCapabilities()[input.Model]
	if !ok {
		result.Errors = append(result.Errors, agentIssue("model_not_found", "model", "Model is not available to the Agent group", input.Model, sortedAgentModelIDs(), "Choose a model returned by /api/v1/agent/models"))
		result.Valid = false
		return result
	}
	result.CapabilityVersion = capability.CapabilityVersion
	if !containsAgentString(capability.Modes, input.Mode) {
		result.Errors = append(result.Errors, agentIssue("unsupported_mode", "mode", "Mode is not supported by this model", input.Mode, capability.Modes, "Choose a supported mode"))
	}
	switch input.Model {
	case "seedance-2.0":
		preflightSeedance(&result)
	case "gpt-image-2":
		preflightSimpleMedia(&result, map[string]int64{"image": 30 << 20})
	case "gemini-3.5-flash":
		preflightSimpleMedia(&result, map[string]int64{"image": 30 << 20, "video": 200 << 20, "audio": 15 << 20})
	}
	result.Valid = len(result.Errors) == 0
	return result
}

func preflightSimpleMedia(result *AgentMediaPreflightResult, limits map[string]int64) {
	for index, ref := range result.Normalized.References {
		path := fmt.Sprintf("references[%d]", index)
		limit, ok := limits[ref.MediaType]
		if !ok {
			result.Errors = append(result.Errors, agentIssue("unsupported_media_type", path+".media_type", "Media type is not supported by this model", ref.MediaType, mapKeysInt64(limits), "Remove or convert this reference"))
			continue
		}
		if ref.SizeBytes <= 0 || ref.SizeBytes > limit {
			result.Errors = append(result.Errors, agentIssue("invalid_media_size", path+".size_bytes", "Media size is outside the supported range", ref.SizeBytes, map[string]any{"min": 1, "max": limit}, "Use a smaller derived file"))
		}
	}
}

func preflightSeedance(result *AgentMediaPreflightResult) {
	input := result.Normalized
	var images, videos, audio, firstFrames, lastFrames int
	var videoSeconds, audioSeconds float64
	var referenceBytes int64
	for index, ref := range input.References {
		path := fmt.Sprintf("references[%d]", index)
		referenceBytes += ref.SizeBytes
		switch ref.MediaType {
		case "image":
			images++
			if ref.Role == "first_frame" || ref.Role == "first-frame-intent" {
				firstFrames++
			}
			if ref.Role == "last_frame" || ref.Role == "last-frame-intent" {
				lastFrames++
			}
			validateSeedanceImage(result, path, ref)
		case "video":
			videos++
			videoSeconds += ref.DurationSeconds
			validateSeedanceVideo(result, path, ref)
		case "audio":
			audio++
			audioSeconds += ref.DurationSeconds
			validateSeedanceAudio(result, path, ref)
		default:
			result.Errors = append(result.Errors, agentIssue("unsupported_media_type", path+".media_type", "Seedance references must be image, video, or audio", ref.MediaType, []string{"image", "video", "audio"}, "Remove this reference"))
		}
	}
	result.ReferenceBudget = map[string]any{
		"images":          map[string]any{"used": images, "max": 9},
		"videos":          map[string]any{"used": videos, "max": 3},
		"audio":           map[string]any{"used": audio, "max": 3},
		"video_seconds":   map[string]any{"used": videoSeconds, "max": 15},
		"audio_seconds":   map[string]any{"used": audioSeconds, "max": 15},
		"request_bytes":   map[string]any{"used": input.RequestBytes, "max": int64(64 << 20)},
		"reference_bytes": map[string]any{"used": referenceBytes},
	}
	if images > 9 {
		result.Errors = append(result.Errors, agentIssue("too_many_images", "references", "Seedance supports at most 9 images", images, 9, "Reduce the reference bundle"))
	}
	if videos > 3 {
		result.Errors = append(result.Errors, agentIssue("too_many_videos", "references", "Seedance supports at most 3 videos", videos, 3, "Reduce the reference bundle"))
	}
	if audio > 3 {
		result.Errors = append(result.Errors, agentIssue("too_many_audio_files", "references", "Seedance supports at most 3 audio files", audio, 3, "Reduce the reference bundle"))
	}
	if videoSeconds > 15 {
		result.Errors = append(result.Errors, agentIssue("reference_video_total_too_long", "references", "Reference videos exceed the 15 second total", videoSeconds, 15, "Trim or remove video references"))
	}
	if audioSeconds > 15 {
		result.Errors = append(result.Errors, agentIssue("reference_audio_total_too_long", "references", "Reference audio exceeds the 15 second total", audioSeconds, 15, "Trim or remove audio references"))
	}
	requestBytes := input.RequestBytes
	if requestBytes > 64<<20 {
		result.Errors = append(result.Errors, agentIssue("request_too_large", "request_bytes", "Seedance request body exceeds 64 MB", requestBytes, int64(64<<20), "Upload large references and use public URLs"))
	}
	validateSeedanceMode(result, images, videos, audio, firstFrames, lastFrames)
	if input.DurationSeconds != 0 && (input.DurationSeconds < 4 || input.DurationSeconds > 15) {
		result.Errors = append(result.Errors, agentIssue("invalid_output_duration", "duration_seconds", "Generated duration must be from 4 through 15 seconds", input.DurationSeconds, []int{4, 15}, "Choose a supported duration"))
	}
	if input.Ratio != "" && !containsAgentString([]string{"16:9", "4:3", "1:1", "3:4", "9:16", "21:9", "adaptive"}, input.Ratio) {
		result.Errors = append(result.Errors, agentIssue("invalid_output_ratio", "ratio", "Output ratio is unsupported", input.Ratio, []string{"16:9", "4:3", "1:1", "3:4", "9:16", "21:9", "adaptive"}, "Choose a supported ratio"))
	}
	if input.Resolution != "" && !containsAgentString([]string{"480P", "720P", "1080P"}, input.Resolution) {
		result.Errors = append(result.Errors, agentIssue("invalid_output_resolution", "resolution", "Output resolution is unsupported", input.Resolution, []string{"480P", "720P", "1080P"}, "Choose a supported resolution"))
	}
	if result.PromptMetrics.Language == "zh" && result.PromptMetrics.Characters > 500 {
		result.Warnings = append(result.Warnings, agentIssue("prompt_quality_length", "prompt", "Chinese prompt exceeds the official 500 character recommendation", result.PromptMetrics.Characters, 500, "Semantically compress the prompt without dropping creative intent"))
	}
	if result.PromptMetrics.Language == "en" && result.PromptMetrics.Words > 1000 {
		result.Warnings = append(result.Warnings, agentIssue("prompt_quality_length", "prompt", "English prompt exceeds the official 1000 word recommendation", result.PromptMetrics.Words, 1000, "Semantically compress the prompt without dropping creative intent"))
	}
}

func validateSeedanceMode(result *AgentMediaPreflightResult, images, videos, audio, firstFrames, lastFrames int) {
	mode := result.Normalized.Mode
	switch mode {
	case videoAbilityTextToVideo:
		if images+videos+audio > 0 {
			result.Errors = append(result.Errors, agentIssue("mode_conflict", "references", "Text-to-video cannot include media references", images+videos+audio, 0, "Remove references or choose another mode"))
		}
	case videoAbilityImageToVideo:
		if images != 1 || firstFrames != 1 || videos+audio+lastFrames > 0 {
			result.Errors = append(result.Errors, agentIssue("mode_conflict", "references", "Image-to-video requires exactly one first-frame image", map[string]int{"images": images, "first_frames": firstFrames, "last_frames": lastFrames, "videos": videos, "audio": audio}, map[string]int{"images": 1, "first_frames": 1}, "Correct roles or choose reference mode"))
		}
	case videoAbilityStartEndToVideo:
		if images != 2 || firstFrames != 1 || lastFrames != 1 || videos+audio > 0 {
			result.Errors = append(result.Errors, agentIssue("mode_conflict", "references", "Start-end mode requires one first frame and one last frame", map[string]int{"images": images, "first_frames": firstFrames, "last_frames": lastFrames, "videos": videos, "audio": audio}, map[string]int{"images": 2, "first_frames": 1, "last_frames": 1}, "Correct roles or choose reference mode"))
		}
	case videoAbilityReferenceToVideo:
		if images+videos == 0 {
			result.Errors = append(result.Errors, agentIssue("reference_visual_required", "references", "Reference mode requires at least one image or video", map[string]int{"images": images, "videos": videos, "audio": audio}, "image or video", "Add a visual reference"))
		}
		if firstFrames+lastFrames > 0 {
			result.Errors = append(result.Errors, agentIssue("mode_conflict", "references", "Strict first/last-frame roles cannot be mixed with multimodal reference mode", map[string]int{"first_frames": firstFrames, "last_frames": lastFrames}, 0, "Use reference_image roles and describe frame intent in the prompt"))
		}
	}
}

func validateSeedanceImage(result *AgentMediaPreflightResult, path string, ref AgentMediaMetadata) {
	allowedMIME := []string{"image/jpeg", "image/png", "image/webp", "image/bmp", "image/tiff", "image/gif", "image/heic", "image/heif"}
	if !containsAgentString(allowedMIME, ref.MIMEType) {
		result.Errors = append(result.Errors, agentIssue("unsupported_image_format", path+".mime_type", "Unsupported Seedance image format", ref.MIMEType, allowedMIME, "Convert to PNG, JPEG, or WebP"))
	}
	validateAgentSize(result, path, ref.SizeBytes, 30<<20)
	validateSeedanceDimensions(result, path, ref.Width, ref.Height, false)
}

func validateSeedanceVideo(result *AgentMediaPreflightResult, path string, ref AgentMediaMetadata) {
	if !containsAgentString([]string{"mp4", "mov"}, ref.Container) {
		result.Errors = append(result.Errors, agentIssue("unsupported_video_container", path+".container", "Unsupported Seedance video container", ref.Container, []string{"mp4", "mov"}, "Remux to MP4 or MOV"))
	}
	if !containsAgentString([]string{"h264", "avc", "h265", "hevc"}, ref.VideoCodec) {
		result.Errors = append(result.Errors, agentIssue("unsupported_video_codec", path+".video_codec", "Unsupported Seedance video codec", ref.VideoCodec, []string{"h264", "avc", "h265", "hevc"}, "Transcode to H.264 or H.265"))
	}
	if ref.AudioCodec != "" && !containsAgentString([]string{"aac", "mp3"}, ref.AudioCodec) {
		result.Errors = append(result.Errors, agentIssue("unsupported_audio_codec", path+".audio_codec", "Unsupported audio codec in reference video", ref.AudioCodec, []string{"aac", "mp3"}, "Transcode the audio track"))
	}
	validateAgentSize(result, path, ref.SizeBytes, 200<<20)
	validateSeedanceDimensions(result, path, ref.Width, ref.Height, true)
	if ref.FPS < 24 || ref.FPS > 60 {
		result.Errors = append(result.Errors, agentIssue("invalid_video_fps", path+".fps", "Reference video FPS must be from 24 through 60", ref.FPS, []float64{24, 60}, "Convert to a supported frame rate"))
	}
	if ref.DurationSeconds < 2 || ref.DurationSeconds > 15 {
		result.Errors = append(result.Errors, agentIssue("invalid_reference_duration", path+".duration_seconds", "Reference video duration must be from 2 through 15 seconds", ref.DurationSeconds, []float64{2, 15}, "Trim the reference video"))
	}
}

func validateSeedanceAudio(result *AgentMediaPreflightResult, path string, ref AgentMediaMetadata) {
	if !containsAgentString([]string{"audio/wav", "audio/x-wav", "audio/mpeg"}, ref.MIMEType) {
		result.Errors = append(result.Errors, agentIssue("unsupported_audio_format", path+".mime_type", "Unsupported Seedance audio format", ref.MIMEType, []string{"audio/wav", "audio/mpeg"}, "Convert to WAV or MP3"))
	}
	validateAgentSize(result, path, ref.SizeBytes, 15<<20)
	if ref.DurationSeconds < 2 || ref.DurationSeconds > 15 {
		result.Errors = append(result.Errors, agentIssue("invalid_reference_duration", path+".duration_seconds", "Reference audio duration must be from 2 through 15 seconds", ref.DurationSeconds, []float64{2, 15}, "Trim the reference audio"))
	}
}

func validateAgentSize(result *AgentMediaPreflightResult, path string, size, max int64) {
	if size <= 0 || size > max {
		result.Errors = append(result.Errors, agentIssue("invalid_media_size", path+".size_bytes", "Media size is outside the supported range", size, map[string]any{"min": 1, "max": max}, "Create a smaller derived file"))
	}
}

func validateSeedanceDimensions(result *AgentMediaPreflightResult, path string, width, height int, video bool) {
	if width < 300 || width > 6000 {
		result.Errors = append(result.Errors, agentIssue("invalid_media_width", path+".width", "Media width must be from 300 through 6000 pixels", width, []int{300, 6000}, "Resize the media"))
	}
	if height < 300 || height > 6000 {
		result.Errors = append(result.Errors, agentIssue("invalid_media_height", path+".height", "Media height must be from 300 through 6000 pixels", height, []int{300, 6000}, "Resize the media"))
	}
	if width > 0 && height > 0 {
		ratio := float64(width) / float64(height)
		if ratio < 0.4 || ratio > 2.5 {
			result.Errors = append(result.Errors, agentIssue("invalid_media_aspect_ratio", path, "Media aspect ratio must be from 0.4 through 2.5", math.Round(ratio*1000)/1000, []float64{0.4, 2.5}, "Resize or pad without changing the intended composition"))
		}
		if video {
			pixels := width * height
			if pixels < 409600 || pixels > 8295044 {
				result.Errors = append(result.Errors, agentIssue("invalid_video_pixels", path, "Reference video pixel count is unsupported", pixels, []int{409600, 8295044}, "Transcode to 480p, 720p, 1080p, or supported 4K dimensions"))
			}
		}
	}
}

func measureAgentPrompt(prompt string) AgentPromptMetrics {
	metrics := AgentPromptMetrics{Language: "en", Characters: utf8.RuneCountInString(prompt), Words: len(strings.Fields(prompt))}
	for _, r := range prompt {
		if unicode.Is(unicode.Han, r) {
			metrics.Language = "zh"
			break
		}
	}
	return metrics
}

func normalizeAgentCodec(codec string) string {
	codec = strings.ToLower(strings.TrimSpace(codec))
	codec = strings.ReplaceAll(codec, ".", "")
	codec = strings.ReplaceAll(codec, "-", "")
	switch codec {
	case "avc1", "avc", "h264":
		return "h264"
	case "hev1", "hvc1", "hevc", "h265":
		return "h265"
	case "mp4a", "aac":
		return "aac"
	case "mp3", "mpeg3":
		return "mp3"
	default:
		return codec
	}
}

func agentIssue(code, path, message string, actual, allowed any, suggestion string) AgentValidationIssue {
	return AgentValidationIssue{Code: code, Path: path, Message: message, Actual: actual, Allowed: allowed, Suggestion: suggestion}
}

func containsAgentString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func maxAgentInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func sortedAgentModelIDs() []string {
	return []string{"gemini-3.5-flash", "gpt-image-2", "seedance-2.0"}
}

func mapKeysInt64(values map[string]int64) []string {
	keys := make([]string, 0, len(values))
	for _, key := range []string{"image", "video", "audio"} {
		if _, ok := values[key]; ok {
			keys = append(keys, key)
		}
	}
	return keys
}
