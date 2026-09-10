package service

import "strings"

const ycyapiContentBodyKey = "_sub2api_ycyapi_content"

type ycyapiVideoProviderAdapter struct{}

func (y ycyapiVideoProviderAdapter) Provider() string {
	return videoProviderYCYAPI
}

func (y ycyapiVideoProviderAdapter) DefaultBaseURL() string {
	return videoDefaultYCYAPIBaseURL
}

func (y ycyapiVideoProviderAdapter) DefaultAPIPath() string {
	return videoDefaultAPIPath
}

func (y ycyapiVideoProviderAdapter) Compatible(model, resolution string) bool {
	model = strings.TrimSpace(model)
	resolution = strings.TrimSpace(resolution)
	switch model {
	case VideoModelSeedance20:
		return resolution == VideoResolution480P ||
			resolution == VideoResolution720P ||
			resolution == VideoResolution1080P
	case VideoModelSeedance20Fast, VideoModelSeedance25:
		return resolution == VideoResolution480P || resolution == VideoResolution720P
	default:
		return false
	}
}

func (y ycyapiVideoProviderAdapter) CompatibleRequest(normalized *normalizedVideoRequest) bool {
	if normalized == nil || !y.Compatible(normalized.Model, normalized.Resolution) {
		return false
	}
	switch normalized.Model {
	case VideoModelSeedance20, VideoModelSeedance20Fast:
		if normalized.GeneratedSeconds != 5 && normalized.GeneratedSeconds != 10 && normalized.GeneratedSeconds != 15 {
			return false
		}
		if normalized.RatioProvided && !isYCYAPIFireflyRatio(normalized.Ratio) {
			return false
		}
	case VideoModelSeedance25:
		if normalized.GeneratedSeconds < 4 || normalized.GeneratedSeconds > 29 {
			return false
		}
		if normalized.RatioProvided && !isYCYAPISeedance25Ratio(normalized.Ratio) {
			return false
		}
		stats := inspectVideoContent(normalized.Content)
		ordinaryImages := stats.ImageCount - stats.FirstFrameCount - stats.LastFrameCount
		if stats.LastFrameCount > 0 && stats.FirstFrameCount == 0 {
			return false
		}
		if stats.FirstFrameCount+stats.LastFrameCount > 0 && ordinaryImages > 0 {
			return false
		}
		if stats.AudioCount > 0 && stats.ImageCount+stats.VideoCount == 0 {
			return false
		}
		if stats.VideoCount > 0 && normalized.Resolution == VideoResolution720P && normalized.GeneratedSeconds > 18 {
			return false
		}
		for _, item := range normalized.Content {
			if item.Type == "video_url" && item.DurationSeconds != nil && (*item.DurationSeconds < 3 || *item.DurationSeconds > 10) {
				return false
			}
		}
	}
	return true
}

func (y ycyapiVideoProviderAdapter) UpstreamModel(account *Account, normalized *normalizedVideoRequest) string {
	if normalized == nil {
		return ""
	}
	if mappedModel := resolvedMappedVideoModel(account, normalized.Model); mappedModel != "" {
		return mappedModel
	}
	switch normalized.Model {
	case VideoModelSeedance20:
		return videoYCYAPISeedance20Model
	case VideoModelSeedance20Fast:
		return videoYCYAPISeedance20FastModel
	case VideoModelSeedance25:
		return videoYCYAPISeedance25Model
	default:
		return ""
	}
}

func (y ycyapiVideoProviderAdapter) BuildCreateBody(normalized *normalizedVideoRequest, upstreamModel string) map[string]any {
	if normalized == nil {
		return nil
	}
	body := map[string]any{
		"model":      upstreamModel,
		"prompt":     normalized.Prompt,
		"duration":   normalized.GeneratedSeconds,
		"resolution": strings.ToLower(normalized.Resolution),
	}
	if normalized.RatioProvided {
		body["aspect_ratio"] = normalized.Ratio
	}
	if normalized.GenerateAudio != nil {
		body["generate_audio"] = *normalized.GenerateAudio
	}
	if normalized.Raw != nil {
		if seed, ok := normalized.Raw["seed"]; ok && seed != nil {
			body["seed"] = seed
		}
		if normalized.Model != VideoModelSeedance25 {
			if negativePrompt, ok := normalized.Raw["negative_prompt"].(string); ok && strings.TrimSpace(negativePrompt) != "" {
				body["negative_prompt"] = strings.TrimSpace(negativePrompt)
			}
			if shots, ok := normalized.Raw["shots"]; ok && shots != nil {
				body["shots"] = shots
			}
		}
	}
	if media := ycyapiMediaContent(normalized.Content); len(media) > 0 {
		body[ycyapiContentBodyKey] = media
	}
	return body
}

func ycyapiMediaContent(content []VideoContent) []VideoContent {
	out := make([]VideoContent, 0, len(content))
	for _, item := range content {
		switch item.Type {
		case "image_url", "video_url", "audio_url":
			out = append(out, item)
		}
	}
	return out
}

func isYCYAPIFireflyRatio(ratio string) bool {
	switch strings.ToLower(strings.TrimSpace(ratio)) {
	case "21:9", "16:9", "4:3", "1:1", "3:4", "9:16":
		return true
	default:
		return false
	}
}

func isYCYAPISeedance25Ratio(ratio string) bool {
	switch strings.ToLower(strings.TrimSpace(ratio)) {
	case "16:9", "1:1", "9:16":
		return true
	default:
		return false
	}
}
