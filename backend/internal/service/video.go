package service

import (
	"context"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	VideoModelSeedance20     = "seedance-2.0"
	VideoModelSeedance20Fast = "seedance-2.0-fast"
	VideoModelSeedance25     = "seedance-2.5"
	VideoModelMinimaxH3      = "minimax-h3"
	VideoModelMinimaxH3Max   = "minimax-h3-max"
	VideoModelWan3           = "wan-3"

	VideoResolution480P  = "480p"
	VideoResolution720P  = "720p"
	VideoResolution768P  = "768p"
	VideoResolution1080P = "1080p"
	VideoResolution2K    = "2K"
	VideoResolution4K    = "4K"

	VideoTaskStatusQueued     = "queued"
	VideoTaskStatusProcessing = "processing"
	VideoTaskStatusCompleted  = "completed"
	VideoTaskStatusFailed     = "failed"
	VideoTaskStatusCancelled  = "cancelled"

	VideoRefundStatusNotApplicable = "not-applicable"
	VideoRefundStatusPending       = "pending"
	VideoRefundStatusRefunded      = "refunded"
)

// videoModelSpec collects the downstream limits of one video model. The
// upstream clamps out-of-range parameters to the nearest allowed value instead
// of rejecting them, so anything we let through silently produces a video that
// differs from the one we billed for. Every limit therefore has to be enforced
// here, not left to the provider.
type videoModelSpec struct {
	Resolutions       []string
	DefaultResolution string
	// AutoDurationSeconds is the duration substituted for the "auto" sentinel
	// (-1). Zero means the model has no auto duration and rejects the sentinel.
	AutoDurationSeconds int
	MinSeconds          int
	MaxSeconds          int
	MaxRefImages        int
	MaxRefVideos        int
	MaxRefAudios        int
	// MaxRefTotal caps images+videos+audios together. Zero means no aggregate
	// cap beyond the per-kind ones.
	MaxRefTotal int
	// MaxRefVideoSeconds caps a single reference clip, MaxRefVideoTotalSeconds
	// caps their sum.
	MaxRefVideoSeconds      int
	MaxRefVideoTotalSeconds int
	// AudioNeedsVisual rejects reference sets whose only media is audio.
	AudioNeedsVisual bool
}

