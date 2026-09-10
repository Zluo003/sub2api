package service

import "strings"

type aigodVideoProviderAdapter struct{}

func (a aigodVideoProviderAdapter) Provider() string {
	return videoProviderAigod
}

func (a aigodVideoProviderAdapter) DefaultBaseURL() string {
	return videoDefaultBaseURL
}

func (a aigodVideoProviderAdapter) DefaultAPIPath() string {
	return videoDefaultAPIPath
}

func (a aigodVideoProviderAdapter) Compatible(model, resolution string) bool {
	model = strings.TrimSpace(model)
	resolution = strings.TrimSpace(resolution)
	// aigod only carries the Seedance family. Without this guard the shared
	// resolution table would make the default adapter claim every video model
	// and pull requests onto accounts that cannot serve them.
	switch model {
	case VideoModelSeedance20, VideoModelSeedance20Fast, VideoModelSeedance25:
	default:
		return false
	}
	if resolution == VideoResolution4K {
		return false
	}
	return IsSupportedVideoResolution(model, resolution)
}

func (a aigodVideoProviderAdapter) CompatibleRequest(normalized *normalizedVideoRequest) bool {
	return normalized != nil && a.Compatible(normalized.Model, normalized.Resolution)
}

func (a aigodVideoProviderAdapter) UpstreamModel(account *Account, normalized *normalizedVideoRequest) string {
	if normalized == nil {
		return ""
	}
	if mappedModel := resolvedMappedVideoModel(account, normalized.Model); mappedModel != "" {
		return mappedModel
	}
	return SeedanceUpstreamModel(normalized.Model, normalized.Resolution)
}

func (a aigodVideoProviderAdapter) BuildCreateBody(normalized *normalizedVideoRequest, upstreamModel string) map[string]any {
	if normalized == nil {
		return nil
	}
	return normalized.UpstreamBody(upstreamModel)
}
