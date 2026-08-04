package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type gatewayAgentModelRepoStub struct {
	models []service.AgentGroupModel
	rates  []service.AgentPlatformRate
}

func (r *gatewayAgentModelRepoStub) SyncDiscovered(context.Context, int64, []service.AgentModelDiscovery, time.Time) error {
	return nil
}
func (r *gatewayAgentModelRepoStub) ListModels(context.Context, int64, bool) ([]service.AgentGroupModel, error) {
	return append([]service.AgentGroupModel(nil), r.models...), nil
}
func (r *gatewayAgentModelRepoStub) GetModelByID(context.Context, int64, int64) (*service.AgentGroupModel, error) {
	return nil, sql.ErrNoRows
}
func (r *gatewayAgentModelRepoStub) GetEnabledModel(_ context.Context, groupID int64, platform, modelCode string) (*service.AgentGroupModel, error) {
	for _, model := range r.models {
		if model.GroupID != groupID || model.Platform != platform || model.ModelCode != modelCode || !model.Enabled || !model.Available || model.Excluded {
			continue
		}
		copy := model
		copy.Prices = append([]service.AgentModelPrice(nil), model.Prices...)
		return &copy, nil
	}
	return nil, sql.ErrNoRows
}
func (r *gatewayAgentModelRepoStub) UpdateModelConfig(context.Context, int64, int64, string, bool, []service.AgentModelPrice) error {
	return nil
}
func (r *gatewayAgentModelRepoStub) ExcludeModel(context.Context, int64, int64, time.Time) error {
	return nil
}
func (r *gatewayAgentModelRepoStub) ListPlatformRates(context.Context, int64) ([]service.AgentPlatformRate, error) {
	return append([]service.AgentPlatformRate(nil), r.rates...), nil
}
func (r *gatewayAgentModelRepoStub) UpsertPlatformRate(context.Context, int64, string, float64) error {
	return nil
}
func (r *gatewayAgentModelRepoStub) GetPlatformRate(context.Context, int64, string) (*service.AgentPlatformRate, error) {
	return nil, sql.ErrNoRows
}

func newGatewayAgentCatalogForTest(repo service.AccountRepository, models ...service.AgentGroupModel) *service.AgentModelCatalogService {
	return service.NewAgentModelCatalogService(repo, nil, &gatewayAgentModelRepoStub{models: models})
}

func enabledGatewayAgentModel(platform, modelCode, mediaType string) service.AgentGroupModel {
	return service.AgentGroupModel{
		Platform: platform, ModelCode: modelCode, MediaType: mediaType,
		Enabled: true, Available: true, Prices: []service.AgentModelPrice{},
	}
}

type gatewayModelsAccountRepoStub struct {
	service.AccountRepository

	byGroup map[int64][]service.Account
	err     error
}

type gatewayModelsResponseForTest struct {
	Object                 string                    `json:"object"`
	CatalogSchemaVersion   int                       `json:"catalog_schema_version"`
	GatewayContractVersion string                    `json:"gateway_contract_version"`
	Data                   []gatewayModelItemForTest `json:"data"`
	Provenance             struct {
		CatalogSource     string   `json:"catalog_source"`
		CapabilitySources []string `json:"capability_sources"`
	} `json:"provenance"`
}

type gatewayModelItemForTest struct {
	ID                      string                                `json:"id"`
	Object                  string                                `json:"object"`
	Created                 int64                                 `json:"created"`
	OwnedBy                 string                                `json:"owned_by"`
	CreatedAt               string                                `json:"created_at"`
	SupportsReasoningEffort bool                                  `json:"supportsReasoningEffort"`
	ReasoningEffort         string                                `json:"reasoningEffort"`
	ReasoningEfforts        []gatewayReasoningEffortOptionForTest `json:"reasoningEfforts"`
	Source                  string                                `json:"source"`
	Availability            string                                `json:"availability"`
	CapabilitySource        string                                `json:"capability_source"`
	Capabilities            struct {
		MediaTypes       []string `json:"media_types"`
		Platforms        []string `json:"platforms"`
		Interfaces       []string `json:"interfaces"`
		InputModalities  []string `json:"input_modalities"`
		OutputModalities []string `json:"output_modalities"`
		Operations       []string `json:"operations"`
		Streaming        bool     `json:"streaming"`
		Asynchronous     bool     `json:"asynchronous"`
	} `json:"capabilities"`
}

