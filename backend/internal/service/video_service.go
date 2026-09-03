package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"golang.org/x/sync/singleflight"
)

const (
	videoProviderAigod             = "aigod"
	videoProviderJingyu            = "jingyu"
	videoProviderYCYAPI            = "ycyapi"
	videoProviderNewtoken          = "newtoken"
	videoDefaultBaseURL            = "https://api.aigod.one"
	videoDefaultYCYAPIBaseURL      = "https://ycyapi.cn"
	videoDefaultNewtokenBaseURL    = "https://newtoken.club"
	videoDefaultAPIPath            = "/v1/videos"
	videoDefaultJingyuBaseURL      = "https://api.jingyuapi.art"
	videoDefaultJingyuAPIPath      = "/v1/video/generations"
	videoJingyuSeedance20Model     = "yu-video-2-pro"
	videoJingyuSeedance25Model     = "yu-video-2.5-pro"
	videoYCYAPISeedance20Model     = "firefly-video-v2"
	videoYCYAPISeedance20FastModel = "firefly-video-v2-fast"
	videoYCYAPISeedance25Model     = "leonardo-seedance-2.5"
	// newtoken encodes the output resolution into the upstream model id, so the
	// adapter routes on (downstream model, resolution) instead of a static map.
	videoNewtokenSeedance20720PModel     = "sd2.0-720p-official"
	videoNewtokenSeedance201080PModel    = "sd2.0-1080p-official"
	videoNewtokenSeedance20Fast720PModel = "sd2.0-720p-fast-official"
	videoNewtokenSeedance25720PModel     = "sd2.5-720p-official"
	videoDefaultPollInterval       = 2 * time.Second
	videoDefaultPollTimeout        = 5 * time.Minute
	videoDefaultRequestTimeout     = 60 * time.Second
	videoDefaultConnectTimeout     = 15 * time.Second
	videoJingyuPollInterval        = 5 * time.Second
	videoJingyuPollTimeout         = 30 * time.Minute
	videoJingyuRequestTimeout      = 30 * time.Minute
	videoJingyuConnectTimeout      = 60 * time.Second
	videoYCYAPIPollInterval        = 5 * time.Second
	videoYCYAPIPollTimeout         = 60 * time.Minute
	videoYCYAPIRequestTimeout      = 5 * time.Minute
	videoYCYAPIConnectTimeout      = 15 * time.Second
	videoNewtokenPollInterval      = 5 * time.Second
	videoNewtokenPollTimeout       = 60 * time.Minute
	videoNewtokenRequestTimeout    = 5 * time.Minute
	videoNewtokenConnectTimeout    = 15 * time.Second
	videoMinDurationSeconds        = 4
	videoMaxDurationSeconds        = 15
	videoSeedance25MaxDuration     = 30
	videoMaxReferenceVideoTotal    = 15
	videoSeedance25MaxReferences   = 50
	videoPublicIDPrefix            = "video_"
	videoObject                    = "video"
	videoAbilityTextToVideo        = "video_text_to_video"
	videoAbilityImageToVideo       = "video_image_to_video"
	videoAbilityStartEndToVideo    = "video_start_end_to_video"
	videoAbilityReferenceToVideo   = "video_reference_to_video"
	jingyuVideoCallbackPath        = "/api/v1/webhooks/jingyu/videos/"
)

type VideoService struct {
	accountRepo          VideoAccountRepository
	taskRepo             VideoTaskRepository
	pricingRepo          VideoGroupPricingRuleRepository
	usageLogRepo         UsageLogRepository
	usageBillingRepo     UsageBillingRepository
	userRepo             UserRepository
	userSubRepo          UserSubscriptionRepository
	apiKeyService        APIKeyQuotaUpdater
	billingCache         *BillingCacheService
	deferredService      *DeferredService
	balanceNotify        *BalanceNotifyService
	quotaRepo            UserPlatformQuotaRepository
	httpUpstream         HTTPUpstream
	cfg                  *config.Config
	agentModels          *AgentModelCatalogService
	videoResultPublisher VideoResultPublisher
	videoReferenceClient *http.Client
	apiKeyLoader         *APIKeyService
	jingyuCallbackGroup  singleflight.Group

	startLifecycleFunc func(VideoTaskLifecycleInput)
}

func (s *VideoService) SetAgentModelCatalog(catalog *AgentModelCatalogService) {
	if s != nil {
		s.agentModels = catalog
	}
}

func (s *VideoService) SetVideoResultPublisher(publisher VideoResultPublisher) {
	if s != nil {
		s.videoResultPublisher = publisher
	}
}

func NewVideoService(
	accountRepo VideoAccountRepository,
	taskRepo VideoTaskRepository,
	pricingRepo VideoGroupPricingRuleRepository,
	usageLogRepo UsageLogRepository,
	usageBillingRepo UsageBillingRepository,
	userRepo UserRepository,
	userSubRepo UserSubscriptionRepository,
	apiKeyService *APIKeyService,
	billingCache *BillingCacheService,
	deferredService *DeferredService,
	balanceNotify *BalanceNotifyService,
	quotaRepo UserPlatformQuotaRepository,
	httpUpstream HTTPUpstream,
	cfg *config.Config,
) *VideoService {
	return &VideoService{
		accountRepo:      accountRepo,
		taskRepo:         taskRepo,
		pricingRepo:      pricingRepo,
		usageLogRepo:     usageLogRepo,
		usageBillingRepo: usageBillingRepo,
		userRepo:         userRepo,
		userSubRepo:      userSubRepo,
		apiKeyService:    apiKeyService,
		apiKeyLoader:     apiKeyService,
		billingCache:     billingCache,
		deferredService:  deferredService,
		balanceNotify:    balanceNotify,
		quotaRepo:        quotaRepo,
		httpUpstream:     httpUpstream,
		cfg:              cfg,
	}
}

func (s *VideoService) CreateTask(ctx context.Context, input *VideoCreateInput) (*VideoResponse, error) {
	if input == nil || input.APIKey == nil || input.APIKey.User == nil || input.APIKey.Group == nil || input.Request == nil {
		return nil, videoBadRequest("invalid_video_request", "Invalid video request")
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		return s.createTask(ctx, input)
	}

	coordinator := DefaultIdempotencyCoordinator()
	if coordinator == nil {
		return nil, ErrIdempotencyStoreUnavail
	}
	actorScope := "api_key:" + strconv.FormatInt(input.APIKey.ID, 10)
	result, err := coordinator.Execute(ctx, IdempotencyExecuteOptions{
		// The repository uniqueness key is (scope, key hash), so the API key is
		// part of the scope as well as the fingerprint to prevent cross-key collisions.
		Scope:          "openai.videos.create." + actorScope,
		ActorScope:     actorScope,
		Method:         http.MethodPost,
		Route:          videoDefaultAPIPath,
		IdempotencyKey: input.IdempotencyKey,
		Payload:        videoIdempotencyPayload(input),
		TTL:            DefaultWriteIdempotencyTTL(),
	}, func(execCtx context.Context) (any, error) {
		return s.createTask(execCtx, input)
	})
	if err != nil {
		return nil, err
	}
	input.IdempotencyReplayed = result.Replayed
	return decodeIdempotentVideoResponse(result.Data)
}

func (s *VideoService) createTask(ctx context.Context, input *VideoCreateInput) (*VideoResponse, error) {
	if input == nil || input.APIKey == nil || input.APIKey.User == nil || input.APIKey.Group == nil || input.Request == nil {
		return nil, videoBadRequest("invalid_video_request", "Invalid video request")
	}
	if input.APIKey.Group.Platform != PlatformSeedance && !input.APIKey.Group.IsAgent() {
		return nil, videoBadRequest("video_platform_required", "Video API is not available for this API key group")
	}

	normalized, rule, estimate, err := s.estimateGenerationCost(ctx, input.APIKey, input.Request, 1)
	if err != nil {
		return nil, err
	}
	totalCost := estimate.TotalCost
	actualCost := estimate.ActualCost
	if err := validateVideoEstimatedCost(input.APIKey, input.Subscription, actualCost); err != nil {
		return nil, err
	}

	account, err := s.selectAccountForRequest(ctx, input.APIKey.Group.ID, normalized, input.APIKey.Group.IsAgent())
	if err != nil {
		return nil, err
	}
	if videoAccountProvider(account) == videoProviderJingyu && strings.TrimSpace(normalized.Prompt) == "" {
		return nil, videoBadRequest("invalid_video_prompt", "Video prompt is required")
	}
	upstreamModel := videoUpstreamModelForAccount(account, normalized)
	upstreamBody := videoUpstreamBodyForAccount(account, normalized, upstreamModel)
	upstreamEndpoint := normalizedVideoEndpoint(input.UpstreamEndpoint)
	if accountEndpoint := videoAccountAPIPath(account); accountEndpoint != "" {
		upstreamEndpoint = accountEndpoint
	}

	publicID := generateVideoPublicID()
	task, err := s.taskRepo.Create(ctx, &VideoTaskCreateInput{
		PublicID:                 publicID,
		RequestID:                optionalTrimmedPtr(input.RequestID),
		UserID:                   input.APIKey.User.ID,
		APIKeyID:                 input.APIKey.ID,
		GroupID:                  input.APIKey.Group.ID,
		AccountID:                account.ID,
		Model:                    normalized.Model,
		UpstreamModel:            upstreamModel,
		Resolution:               normalized.Resolution,
		DurationSeconds:          normalized.GeneratedSeconds,
		ReferenceDurationSeconds: normalized.ReferenceVideoSeconds,
		BillableSeconds:          estimate.BillableSeconds,
		CostPerSecond:            rule.CreditsPerSecond,
		TotalCost:                totalCost,
		ActualCost:               actualCost,
		Status:                   VideoTaskStatusQueued,
	})
	if err != nil {
		return nil, err
	}
	inboundEndpoint := normalizedVideoEndpoint(input.InboundEndpoint)
	if err := s.billCreatedTask(ctx, task, input.APIKey, input.Subscription, account, input.RequestPayloadHash, input.UserAgent, input.IPAddress, inboundEndpoint, upstreamEndpoint); err != nil {
		return nil, err
	}

	lifecycleInput := VideoTaskLifecycleInput{
		PublicID:            task.PublicID,
		Account:             account,
		APIKey:              input.APIKey,
		Subscription:        input.Subscription,
		UpstreamBody:        upstreamBody,
		RequestPayloadHash:  input.RequestPayloadHash,
		UserAgent:           input.UserAgent,
		IPAddress:           input.IPAddress,
		InboundEndpoint:     inboundEndpoint,
		UpstreamEndpoint:    upstreamEndpoint,
		ResultPublicBaseURL: input.ResultPublicBaseURL,
	}
	if s.startLifecycleFunc != nil {
		s.startLifecycleFunc(lifecycleInput)
	} else {
		s.startLifecycle(lifecycleInput)
	}
	return videoResponseFromTask(task), nil
}

