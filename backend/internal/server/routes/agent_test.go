package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/agent/generation/estimates", nil))
	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "agent_credential_required")
}