type gatewayReasoningEffortOptionForTest struct {
	Value   string `json:"value"`
	Label   string `json:"label"`
	Default bool   `json:"default"`
}

func (s *gatewayModelsAccountRepoStub) ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]service.Account, error) {
	if s.err != nil {
		return nil, s.err
	}
	accounts, ok := s.byGroup[groupID]
	if !ok {
		return nil, nil
	}
	out := make([]service.Account, len(accounts))
	copy(out, accounts)
	return out, nil
}

func newGatewayModelsHandlerForTest(repo service.AccountRepository) *GatewayHandler {
	return &GatewayHandler{
		gatewayService: service.NewGatewayService(
			repo,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		),
	}
}

func newGatewayModelsHandlerWithCatalogForTest(repo service.AccountRepository, catalog *service.AgentModelCatalogService) *GatewayHandler {
	h := newGatewayModelsHandlerForTest(repo)
	h.agentModelCatalog = catalog
	return h
}

func TestDefaultModelIDsForCompositeIncludesAntigravityDefaults(t *testing.T) {
	antigravityIDs := defaultModelIDsForPlatform(service.PlatformAntigravity)
	require.NotEmpty(t, antigravityIDs)

	compositeIDs := defaultModelIDsForPlatform(service.PlatformComposite)
	require.Contains(t, compositeIDs, antigravityIDs[0])
}

func TestGatewayModels_GeminiGroupFallsBackToGeminiModels(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(20)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{ID: 1, Platform: service.PlatformGemini},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{ID: groupID, Platform: service.PlatformGemini},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "list", got.Object)
	require.Contains(t, modelIDsForTest(got.Data), "gemini-2.5-flash")
	require.NotContains(t, modelIDsForTest(got.Data), "claude-sonnet-4-6")
}

func TestGatewayModels_Grok45AdvertisesReasoningEffortForGrokBuild(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(4409)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{
						ID:       1,
						Platform: service.PlatformGrok,
						Credentials: map[string]any{
							"model_mapping": map[string]any{"grok-4.5": "grok-4.5"},
						},
					},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{ID: groupID, Platform: service.PlatformGrok},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got.Data, 1)
	model := got.Data[0]
	require.Equal(t, "grok-4.5", model.ID)
	require.True(t, model.SupportsReasoningEffort)
	require.Equal(t, "high", model.ReasoningEffort)
	require.Equal(t, []gatewayReasoningEffortOptionForTest{
		{Value: "low", Label: "Low"},
		{Value: "medium", Label: "Medium"},
		{Value: "high", Label: "High", Default: true},
	}, model.ReasoningEfforts)
}

func TestGatewayModels_GeminiGroupFiltersMappedModelsByPlatform(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(21)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{
						ID:       1,
						Platform: service.PlatformAnthropic,
						Credentials: map[string]any{
							"model_mapping": map[string]any{
								"claude-sonnet-4-6": "claude-sonnet-4-6",
							},
						},
					},
					{
						ID:       2,
						Platform: service.PlatformGemini,
						Credentials: map[string]any{
							"model_mapping": map[string]any{
								"gemini-2.5-flash": "gemini-2.5-flash",
							},
						},
					},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{ID: groupID, Platform: service.PlatformGemini},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, []string{"gemini-2.5-flash"}, modelIDsForTest(got.Data))
}