func videoIdempotencyPayload(input *VideoCreateInput) any {
	if input == nil {
		return map[string]string{"request_sha256": ""}
	}
	hash := strings.TrimSpace(input.RequestPayloadHash)
	if hash == "" {
		hash = HashUsageRequestPayload(input.RawBody)
	}
	if hash != "" {
		return map[string]string{"request_sha256": hash}
	}
	return input.Request
}

func decodeIdempotentVideoResponse(data any) (*VideoResponse, error) {
	if response, ok := data.(*VideoResponse); ok && response != nil {
		return response, nil
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, ErrIdempotencyStoreUnavail.WithCause(err)
	}
	var response VideoResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, ErrIdempotencyStoreUnavail.WithCause(err)
	}
	if strings.TrimSpace(response.ID) == "" {
		return nil, ErrIdempotencyStoreUnavail
	}
	return &response, nil
}

// EstimateGenerationCost is the authoritative pre-charge estimate used by
// Yingzo confirmation quotes. It intentionally shares normalization and the
// pricing rule lookup with CreateTask.
func (s *VideoService) EstimateGenerationCost(
	ctx context.Context,
	apiKey *APIKey,
	request *VideoCreateRequest,
	count int,
) (*VideoCostEstimate, error) {
	_, _, estimate, err := s.estimateGenerationCost(ctx, apiKey, request, count)
	return estimate, err
}

func (s *VideoService) HasEnabledPricing(ctx context.Context, groupID int64, model string) (bool, error) {
	if s == nil || s.pricingRepo == nil || groupID <= 0 {
		return false, nil
	}
	rules, err := s.pricingRepo.ListByGroupID(ctx, groupID)
	if err != nil {
		return false, err
	}
	for _, rule := range rules {
		if rule.Enabled && rule.ModelCode == model && IsSupportedVideoResolution(model, rule.Resolution) {
			return true, nil
		}
	}
	return false, nil
}

func (s *VideoService) estimateGenerationCost(
	ctx context.Context,
	apiKey *APIKey,
	request *VideoCreateRequest,
	count int,
) (*normalizedVideoRequest, *VideoGroupPricingRule, *VideoCostEstimate, error) {
	if apiKey == nil || apiKey.Group == nil || request == nil {
		return nil, nil, nil, videoBadRequest("invalid_video_request", "Invalid video request")
	}
	if count <= 0 {
		return nil, nil, nil, videoBadRequest("invalid_video_count", "Video count must be positive")
	}
	var normalized *normalizedVideoRequest
	var err error
	if apiKey.Group.IsAgent() {
		normalized, err = normalizeAgentVideoCreateRequest(request)
	} else {
		normalized, err = normalizeVideoCreateRequest(request)
	}
	if err != nil {
		return nil, nil, nil, err
	}
	var rule *VideoGroupPricingRule
	if apiKey.Group.IsAgent() {
		if s.agentModels == nil {
			return nil, nil, nil, videoBadRequest("video_pricing_rule_not_found", "Video pricing rule is not configured")
		}
		unitPrice, modelCode, priceErr := s.agentModels.ResolveMediaUnitPrice(
			ctx,
			apiKey.Group.ID,
			PlatformSeedance,
			AgentMediaTypeVideo,
			normalized.Resolution,
			normalized.Model,
		)
		if priceErr != nil {
			return nil, nil, nil, videoBadRequest("video_pricing_rule_not_found", "Video pricing rule is not configured")
		}
		rule = &VideoGroupPricingRule{
			GroupID:          apiKey.Group.ID,
			ModelCode:        modelCode,
			Resolution:       normalized.Resolution,
			CreditsPerSecond: unitPrice,
			Enabled:          true,
		}
	} else {
		rule, err = s.pricingRepo.GetEnabledRule(ctx, apiKey.Group.ID, normalized.Model, normalized.Resolution)
		if err != nil {
			if errors.Is(err, ErrVideoPricingRuleNotFound) {
				return nil, nil, nil, videoBadRequest("video_pricing_rule_not_found", "Video pricing rule is not configured")
			}
			return nil, nil, nil, err
		}
	}
	referenceMultiplier := 1.0
	billableSecondsPerOutput := normalized.BillableSeconds
	perOutputCost := float64(billableSecondsPerOutput) * rule.CreditsPerSecond
	rateMultiplier := apiKey.Group.RateMultiplier
	if rateMultiplier <= 0 {
		rateMultiplier = 1
	}
	if apiKey.Group.IsAgent() {
		referenceMultiplier = 0
		billableSecondsPerOutput = normalized.GeneratedSeconds
		perOutputCost = float64(billableSecondsPerOutput) * rule.CreditsPerSecond
		rateMultiplier = 1
	}
	totalCost := perOutputCost * float64(count)
	estimate := &VideoCostEstimate{
		Model:                    normalized.Model,
		Resolution:               normalized.Resolution,
		AbilityCode:              normalized.AbilityCode,
		Count:                    count,
		GeneratedSeconds:         normalized.GeneratedSeconds,
		ReferenceVideoSeconds:    normalized.ReferenceVideoSeconds,
		BillableSeconds:          billableSecondsPerOutput * count,
		CreditsPerSecond:         rule.CreditsPerSecond,
		ReferenceVideoMultiplier: referenceMultiplier,
		RateMultiplier:           rateMultiplier,
		TotalCost:                totalCost,
		ActualCost:               totalCost * rateMultiplier,
		PricingRuleUpdatedAt:     rule.UpdatedAt,
	}
	return normalized, rule, estimate, nil
}

func (s *VideoService) GetTask(ctx context.Context, publicID string, apiKey *APIKey) (*VideoResponse, error) {
	publicID = strings.TrimSpace(publicID)
	if publicID == "" || apiKey == nil {
		return nil, ErrVideoTaskNotFound
	}
	task, err := s.taskRepo.GetByPublicID(ctx, publicID)
	if err != nil {
		return nil, err
	}
	if task.APIKeyID != apiKey.ID || task.UserID != apiKey.UserID {
		return nil, ErrVideoTaskNotFound
	}
	return videoResponseFromTask(task), nil
}

func (s *VideoService) selectAccount(ctx context.Context, groupID int64, model string, resolution string, agentGroup bool) (*Account, error) {
	return s.selectAccountForRequest(ctx, groupID, &normalizedVideoRequest{Model: model, Resolution: resolution}, agentGroup)
}

func (s *VideoService) selectAccountForRequest(ctx context.Context, groupID int64, normalized *normalizedVideoRequest, agentGroup bool) (*Account, error) {
	if normalized == nil {
		return nil, ErrVideoAccountNotFound
	}
	model := normalized.Model
	accounts, err := s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, groupID, PlatformSeedance)
	if err != nil {
		return nil, err
	}
	candidates := make([]Account, 0, len(accounts))
	for _, account := range accounts {
		if account.Platform != PlatformSeedance || account.Type != AccountTypeAPIKey || !account.IsSchedulable() {
			continue
		}
		if strings.TrimSpace(account.GetCredential("api_key")) == "" {
			continue
		}
		if !account.IsModelSupported(model) {
			continue
		}
		if agentGroup {
			current := agentDiscoverySet(discoverAgentModels([]Account{account}))
			if _, supported := current[agentModelKey(PlatformSeedance, model)]; !supported {
				continue
			}
			if videoProviderNeedsRequestCompatibility(videoAccountProvider(&account)) && !isVideoAccountCompatibleForRequest(&account, normalized) {
				continue
			}
		} else if !isVideoAccountCompatibleForRequest(&account, normalized) {
			continue
		}
		candidates = append(candidates, account)
	}
	if len(candidates) == 0 {
		return nil, ErrVideoAccountNotFound
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		if a.LastUsedAt == nil && b.LastUsedAt != nil {
			return true
		}
		if a.LastUsedAt != nil && b.LastUsedAt == nil {
			return false
		}
		if a.LastUsedAt == nil && b.LastUsedAt == nil {
			return a.ID < b.ID
		}
		if !a.LastUsedAt.Equal(*b.LastUsedAt) {
			return a.LastUsedAt.Before(*b.LastUsedAt)
		}
		return a.ID < b.ID
	})
	selected := candidates[0]
	return &selected, nil
}

