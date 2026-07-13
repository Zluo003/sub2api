package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type VideoHandler struct {
	videoService *service.VideoService
	agentHandler *AgentHandler
}

func NewVideoHandler(videoService *service.VideoService, agentHandler *AgentHandler) *VideoHandler {
	return &VideoHandler{videoService: videoService, agentHandler: agentHandler}
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
	if apiKey.Group != nil && apiKey.Group.IsAgent() {
		if h.agentHandler == nil {
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "agent_preflight_unavailable", "Agent media preflight is unavailable")
			return
		}
		publicBaseURL, publicBaseErr := agentAssetsPublicBaseURL(c.Request)
		if publicBaseErr != nil {
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "temporary_asset_public_url_invalid", publicBaseErr.Error())
			return
		}
		preflight, preflightErr := h.agentHandler.PreflightVideoSubmission(c.Request.Context(), apiKey.ID, &req, int64(len(body)), publicBaseURL)
		if preflightErr != nil {
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "trusted_media_metadata_required", preflightErr.Error())
			return
		}
		if !preflight.Valid {
			issue := preflight.Errors[0]
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", issue.Code, issue.Message)
			return
		}
	}

	setOpsRequestContext(c, req.Model, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeVideo))
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := GetUpstreamEndpoint(c, service.PlatformVideo)

	requestID, _ := c.Request.Context().Value(ctxkey.RequestID).(string)
	resp, err := h.videoService.CreateTask(c.Request.Context(), &service.VideoCreateInput{
		APIKey:             apiKey,
		Subscription:       subscription,
		Request:            &req,
		RawBody:            body,
		RequestID:          requestID,
		RequestPayloadHash: service.HashUsageRequestPayload(body),
		UserAgent:          c.GetHeader("User-Agent"),
		IPAddress:          ip.GetClientIP(c),
		InboundEndpoint:    inboundEndpoint,
		UpstreamEndpoint:   upstreamEndpoint,
	})
	if err != nil {
		h.errorFrom(c, err)
		return
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
