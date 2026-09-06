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

	VideoResolution480P  = "480p"
	VideoResolution720P  = "720p"
	VideoResolution1080P = "1080p"
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

func IsSupportedVideoResolution(model, resolution string) bool {
	model = strings.TrimSpace(model)
	resolution = strings.TrimSpace(resolution)
	switch model {
	case VideoModelSeedance20:
		return resolution == VideoResolution480P ||
			resolution == VideoResolution720P ||
			resolution == VideoResolution1080P ||
			resolution == VideoResolution4K
	case VideoModelSeedance20Fast:
		return resolution == VideoResolution480P ||
			resolution == VideoResolution720P
	case VideoModelSeedance25:
		return resolution == VideoResolution480P ||
			resolution == VideoResolution720P ||
			resolution == VideoResolution1080P
	default:
		return false
	}
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