func (s *VideoService) createUpstreamTask(ctx context.Context, account *Account, body map[string]any) (*videoUpstreamCreateResult, error) {
	if videoAccountProvider(account) == videoProviderYCYAPI {
		return s.createYCYAPIUpstreamTask(ctx, account, body)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	endpoint, err := videoAccountEndpoint(account)
	if err != nil {
		return nil, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, videoAccountDuration(account, "request_timeout_ms", videoAccountDefaultDuration(account, "request_timeout_ms")))
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(account.GetCredential("api_key")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.doUpstream(req, account)
	if err != nil {
		return nil, &videoUpstreamError{StatusCode: 0, Err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &videoUpstreamError{StatusCode: resp.StatusCode, Body: respBody}
	}
	var payload map[string]any
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, &videoUpstreamError{StatusCode: resp.StatusCode, Body: respBody, Err: err}
	}
	id := firstNonEmptyVideoString(
		stringFromMap(payload, "task_id"),
		stringFromMap(payload, "id"),
	)
	if id == "" {
		return nil, &videoUpstreamError{StatusCode: resp.StatusCode, Body: respBody, Err: errors.New("missing upstream task id")}
	}
	return &videoUpstreamCreateResult{ID: id}, nil
}

func (s *VideoService) pollUpstreamTask(ctx context.Context, account *Account, upstreamTaskID string) (*videoPollResult, error) {
	if videoAccountProvider(account) == videoProviderJingyu {
		return nil, errors.New("jingyu video tasks use completion callbacks")
	}
	endpoint, err := videoAccountEndpoint(account)
	if err != nil {
		return nil, err
	}
	endpoint = strings.TrimRight(endpoint, "/") + "/" + url.PathEscape(upstreamTaskID)
	reqCtx, cancel := context.WithTimeout(ctx, videoAccountDuration(account, "request_timeout_ms", videoAccountDefaultDuration(account, "request_timeout_ms")))
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(account.GetCredential("api_key")))
	resp, err := s.doUpstream(req, account)
	if err != nil {
		return nil, &videoUpstreamError{StatusCode: 0, Err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &videoUpstreamError{StatusCode: resp.StatusCode, Body: respBody}
	}
	var payload map[string]any
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, &videoUpstreamError{StatusCode: resp.StatusCode, Body: respBody, Err: err}
	}
	rawStatus := stringFromMap(payload, "status")
	status := normalizeVideoUpstreamStatus(rawStatus)
	result := &videoPollResult{Status: status}
	if status == VideoTaskStatusCompleted {
		result.VideoURL = videoResultURLFromPayload(payload)
		if result.VideoURL == "" {
			return nil, &videoUpstreamError{StatusCode: resp.StatusCode, Body: respBody, Err: errors.New("missing video result URL")}
		}
	}
	return result, nil
}

func (s *VideoService) doUpstream(req *http.Request, account *Account) (*http.Response, error) {
	if s.httpUpstream == nil {
		return http.DefaultClient.Do(req)
	}
	proxyURL := ""
	if account != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	concurrency := 1
	if account != nil {
		concurrency = account.Concurrency
	}
	return s.httpUpstream.Do(req, proxyURL, account.ID, concurrency)
}

func (s *VideoService) startLifecycle(input VideoTaskLifecycleInput) {
	if s == nil || input.Account == nil || input.APIKey == nil || strings.TrimSpace(input.PublicID) == "" {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("video lifecycle panic", "task_id", input.PublicID, "panic", r)
			}
		}()

		if videoAccountProvider(input.Account) == videoProviderJingyu {
			if err := s.configureJingyuVideoCallback(context.Background(), input); err != nil {
				clientErr := videoClientError("video_service_unavailable", "Video service is temporarily unavailable. Please retry later.")
				task, _, _ := s.taskRepo.TransitionTerminalByPublicID(context.Background(), input.PublicID, VideoTaskUpdate{
					Status:    stringPtr(VideoTaskStatusFailed),
					ErrorJSON: videoErrorJSON(clientErr),
				})
				_ = s.refundFailedTask(context.Background(), task, input.APIKey, input.Subscription, input.Account, input.RequestPayloadHash, input.UserAgent, input.IPAddress, input.InboundEndpoint, input.UpstreamEndpoint)
				slog.Warn("jingyu video callback configuration failed", "task_id", input.PublicID, "error", err)
				return
			}
		}

		created, err := s.createUpstreamTask(context.Background(), input.Account, input.UpstreamBody)
		if err != nil {
			clientErr := mapVideoUpstreamError(err, false)
			s.recordVideoAccountFailure(context.Background(), input.Account, clientErr, err)
			task, _ := s.taskRepo.UpdateByPublicID(context.Background(), input.PublicID, VideoTaskUpdate{
				Status:    stringPtr(VideoTaskStatusFailed),
				ErrorJSON: videoErrorJSON(clientErr.VideoClientError),
			})
			_ = s.refundFailedTask(context.Background(), task, input.APIKey, input.Subscription, input.Account, input.RequestPayloadHash, input.UserAgent, input.IPAddress, input.InboundEndpoint, input.UpstreamEndpoint)
			return
		}

		if videoAccountProvider(input.Account) == videoProviderJingyu {
			_, changed, err := s.taskRepo.MarkProcessingByPublicID(context.Background(), input.PublicID, created.ID)
			if err != nil {
				slog.Warn("video task submit state update failed", "task_id", input.PublicID, "error", err)
				return
			}
			if !changed {
				return
			}
			s.waitForJingyuVideoCallback(input)
			return
		}

		processingStatus := VideoTaskStatusProcessing
		if _, err := s.taskRepo.UpdateByPublicID(context.Background(), input.PublicID, VideoTaskUpdate{
			Status:         &processingStatus,
			UpstreamTaskID: &created.ID,
		}); err != nil {
			slog.Warn("video task submit state update failed", "task_id", input.PublicID, "error", err)
			return
		}
		s.pollLifecycle(input, created.ID)
	}()
}

type videoPublicBaseURLResolver interface {
	ResolvePublicBaseURL(ctx context.Context, fallback string) (string, error)
}

func (s *VideoService) configureJingyuVideoCallback(ctx context.Context, input VideoTaskLifecycleInput) error {
	if input.Account == nil || videoAccountProvider(input.Account) != videoProviderJingyu {
		return nil
	}
	if input.UpstreamBody == nil {
		return errors.New("jingyu video request body is unavailable")
	}
	baseURL := input.ResultPublicBaseURL
	if resolver, ok := s.videoResultPublisher.(videoPublicBaseURLResolver); ok {
		resolved, err := resolver.ResolvePublicBaseURL(ctx, baseURL)
		if err != nil {
			return err
		}
		baseURL = resolved
	}
	normalizedBaseURL, err := normalizeFileStoragePublicBaseURL(baseURL)
	if err != nil {
		return fmt.Errorf("resolve jingyu callback public URL: %w", err)
	}
	secret, err := s.jingyuVideoCallbackSecret(input.PublicID)
	if err != nil {
		return err
	}
	input.UpstreamBody["callback_url"] = strings.TrimRight(normalizedBaseURL, "/") + jingyuVideoCallbackPath + url.PathEscape(input.PublicID)
	input.UpstreamBody["callback_secret"] = secret
	return nil
}