func TestGatewayModels_CustomModelsListDisabledKeepsOriginalModels(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(22)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{
						ID:       1,
						Platform: service.PlatformOpenAI,
						Credentials: map[string]any{
							"model_mapping": map[string]any{
								"gpt-5.5": "gpt-5.5",
								"gpt-5.4": "gpt-5.4",
							},
						},
					},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformOpenAI,
			ModelsListConfig: service.GroupModelsListConfig{
				Enabled: false,
				Models:  []string{"gpt-5.5"},
			},
		},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, []string{"gpt-5.4", "gpt-5.5"}, modelIDsForTest(got.Data))
}

func TestGatewayModels_AgentGroupUsesConfiguredCapabilitiesAndSchedulableAccounts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(7)
	repo := &gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{
		groupID: {
			{ID: 1, Platform: service.PlatformOpenAI, Credentials: map[string]any{"model_mapping": map[string]any{
				"gpt-5.4": "gpt-5.4", "gpt-image-2": "gpt-image-2",
			}}},
			{ID: 2, Platform: service.PlatformAnthropic, Credentials: map[string]any{"model_mapping": map[string]any{
				"claude-opus-4.8": "claude-opus-4.8",
			}}},
			{ID: 3, Platform: service.PlatformSeedance, Credentials: map[string]any{"model_mapping": map[string]any{
				"seedance-2.0": "seedance-api-2.0",
			}}},
			{ID: 4, Platform: service.PlatformGemini, Credentials: map[string]any{"model_mapping": map[string]any{
				"gemini-3.1-flash-image": "gemini-3.1-flash-image",
			}}},
		},
	}}
	h := newGatewayModelsHandlerWithCatalogForTest(repo, newGatewayAgentCatalogForTest(repo,
		enabledGatewayAgentModel(service.PlatformOpenAI, "gpt-5.4", service.AgentMediaTypeText),
		enabledGatewayAgentModel(service.PlatformOpenAI, "gpt-image-2", service.AgentMediaTypeImage),
		enabledGatewayAgentModel(service.PlatformAnthropic, "claude-opus-4.8", service.AgentMediaTypeText),
		enabledGatewayAgentModel(service.PlatformSeedance, "seedance-2.0", service.AgentMediaTypeVideo),
		enabledGatewayAgentModel(service.PlatformGemini, "gemini-3.1-flash-image", service.AgentMediaTypeImage),
	))
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{Group: &service.Group{
		ID: groupID, Platform: service.PlatformOpenAI, Kind: "agent", SystemCode: "yingzo",
	}})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, service.AgentModelCatalogSchemaVersion, got.CatalogSchemaVersion)
	require.Equal(t, service.AgentAPIContractVersion, got.GatewayContractVersion)
	require.Equal(t, service.AgentModelCatalogSource, got.Provenance.CatalogSource)
	require.Equal(t, []string{"gateway"}, got.Provenance.CapabilitySources)
	require.Equal(t, []string{"claude-opus-4.8", "gemini-3.1-flash-image", "gpt-5.4", "gpt-image-2", "seedance-2.0"}, modelIDsForTest(got.Data))
	require.Equal(t, []string{"text"}, got.Data[0].Capabilities.MediaTypes)
	require.Equal(t, []string{"anthropic"}, got.Data[0].Capabilities.Platforms)
	require.Equal(t, []string{service.AgentInterfaceAnthropicMessages}, got.Data[0].Capabilities.Interfaces)
	require.Equal(t, []string{"text"}, got.Data[0].Capabilities.InputModalities)
	require.Equal(t, []string{"text"}, got.Data[0].Capabilities.OutputModalities)
	require.Equal(t, []string{"text.generate"}, got.Data[0].Capabilities.Operations)
	require.True(t, got.Data[0].Capabilities.Streaming)
	require.Equal(t, []string{"image"}, got.Data[1].Capabilities.MediaTypes)
	require.Equal(t, []string{"gemini"}, got.Data[1].Capabilities.Platforms)
	require.Equal(t, []string{service.AgentInterfaceGeminiGenerateContent}, got.Data[1].Capabilities.Interfaces)
	require.Equal(t, []string{"text", "image"}, got.Data[1].Capabilities.InputModalities)
	require.Equal(t, []string{"text", "image"}, got.Data[3].Capabilities.InputModalities)
	require.Equal(t, []string{"image.generate"}, got.Data[3].Capabilities.Operations)
	require.Equal(t, []string{service.AgentInterfaceOpenAIImages}, got.Data[3].Capabilities.Interfaces)
	require.Equal(t, []string{"text", "image", "video", "audio"}, got.Data[4].Capabilities.InputModalities)
	require.Equal(t, []string{"video.generate"}, got.Data[4].Capabilities.Operations)
	require.Equal(t, []string{service.AgentInterfaceSeedanceVideos}, got.Data[4].Capabilities.Interfaces)
	require.True(t, got.Data[4].Capabilities.Asynchronous)
	require.Equal(t, "sub2api", got.Data[0].OwnedBy)
	require.Equal(t, "gateway", got.Data[0].Source)
	require.Equal(t, "advertised", got.Data[0].Availability)
	require.Equal(t, "gateway", got.Data[0].CapabilitySource)
	require.NotEmpty(t, rec.Header().Get("ETag"))
	require.Equal(t, "private, max-age=30, must-revalidate", rec.Header().Get("Cache-Control"))
	require.Equal(t, "Authorization", rec.Header().Get("Vary"))
	require.Equal(t, "3", rec.Header().Get("X-Model-Catalog-Schema-Version"))
	require.Equal(t, service.AgentAPIContractVersion, rec.Header().Get("X-Gateway-Contract-Version"))
}

