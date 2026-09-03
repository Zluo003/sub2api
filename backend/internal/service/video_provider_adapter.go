package service

import "strings"

type videoProviderAdapter interface {
	Provider() string
	DefaultBaseURL() string
	DefaultAPIPath() string
	Compatible(model, resolution string) bool
	CompatibleRequest(normalized *normalizedVideoRequest) bool
	UpstreamModel(account *Account, normalized *normalizedVideoRequest) string
	BuildCreateBody(normalized *normalizedVideoRequest, upstreamModel string) map[string]any
}

func videoProviderAdapterForAccount(account *Account) videoProviderAdapter {
	switch videoAccountProvider(account) {
	case videoProviderYCYAPI:
		return ycyapiVideoProviderAdapter{}
	case videoProviderNewtoken:
		return newtokenVideoProviderAdapter{}
	case videoProviderJingyu:
		return jingyuVideoProviderAdapter{}
	default:
		return aigodVideoProviderAdapter{}
	}
}

func videoProviderAdapterByName(provider string) videoProviderAdapter {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case videoProviderYCYAPI:
		return ycyapiVideoProviderAdapter{}
	case videoProviderNewtoken:
		return newtokenVideoProviderAdapter{}
	case videoProviderJingyu:
		return jingyuVideoProviderAdapter{}
	default:
		return aigodVideoProviderAdapter{}
	}
}

func resolvedMappedVideoModel(account *Account, downstreamModel string) string {
	if account == nil {
		return ""
	}
	mappedModel, matched := account.ResolveMappedModel(strings.TrimSpace(downstreamModel))
	if !matched {
		return ""
	}
	mappedModel = strings.TrimSpace(mappedModel)
	if mappedModel == "" || mappedModel == strings.TrimSpace(downstreamModel) {
		return ""
	}
	return mappedModel
}
