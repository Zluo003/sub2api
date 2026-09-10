package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func channelMonitorModeV2Guard(settingService *service.SettingService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if settingService == nil {
			response.ErrorFrom(c, service.ErrChannelMonitorDisabled)
			c.Abort()
			return
		}
		rt := settingService.GetChannelMonitorRuntime(c.Request.Context())
		if !rt.Enabled {
			response.ErrorFrom(c, service.ErrChannelMonitorDisabled)
			c.Abort()
			return
		}
		if !rt.PassiveAggregationAllowed() {
			response.ErrorFrom(c, service.ErrChannelMonitorModeMismatch)
			c.Abort()
			return
		}
		c.Next()
	}
}