func (s *VideoService) jingyuVideoCallbackSecret(publicID string) (string, error) {
	if s == nil || s.cfg == nil || strings.TrimSpace(s.cfg.JWT.Secret) == "" || strings.TrimSpace(publicID) == "" {
		return "", errors.New("jingyu video callback signing secret is unavailable")
	}
	key := sha256.Sum256([]byte("sub2api/jingyu-video-callback/v1\x00" + s.cfg.JWT.Secret))
	mac := hmac.New(sha256.New, key[:])
	_, _ = mac.Write([]byte(strings.TrimSpace(publicID)))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func (s *VideoService) waitForJingyuVideoCallback(input VideoTaskLifecycleInput) {
	timeout := videoAccountDuration(input.Account, "poll_timeout_ms", videoAccountDefaultDuration(input.Account, "poll_timeout_ms"))
	if timeout <= 0 {
		timeout = videoAccountDefaultDuration(input.Account, "poll_timeout_ms")
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	<-timer.C

	task, err := s.taskRepo.GetByPublicID(context.Background(), input.PublicID)
	if err != nil || task == nil || isTerminalVideoTaskStatus(task.Status) {
		return
	}
	clientErr := videoClientError("video_service_unavailable", "Video service is temporarily unavailable. Please retry later.")
	task, changed, err := s.taskRepo.TransitionTerminalByPublicID(context.Background(), input.PublicID, VideoTaskUpdate{
		Status:    stringPtr(VideoTaskStatusFailed),
		ErrorJSON: videoErrorJSON(clientErr),
	})
	if err != nil || !changed {
		return
	}
	_ = s.refundFailedTask(context.Background(), task, input.APIKey, input.Subscription, input.Account, input.RequestPayloadHash, input.UserAgent, input.IPAddress, input.InboundEndpoint, input.UpstreamEndpoint)
}

func isTerminalVideoTaskStatus(status string) bool {
	switch status {
	case VideoTaskStatusCompleted, VideoTaskStatusFailed, VideoTaskStatusCancelled:
		return true
	default:
		return false
	}
}

// HandleJingyuVideoCallback processes only Jingyu video task callbacks. Aigod
// video tasks and every image task remain on their existing lifecycle paths.
func (s *VideoService) HandleJingyuVideoCallback(
	ctx context.Context,
	publicID string,
	event string,
	signature string,
	rawBody []byte,
	fallbackPublicBaseURL string,
) error {
	publicID = strings.TrimSpace(publicID)
	if s == nil || publicID == "" || len(rawBody) == 0 || strings.TrimSpace(event) != "task.completed" {
		return ErrJingyuCallbackInvalid
	}
	secret, err := s.jingyuVideoCallbackSecret(publicID)
	if err != nil || !verifyJingyuVideoCallbackSignature(secret, rawBody, signature) {
		return ErrJingyuCallbackUnauthorized
	}
	var payload map[string]any
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return ErrJingyuCallbackInvalid
	}
	rawStatus := stringFromMap(payload, "status")
	status, known := normalizeJingyuVideoUpstreamStatus(rawStatus)
	if !known || (status != VideoTaskStatusCompleted && status != VideoTaskStatusFailed) {
		return ErrJingyuCallbackInvalid
	}
	upstreamTaskID := strings.TrimSpace(stringFromMap(payload, "task_id"))
	if upstreamTaskID == "" {
		return ErrJingyuCallbackInvalid
	}

	processingCtx, cancel := context.WithTimeout(context.Background(), generatedVideoDownloadTimeout)
	defer cancel()
	_, err, _ = s.jingyuCallbackGroup.Do(publicID, func() (any, error) {
		return nil, s.processJingyuVideoCallback(processingCtx, publicID, upstreamTaskID, status, payload, fallbackPublicBaseURL)
	})
	if err != nil {
		return err
	}
	return nil
}

func verifyJingyuVideoCallbackSignature(secret string, rawBody []byte, signature string) bool {
	provided, err := hex.DecodeString(strings.TrimSpace(signature))
	if err != nil || len(provided) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(rawBody)
	return hmac.Equal(mac.Sum(nil), provided)
}

func (s *VideoService) processJingyuVideoCallback(
	ctx context.Context,
	publicID string,
	upstreamTaskID string,
	status string,
	payload map[string]any,
	fallbackPublicBaseURL string,
) error {
	task, err := s.taskRepo.GetByPublicID(ctx, publicID)
	if err != nil || task == nil {
		return ErrJingyuCallbackNotFound
	}
	account, err := s.accountRepo.GetByID(ctx, task.AccountID)
	if err != nil || account == nil {
		return ErrJingyuCallbackFailed
	}
	if videoAccountProvider(account) != videoProviderJingyu {
		return ErrJingyuCallbackInvalid
	}
	if task.UpstreamTaskID != nil && strings.TrimSpace(*task.UpstreamTaskID) != upstreamTaskID {
		return ErrJingyuCallbackInvalid
	}
	if isTerminalVideoTaskStatus(task.Status) {
		if task.Status == VideoTaskStatusFailed && status == VideoTaskStatusFailed {
			return s.refundJingyuCallbackTask(ctx, task, account)
		}
		return nil
	}

	if status == VideoTaskStatusCompleted {
		upstreamURL := videoResultURLFromPayload(payload)
		if upstreamURL == "" || s.videoResultPublisher == nil {
			return ErrJingyuCallbackFailed
		}
		publishedURL, err := s.videoResultPublisher.PublishGeneratedVideo(
			ctx,
			TemporaryAssetOwner{UserID: task.UserID, APIKeyID: task.APIKeyID, GroupID: task.GroupID},
			fallbackPublicBaseURL,
			upstreamURL,
		)
		if err != nil || strings.TrimSpace(publishedURL) == "" {
			return ErrJingyuCallbackFailed.WithCause(err)
		}
		now := time.Now().UTC()
		completedTask, changed, err := s.taskRepo.TransitionTerminalByPublicID(ctx, publicID, VideoTaskUpdate{
			Status:         stringPtr(VideoTaskStatusCompleted),
			UpstreamTaskID: &upstreamTaskID,
			ResultVideoURL: &publishedURL,
			CompletedAt:    &now,
		})
		if err != nil {
			return ErrJingyuCallbackFailed.WithCause(err)
		}
		if changed {
			_ = s.recordCompletedTask(context.Background(), completedTask, &APIKey{ID: task.APIKeyID}, nil, account, "", "", videoDefaultAPIPath, videoAccountAPIPath(account))
		}
		return nil
	}

	clientErr := videoClientError("video_generation_failed", "Video generation failed. Please retry with a different prompt or input.")
	apiKey, subscription, err := s.loadJingyuCallbackBillingContext(ctx, task)
	if err != nil {
		return ErrJingyuCallbackFailed.WithCause(err)
	}
	failedTask, changed, err := s.taskRepo.TransitionTerminalByPublicID(ctx, publicID, VideoTaskUpdate{
		Status:         stringPtr(VideoTaskStatusFailed),
		UpstreamTaskID: &upstreamTaskID,
		ErrorJSON:      videoErrorJSON(clientErr),
	})
	if err != nil {
		return ErrJingyuCallbackFailed.WithCause(err)
	}
	if !changed {
		return nil
	}
	if err := s.refundFailedTask(
		context.Background(),
		failedTask,
		apiKey,
		subscription,
		account,
		"video:"+publicID,
		"",
		"",
		videoDefaultAPIPath,
		videoAccountAPIPath(account),
	); err != nil {
		return ErrJingyuCallbackFailed.WithCause(err)
	}
	return nil
}

func (s *VideoService) refundJingyuCallbackTask(ctx context.Context, task *VideoTask, account *Account) error {
	apiKey, subscription, err := s.loadJingyuCallbackBillingContext(ctx, task)
	if err != nil {
		return ErrJingyuCallbackFailed.WithCause(err)
	}
	if err := s.refundFailedTask(
		context.Background(),
		task,
		apiKey,
		subscription,
		account,
		"video:"+task.PublicID,
		"",
		"",
		videoDefaultAPIPath,
		videoAccountAPIPath(account),
	); err != nil {
		return ErrJingyuCallbackFailed.WithCause(err)
	}
	return nil
}

func (s *VideoService) loadJingyuCallbackBillingContext(ctx context.Context, task *VideoTask) (*APIKey, *UserSubscription, error) {
	if s == nil || s.apiKeyLoader == nil || task == nil {
		return nil, nil, errors.New("jingyu callback billing context is unavailable")
	}
	apiKey, err := s.apiKeyLoader.GetByID(ctx, task.APIKeyID)
	if err != nil {
		return nil, nil, err
	}
	var subscription *UserSubscription
	if s.userSubRepo != nil {
		subscription, err = s.userSubRepo.GetByUserIDAndGroupID(ctx, task.UserID, task.GroupID)
		if err != nil && !errors.Is(err, ErrSubscriptionNotFound) {
			return nil, nil, err
		}
	}
	return apiKey, subscription, nil
}

func (s *VideoService) pollLifecycle(input VideoTaskLifecycleInput, upstreamTaskID string) {
	intervalFallback := videoAccountDefaultDuration(input.Account, "poll_interval_ms")
	timeoutFallback := videoAccountDefaultDuration(input.Account, "poll_timeout_ms")
	interval := videoAccountDuration(input.Account, "poll_interval_ms", intervalFallback)
	timeout := videoAccountDuration(input.Account, "poll_timeout_ms", timeoutFallback)
	if interval <= 0 {
		interval = intervalFallback
	}
	if timeout <= 0 {
		timeout = timeoutFallback
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			clientErr := videoClientError("video_service_unavailable", "Video service is temporarily unavailable. Please retry later.")
			task, _ := s.taskRepo.UpdateByPublicID(context.Background(), input.PublicID, VideoTaskUpdate{
				Status:    stringPtr(VideoTaskStatusFailed),
				ErrorJSON: videoErrorJSON(clientErr),
			})
			_ = s.refundFailedTask(context.Background(), task, input.APIKey, input.Subscription, input.Account, input.RequestPayloadHash, input.UserAgent, input.IPAddress, input.InboundEndpoint, input.UpstreamEndpoint)
			return
		case <-ticker.C:
			result, err := s.pollUpstreamTask(ctx, input.Account, upstreamTaskID)
			if err != nil {
				clientErr := mapVideoUpstreamError(err, true)
				s.recordVideoAccountFailure(context.Background(), input.Account, clientErr, err)
				if clientErr.Retryable {
					continue
				}
				task, _ := s.taskRepo.UpdateByPublicID(context.Background(), input.PublicID, VideoTaskUpdate{
					Status:    stringPtr(VideoTaskStatusFailed),
					ErrorJSON: videoErrorJSON(clientErr.VideoClientError),
				})
				_ = s.refundFailedTask(context.Background(), task, input.APIKey, input.Subscription, input.Account, input.RequestPayloadHash, input.UserAgent, input.IPAddress, input.InboundEndpoint, input.UpstreamEndpoint)
				return
			}
			switch result.Status {
			case VideoTaskStatusQueued, VideoTaskStatusProcessing:
				_, _ = s.taskRepo.UpdateByPublicID(context.Background(), input.PublicID, VideoTaskUpdate{
					Status: &result.Status,
				})
			case VideoTaskStatusCompleted:
				publishedURL, publishErr := s.publishVideoResult(ctx, input, result.VideoURL)
				if publishErr != nil {
					slog.Warn("video result publication failed", "task_id", input.PublicID, "error", publishErr)
					continue
				}
				now := time.Now().UTC()
				task, updateErr := s.taskRepo.UpdateByPublicID(context.Background(), input.PublicID, VideoTaskUpdate{
					Status:         &result.Status,
					ResultVideoURL: &publishedURL,
					CompletedAt:    &now,
				})
				if updateErr == nil {
					_ = s.recordCompletedTask(context.Background(), task, input.APIKey, input.Subscription, input.Account, input.UserAgent, input.IPAddress, input.InboundEndpoint, input.UpstreamEndpoint)
				}
				return
			case VideoTaskStatusFailed, VideoTaskStatusCancelled:
				clientErr := videoClientError("video_generation_failed", "Video generation failed. Please retry with a different prompt or input.")
				task, _ := s.taskRepo.UpdateByPublicID(context.Background(), input.PublicID, VideoTaskUpdate{
					Status:    &result.Status,
					ErrorJSON: videoErrorJSON(clientErr),
				})
				_ = s.refundFailedTask(context.Background(), task, input.APIKey, input.Subscription, input.Account, input.RequestPayloadHash, input.UserAgent, input.IPAddress, input.InboundEndpoint, input.UpstreamEndpoint)
				return
			default:
				_, _ = s.taskRepo.UpdateByPublicID(context.Background(), input.PublicID, VideoTaskUpdate{
					Status: stringPtr(VideoTaskStatusProcessing),
				})
			}
		}
	}
}

func (s *VideoService) publishVideoResult(ctx context.Context, input VideoTaskLifecycleInput, upstreamURL string) (string, error) {
	if s == nil || s.videoResultPublisher == nil {
		return "", errors.New("video result publisher is unavailable")
	}
	if input.APIKey == nil || input.APIKey.User == nil || input.APIKey.Group == nil {
		return "", errors.New("video result owner is unavailable")
	}
	publishedURL, err := s.videoResultPublisher.PublishGeneratedVideo(
		ctx,
		TemporaryAssetOwner{
			UserID:   input.APIKey.User.ID,
			APIKeyID: input.APIKey.ID,
			GroupID:  input.APIKey.Group.ID,
		},
		input.ResultPublicBaseURL,
		upstreamURL,
	)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(publishedURL) == "" {
		return "", errors.New("video result publisher returned an empty URL")
	}
	return publishedURL, nil
}

func (s *VideoService) billCreatedTask(ctx context.Context, task *VideoTask, apiKey *APIKey, subscription *UserSubscription, account *Account, payloadHash, userAgent, ipAddress, inboundEndpoint, upstreamEndpoint string) error {
	if task == nil || apiKey == nil || apiKey.User == nil || apiKey.Group == nil || account == nil {
		return nil
	}
	if task.BilledAt != nil {
		return nil
	}
	usageLog, isSubscriptionBill := buildVideoUsageLog(task, apiKey, subscription, account, videoUsageLogOptions{
		RequestID:        "video:" + task.PublicID,
		UserAgent:        userAgent,
		IPAddress:        ipAddress,
		InboundEndpoint:  inboundEndpoint,
		UpstreamEndpoint: upstreamEndpoint,
		CreatedAt:        time.Now().UTC(),
	})
	if usageLog == nil {
		return nil
	}
	if err := s.applyVideoUsageBilling(ctx, usageLog, task, apiKey, subscription, account, payloadHash, isSubscriptionBill); err != nil {
		slog.Error("video prebilling failed", "task_id", task.PublicID, "error", err)
		return err
	}
	if s.usageLogRepo != nil {
		if _, err := s.usageLogRepo.Create(ctx, usageLog); err != nil {
			slog.Warn("video usage log write failed", "task_id", task.PublicID, "error", err)
		}
	}
	if _, err := s.taskRepo.MarkBilled(ctx, task.PublicID, time.Now().UTC()); err != nil {
		return err
	}
	return nil
}

