package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestVideoHandlerPassesIdempotencyKeyAndRejectsChangedPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousCoordinator := service.DefaultIdempotencyCoordinator()
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(newUserMemoryIdempotencyRepoStub(), service.DefaultIdempotencyConfig()))
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(previousCoordinator) })

	videoService := service.NewVideoService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler := NewVideoHandler(videoService)
	groupID := int64(20)
	apiKey := &service.APIKey{
		ID:      10,
		UserID:  100,
		GroupID: &groupID,
		User:    &service.User{ID: 100},
		Group:   &service.Group{ID: groupID, Platform: service.PlatformSeedance},
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyAPIKey), apiKey)
		c.Next()
	})
	router.POST("/v1/videos", handler.Create)

	call := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "same-video-key")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	first := call(`{"model":"invalid","prompt":"one","duration":8,"resolution":"720p"}`)
	require.Equal(t, http.StatusBadRequest, first.Code)

	conflict := call(`{"model":"invalid","prompt":"changed","duration":8,"resolution":"720p"}`)
	require.Equal(t, http.StatusConflict, conflict.Code)
	require.Contains(t, conflict.Body.String(), "IDEMPOTENCY_KEY_CONFLICT")
}