func TestGatewayModels_AgentCatalogSupportsConditionalGET(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(8)
	repo := &gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{
		groupID: {{ID: 1, Platform: service.PlatformOpenAI}},
	}}
	h := newGatewayModelsHandlerWithCatalogForTest(repo, newGatewayAgentCatalogForTest(repo,
		enabledGatewayAgentModel(service.PlatformOpenAI, "gpt-5.4", service.AgentMediaTypeText),
	))
	request := func(ifNoneMatch string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		c.Request.Header.Set("If-None-Match", ifNoneMatch)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{Group: &service.Group{
			ID: groupID, Platform: service.PlatformOpenAI, Kind: "agent", SystemCode: "yingzo",
		}})
		h.Models(c)
		return recorder
	}

	first := request("")
	require.Equal(t, http.StatusOK, first.Code)
	etag := first.Header().Get("ETag")
	require.NotEmpty(t, etag)
	require.NotEmpty(t, first.Body.Bytes())

	second := request(`W/` + etag)
	require.Equal(t, http.StatusNotModified, second.Code)
	require.Empty(t, second.Body.Bytes())
	require.Equal(t, etag, second.Header().Get("ETag"))
	require.Equal(t, service.AgentAPIContractVersion, second.Header().Get("X-Gateway-Contract-Version"))
}

func TestRequestETagMatches(t *testing.T) {
	require.True(t, requestETagMatches(`"old", W/"current"`, `"current"`))
	require.True(t, requestETagMatches("*", `"current"`))
	require.False(t, requestETagMatches(`"other"`, `"current"`))
}

func TestAgentCapabilitiesUseTheConfiguredTextCategoryForEmbeddings(t *testing.T) {
	capabilities := agentCapabilitiesForModel(
		"text-embedding-3-small",
		[]string{service.AgentMediaTypeText},
		[]string{service.PlatformOpenAI},
		[]string{service.AgentInterfaceOpenAIEmbeddings},
	)

	require.Equal(t, []string{"text"}, capabilities.InputModalities)
	require.Equal(t, []string{"text"}, capabilities.OutputModalities)
	require.Equal(t, []string{"text.generate"}, capabilities.Operations)
	require.True(t, capabilities.Streaming)
	require.False(t, capabilities.Asynchronous)
}