func (s *VideoService) recordCompletedTask(ctx context.Context, task *VideoTask, apiKey *APIKey, subscription *UserSubscription, account *Account, userAgent, ipAddress, inboundEndpoint, upstreamEndpoint string) error {
	if task == nil || apiKey == nil || account == nil || task.ResultVideoURL == nil || s.usageLogRepo == nil {
		return nil
	}
	updater, ok := s.usageLogRepo.(VideoUsageResultUpdater)
	if !ok {
		return nil
	}
	if err := updater.UpdateVideoResult(ctx, "video:"+task.PublicID, apiKey.ID, VideoUsageResultUpdate{
		ResultURL:        *task.ResultVideoURL,
		DurationMs:       videoTaskDurationMs(task),
		InboundEndpoint:  normalizedVideoEndpoint(inboundEndpoint),
		UpstreamEndpoint: normalizedVideoEndpoint(upstreamEndpoint),
	}); err != nil {
		slog.Warn("video completion usage log update failed", "task_id", task.PublicID, "error", err)
		return err
	}
	return nil
}

func (s *VideoService) refundFailedTask(ctx context.Context, task *VideoTask, apiKey *APIKey, subscription *UserSubscription, account *Account, payloadHash, userAgent, ipAddress, inboundEndpoint, upstreamEndpoint string) error {
	if task == nil || apiKey == nil || apiKey.User == nil || apiKey.Group == nil || account == nil {
		return nil
	}
	if s.taskRepo != nil {
		if latest, err := s.taskRepo.GetByPublicID(ctx, task.PublicID); err == nil && latest != nil {
			task = latest
		}
	}
	if task.BilledAt == nil || task.RefundedAt != nil || task.ActualCost <= 0 {
		return nil
	}
	durationMs := videoTaskDurationMs(task)
	inboundEndpoint = normalizedVideoEndpoint(inboundEndpoint)
	upstreamEndpoint = normalizedVideoEndpoint(upstreamEndpoint)
	if updater, ok := s.usageLogRepo.(VideoUsageResultUpdater); ok {
		if err := updater.UpdateVideoResult(ctx, "video:"+task.PublicID, apiKey.ID, VideoUsageResultUpdate{
			DurationMs:       durationMs,
			InboundEndpoint:  inboundEndpoint,
			UpstreamEndpoint: upstreamEndpoint,
		}); err != nil {
			slog.Warn("video failed usage log update failed", "task_id", task.PublicID, "error", err)
		}
	}
	usageLog, isSubscriptionBill := buildVideoUsageLog(task, apiKey, subscription, account, videoUsageLogOptions{
		RequestID:        "video:" + task.PublicID + ":refund",
		UserAgent:        userAgent,
		IPAddress:        ipAddress,
		InboundEndpoint:  inboundEndpoint,
		UpstreamEndpoint: upstreamEndpoint,
		DurationMs:       durationMs,
		CreatedAt:        time.Now().UTC(),
		TotalCost:        -task.TotalCost,
		ActualCost:       -task.ActualCost,
		OutputCost:       -task.TotalCost,
	})
	if usageLog == nil {
		return nil
	}
	if err := s.applyVideoUsageBilling(ctx, usageLog, task, apiKey, subscription, account, payloadHash+":refund", isSubscriptionBill); err != nil {
		slog.Error("video refund billing failed", "task_id", task.PublicID, "error", err)
		return err
	}
	if s.usageLogRepo != nil {
		if _, err := s.usageLogRepo.Create(ctx, usageLog); err != nil {
			slog.Warn("video refund usage log write failed", "task_id", task.PublicID, "error", err)
		}
	}
	if _, err := s.taskRepo.MarkRefunded(ctx, task.PublicID, time.Now().UTC()); err != nil {
		return err
	}
	return nil
}

type videoUsageLogOptions struct {
	RequestID        string
	UserAgent        string
	IPAddress        string
	InboundEndpoint  string
	UpstreamEndpoint string
	CreatedAt        time.Time
	DurationMs       *int
	TotalCost        float64
	ActualCost       float64
	OutputCost       float64
	ResultVideoURL   *string
}

func buildVideoUsageLog(task *VideoTask, apiKey *APIKey, subscription *UserSubscription, account *Account, opts videoUsageLogOptions) (*UsageLog, bool) {
	if task == nil || apiKey == nil || apiKey.User == nil || apiKey.Group == nil || account == nil {
		return nil, false
	}
	billingMode := string(BillingModeVideoDuration)
	createdAt := opts.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	totalCost := opts.TotalCost
	actualCost := opts.ActualCost
	outputCost := opts.OutputCost
	if totalCost == 0 && actualCost == 0 && outputCost == 0 && opts.ResultVideoURL == nil {
		totalCost = task.TotalCost
		actualCost = task.ActualCost
		outputCost = task.TotalCost
	}
	requestID := strings.TrimSpace(opts.RequestID)
	if requestID == "" {
		requestID = "video:" + task.PublicID
	}
	rateMultiplier := apiKey.Group.RateMultiplier
	if apiKey.Group.IsAgent() {
		rateMultiplier = 1
	}
	usageLog := &UsageLog{
		UserID:                        apiKey.User.ID,
		APIKeyID:                      apiKey.ID,
		AccountID:                     account.ID,
		RequestID:                     requestID,
		Model:                         task.Model,
		RequestedModel:                task.Model,
		UpstreamModel:                 optionalNonEqualStringPtr(task.UpstreamModel, task.Model),
		GroupID:                       &task.GroupID,
		InputCost:                     0,
		OutputCost:                    outputCost,
		TotalCost:                     totalCost,
		ActualCost:                    actualCost,
		RateMultiplier:                rateMultiplier,
		AccountRateMultiplier:         videoFloat64Ptr(account.BillingRateMultiplier()),
		BillingType:                   BillingTypeBalance,
		RequestType:                   RequestTypeVideo,
		Stream:                        false,
		OpenAIWSMode:                  false,
		DurationMs:                    opts.DurationMs,
		UserAgent:                     optionalTrimmedPtr(opts.UserAgent),
		IPAddress:                     optionalTrimmedPtr(opts.IPAddress),
		InboundEndpoint:               optionalTrimmedPtr(normalizedVideoEndpoint(opts.InboundEndpoint)),
		UpstreamEndpoint:              optionalTrimmedPtr(normalizedVideoEndpoint(opts.UpstreamEndpoint)),
		BillingMode:                   &billingMode,
		VideoTaskID:                   &task.PublicID,
		VideoResolution:               &task.Resolution,
		VideoDurationSeconds:          task.DurationSeconds,
		VideoReferenceDurationSeconds: task.ReferenceDurationSeconds,
		VideoBillableSeconds:          task.BillableSeconds,
		VideoResultURL:                opts.ResultVideoURL,
		CreatedAt:                     createdAt,
	}
	isSubscriptionBill := false
	if subscription != nil && apiKey.Group.IsSubscriptionType() {
		isSubscriptionBill = true
		usageLog.BillingType = BillingTypeSubscription
		usageLog.SubscriptionID = &subscription.ID
	}
	return usageLog, isSubscriptionBill
}

func (s *VideoService) applyVideoUsageBilling(ctx context.Context, usageLog *UsageLog, task *VideoTask, apiKey *APIKey, subscription *UserSubscription, account *Account, payloadHash string, isSubscriptionBill bool) error {
	if usageLog == nil || task == nil || apiKey == nil || apiKey.User == nil || account == nil {
		return nil
	}
	if usageLog.ActualCost < 0 && s.usageBillingRepo == nil {
		return errors.New("video refund requires usage billing repository")
	}
	billingMode := string(BillingModeVideoDuration)
	applied, billingErr := applyUsageBilling(ctx, usageLog.RequestID, usageLog, &postUsageBillingParams{
		Cost:                  &CostBreakdown{OutputCost: usageLog.OutputCost, TotalCost: usageLog.TotalCost, ActualCost: usageLog.ActualCost, BillingMode: billingMode},
		User:                  apiKey.User,
		APIKey:                apiKey,
		Account:               account,
		Subscription:          subscription,
		RequestPayloadHash:    payloadHash,
		IsSubscriptionBill:    isSubscriptionBill,
		AccountRateMultiplier: account.BillingRateMultiplier(),
		APIKeyService:         s.apiKeyService,
		Platform:              PlatformSeedance,
	}, s.billingDeps(), s.usageBillingRepo)
	if billingErr != nil {
		return billingErr
	}
	if applied {
		s.finalizeVideoRefundCache(ctx, usageLog, task, apiKey, subscription, isSubscriptionBill)
	}
	return nil
}

func (s *VideoService) finalizeVideoRefundCache(ctx context.Context, usageLog *UsageLog, task *VideoTask, apiKey *APIKey, subscription *UserSubscription, isSubscriptionBill bool) {
	if s == nil || s.billingCache == nil || usageLog == nil || usageLog.ActualCost >= 0 || apiKey == nil || apiKey.User == nil {
		return
	}
	refund := -usageLog.ActualCost
	if isSubscriptionBill {
		if subscription != nil && apiKey.GroupID != nil {
			s.billingCache.QueueUpdateSubscriptionUsage(apiKey.User.ID, *apiKey.GroupID, -refund)
		}
		return
	}
	_ = s.billingCache.InvalidateUserBalance(ctx, apiKey.User.ID)
	if apiKey.HasRateLimits() {
		s.billingCache.QueueUpdateAPIKeyRateLimitUsage(apiKey.ID, -refund)
	}
	if task != nil {
		s.billingCache.RollbackUserPlatformQuotaUsage(ctx, apiKey.User.ID, PlatformSeedance, refund)
	}
}

func (s *VideoService) billingDeps() *billingDeps {
	return &billingDeps{
		accountRepo:           s.accountRepo,
		userRepo:              s.userRepo,
		userSubRepo:           s.userSubRepo,
		billingCacheService:   s.billingCache,
		deferredService:       s.deferredService,
		balanceNotifyService:  s.balanceNotify,
		userPlatformQuotaRepo: s.quotaRepo,
		cfg:                   s.cfg,
	}
}

func normalizedVideoEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}
	if endpoint == videoDefaultJingyuAPIPath {
		return videoDefaultJingyuAPIPath
	}
	if !strings.Contains(endpoint, "/videos") {
		return videoDefaultAPIPath
	}
	return videoDefaultAPIPath
}

func videoTaskDurationMs(task *VideoTask) *int {
	if task == nil || task.CreatedAt.IsZero() {
		return nil
	}
	end := task.UpdatedAt
	if task.CompletedAt != nil {
		end = *task.CompletedAt
	}
	if end.IsZero() || end.Before(task.CreatedAt) {
		return nil
	}
	ms := int(end.Sub(task.CreatedAt).Milliseconds())
	if ms < 0 {
		return nil
	}
	return &ms
}

