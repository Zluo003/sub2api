package routes

import (
	"bytes"
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

type agentModelAccountRepo struct {
	service.AccountRepository
	accounts []service.Account
}

func (r *agentModelAccountRepo) ListSchedulableByGroupIDAndPlatforms(context.Context, int64, []string) ([]service.Account, error) {
	return r.accounts, nil
}

type agentModelVideoPricingRepo struct {
	service.VideoGroupPricingRuleRepository
}

func (r *agentModelVideoPricingRepo) ListByGroupID(context.Context, int64) ([]service.VideoGroupPricingRule, error) {
	return []service.VideoGroupPricingRule{{
		ModelCode: service.VideoModelSeedance20, Resolution: service.VideoResolution720P,
		CreditsPerSecond: 1, Enabled: true,
	}}, nil
}

func TestAgentCapabilities(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	cfg := &config.Config{Pricing: config.PricingConfig{DataDir: t.TempDir()}}
	video := service.NewVideoService(nil, nil, &agentModelVideoPricingRepo{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	accounts := &agentModelAccountRepo{accounts: []service.Account{{
		Platform: service.PlatformVideo, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-video"},
	}}}
	agentHandler := handler.NewAgentHandler(db, cfg, service.NewBillingService(cfg, nil), video, nil, accounts, nil)
	t.Cleanup(agentHandler.StopCleanupWorker)
	r := gin.New()
	v := r.Group("/api/v1")
	h := &handler.Handlers{Agent: agentHandler}
	RegisterAgentRoutes(r, v, h, func(c *gin.Context) { c.Next() }, agentAuthForTest(true))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/agent/models/seedance-2.0/capabilities", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), "video_reference_to_video")
}

func agentAuthForTest(agent bool) func(*gin.Context) {
	return func(c *gin.Context) {
		kind, code := "standard", ""
		if agent {
			kind, code = "agent", "yingzo"
		}
		groupID := int64(9)
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{GroupID: &groupID, Group: &service.Group{ID: groupID, Kind: kind, SystemCode: code}})
		c.Next()
	}
}

func TestAgentRoutesRejectOrdinaryAPIKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	v := r.Group("/api/v1")
	h := &handler.Handlers{Agent: &handler.AgentHandler{}}
	RegisterAgentRoutes(r, v, h, func(c *gin.Context) { c.Next() }, agentAuthForTest(false))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/agent/models", nil))
	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "agent_credential_required")
}

func TestAgentPreflightRejectsUntrustedCallerMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	v := r.Group("/api/v1")
	h := &handler.Handlers{Agent: &handler.AgentHandler{}}
	RegisterAgentRoutes(r, v, h, func(c *gin.Context) { c.Next() }, agentAuthForTest(true))
	body := bytes.NewBufferString(`{
      "model":"seedance-2.0",
      "mode":"video_reference_to_video",
      "prompt":"角色进入画面",
      "duration_seconds":5,
      "ratio":"16:9",
      "resolution":"1080P",
      "references":[{"role":"reference_image","media_type":"image","mime_type":"image/png","size_bytes":1024,"width":1920,"height":1080}]
    }`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/media/preflight", body)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), `"trusted_media_metadata_required"`)
}