func TestAgentCapabilitiesAdvertiseReferenceMediaInputs(t *testing.T) {
	tests := []struct {
		name       string
		mediaType  string
		modalities []string
	}{
		{name: "image", mediaType: service.AgentMediaTypeImage, modalities: []string{"text", "image"}},
		{name: "video", mediaType: service.AgentMediaTypeVideo, modalities: []string{"text", "image", "video", "audio"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capabilities := agentCapabilitiesForModel("unknown", []string{tt.mediaType}, nil, nil)

			require.Equal(t, tt.modalities, capabilities.InputModalities)
		})
	}
}

func TestAgentCapabilitiesAdvertisePerModelMediaOptions(t *testing.T) {
	image := agentCapabilitiesForModel(
		"gpt-image-2",
		[]string{service.AgentMediaTypeImage},
		[]string{service.PlatformOpenAI},
		[]string{service.AgentInterfaceOpenAIImages},
	)
	require.Equal(t, []string{"1K", "2K", "4K"}, image.SupportedImageSizes)
	require.Contains(t, image.SupportedAspectRatios, "16:9")
	require.Equal(t, 16, image.MaxInputImages)

	standard := agentCapabilitiesForModel(
		service.VideoModelSeedance20,
		[]string{service.AgentMediaTypeVideo},
		[]string{service.PlatformSeedance},
		[]string{service.AgentInterfaceSeedanceVideos},
	)
	fast := agentCapabilitiesForModel(
		service.VideoModelSeedance20Fast,
		[]string{service.AgentMediaTypeVideo},
		[]string{service.PlatformSeedance},
		[]string{service.AgentInterfaceSeedanceVideos},
	)
	require.Equal(t, []string{"480p", "720p", "1080p", "4K"}, standard.SupportedVideoResolutions)
	require.Equal(t, []string{"480p", "720p"}, fast.SupportedVideoResolutions)
	require.Equal(t, []int{4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}, standard.SupportedVideoDurationsSec)
	require.NotNil(t, standard.SupportsVideoAudio)
	require.True(t, *standard.SupportsVideoAudio)
}

func TestGatewayModels_AgentGroupFailsClosedWhenCatalogUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(7)
	repo := &gatewayModelsAccountRepoStub{err: context.DeadlineExceeded}
	h := newGatewayModelsHandlerWithCatalogForTest(repo, newGatewayAgentCatalogForTest(repo))
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{Group: &service.Group{
		ID: groupID, Platform: service.PlatformOpenAI, Kind: "agent", SystemCode: "yingzo",
	}})

	h.Models(c)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Contains(t, rec.Body.String(), "agent_model_catalog_unavailable")
}

func TestGeminiV1BetaModels_AgentUsesOnlyAssignedGeminiCatalogue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(12)
	repo := &gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{
		groupID: {
			{Platform: service.PlatformGemini, Credentials: map[string]any{"model_mapping": map[string]any{
				"gemini-2.5-flash": "gemini-upstream",
			}}},
			{Platform: service.PlatformOpenAI, Credentials: map[string]any{"model_mapping": map[string]any{
				"gpt-5.4": "gpt-upstream",
			}}},
		},
	}}
	h := newGatewayModelsHandlerWithCatalogForTest(repo, newGatewayAgentCatalogForTest(repo,
		enabledGatewayAgentModel(service.PlatformGemini, "gemini-2.5-flash", service.AgentMediaTypeText),
		enabledGatewayAgentModel(service.PlatformOpenAI, "gpt-5.4", service.AgentMediaTypeText),
	))
	apiKey := &service.APIKey{GroupID: &groupID, Group: &service.Group{
		ID: groupID, Platform: service.PlatformOpenAI, Kind: "agent", SystemCode: "yingzo",
	}}

	listRecorder := httptest.NewRecorder()
	listContext, _ := gin.CreateTestContext(listRecorder)
	listContext.Request = httptest.NewRequest(http.MethodGet, "/v1beta/models", nil)
	listContext.Set(string(middleware2.ContextKeyAPIKey), apiKey)
	middleware2.SetForcePlatform(listContext, service.PlatformGemini)
	h.GeminiV1BetaListModels(listContext)
	require.Equal(t, http.StatusOK, listRecorder.Code)
	var list struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	require.NoError(t, json.Unmarshal(listRecorder.Body.Bytes(), &list))
	require.Equal(t, []struct {
		Name string `json:"name"`
	}{{Name: "models/gemini-2.5-flash"}}, list.Models)

	getRecorder := httptest.NewRecorder()
	getContext, _ := gin.CreateTestContext(getRecorder)
	getContext.Request = httptest.NewRequest(http.MethodGet, "/v1beta/models/gpt-5.4", nil)
	getContext.Params = gin.Params{{Key: "model", Value: "gpt-5.4"}}
	getContext.Set(string(middleware2.ContextKeyAPIKey), apiKey)
	middleware2.SetForcePlatform(getContext, service.PlatformGemini)
	h.GeminiV1BetaGetModel(getContext)
	require.Equal(t, http.StatusNotFound, getRecorder.Code)
}

