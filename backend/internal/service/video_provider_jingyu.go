package service

import "strings"

type jingyuVideoProviderAdapter struct{}

func (j jingyuVideoProviderAdapter) Provider() string {
	return videoProviderJingyu
}

func (j jingyuVideoProviderAdapter) DefaultBaseURL() string {
	return videoDefaultJingyuBaseURL
}

func (j jingyuVideoProviderAdapter) DefaultAPIPath() string {
	return videoDefaultJingyuAPIPath
}

func (j jingyuVideoProviderAdapter) Compatible(model, resolution string) bool {
	model = strings.TrimSpace(model)
	resolution = strings.TrimSpace(resolution)
	switch model {
	case VideoModelSeedance20:
		return resolution == VideoResolution480P ||
			resolution == VideoResolution720P ||
			resolution == VideoResolution1080P ||
			resolution == VideoResolution4K
	case VideoModelSeedance25:
		return resolution == VideoResolution480P || resolution == VideoResolution720P
	default:
		return false
	}
}

func (j jingyuVideoProviderAdapter) CompatibleRequest(normalized *normalizedVideoRequest) bool {
	return normalized != nil && j.Compatible(normalized.Model, normalized.Resolution)
}

func (j jingyuVideoProviderAdapter) UpstreamModel(account *Account, normalized *normalizedVideoRequest) string {
	if normalized == nil {
		return ""
	}
	if mappedModel := resolvedMappedVideoModel(account, normalized.Model); mappedModel != "" {
		return mappedModel
	}
	return defaultJingyuUpstreamModel(normalized.Model)
}

func (j jingyuVideoProviderAdapter) BuildCreateBody(normalized *normalizedVideoRequest, upstreamModel string) map[string]any {
	if normalized == nil {
		return nil
	}
	return normalized.JingyuUpstreamBody(upstreamModel)
}
