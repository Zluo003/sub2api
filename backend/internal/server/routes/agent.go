package routes

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

func RegisterAgentRoutes(r *gin.Engine, v1 *gin.RouterGroup, h *handler.Handlers, jwtAuth middleware.JWTAuthMiddleware, apiKeyAuth middleware.APIKeyAuthMiddleware) {
	r.GET("/.well-known/yingzo.json", h.Agent.GetYingzoDiscovery)
	v1.POST("/agent/device/authorizations", h.Agent.StartDeviceAuthorization)
	v1.POST("/agent/device/token", h.Agent.PollDeviceAuthorization)
	v1.GET("/agent/plugin/releases/latest", h.Agent.GetLatestYingzoRelease)
	v1.GET("/agent/plugin/download/:ticket/:filename", h.Agent.DownloadYingzoRelease)
	v1.HEAD("/agent/plugin/download/:ticket/:filename", h.Agent.DownloadYingzoRelease)
	user := v1.Group("/agent")
	user.Use(gin.HandlerFunc(jwtAuth))
	user.GET("/device/authorizations/:user_code", h.Agent.GetDeviceAuthorization)
	user.POST("/device/approve", h.Agent.ApproveDeviceAuthorization)
	user.DELETE("/installations/:id", h.Agent.RevokeInstallation)
	user.POST("/plugin/install-instructions", h.Agent.CreateYingzoInstallInstructions)
	g := v1.Group("/agent")
	g.Use(gin.HandlerFunc(apiKeyAuth))
	g.Use(requireAgentGroup())
	g.GET("/pricing", h.Agent.GetAgentPricingSnapshot)
	g.POST("/generation/estimates", h.Agent.EstimateGeneration)
	g.GET("/generation/estimates/:id", h.Agent.GetGenerationEstimate)
	g.POST("/assets", h.Agent.UploadTemporaryAsset)
	g.GET("/assets/:id", h.Agent.GetTemporaryAsset)
	g.DELETE("/installation", h.Agent.RevokeCurrentInstallation)
	r.GET("/temporary-assets/:token", h.Agent.ServeTemporaryAsset)
	r.HEAD("/temporary-assets/:token", h.Agent.ServeTemporaryAsset)
	r.GET("/media/:id/:filename", h.Agent.ServeCleanTemporaryAsset)
	r.HEAD("/media/:id/:filename", h.Agent.ServeCleanTemporaryAsset)
}

func requireAgentGroup() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey, ok := middleware.GetAPIKeyFromContext(c)
		if !ok || apiKey.Group == nil || !apiKey.Group.IsAgent() {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": gin.H{
				"code":    "agent_credential_required",
				"message": "This endpoint requires a Yingzo Agent credential",
			}})
			return
		}
		c.Next()
	}
}