func TestGatewayModels_CustomModelsListFiltersAndOrdersMappedModels(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(23)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{
						ID:       1,
						Platform: service.PlatformOpenAI,
						Credentials: map[string]any{
							"model_mapping": map[string]any{
								"gpt-5.4":         "gpt-5.4",
								"gpt-5.5":         "gpt-5.5",
								"legacy-gpt-2024": "legacy-gpt-2024",
							},
						},
					},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformOpenAI,
			ModelsListConfig: service.GroupModelsListConfig{
				Enabled: true,
				Models:  []string{"gpt-5.5", "missing-model", "gpt-5.4"},
			},
		},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, []string{"gpt-5.5", "gpt-5.4"}, modelIDsForTest(got.Data))
}

func TestGatewayModels_CompositeCustomModelsListFiltersAcrossConcretePlatforms(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(33)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{
						ID:       1,
						Platform: service.PlatformOpenAI,
						Credentials: map[string]any{
							"model_mapping": map[string]any{
								"gpt-5.4": "gpt-5.4",
								"gpt-5.5": "gpt-5.5",
							},
						},
					},
					{
						ID:       2,
						Platform: service.PlatformGemini,
						Credentials: map[string]any{
							"model_mapping": map[string]any{
								"gemini-2.5-flash": "gemini-2.5-flash",
							},
						},
					},
					{
						ID:       3,
						Platform: service.PlatformAntigravity,
						Credentials: map[string]any{
							"model_mapping": map[string]any{
								"ag-custom-model": "ag-custom-model",
							},
						},
					},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformComposite,
			ModelsListConfig: service.GroupModelsListConfig{
				Enabled: true,
				Models:  []string{"gemini-2.5-flash", "missing-model", "ag-custom-model", "gpt-5.5"},
			},
		},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, []string{"gemini-2.5-flash", "ag-custom-model", "gpt-5.5"}, modelIDsForTest(got.Data))
}

func TestGatewayModels_CompositeUnmappedAccountsFallbackToLinkedPlatformsOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(34)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{ID: 1, Platform: service.PlatformOpenAI},
					{ID: 2, Platform: service.PlatformGrok},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{ID: groupID, Platform: service.PlatformComposite},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))

	ids := modelIDsForTest(got.Data)
	require.Contains(t, ids, "gpt-5.5")
	require.Contains(t, ids, "grok-4.3")
	require.NotContains(t, ids, "claude-sonnet-4-6")
	require.NotContains(t, ids, "gemini-2.5-flash")
}

func TestGatewayModels_CustomModelsListKeepsConcreteModelAllowedByWildcardMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(26)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{
						ID:       1,
						Platform: service.PlatformAnthropic,
						Credentials: map[string]any{
							"model_mapping": map[string]any{
								"claude-*": "claude-sonnet-4-6",
							},
						},
					},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformAnthropic,
			ModelsListConfig: service.GroupModelsListConfig{
				Enabled: true,
				Models:  []string{"claude-sonnet-4-6"},
			},
		},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, []string{"claude-sonnet-4-6"}, modelIDsForTest(got.Data))
}