// videoModelSpecs is the single source of truth for per-model video limits.
// Models absent from the table are only reachable through agent groups, where
// the catalog comes from each account's model_mapping; those fall back to
// videoDefaultModelSpec.
var videoModelSpecs = map[string]videoModelSpec{
	VideoModelSeedance20: {
		Resolutions:             []string{VideoResolution480P, VideoResolution720P, VideoResolution1080P, VideoResolution4K},
		DefaultResolution:       VideoResolution720P,
		MinSeconds:              videoMinDurationSeconds,
		MaxSeconds:              videoMaxDurationSeconds,
		MaxRefImages:            9,
		MaxRefVideos:            3,
		MaxRefAudios:            3,
		MaxRefVideoSeconds:      videoMaxDurationSeconds,
		MaxRefVideoTotalSeconds: videoMaxReferenceVideoTotal,
		AudioNeedsVisual:        true,
	},
	VideoModelSeedance20Fast: {
		Resolutions:             []string{VideoResolution480P, VideoResolution720P},
		DefaultResolution:       VideoResolution720P,
		MinSeconds:              videoMinDurationSeconds,
		MaxSeconds:              videoMaxDurationSeconds,
		MaxRefImages:            9,
		MaxRefVideos:            3,
		MaxRefAudios:            3,
		MaxRefVideoSeconds:      videoMaxDurationSeconds,
		MaxRefVideoTotalSeconds: videoMaxReferenceVideoTotal,
		AudioNeedsVisual:        true,
	},
	VideoModelSeedance25: {
		Resolutions:             []string{VideoResolution480P, VideoResolution720P, VideoResolution1080P},
		DefaultResolution:       VideoResolution720P,
		AutoDurationSeconds:     5,
		MinSeconds:              videoMinDurationSeconds,
		MaxSeconds:              videoSeedance25MaxDuration,
		MaxRefImages:            30,
		MaxRefVideos:            10,
		MaxRefAudios:            10,
		MaxRefTotal:             videoSeedance25MaxReferences,
		MaxRefVideoSeconds:      videoSeedance25MaxDuration,
		MaxRefVideoTotalSeconds: videoSeedance25MaxDuration,
	},
	VideoModelMinimaxH3: {
		Resolutions:             []string{VideoResolution768P, VideoResolution2K},
		DefaultResolution:       VideoResolution768P,
		MinSeconds:              5,
		MaxSeconds:              15,
		MaxRefImages:            9,
		MaxRefVideos:            3,
		MaxRefAudios:            3,
		MaxRefVideoSeconds:      videoMaxDurationSeconds,
		MaxRefVideoTotalSeconds: videoMaxReferenceVideoTotal,
		AudioNeedsVisual:        true,
	},
	VideoModelMinimaxH3Max: {
		Resolutions:             []string{VideoResolution480P, VideoResolution768P},
		DefaultResolution:       VideoResolution768P,
		MinSeconds:              5,
		MaxSeconds:              15,
		MaxRefImages:            12,
		MaxRefVideos:            12,
		MaxRefAudios:            12,
		MaxRefVideoSeconds:      videoMaxDurationSeconds,
		MaxRefVideoTotalSeconds: videoMaxReferenceVideoTotal,
	},
	VideoModelWan3: {
		Resolutions:             []string{VideoResolution480P, VideoResolution720P, VideoResolution1080P},
		DefaultResolution:       VideoResolution720P,
		MinSeconds:              2,
		MaxSeconds:              30,
		MaxRefImages:            10,
		MaxRefVideos:            5,
		MaxRefAudios:            5,
		MaxRefVideoSeconds:      videoMaxDurationSeconds,
		MaxRefVideoTotalSeconds: videoMaxReferenceVideoTotal,
		AudioNeedsVisual:        true,
	},
}

// videoDefaultModelSpec mirrors the limits that applied to every non-2.5 model
// before videoModelSpecs existed. It keeps agent groups, whose model catalog is
// account-defined, on exactly the validation they had before.
var videoDefaultModelSpec = videoModelSpec{
	DefaultResolution:       VideoResolution720P,
	MinSeconds:              videoMinDurationSeconds,
	MaxSeconds:              videoMaxDurationSeconds,
	MaxRefImages:            9,
	MaxRefVideos:            3,
	MaxRefAudios:            3,
	MaxRefVideoSeconds:      videoMaxDurationSeconds,
	MaxRefVideoTotalSeconds: videoMaxReferenceVideoTotal,
	AudioNeedsVisual:        true,
}

// videoSpecForModel returns the model's row, or the legacy defaults for models
// the gateway does not know about. The bool reports whether the model is one of
// the built-in ones.
func videoSpecForModel(model string) (videoModelSpec, bool) {
	spec, ok := videoModelSpecs[strings.TrimSpace(model)]
	if !ok {
		return videoDefaultModelSpec, false
	}
	return spec, true
}

func IsSupportedVideoModel(model string) bool {
	_, ok := videoModelSpecs[strings.TrimSpace(model)]
	return ok
}

func SupportedVideoModels() []string {
	return []string{
		VideoModelSeedance20,
		VideoModelSeedance20Fast,
		VideoModelSeedance25,
		VideoModelMinimaxH3,
		VideoModelMinimaxH3Max,
		VideoModelWan3,
	}
}

// SupportedVideoResolutions returns a copy of the model's allow-list so callers
// cannot mutate the shared spec table.
func SupportedVideoResolutions(model string) []string {
	spec, ok := videoSpecForModel(model)
	if !ok {
		return nil
	}
	return append([]string(nil), spec.Resolutions...)
}

func IsSupportedVideoResolution(model, resolution string) bool {
	spec, ok := videoSpecForModel(model)
	if !ok {
		return false
	}
	resolution = strings.TrimSpace(resolution)
	for _, allowed := range spec.Resolutions {
		if allowed == resolution {
			return true
		}
	}
	return false
}

