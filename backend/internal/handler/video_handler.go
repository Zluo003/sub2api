package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type VideoHandler struct {
	videoService             *service.VideoService
	securityAuditCoordinator *securityaudit.Coordinator
	contentModerationService *service.ContentModerationService
}

func NewVideoHandler(videoService *service.VideoService) *VideoHandler {
	return &VideoHandler{videoService: videoService}
}

// checkSecurityAudit 让视频提示词与图片生成共用上游的审核协调器。
func (h *VideoHandler) checkSecurityAudit(c *gin.Context, reqLog *zap.Logger, apiKey *service.APIKey, subject middleware2.AuthSubject, model string, body []byte) *securityaudit.Decision {
	if h == nil {
		return nil
	}
	return runSecurityAudit(c, reqLog, h.securityAuditCoordinator, h.contentModerationService, apiKey, subject,
		service.ContentModerationProtocolVideos, model, body, "http")
}

func (h *VideoHandler) Create(c *gin.Context) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "invalid_api_key", "Invalid API key")
		return
	}
	subscription, _ := middleware2.GetSubscriptionFromContext(c)

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.errorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", "request_body_too_large", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "invalid_video_request", "Failed to read request body")
		return
	}
	if len(body) == 0 {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "invalid_video_request", "Request body is empty")
		return
	}

	var req service.VideoCreateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "invalid_video_request", "Failed to parse request body")
		return
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err == nil {
		req.Raw = raw
	}
	setOpsRequestContext(c, req.Model, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeVideo))

	subject, hasSubject := middleware2.GetAuthSubjectFromContext(c)
	if hasSubject {
		reqLog := requestLogger(c, "handler.video", zap.Int64("api_key_id", apiKey.ID))
		if decision := h.checkSecurityAudit(c, reqLog, apiKey, subject, req.Model, body); decision != nil && !decision.AllowNextStage {
			return
		}
	}
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := GetUpstreamEndpoint(c, service.PlatformSeedance)

	requestID, _ := c.Request.Context().Value(ctxkey.RequestID).(string)
	createInput := &service.VideoCreateInput{
		APIKey:             apiKey,
		Subscription:       subscription,
		Request:            &req,
		RawBody:            body,
		IdempotencyKey:     c.GetHeader("Idempotency-Key"),
		RequestID:          requestID,
		RequestPayloadHash: service.HashUsageRequestPayload(body),
		UserAgent:          c.GetHeader("User-Agent"),
		IPAddress:          ip.GetClientIP(c),
		InboundEndpoint:    inboundEndpoint,
		UpstreamEndpoint:   upstreamEndpoint,
	}
	resp, err := h.videoService.CreateTask(c.Request.Context(), createInput)
	if err != nil {
		h.errorFrom(c, err)
		return
	}
	if createInput.IdempotencyReplayed {
		c.Header("X-Idempotency-Replayed", "true")
	}
	c.JSON(http.StatusOK, resp)
}

func (h *VideoHandler) Get(c *gin.Context) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "invalid_api_key", "Invalid API key")
		return
	}
	setOpsRequestContext(c, "", false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeVideo))

	resp, err := h.videoService.GetTask(c.Request.Context(), c.Param("id"), apiKey)
	if err != nil {
		h.errorFrom(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *VideoHandler) errorFrom(c *gin.Context, err error) {
	if retryAfter := service.RetryAfterSecondsFromError(err); retryAfter > 0 {
		c.Header("Retry-After", strconv.Itoa(retryAfter))
	}
	status, body := infraerrors.ToHTTP(err)
	code := strings.TrimSpace(body.Reason)
	if code == "" {
		code = "video_service_unavailable"
	}
	message := strings.TrimSpace(body.Message)
	if message == "" {
		message = "Video service is temporarily unavailable. Please retry later."
	}
	h.errorResponse(c, status, videoErrorType(status), code, message)
}

func (h *VideoHandler) errorResponse(c *gin.Context, status int, errType, code, message string) {
	code, message = service.SanitizeVideoClientError(code, message)
	c.JSON(status, gin.H{
		"error": gin.H{
			"type":    errType,
			"code":    code,
			"message": message,
		},
	})
}

func videoErrorType(status int) string {
	switch status {
	case http.StatusBadRequest, http.StatusRequestEntityTooLarge:
		return "invalid_request_error"
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusNotFound:
		return "not_found_error"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	default:
		return "api_error"
	}
}