type normalizedVideoRequest struct {
	Model                 string
	Prompt                string
	Content               []VideoContent
	Ratio                 string
	RatioProvided         bool
	Duration              float64
	RequestedDuration     float64
	GeneratedSeconds      int
	Resolution            string
	GenerateAudio         *bool
	SafetyIdentifier      string
	AbilityCode           string
	ReferenceVideoSeconds int
	BillableSeconds       int
	Raw                   map[string]any
}

func normalizeVideoCreateRequest(req *VideoCreateRequest) (*normalizedVideoRequest, error) {
	return normalizeVideoCreateRequestWithDynamicModel(req, false)
}

func normalizeAgentVideoCreateRequest(req *VideoCreateRequest) (*normalizedVideoRequest, error) {
	return normalizeVideoCreateRequestWithDynamicModel(req, true)
}

func normalizeVideoCreateRequestWithDynamicModel(req *VideoCreateRequest, dynamicModel bool) (*normalizedVideoRequest, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" || (!dynamicModel && model != VideoModelSeedance20 && model != VideoModelSeedance20Fast && model != VideoModelSeedance25) {
		return nil, videoBadRequest("invalid_video_model", "Invalid video model")
	}
	resolution := strings.TrimSpace(req.Resolution)
	if resolution == "" {
		resolution = VideoResolution720P
	}
	if dynamicModel {
		var err error
		resolution, err = normalizeAgentPriceResolution(AgentMediaTypeVideo, resolution)
		if err != nil {
			return nil, videoBadRequest("invalid_video_resolution", "Invalid video resolution")
		}
		if model == VideoModelSeedance25 && !IsSupportedVideoResolution(model, resolution) {
			return nil, videoBadRequest("invalid_video_resolution", "Invalid video resolution")
		}
	} else if !IsSupportedVideoResolution(model, resolution) {
		return nil, videoBadRequest("invalid_video_resolution", "Invalid video resolution")
	}
	requestedDuration := req.Duration
	duration := requestedDuration
	maxDuration := float64(videoMaxDurationSeconds)
	if model == VideoModelSeedance25 {
		if duration == -1 {
			duration = 5
		}
		maxDuration = videoSeedance25MaxDuration
	}
	if duration != math.Trunc(duration) || duration < videoMinDurationSeconds || duration > maxDuration {
		return nil, videoBadRequest("invalid_video_duration", "Invalid video duration")
	}
	generatedSeconds := int(math.Ceil(duration))
	if generatedSeconds <= 0 {
		return nil, videoBadRequest("invalid_video_duration", "Invalid video duration")
	}
	prompt := strings.TrimSpace(req.Prompt)
	content := normalizeVideoContent(req.Content)
	stats := inspectVideoContent(content)
	if prompt == "" {
		for _, item := range content {
			if item.Type == "text" && strings.TrimSpace(item.Text) != "" {
				prompt = strings.TrimSpace(item.Text)
				break
			}
		}
	}
	ability := strings.TrimSpace(req.AbilityCode)
	if ability == "" {
		ability = inferVideoAbility(stats)
	}
	if !isValidVideoAbility(ability) {
		return nil, videoBadRequest("invalid_video_ability", "Invalid video ability")
	}
	if err := validateVideoAbilityInput(model, ability, prompt, stats); err != nil {
		return nil, err
	}
	referenceSeconds, err := referenceVideoSeconds(model, content)
	if err != nil {
		return nil, err
	}
	ratio := firstNonEmptyVideoString(req.Ratio, req.AspectRatio, req.AspectRatioCamel)
	ratioProvided := ratio != ""
	if model == VideoModelSeedance25 {
		ratio = strings.ToLower(strings.TrimSpace(ratio))
		if ratio == "adaptive" {
			ratio = "auto"
		}
		if ratio == "" && (ability == videoAbilityImageToVideo || ability == videoAbilityStartEndToVideo) {
			ratio = "auto"
		}
		if ratio != "" && !isSeedance25Ratio(ratio) {
			return nil, videoBadRequest("invalid_video_ratio", "Invalid video ratio")
		}
	}
	if ratio == "" {
		ratio = "16:9"
	}
	return &normalizedVideoRequest{
		Model:                 model,
		Prompt:                prompt,
		Content:               content,
		Ratio:                 ratio,
		RatioProvided:         ratioProvided,
		Duration:              duration,
		RequestedDuration:     requestedDuration,
		GeneratedSeconds:      generatedSeconds,
		Resolution:            resolution,
		GenerateAudio:         req.GenerateAudio,
		SafetyIdentifier:      strings.TrimSpace(req.SafetyIdentifier),
		AbilityCode:           ability,
		ReferenceVideoSeconds: referenceSeconds,
		BillableSeconds:       generatedSeconds + referenceSeconds,
		Raw:                   cloneMap(req.Raw),
	}, nil
}

func (r *normalizedVideoRequest) UpstreamBody(upstreamModel string) map[string]any {
	body := map[string]any{
		"model":    upstreamModel,
		"prompt":   r.Prompt,
		"content":  videoContentForUpstream(r.Content, r.Prompt),
		"ratio":    r.Ratio,
		"duration": r.GeneratedSeconds,
	}
	if r.GenerateAudio != nil {
		body["generate_audio"] = *r.GenerateAudio
	}
	if r.SafetyIdentifier != "" {
		body["safety_identifier"] = r.SafetyIdentifier
	}
	return body
}

func (r *normalizedVideoRequest) JingyuUpstreamBody(upstreamModel string) map[string]any {
	upstreamModel = strings.TrimSpace(upstreamModel)
	if upstreamModel == "" {
		upstreamModel = defaultJingyuUpstreamModel(r.Model)
	}
	duration := any(r.GeneratedSeconds)
	if r.Model == VideoModelSeedance25 && r.RequestedDuration == -1 {
		duration = r.RequestedDuration
	}
	body := map[string]any{
		"model":      upstreamModel,
		"prompt":     r.Prompt,
		"duration":   duration,
		"resolution": strings.ToLower(r.Resolution),
	}
	if r.RatioProvided {
		body["aspect_ratio"] = r.Ratio
	}
	if r.GenerateAudio != nil {
		body["generate_audio"] = *r.GenerateAudio
	}
	if r.Raw != nil {
		if seed, ok := r.Raw["seed"]; ok && seed != nil {
			body["seed"] = seed
		}
	}
	if refs := jingyuReferencesFromContent(r.Content, r.AbilityCode); len(refs) > 0 {
		body["references"] = refs
	}
	return body
}

func normalizeVideoContent(content []VideoContent) []VideoContent {
	out := make([]VideoContent, 0, len(content))
	for _, item := range content {
		item.Type = strings.TrimSpace(strings.ToLower(item.Type))
		item.Role = strings.TrimSpace(strings.ToLower(item.Role))
		item.Text = strings.TrimSpace(item.Text)
		if item.SubjectType == "" && (item.Type == "image_url" || item.Type == "video_url") {
			item.SubjectType = "person"
		}
		out = append(out, item)
	}
	return out
}

type videoContentStats struct {
	ImageCount          int
	VideoCount          int
	AudioCount          int
	FirstFrameCount     int
	LastFrameCount      int
	ReferenceImageCount int
	ReferenceVideoCount int
	ReferenceAudioCount int
	HasReference        bool
}

func inspectVideoContent(content []VideoContent) videoContentStats {
	var stats videoContentStats
	for _, item := range content {
		switch item.Type {
		case "image_url":
			stats.ImageCount++
			switch item.Role {
			case "first_frame":
				stats.FirstFrameCount++
			case "last_frame":
				stats.LastFrameCount++
			case "reference_image":
				stats.ReferenceImageCount++
				stats.HasReference = true
			}
		case "video_url":
			stats.VideoCount++
			if item.Role == "reference_video" || item.Role == "" {
				stats.ReferenceVideoCount++
				stats.HasReference = true
			}
		case "audio_url":
			stats.AudioCount++
			if item.Role == "reference_audio" || item.Role == "" {
				stats.ReferenceAudioCount++
				stats.HasReference = true
			}
		}
	}
	return stats
}

func inferVideoAbility(stats videoContentStats) string {
	if stats.HasReference || stats.VideoCount > 0 || stats.AudioCount > 0 || stats.ReferenceImageCount > 0 {
		return videoAbilityReferenceToVideo
	}
	if stats.FirstFrameCount == 1 && stats.LastFrameCount == 1 && stats.ImageCount == 2 {
		return videoAbilityStartEndToVideo
	}
	if stats.FirstFrameCount == 1 && stats.ImageCount == 1 {
		return videoAbilityImageToVideo
	}
	return videoAbilityTextToVideo
}

func isValidVideoAbility(ability string) bool {
	switch ability {
	case videoAbilityTextToVideo, videoAbilityImageToVideo, videoAbilityStartEndToVideo, videoAbilityReferenceToVideo:
		return true
	default:
		return false
	}
}

func validateVideoAbilityInput(model, ability, prompt string, stats videoContentStats) error {
	switch ability {
	case videoAbilityTextToVideo:
		if strings.TrimSpace(prompt) == "" {
			return videoBadRequest("invalid_video_prompt", "Video prompt is required")
		}
		if stats.ImageCount > 0 || stats.VideoCount > 0 || stats.AudioCount > 0 {
			return videoBadRequest("invalid_video_content", "Text-to-video requests cannot include media references")
		}
	case videoAbilityImageToVideo:
		if stats.ImageCount != 1 || stats.FirstFrameCount != 1 {
			return videoBadRequest("invalid_video_content", "Image-to-video requires exactly one first frame image")
		}
		if stats.VideoCount > 0 || stats.AudioCount > 0 {
			return videoBadRequest("invalid_video_content", "Image-to-video cannot include video or audio references")
		}
	case videoAbilityStartEndToVideo:
		if stats.ImageCount != 2 || stats.FirstFrameCount != 1 || stats.LastFrameCount != 1 {
			return videoBadRequest("invalid_video_content", "Start-end video requires exactly one first frame and one last frame image")
		}
		if stats.VideoCount > 0 || stats.AudioCount > 0 {
			return videoBadRequest("invalid_video_content", "Start-end video cannot include video or audio references")
		}
	case videoAbilityReferenceToVideo:
		if model == VideoModelSeedance25 {
			referenceCount := stats.ImageCount + stats.VideoCount + stats.AudioCount
			if stats.ImageCount > 30 || stats.VideoCount > 10 || stats.AudioCount > 10 || referenceCount > videoSeedance25MaxReferences {
				return videoBadRequest("invalid_video_content", "Seedance 2.5 reference video requests support up to 30 images, 10 videos, 10 audio files, and 50 total media references")
			}
			if referenceCount == 0 {
				return videoBadRequest("invalid_video_content", "Seedance 2.5 reference video requests require at least one image, video, or audio reference")
			}
			break
		}
		if stats.ImageCount > 9 || stats.VideoCount > 3 || stats.AudioCount > 3 {
			return videoBadRequest("invalid_video_content", "Reference video requests support up to 9 images, 3 videos, and 3 audio files")
		}
		if stats.ImageCount+stats.VideoCount == 0 {
			return videoBadRequest("invalid_video_content", "Reference video requests require at least one image or video reference")
		}
	}
	return nil
}

