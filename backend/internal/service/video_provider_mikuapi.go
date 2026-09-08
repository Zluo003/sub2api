package service

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type mikuapiVideoProviderAdapter struct{}

func (m mikuapiVideoProviderAdapter) Provider() string       { return videoProviderMikuapi }
func (m mikuapiVideoProviderAdapter) DefaultBaseURL() string { return videoDefaultMikuapiBaseURL }
func (m mikuapiVideoProviderAdapter) DefaultAPIPath() string { return videoDefaultAPIPath }

func (m mikuapiVideoProviderAdapter) Compatible(model, resolution string) bool {
	return IsSupportedVideoResolution(strings.TrimSpace(model), mikuapiResolution(resolution))
}

func (m mikuapiVideoProviderAdapter) CompatibleRequest(r *normalizedVideoRequest) bool {
	if r == nil || !m.Compatible(r.Model, r.Resolution) {
		return false
	}
	if r.RatioProvided && !isMikuapiRatio(r.Ratio) {
		return false
	}
	spec, known := videoSpecForModel(r.Model)
	if !known {
		return false
	}
	// seedance-2.0-fast only renders two fixed lengths, which no other model
	// shares, so it stays a special case rather than a spec field.
	if r.Model == VideoModelSeedance20Fast {
		if r.GeneratedSeconds != 5 && r.GeneratedSeconds != 10 {
			return false
		}
	} else if r.GeneratedSeconds < spec.MinSeconds || r.GeneratedSeconds > spec.MaxSeconds {
		return false
	}
	stats := inspectVideoContent(r.Content)
	if stats.ImageCount > spec.MaxRefImages || stats.VideoCount > spec.MaxRefVideos || stats.AudioCount > spec.MaxRefAudios {
		return false
	}
	if stats.LastFrameCount > 0 && stats.FirstFrameCount == 0 {
		return false
	}
	if stats.FirstFrameCount+stats.LastFrameCount > 0 {
		// MikuAPI treats start/end frames and reference_* as mutually exclusive modes.
		if stats.ImageCount > stats.FirstFrameCount+stats.LastFrameCount || stats.VideoCount > 0 || stats.AudioCount > 0 {
			return false
		}
	}
	if spec.AudioNeedsVisual && stats.AudioCount > 0 && stats.ImageCount+stats.VideoCount == 0 {
		return false
	}
	return true
}

func (m mikuapiVideoProviderAdapter) UpstreamModel(account *Account, r *normalizedVideoRequest) string {
	if r == nil {
		return ""
	}
	if mapped := resolvedMappedVideoModel(account, r.Model); mapped != "" {
		return mapped
	}
	switch r.Model {
	case VideoModelSeedance20:
		return "seedance-2-pro"
	case VideoModelSeedance20Fast:
		return "seedance-2-fast"
	case VideoModelSeedance25:
		return "seedance-2.5-pro"
	case VideoModelMinimaxH3, VideoModelMinimaxH3Max, VideoModelWan3:
		// MikuAPI publishes these under the same names we expose downstream.
		return r.Model
	}
	return ""
}

func (m mikuapiVideoProviderAdapter) BuildCreateBody(r *normalizedVideoRequest, upstreamModel string) map[string]any {
	if r == nil {
		return nil
	}
	body := map[string]any{"model": upstreamModel, "prompt": r.Prompt, "seconds": r.GeneratedSeconds, "resolution": mikuapiResolution(r.Resolution)}
	if r.RatioProvided {
		body["aspect_ratio"] = r.Ratio
	}
	if r.GenerateAudio != nil {
		body["sound_effects"] = *r.GenerateAudio
	}
	images, videos, audios := make([]string, 0), make([]string, 0), make([]string, 0)
	for _, item := range r.Content {
		var u string
		switch item.Type {
		case "image_url":
			if item.ImageURL != nil {
				u = item.ImageURL.URL
			}
		case "video_url":
			if item.VideoURL != nil {
				u = item.VideoURL.URL
			}
		case "audio_url":
			if item.AudioURL != nil {
				u = item.AudioURL.URL
			}
		}
		if strings.TrimSpace(u) == "" {
			continue
		}
		if item.Role == "first_frame" {
			body["input_reference"] = u
			continue
		}
		if item.Role == "last_frame" {
			body["image_end"] = u
			continue
		}
		switch item.Type {
		case "image_url":
			images = append(images, u)
		case "video_url":
			videos = append(videos, u)
		case "audio_url":
			audios = append(audios, u)
		}
	}
	if len(images) > 0 {
		body["reference_images"] = images
	}
	if len(videos) > 0 {
		body["reference_videos"] = videos
	}
	if len(audios) > 0 {
		body["reference_audios"] = audios
	}
	return body
}

// mikuapiResolution matches the upstream allow-list casing exactly. MikuAPI
// snaps unrecognised resolutions to the nearest tier instead of rejecting them,
// so a lowercased "4k" would silently downgrade the video while still billing
// the 4K rate.
func mikuapiResolution(resolution string) string {
	resolution = strings.TrimSpace(resolution)
	switch strings.ToLower(resolution) {
	case "2k":
		return "2K"
	case "4k":
		return "4K"
	default:
		return strings.ToLower(resolution)
	}
}

func isMikuapiRatio(ratio string) bool {
	switch strings.ToLower(strings.TrimSpace(ratio)) {
	case "21:9", "16:9", "4:3", "1:1", "3:4", "9:16":
		return true
	default:
		return false
	}
}

// Only query responses use this classification; a failed POST must never be
// automatically resubmitted because the upstream may already have billed it.
func mikuapiPollResponseRetryable(status int, body []byte) bool {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return false
	}
	if status == 0 || status == http.StatusNotFound || status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500 {
		return true
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return status >= 200 && status < 300
	}
	return mikuapiTemporaryQueryMessage(payload, 0)
}

func mikuapiTemporaryQueryMessage(value any, depth int) bool {
	if depth > 6 {
		return false
	}
	switch value := value.(type) {
	case string:
		message := strings.ToLower(strings.Join(strings.Fields(value), " "))
		return strings.Contains(message, "temporarily unavailable") || strings.Contains(message, "暂时不可用")
	case map[string]any:
		if code, ok := value["code"].(string); ok && strings.EqualFold(strings.TrimSpace(code), "query_failed") {
			return true
		}
		for _, key := range []string{"message", "detail", "error", "data"} {
			if mikuapiTemporaryQueryMessage(value[key], depth+1) {
				return true
			}
		}
	}
	return false
}

func mikuapiPollRetryDelay(interval time.Duration, failures int) time.Duration {
	const maximum = 30 * time.Second
	delay := interval
	for i := 1; i < failures && delay < maximum; i++ {
		delay *= 2
	}
	if delay > maximum {
		return maximum
	}
	return delay
}