var (
	ErrVideoTaskNotFound          = infraerrors.NotFound("VIDEO_TASK_NOT_FOUND", "Video task not found")
	ErrVideoPricingRuleNotFound   = infraerrors.BadRequest("video_pricing_rule_not_found", "Video pricing rule is not configured")
	ErrReferenceVideoDurationMiss = infraerrors.BadRequest("reference_video_duration_required", "Reference video duration is required")
	ErrVideoInvalidRequest        = infraerrors.BadRequest("invalid_video_request", "Invalid video request")
	ErrVideoAccountNotFound       = infraerrors.ServiceUnavailable("video_service_unavailable", "Video service is temporarily unavailable. Please retry later.")
	ErrJingyuCallbackUnauthorized = infraerrors.Unauthorized("jingyu_callback_unauthorized", "Invalid Jingyu video callback signature")
	ErrJingyuCallbackInvalid      = infraerrors.BadRequest("jingyu_callback_invalid", "Invalid Jingyu video callback")
	ErrJingyuCallbackNotFound     = infraerrors.NotFound("jingyu_callback_task_not_found", "Jingyu video callback task not found")
	ErrJingyuCallbackFailed       = infraerrors.InternalServer("jingyu_callback_processing_failed", "Jingyu video callback processing failed")
)

type VideoGroupPricingRule struct {
	ID                       int64     `json:"id"`
	GroupID                  int64     `json:"group_id"`
	ModelCode                string    `json:"model_code"`
	Resolution               string    `json:"resolution"`
	CreditsPerSecond         float64   `json:"credits_per_second"`
	ReferenceVideoMultiplier float64   `json:"reference_video_multiplier"`
	Enabled                  bool      `json:"enabled"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type VideoTask struct {
	ID                       int64
	PublicID                 string
	RequestID                *string
	UserID                   int64
	APIKeyID                 int64
	GroupID                  int64
	AccountID                int64
	Model                    string
	UpstreamModel            string
	Resolution               string
	DurationSeconds          int
	ReferenceDurationSeconds int
	BillableSeconds          int
	CostPerSecond            float64
	TotalCost                float64
	ActualCost               float64
	Status                   string
	UpstreamTaskID           *string
	RequestJSON              map[string]any
	UpstreamResponseJSON     map[string]any
	ErrorJSON                map[string]any
	ResultVideoURL           *string
	CreatedAt                time.Time
	UpdatedAt                time.Time
	CompletedAt              *time.Time
	BilledAt                 *time.Time
	RefundedAt               *time.Time
}

type VideoClientError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type VideoCreateRequest struct {
	Model            string         `json:"model"`
	Prompt           string         `json:"prompt"`
	Content          []VideoContent `json:"content"`
	Ratio            string         `json:"ratio"`
	AspectRatio      string         `json:"aspect_ratio"`
	AspectRatioCamel string         `json:"aspectRatio"`
	Duration         float64        `json:"duration"`
	Resolution       string         `json:"resolution"`
	GenerateAudio    *bool          `json:"generate_audio"`
	SafetyIdentifier string         `json:"safety_identifier"`
	AbilityCode      string         `json:"ability_code"`
	Raw              map[string]any `json:"-"`
}

type VideoContent struct {
	Type            string           `json:"type"`
	Text            string           `json:"text"`
	ImageURL        *VideoContentURL `json:"image_url"`
	VideoURL        *VideoContentURL `json:"video_url"`
	AudioURL        *VideoContentURL `json:"audio_url"`
	Role            string           `json:"role"`
	SubjectType     string           `json:"subject_type,omitempty"`
	DurationSeconds *float64         `json:"duration_seconds,omitempty"`
	Extra           map[string]any   `json:"-"`
}

type VideoContentURL struct {
	URL string `json:"url"`
}

type VideoCreateInput struct {
	APIKey              *APIKey
	Subscription        *UserSubscription
	Request             *VideoCreateRequest
	RawBody             []byte
	IdempotencyKey      string
	IdempotencyReplayed bool
	RequestID           string
	RequestPayloadHash  string
	UserAgent           string
	IPAddress           string
	InboundEndpoint     string
	UpstreamEndpoint    string
	ResultPublicBaseURL string
}

type VideoResponse struct {
	ID           string            `json:"id"`
	Object       string            `json:"object"`
	Model        string            `json:"model"`
	Status       string            `json:"status"`
	VideoURL     *string           `json:"video_url,omitempty"`
	Error        *VideoClientError `json:"error,omitempty"`
	RefundStatus string            `json:"refund_status"`
	CreatedAt    int64             `json:"created_at"`
	CompletedAt  *int64            `json:"completed_at,omitempty"`
}

type VideoCostEstimate struct {
	Model                    string    `json:"model"`
	Resolution               string    `json:"resolution"`
	AbilityCode              string    `json:"ability_code"`
	Count                    int       `json:"count"`
	GeneratedSeconds         int       `json:"generated_seconds"`
	ReferenceVideoSeconds    int       `json:"reference_video_seconds"`
	BillableSeconds          int       `json:"billable_seconds"`
	CreditsPerSecond         float64   `json:"credits_per_second"`
	ReferenceVideoMultiplier float64   `json:"reference_video_multiplier"`
	RateMultiplier           float64   `json:"rate_multiplier"`
	TotalCost                float64   `json:"total_cost"`
	ActualCost               float64   `json:"actual_cost"`
	PricingRuleUpdatedAt     time.Time `json:"pricing_rule_updated_at"`
}

type VideoTaskUpdateInput struct {
	PublicID string
	Status   string
	Error    *VideoClientError
}

type VideoTaskLifecycleInput struct {
	PublicID            string
	Account             *Account
	APIKey              *APIKey
	Subscription        *UserSubscription
	UpstreamBody        map[string]any
	RequestPayloadHash  string
	UserAgent           string
	IPAddress           string
	InboundEndpoint     string
	UpstreamEndpoint    string
	ResultPublicBaseURL string
	UpstreamTaskID      string
}

type VideoTaskCreateInput struct {
	PublicID                 string
	RequestID                *string
	UserID                   int64
	APIKeyID                 int64
	GroupID                  int64
	AccountID                int64
	Model                    string
	UpstreamModel            string
	Resolution               string
	DurationSeconds          int
	ReferenceDurationSeconds int
	BillableSeconds          int
	CostPerSecond            float64
	TotalCost                float64
	ActualCost               float64
	Status                   string
	UpstreamTaskID           *string
}

type VideoTaskUpdate struct {
	Status         *string
	UpstreamTaskID *string
	ErrorJSON      map[string]any
	ResultVideoURL *string
	CompletedAt    *time.Time
	BilledAt       *time.Time
	RefundedAt     *time.Time
}

type VideoTaskRepository interface {
	Create(ctx context.Context, input *VideoTaskCreateInput) (*VideoTask, error)
	GetByPublicID(ctx context.Context, publicID string) (*VideoTask, error)
	UpdateByPublicID(ctx context.Context, publicID string, update VideoTaskUpdate) (*VideoTask, error)
	MarkProcessingByPublicID(ctx context.Context, publicID string, upstreamTaskID string) (*VideoTask, bool, error)
	TransitionTerminalByPublicID(ctx context.Context, publicID string, update VideoTaskUpdate) (*VideoTask, bool, error)
	MarkBilled(ctx context.Context, publicID string, billedAt time.Time) (bool, error)
	MarkRefunded(ctx context.Context, publicID string, refundedAt time.Time) (bool, error)
}

type VideoGroupPricingRuleRepository interface {
	ListByGroupID(ctx context.Context, groupID int64) ([]VideoGroupPricingRule, error)
	ReplaceForGroup(ctx context.Context, groupID int64, rules []VideoGroupPricingRule) error
	GetEnabledRule(ctx context.Context, groupID int64, modelCode string, resolution string) (*VideoGroupPricingRule, error)
}

type VideoAccountRepository interface {
	GetByID(ctx context.Context, id int64) (*Account, error)
	ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error)
	SetError(ctx context.Context, id int64, errorMsg string) error
	SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error
	SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error
	IncrementQuotaUsed(ctx context.Context, id int64, amount float64) error
}
