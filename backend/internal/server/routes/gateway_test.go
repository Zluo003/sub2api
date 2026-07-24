package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestForceAgentPlatformOnlyAffectsAgentGroups(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name  string
		group *service.Group
		want  bool
	}{
		{name: "agent", group: &service.Group{Kind: "agent", SystemCode: "yingzo", Platform: service.PlatformOpenAI}, want: true},
		{name: "ordinary", group: &service.Group{Kind: "standard", Platform: service.PlatformOpenAI}, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.Use(func(c *gin.Context) {
				c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{Group: tc.group})
				c.Next()
			})
			r.Use(forceAgentPlatform(service.PlatformGemini))
			r.GET("/test", func(c *gin.Context) {
				platform, ok := servermiddleware.GetForcePlatformFromContext(c)
				c.JSON(http.StatusOK, gin.H{"forced": ok, "platform": platform})
			})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test", nil))
			require.Equal(t, http.StatusOK, w.Code)
			require.Contains(t, w.Body.String(), `"forced":`+map[bool]string{true: "true", false: "false"}[tc.want])
			if tc.want {
				require.Contains(t, w.Body.String(), service.PlatformGemini)
			}
		})
	}
}

func TestSetAgentRequestPlatformUsesNativeEndpointPlatform(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, platform := range []string{
		service.PlatformOpenAI,
		service.PlatformAnthropic,
		service.PlatformGemini,
		service.PlatformSeedance,
	} {
		t.Run(platform, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
			c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{Group: &service.Group{
				Kind: "agent", SystemCode: "yingzo", Platform: service.PlatformOpenAI,
			}})

			require.True(t, setAgentRequestPlatform(c, platform))
			forced, ok := servermiddleware.GetForcePlatformFromContext(c)
			require.True(t, ok)
			require.Equal(t, platform, forced)
		})
	}
}

func newGatewayRoutesTestRouter(platform ...string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	groupPlatform := service.PlatformOpenAI
	ungrouped := false
	if len(platform) > 0 && platform[0] != "" {
		if platform[0] == "__ungrouped__" {
			ungrouped = true
		} else {
			groupPlatform = platform[0]
		}
	}
	settingService := service.NewSettingService(gatewayRoutesSettingRepoStub{}, &config.Config{})

	RegisterGatewayRoutes(
		router,
		&handler.Handlers{
			Gateway:       &handler.GatewayHandler{},
			OpenAIGateway: &handler.OpenAIGatewayHandler{},
			Video:         handler.NewVideoHandler(nil),
		},
		servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
			groupID := int64(1)
			apiKey := &service.APIKey{}
			if !ungrouped {
				apiKey.GroupID = &groupID
				apiKey.Group = &service.Group{Platform: groupPlatform}
			}
			c.Set(string(servermiddleware.ContextKeyAPIKey), apiKey)
			c.Next()
		}),
		nil,
		nil,
		nil,
		settingService,
		&config.Config{},
	)

	return router
}

type gatewayRoutesSettingRepoStub struct{}

func (gatewayRoutesSettingRepoStub) Get(ctx context.Context, key string) (*service.Setting, error) {
	return nil, service.ErrSettingNotFound
}

func (gatewayRoutesSettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	return "", service.ErrSettingNotFound
}

func (gatewayRoutesSettingRepoStub) Set(ctx context.Context, key, value string) error {
	return nil
}

func (gatewayRoutesSettingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	return map[string]string{}, nil
}

func (gatewayRoutesSettingRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	return nil
}

func (gatewayRoutesSettingRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}

func (gatewayRoutesSettingRepoStub) Delete(ctx context.Context, key string) error {
	return service.ErrSettingNotFound
}

func TestGatewayRoutesOpenAIResponsesCompactPathIsRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter()

	for _, path := range []string{
		"/v1/responses/compact",
		"/responses/compact",
		"/backend-api/codex/responses",
		"/backend-api/codex/responses/compact",
	} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gpt-5"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code, "path=%s should hit OpenAI responses handler", path)
	}
}

func TestGatewayRoutesOpenAIImagesPathsAreRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter()

	for _, path := range []string{
		"/v1/images/generations",
		"/v1/images/edits",
		"/images/generations",
		"/images/edits",
	} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gpt-image-2","prompt":"draw a cat"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code, "path=%s should hit OpenAI images handler", path)
	}
}

func TestGatewayRoutesVideosUngroupedKeyUsesOpenAIErrorShape(t *testing.T) {
	router := newGatewayRoutesTestRouter("__ungrouped__")

	req := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"seedance-2.0"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), `"error"`)
	require.Contains(t, w.Body.String(), `"type":"permission_error"`)
	require.Contains(t, w.Body.String(), `"code":"api_key_group_required"`)
	require.NotContains(t, w.Body.String(), `"type":"error"`)
}

func TestGatewayRoutesGrokOnlyAllowsResponsesHTTP(t *testing.T) {
	router := newGatewayRoutesTestRouter(service.PlatformGrok)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v1/messages"},
		{http.MethodPost, "/v1/chat/completions"},
		{http.MethodPost, "/chat/completions"},
		{http.MethodGet, "/v1/responses"},
		{http.MethodGet, "/responses"},
		{http.MethodGet, "/backend-api/codex/responses"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"model":"grok"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusNotFound, w.Code, "method=%s path=%s", tc.method, tc.path)
		require.Contains(t, w.Body.String(), "not supported for Grok groups")
	}

	for _, path := range []string{
		"/v1/responses",
		"/responses",
		"/backend-api/codex/responses",
	} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"grok","input":"hi"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code, "path=%s should still reach Responses handler", path)
	}
}