func TestGatewayModels_AnthropicCustomModelsListIncludesOAuthClaudeAndMappedDeepSeek(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(28)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{
						ID:       1,
						Platform: service.PlatformAnthropic,
						Type:     service.AccountTypeOAuth,
					},
					{
						ID:       2,
						Platform: service.PlatformAnthropic,
						Type:     service.AccountTypeAPIKey,
						Credentials: map[string]any{
							"model_mapping": map[string]any{
								"deepseek-v4-pro": "deepseek-v4-pro",
							},
						},
					},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformAnthropic,
			ModelsListConfig: service.GroupModelsListConfig{
				Enabled: true,
				Models:  []string{"claude-fable-5", "claude-opus-4-8", "deepseek-v4-pro"},
			},
		},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, []string{"claude-fable-5", "claude-opus-4-8", "deepseek-v4-pro"}, modelIDsForTest(got.Data))
}

func TestGatewayModels_AnthropicCustomModelsListDisabledKeepsMappedModelList(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(29)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{
						ID:       1,
						Platform: service.PlatformAnthropic,
						Type:     service.AccountTypeOAuth,
					},
					{
						ID:       2,
						Platform: service.PlatformAnthropic,
						Type:     service.AccountTypeAPIKey,
						Credentials: map[string]any{
							"model_mapping": map[string]any{
								"deepseek-v4-pro": "deepseek-v4-pro",
							},
						},
					},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformAnthropic,
			ModelsListConfig: service.GroupModelsListConfig{
				Enabled: false,
				Models:  []string{"claude-fable-5", "deepseek-v4-pro"},
			},
		},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, []string{"deepseek-v4-pro"}, modelIDsForTest(got.Data))
}

func TestGatewayModels_AnthropicCustomModelsListIncludesOAuthClaudeWithoutMappings(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(30)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{
						ID:       1,
						Platform: service.PlatformAnthropic,
						Type:     service.AccountTypeOAuth,
					},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformAnthropic,
			ModelsListConfig: service.GroupModelsListConfig{
				Enabled: true,
				Models:  []string{"claude-opus-4-6-thinking", "claude-sonnet-4-5"},
			},
		},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, []string{"claude-opus-4-6-thinking", "claude-sonnet-4-5"}, modelIDsForTest(got.Data))
}

func TestGatewayModels_CustomModelsListCanReturnEmptyWhenSelectionsUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(24)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{
						ID:       1,
						Platform: service.PlatformOpenAI,
						Credentials: map[string]any{
							"model_mapping": map[string]any{
								"gpt-5.4": "gpt-5.4",
							},
						},
					},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformOpenAI,
			ModelsListConfig: service.GroupModelsListConfig{
				Enabled: true,
				Models:  []string{"gpt-5.5"},
			},
		},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Empty(t, modelIDsForTest(got.Data))
}

func TestGatewayModels_CustomModelsListFiltersDefaultFallbackModels(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(25)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{ID: 1, Platform: service.PlatformOpenAI},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformOpenAI,
			ModelsListConfig: service.GroupModelsListConfig{
				Enabled: true,
				Models:  []string{"gpt-5.5", "legacy-gpt-2024", "gpt-5.4"},
			},
		},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, []string{"gpt-5.5", "gpt-5.4"}, modelIDsForTest(got.Data))
}

func TestGatewayModels_OpenAICustomModelsListKeepsOpenAIResponseShapeForDefaultFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(27)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{ID: 1, Platform: service.PlatformOpenAI},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformOpenAI,
			ModelsListConfig: service.GroupModelsListConfig{
				Enabled: true,
				Models:  []string{"gpt-5.5", "gpt-5.4"},
			},
		},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, []string{"gpt-5.5", "gpt-5.4"}, modelIDsForTest(got.Data))
	require.Equal(t, "model", got.Data[0].Object)
	require.NotZero(t, got.Data[0].Created)
	require.Equal(t, "openai", got.Data[0].OwnedBy)
	require.Empty(t, got.Data[0].CreatedAt)
}

func modelIDsForTest(models []gatewayModelItemForTest) []string {
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	return ids
}