func referenceVideoSeconds(model string, content []VideoContent) (int, error) {
	maxDuration := float64(videoMaxDurationSeconds)
	maxTotal := videoMaxReferenceVideoTotal
	if model == VideoModelSeedance25 {
		maxDuration = videoSeedance25MaxDuration
		maxTotal = videoSeedance25MaxDuration
	}
	total := 0
	for _, item := range content {
		if item.Type != "video_url" || (item.Role != "" && item.Role != "reference_video") {
			continue
		}
		if item.DurationSeconds == nil || *item.DurationSeconds <= 0 {
			return 0, videoBadRequest("reference_video_duration_required", "Reference video duration is required")
		}
		if *item.DurationSeconds < 2 || *item.DurationSeconds > maxDuration {
			return 0, videoBadRequest("invalid_reference_video_duration", "Invalid reference video duration")
		}
		total += int(math.Ceil(*item.DurationSeconds))
		if total > maxTotal {
			return 0, videoBadRequest("invalid_reference_video_duration", "Invalid reference video duration")
		}
	}
	return total, nil
}

func isSeedance25Ratio(ratio string) bool {
	switch strings.TrimSpace(strings.ToLower(ratio)) {
	case "auto", "16:9", "4:3", "1:1", "3:4", "9:16", "21:9":
		return true
	default:
		return false
	}
}

func videoContentForUpstream(content []VideoContent, prompt string) []map[string]any {
	out := make([]map[string]any, 0, len(content)+1)
	hasText := false
	for _, item := range content {
		entry := map[string]any{"type": item.Type}
		switch item.Type {
		case "text":
			if item.Text == "" {
				continue
			}
			entry["text"] = item.Text
			hasText = true
		case "image_url":
			if item.ImageURL == nil || strings.TrimSpace(item.ImageURL.URL) == "" {
				continue
			}
			entry["image_url"] = map[string]any{"url": strings.TrimSpace(item.ImageURL.URL)}
			if item.Role != "" {
				entry["role"] = item.Role
			}
			if item.SubjectType != "" {
				entry["subject_type"] = item.SubjectType
			}
		case "video_url":
			if item.VideoURL == nil || strings.TrimSpace(item.VideoURL.URL) == "" {
				continue
			}
			entry["video_url"] = map[string]any{"url": strings.TrimSpace(item.VideoURL.URL)}
			if item.Role != "" {
				entry["role"] = item.Role
			}
			if item.SubjectType != "" {
				entry["subject_type"] = item.SubjectType
			}
		case "audio_url":
			if item.AudioURL == nil || strings.TrimSpace(item.AudioURL.URL) == "" {
				continue
			}
			entry["audio_url"] = map[string]any{"url": strings.TrimSpace(item.AudioURL.URL)}
			if item.Role != "" {
				entry["role"] = item.Role
			}
		default:
			continue
		}
		out = append(out, entry)
	}
	if !hasText && strings.TrimSpace(prompt) != "" {
		out = append([]map[string]any{{"type": "text", "text": strings.TrimSpace(prompt)}}, out...)
	}
	return out
}

func jingyuReferencesFromContent(content []VideoContent, ability string) []map[string]any {
	out := make([]map[string]any, 0, len(content))
	imageIndex := 0
	for _, item := range content {
		entry := map[string]any{}
		switch item.Type {
		case "image_url":
			if item.ImageURL == nil || strings.TrimSpace(item.ImageURL.URL) == "" {
				continue
			}
			entry["type"] = "image"
			entry["url"] = strings.TrimSpace(item.ImageURL.URL)
			switch ability {
			case videoAbilityImageToVideo:
				entry["role"] = "first_frame"
			case videoAbilityStartEndToVideo:
				if imageIndex == 0 {
					entry["role"] = "first_frame"
				} else {
					entry["role"] = "last_frame"
				}
			case videoAbilityReferenceToVideo:
				entry["role"] = "reference_image"
			}
			imageIndex++
		case "video_url":
			if item.VideoURL == nil || strings.TrimSpace(item.VideoURL.URL) == "" {
				continue
			}
			entry["type"] = "video"
			entry["url"] = strings.TrimSpace(item.VideoURL.URL)
			if ability == videoAbilityReferenceToVideo {
				entry["role"] = "reference_video"
			}
		case "audio_url":
			if item.AudioURL == nil || strings.TrimSpace(item.AudioURL.URL) == "" {
				continue
			}
			entry["type"] = "audio"
			entry["url"] = strings.TrimSpace(item.AudioURL.URL)
			if ability == videoAbilityReferenceToVideo {
				entry["role"] = "reference_audio"
			}
		default:
			continue
		}
		if _, hasRole := entry["role"]; !hasRole && item.Role != "" {
			entry["role"] = item.Role
		}
		out = append(out, entry)
	}
	return out
}

func validateVideoEstimatedCost(apiKey *APIKey, subscription *UserSubscription, actualCost float64) error {
	if actualCost <= 0 || apiKey == nil || apiKey.User == nil || apiKey.Group == nil {
		return nil
	}
	if apiKey.Quota > 0 && apiKey.QuotaUsed+actualCost > apiKey.Quota {
		return infraerrors.TooManyRequests("API_KEY_QUOTA_EXHAUSTED", "API key quota is exhausted")
	}
	if apiKey.HasRateLimits() {
		if apiKey.RateLimit5h > 0 && apiKey.EffectiveUsage5h()+actualCost > apiKey.RateLimit5h {
			return infraerrors.TooManyRequests("API_KEY_RATE_5H_EXCEEDED", "API key rate limit is exhausted")
		}
		if apiKey.RateLimit1d > 0 && apiKey.EffectiveUsage1d()+actualCost > apiKey.RateLimit1d {
			return infraerrors.TooManyRequests("API_KEY_RATE_1D_EXCEEDED", "API key rate limit is exhausted")
		}
		if apiKey.RateLimit7d > 0 && apiKey.EffectiveUsage7d()+actualCost > apiKey.RateLimit7d {
			return infraerrors.TooManyRequests("API_KEY_RATE_7D_EXCEEDED", "API key rate limit is exhausted")
		}
	}
	if apiKey.Group.IsSubscriptionType() {
		if subscription == nil {
			return infraerrors.Forbidden("SUBSCRIPTION_NOT_FOUND", "No active subscription found for this group")
		}
		daily, weekly, monthly := subscription.CheckAllLimits(apiKey.Group, actualCost)
		if !daily || !weekly || !monthly {
			return infraerrors.TooManyRequests("USAGE_LIMIT_EXCEEDED", "Usage limit exceeded")
		}
		return nil
	}
	if apiKey.User.Balance < actualCost {
		return infraerrors.Forbidden("INSUFFICIENT_BALANCE", "Insufficient account balance")
	}
	return nil
}

func SeedanceUpstreamModel(model, resolution string) string {
	return strings.TrimSpace(model) + "-" + strings.TrimSpace(resolution)
}

func defaultJingyuUpstreamModel(model string) string {
	switch strings.TrimSpace(model) {
	case VideoModelSeedance25:
		return videoJingyuSeedance25Model
	case VideoModelSeedance20:
		return videoJingyuSeedance20Model
	default:
		return ""
	}
}

func videoUpstreamModelForAccount(account *Account, normalized *normalizedVideoRequest) string {
	return videoProviderAdapterForAccount(account).UpstreamModel(account, normalized)
}

func videoUpstreamBodyForAccount(account *Account, normalized *normalizedVideoRequest, upstreamModel string) map[string]any {
	return videoProviderAdapterForAccount(account).BuildCreateBody(normalized, upstreamModel)
}

func isVideoAccountCompatible(account *Account, model string, resolution string) bool {
	return videoProviderAdapterForAccount(account).Compatible(model, resolution)
}

func isVideoAccountCompatibleForRequest(account *Account, normalized *normalizedVideoRequest) bool {
	return videoProviderAdapterForAccount(account).CompatibleRequest(normalized)
}

// videoProviderNeedsRequestCompatibility reports whether a provider constrains
// requests beyond the (model, resolution) pair the agent discovery set already
// checks — duration, aspect ratio and reference-media limits. Those providers
// must still run CompatibleRequest during agent-group scheduling so an
// unsupported request is routed to another upstream instead of failing there.
func videoProviderNeedsRequestCompatibility(provider string) bool {
	switch provider {
	case videoProviderYCYAPI, videoProviderNewtoken:
		return true
	default:
		return false
	}
}

type videoUpstreamCreateResult struct {
	ID string
}

type videoPollResult struct {
	Status   string
	VideoURL string
}

type videoUpstreamError struct {
	StatusCode int
	Body       []byte
	Err        error
}

func (e *videoUpstreamError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("video upstream status %d", e.StatusCode)
}

type mappedVideoClientError struct {
	VideoClientError
	StatusCode int
	Retryable  bool
}

func mapVideoUpstreamError(err error, polling bool) mappedVideoClientError {
	var upstreamErr *videoUpstreamError
	if errors.As(err, &upstreamErr) {
		switch upstreamErr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return mappedVideoClientError{VideoClientError: videoClientError("video_provider_unavailable", "Video service is temporarily unavailable. Please contact support if the issue persists."), StatusCode: upstreamErr.StatusCode}
		case http.StatusTooManyRequests:
			return mappedVideoClientError{VideoClientError: videoClientError("video_service_busy", "Video service is busy. Please retry later."), StatusCode: upstreamErr.StatusCode, Retryable: polling}
		}
		if upstreamErr.StatusCode >= 500 || upstreamErr.StatusCode == 0 {
			return mappedVideoClientError{VideoClientError: videoClientError("video_service_unavailable", "Video service is temporarily unavailable. Please retry later."), StatusCode: upstreamErr.StatusCode, Retryable: polling}
		}
		return mappedVideoClientError{VideoClientError: videoClientError("video_service_unavailable", "Video service is temporarily unavailable. Please retry later."), StatusCode: upstreamErr.StatusCode}
	}
	return mappedVideoClientError{VideoClientError: videoClientError("video_service_unavailable", "Video service is temporarily unavailable. Please retry later.")}
}

