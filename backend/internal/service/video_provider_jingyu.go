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
	return strings.TrimSpace(model) == VideoModelSeedance20 && strings.TrimSpace(resolution) == VideoResolution720P
}

func (j jingyuVideoProviderAdapter) UpstreamModel(account *Account, normalized *normalizedVideoRequest) string {
	if normalized == nil {
		return ""
	}
	if mappedModel := resolvedMappedVideoModel(account, normalized.Model); mappedModel != "" {
		return mappedModel
	}
	return videoJingyuSeedanceModel
}

func (j jingyuVideoProviderAdapter) BuildCreateBody(normalized *normalizedVideoRequest, upstreamModel string) map[string]any {
	if normalized == nil {
		return nil
	}
	return normalized.JingyuUpstreamBody(upstreamModel)
}