func (s *VideoService) recordVideoAccountFailure(ctx context.Context, account *Account, mapped mappedVideoClientError, cause error) {
	if s == nil || s.accountRepo == nil || account == nil {
		return
	}
	message := mapped.Message
	if message == "" {
		message = "Video service request failed"
	}
	switch mapped.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		_ = s.accountRepo.SetError(ctx, account.ID, message)
	case http.StatusTooManyRequests:
		_ = s.accountRepo.SetRateLimited(ctx, account.ID, time.Now().UTC().Add(1*time.Minute))
	case 0:
		reason := "video service temporary failure"
		if cause != nil {
			reason = "video service temporary failure: " + cause.Error()
		}
		_ = s.accountRepo.SetTempUnschedulable(ctx, account.ID, time.Now().UTC().Add(1*time.Minute), reason)
	}
}

func normalizeVideoUpstreamStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued":
		return VideoTaskStatusQueued
	case "processing", "in_progress", "progress", "running":
		return VideoTaskStatusProcessing
	case "completed", "succeeded", "success":
		return VideoTaskStatusCompleted
	case "failed", "error":
		return VideoTaskStatusFailed
	case "cancelled", "canceled":
		return VideoTaskStatusCancelled
	default:
		return VideoTaskStatusProcessing
	}
}

func normalizeJingyuVideoUpstreamStatus(status string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued":
		return VideoTaskStatusQueued, true
	case "processing", "in_progress", "progress":
		return VideoTaskStatusProcessing, true
	case "completed", "succeeded", "success":
		return VideoTaskStatusCompleted, true
	case "failed", "failure":
		return VideoTaskStatusFailed, true
	default:
		return "", false
	}
}

func videoResponseFromTask(task *VideoTask) *VideoResponse {
	if task == nil {
		return nil
	}
	resp := &VideoResponse{
		ID:           task.PublicID,
		Object:       videoObject,
		Model:        task.Model,
		Status:       task.Status,
		RefundStatus: videoRefundStatus(task),
		CreatedAt:    task.CreatedAt.Unix(),
	}
	if task.Status == VideoTaskStatusCompleted {
		resp.VideoURL = task.ResultVideoURL
		if task.CompletedAt != nil {
			completed := task.CompletedAt.Unix()
			resp.CompletedAt = &completed
		}
	}
	if task.Status == VideoTaskStatusFailed {
		resp.Error = videoErrorFromJSON(task.ErrorJSON)
		if resp.Error == nil {
			err := videoClientError("video_generation_failed", "Video generation failed. Please retry later.")
			resp.Error = &err
		}
	}
	return resp
}

func videoRefundStatus(task *VideoTask) string {
	if task == nil {
		return VideoRefundStatusNotApplicable
	}
	if task.RefundedAt != nil {
		return VideoRefundStatusRefunded
	}
	if (task.Status == VideoTaskStatusFailed || task.Status == VideoTaskStatusCancelled) && task.BilledAt != nil && task.ActualCost > 0 {
		return VideoRefundStatusPending
	}
	return VideoRefundStatusNotApplicable
}

func videoBadRequest(code, message string) error {
	return infraerrors.BadRequest(code, message)
}

func videoClientError(code, message string) VideoClientError {
	return VideoClientError{Code: code, Message: message}
}

func SanitizeVideoClientError(code, message string) (string, string) {
	code = strings.TrimSpace(code)
	message = strings.TrimSpace(message)
	if code == "" {
		code = "video_service_unavailable"
	}
	if message == "" {
		message = "Video service is temporarily unavailable. Please retry later."
	}
	joined := strings.ToLower(code + " " + message)
	forbidden := []string{
		"aigod",
		"api.aigod.one",
		"ycyapi",
		"ycyapi.cn",
		"newtoken",
		"newtoken.club",
		// Covers every newtoken upstream model id (sd2.0-720p-official, ...).
		"-official",
		"jingyu",
		"jingyuapi",
		"api.jingyuapi.art",
		"upstream",
		"upstream_task",
		"upstream_task_id",
	}
	for _, token := range forbidden {
		if strings.Contains(joined, token) {
			return "video_service_unavailable", "Video service is temporarily unavailable. Please retry later."
		}
	}
	return code, message
}

func videoErrorJSON(err VideoClientError) map[string]any {
	return map[string]any{"code": err.Code, "message": err.Message}
}

func videoErrorFromJSON(raw map[string]any) *VideoClientError {
	if raw == nil {
		return nil
	}
	code := strings.TrimSpace(stringFromMap(raw, "code"))
	message := strings.TrimSpace(stringFromMap(raw, "message"))
	if code == "" || message == "" {
		return nil
	}
	code, message = SanitizeVideoClientError(code, message)
	return &VideoClientError{Code: code, Message: message}
}

func videoAccountEndpoint(account *Account) (string, error) {
	baseURL := strings.TrimSpace(accountExtraString(account, "base_url"))
	if baseURL == "" {
		baseURL = videoDefaultBaseURLForProvider(videoAccountProvider(account))
	}
	apiPath := videoAccountAPIPath(account)
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", ErrVideoAccountNotFound
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/" + strings.TrimLeft(apiPath, "/")
	return parsed.String(), nil
}

func videoAccountAPIPath(account *Account) string {
	apiPath := strings.TrimSpace(accountExtraString(account, "api_path"))
	if apiPath == "" {
		return videoDefaultAPIPathForProvider(videoAccountProvider(account))
	}
	return apiPath
}

func videoAccountProvider(account *Account) string {
	provider := strings.ToLower(strings.TrimSpace(accountExtraString(account, "video_provider")))
	switch provider {
	case videoProviderYCYAPI:
		return videoProviderYCYAPI
	case videoProviderNewtoken:
		return videoProviderNewtoken
	case videoProviderJingyu:
		return videoProviderJingyu
	default:
		return videoProviderAigod
	}
}

func videoDefaultBaseURLForProvider(provider string) string {
	return videoProviderAdapterByName(provider).DefaultBaseURL()
}

func videoDefaultAPIPathForProvider(provider string) string {
	return videoProviderAdapterByName(provider).DefaultAPIPath()
}

func videoAccountDuration(account *Account, key string, fallback time.Duration) time.Duration {
	ms := accountExtraInt(account, key)
	if ms <= 0 {
		return fallback
	}
	return time.Duration(ms) * time.Millisecond
}

func videoAccountDefaultDuration(account *Account, key string) time.Duration {
	if videoAccountProvider(account) == videoProviderYCYAPI {
		switch key {
		case "poll_interval_ms":
			return videoYCYAPIPollInterval
		case "poll_timeout_ms":
			return videoYCYAPIPollTimeout
		case "request_timeout_ms":
			return videoYCYAPIRequestTimeout
		case "connect_timeout_ms":
			return videoYCYAPIConnectTimeout
		}
	}
	if videoAccountProvider(account) == videoProviderNewtoken {
		switch key {
		case "poll_interval_ms":
			return videoNewtokenPollInterval
		case "poll_timeout_ms":
			return videoNewtokenPollTimeout
		case "request_timeout_ms":
			return videoNewtokenRequestTimeout
		case "connect_timeout_ms":
			return videoNewtokenConnectTimeout
		}
	}
	if videoAccountProvider(account) == videoProviderJingyu {
		switch key {
		case "poll_interval_ms":
			return videoJingyuPollInterval
		case "poll_timeout_ms":
			return videoJingyuPollTimeout
		case "request_timeout_ms":
			return videoJingyuRequestTimeout
		case "connect_timeout_ms":
			return videoJingyuConnectTimeout
		}
	}
	switch key {
	case "poll_interval_ms":
		return videoDefaultPollInterval
	case "poll_timeout_ms":
		return videoDefaultPollTimeout
	case "connect_timeout_ms":
		return videoDefaultConnectTimeout
	default:
		return videoDefaultRequestTimeout
	}
}

func accountExtraString(account *Account, key string) string {
	if account == nil {
		return ""
	}
	if account.Extra != nil {
		if s, ok := account.Extra[key].(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return strings.TrimSpace(account.GetCredential(key))
}

func accountExtraInt(account *Account, key string) int {
	if account == nil {
		return 0
	}
	value, ok := account.Extra[key]
	if !ok || value == nil {
		value, ok = account.Credentials[key]
		if !ok || value == nil {
			return 0
		}
	}
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		i, _ := strconv.Atoi(v.String())
		return i
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(v))
		return i
	default:
		return 0
	}
}

func firstNonEmptyVideoString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func videoResultURLFromPayload(payload map[string]any) string {
	resultURL := firstNonEmptyVideoString(
		stringFromMap(payload, "download_url"),
		stringFromMap(payload, "result_asset_url"),
		stringFromMap(payload, "url"),
		stringFromMap(payload, "video_url"),
		stringFromMap(payload, "result_url"),
	)
	if resultURL != "" {
		return resultURL
	}
	metadata, ok := mapFromAny(payload["metadata"])
	if !ok {
		return ""
	}
	return strings.TrimSpace(stringFromMap(metadata, "url"))
}

func stringFromMap(raw map[string]any, key string) string {
	if raw == nil {
		return ""
	}
	value := raw[key]
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	default:
		return ""
	}
}

func mapFromAny(value any) (map[string]any, bool) {
	switch v := value.(type) {
	case map[string]any:
		return v, true
	default:
		return nil, false
	}
}

func optionalTrimmedPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func stringPtr(value string) *string {
	return &value
}

func videoFloat64Ptr(value float64) *float64 {
	return &value
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	raw, err := json.Marshal(input)
	if err != nil {
		out := make(map[string]any, len(input))
		for k, v := range input {
			out[k] = v
		}
		return out
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return input
	}
	return out
}

func generateVideoPublicID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return videoPublicIDPrefix + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return videoPublicIDPrefix + hex.EncodeToString(b[:])
}
