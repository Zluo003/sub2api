package handler

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type sha256HexArgument struct{}

type authInvalidatorSpy struct{ keys []string }

func (s *authInvalidatorSpy) InvalidateAuthCacheByKey(_ context.Context, key string) {
	s.keys = append(s.keys, key)
}
func (s *authInvalidatorSpy) InvalidateAuthCacheByUserID(context.Context, int64)  {}
func (s *authInvalidatorSpy) InvalidateAuthCacheByGroupID(context.Context, int64) {}

func (sha256HexArgument) Match(value driver.Value) bool {
	s, ok := value.(string)
	if !ok || len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

type memoryObjectStore struct {
	objects map[string][]byte
	deleted []string
}

func newMemoryObjectStore() *memoryObjectStore {
	return &memoryObjectStore{objects: make(map[string][]byte)}
}

func (s *memoryObjectStore) Upload(_ context.Context, key string, body io.Reader, _ string) (int64, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return 0, err
	}
	s.objects[key] = data
	return int64(len(data)), nil
}

func (s *memoryObjectStore) Download(_ context.Context, key string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(s.objects[key])), nil
}

func (s *memoryObjectStore) Delete(_ context.Context, key string) error {
	delete(s.objects, key)
	s.deleted = append(s.deleted, key)
	return nil
}

func (s *memoryObjectStore) PresignURL(context.Context, string, time.Duration) (string, error) {
	return "", nil
}

func (s *memoryObjectStore) HeadBucket(context.Context) error { return nil }

func newAgentHandlerMock(t *testing.T) (*AgentHandler, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return &AgentHandler{db: db, dataDir: t.TempDir()}, mock
}

func responseCode(body *httptest.ResponseRecorder) string {
	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body.Body.Bytes(), &response)
	return response.Error.Code
}

func TestStartDeviceAuthorizationStoresOnlyTokenHashes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	installationID := uuid.New()
	mock.ExpectExec("INSERT INTO agent_device_authorizations").
		WithArgs(sqlmock.AnyArg(), sha256HexArgument{}, sha256HexArgument{}, installationID, "Mac Studio", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	r := gin.New()
	r.POST("/device", h.StartDeviceAuthorization)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "https://staging.example.com/device", strings.NewReader(`{"installation_id":"`+installationID.String()+`","installation_name":"Mac Studio"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.NotEmpty(t, response["device_code"])
	require.NotEmpty(t, response["user_code"])
	require.Equal(t, "https://staging.example.com/agent/authorize", response["verification_uri"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStartDeviceAuthorizationRejectsInvalidInstallationID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	r := gin.New()
	r.POST("/device", h.StartDeviceAuthorization)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/device", strings.NewReader(`{"installation_id":"not-a-uuid","installation_name":"Mac"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Equal(t, "invalid_request", responseCode(w))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApproveDeviceAuthorizationHashesNormalizedUserCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	mock.ExpectExec("UPDATE agent_device_authorizations").
		WithArgs(int64(42), hashToken("AB-CD123")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	r := gin.New()
	r.POST("/approve", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
		h.ApproveDeviceAuthorization(c)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/approve", strings.NewReader(`{"user_code":"  ab-cd123 "}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetDeviceAuthorizationReturnsOnlyPendingRequestMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	installationID := uuid.New()
	expiresAt := time.Now().Add(5 * time.Minute)
	mock.ExpectQuery("SELECT installation_id,installation_name,status,expires_at").
		WithArgs(hashToken("AB-CD123")).
		WillReturnRows(sqlmock.NewRows([]string{"installation_id", "installation_name", "status", "expires_at"}).
			AddRow(installationID, "Mac Studio", "pending", expiresAt))
	r := gin.New()
	r.GET("/authorization/:user_code", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
		h.GetDeviceAuthorization(c)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/authorization/ab-cd123", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "Mac Studio")
	require.NotContains(t, w.Body.String(), "device_code")
	require.NotContains(t, w.Body.String(), "api_key")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPollDeviceAuthorizationStatesAndCredentialReuse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("pending", func(t *testing.T) {
		h, mock := newAgentHandlerMock(t)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT id,installation_id,installation_name,status,user_id,expires_at").
			WithArgs(hashToken("pending-device")).
			WillReturnRows(sqlmock.NewRows([]string{"id", "installation_id", "installation_name", "status", "user_id", "expires_at"}).
				AddRow(uuid.New(), uuid.New(), "Mac", "pending", nil, time.Now().Add(time.Minute)))
		mock.ExpectRollback()
		w := pollDevice(t, h, "pending-device")
		require.Equal(t, http.StatusPreconditionRequired, w.Code)
		require.Equal(t, "authorization_pending", responseCode(w))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("expired", func(t *testing.T) {
		h, mock := newAgentHandlerMock(t)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT id,installation_id,installation_name,status,user_id,expires_at").
			WithArgs(hashToken("expired-device")).
			WillReturnRows(sqlmock.NewRows([]string{"id", "installation_id", "installation_name", "status", "user_id", "expires_at"}).
				AddRow(uuid.New(), uuid.New(), "Mac", "approved", int64(7), time.Now().Add(-time.Minute)))
		mock.ExpectRollback()
		w := pollDevice(t, h, "expired-device")
		require.Equal(t, http.StatusBadRequest, w.Code)
		require.Equal(t, "expired_token", responseCode(w))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("reuses active installation credential", func(t *testing.T) {
		h, mock := newAgentHandlerMock(t)
		invalidator := &authInvalidatorSpy{}
		h.authInvalidator = invalidator
		authorizationID, installationID := uuid.New(), uuid.New()
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT id,installation_id,installation_name,status,user_id,expires_at").
			WithArgs(hashToken("approved-device")).
			WillReturnRows(sqlmock.NewRows([]string{"id", "installation_id", "installation_name", "status", "user_id", "expires_at"}).
				AddRow(authorizationID, installationID, "Mac", "approved", int64(7), time.Now().Add(time.Minute)))
		mock.ExpectQuery("SELECT ai.api_key_id,ak.key").
			WithArgs(installationID, int64(7)).
			WillReturnRows(sqlmock.NewRows([]string{"api_key_id", "key"}).AddRow(int64(81), "sk-agent-existing"))
		mock.ExpectExec("UPDATE agent_device_authorizations SET status='consumed'").
			WithArgs(authorizationID).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		w := pollDevice(t, h, "approved-device")
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), "sk-agent-existing")
		require.Equal(t, []string{"sk-agent-existing"}, invalidator.keys)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("rejects replay of a consumed device credential", func(t *testing.T) {
		h, mock := newAgentHandlerMock(t)
		authorizationID, installationID := uuid.New(), uuid.New()
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT id,installation_id,installation_name,status,user_id,expires_at").
			WithArgs(hashToken("consumed-device")).
			WillReturnRows(sqlmock.NewRows([]string{"id", "installation_id", "installation_name", "status", "user_id", "expires_at"}).
				AddRow(authorizationID, installationID, "Mac", "consumed", int64(7), time.Now().Add(time.Minute)))
		mock.ExpectRollback()
		w := pollDevice(t, h, "consumed-device")
		require.Equal(t, http.StatusBadRequest, w.Code)
		require.Equal(t, "invalid_grant", responseCode(w))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("does not take over an installation owned by another user", func(t *testing.T) {
		h, mock := newAgentHandlerMock(t)
		authorizationID, installationID := uuid.New(), uuid.New()
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT id,installation_id,installation_name,status,user_id,expires_at").
			WithArgs(hashToken("conflicting-device")).
			WillReturnRows(sqlmock.NewRows([]string{"id", "installation_id", "installation_name", "status", "user_id", "expires_at"}).
				AddRow(authorizationID, installationID, "Mac", "approved", int64(7), time.Now().Add(time.Minute)))
		mock.ExpectQuery("SELECT ai.api_key_id,ak.key").
			WithArgs(installationID, int64(7)).
			WillReturnError(sql.ErrNoRows)
		mock.ExpectQuery("SELECT id FROM groups").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(9)))
		mock.ExpectQuery("INSERT INTO api_keys").
			WithArgs(int64(7), sqlmock.AnyArg(), "Yingzo Mac", int64(9)).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(82)))
		mock.ExpectExec("INSERT INTO agent_installations").
			WithArgs(installationID, int64(7), int64(82), "Mac").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()
		w := pollDevice(t, h, "conflicting-device")
		require.Equal(t, http.StatusInternalServerError, w.Code)
		require.Equal(t, "credential_provision_failed", responseCode(w))
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func pollDevice(t *testing.T, h *AgentHandler, code string) *httptest.ResponseRecorder {
	t.Helper()
	r := gin.New()
	r.POST("/token", h.PollDeviceAuthorization)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(`{"device_code":"`+code+`"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestRevokeInstallationDisablesOnlyOwnedCredential(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	invalidator := &authInvalidatorSpy{}
	h.authInvalidator = invalidator
	installationID := uuid.New()
	mock.ExpectQuery("WITH revoked AS").
		WithArgs(installationID.String(), int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"key"}).AddRow("sk-agent-revoked"))
	r := gin.New()
	r.DELETE("/installations/:id", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
		h.RevokeInstallation(c)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/installations/"+installationID.String(), nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"revoked":true`)
	require.Equal(t, []string{"sk-agent-revoked"}, invalidator.keys)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRevokeCurrentInstallationDisablesCallingAgentCredential(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	invalidator := &authInvalidatorSpy{}
	h.authInvalidator = invalidator
	mock.ExpectQuery("WITH revoked AS").
		WithArgs(int64(81), int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"key"}).AddRow("sk-agent-current"))
	r := gin.New()
	r.DELETE("/installation", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 81, UserID: 42})
		h.RevokeCurrentInstallation(c)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/installation", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"revoked":true`)
	require.Equal(t, []string{"sk-agent-current"}, invalidator.keys)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEstimateImageGenerationPersistsAuthoritativeQuote(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	h.billingService = service.NewBillingService(&config.Config{}, nil)
	groupID := int64(9)
	price := 0.3
	group := &service.Group{
		ID:             groupID,
		Kind:           "agent",
		SystemCode:     "yingzo",
		RateMultiplier: 2,
		ImagePrice2K:   &price,
		UpdatedAt:      time.Unix(1783900000, 0),
	}
	mock.ExpectExec("INSERT INTO agent_generation_quotes").
		WithArgs(
			sqlmock.AnyArg(), int64(81), groupID, "image", "gpt-image-2", sha256HexArgument{},
			sqlmock.AnyArg(), "image", float64(2), 2, 0.3, 0.6, 1.2, "credit",
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	r := gin.New()
	r.POST("/estimate", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 81, UserID: 42, GroupID: &groupID, Group: group})
		h.EstimateGeneration(c)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/estimate", strings.NewReader(`{
		"kind":"image","model":"gpt-image-2","count":2,
		"request":{"model":"gpt-image-2","size":"2048x1152","prompt":"not persisted"}
	}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"quote_id":"quote_`)
	require.Contains(t, w.Body.String(), `"actual_price":1.2`)
	require.NotContains(t, w.Body.String(), "not persisted")
	require.NoError(t, mock.ExpectationsWereMet())
}

func multipartRequest(t *testing.T, filename, contentType string, data []byte) (*http.Request, int64) {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	header.Set("Content-Type", contentType)
	part, err := w.CreatePart(header)
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	req := httptest.NewRequest(http.MethodPost, "/assets", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req, int64(len(data))
}

func uploadRouter(h *AgentHandler) *gin.Engine {
	r := gin.New()
	r.POST("/assets", func(c *gin.Context) {
		c.Request.Header.Set("X-Forwarded-Proto", "https")
		groupID := int64(3)
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 2, UserID: 1, GroupID: &groupID})
		h.UploadTemporaryAsset(c)
	})
	return r
}

func expectTemporaryAssetInsert(mock sqlmock.Sqlmock, backend string, size int64) {
	mock.ExpectQuery("SELECT COUNT\\(\\*\\),COALESCE\\(SUM\\(size_bytes\\),0\\)").
		WithArgs(int64(2), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count", "bytes"}).AddRow(int64(0), int64(0)))
	mock.ExpectExec("INSERT INTO temporary_assets").
		WithArgs(sqlmock.AnyArg(), int64(1), int64(2), sqlmock.AnyArg(), sha256HexArgument{}, backend, sqlmock.AnyArg(), "pixel.png", "image", "image/png", size, sha256HexArgument{}, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func onePixelPNG(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	require.NoError(t, err)
	return data
}

func TestTemporaryAssetUploadServeRangeHeadAndExpiry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	png := onePixelPNG(t)
	expectTemporaryAssetInsert(mock, "local", int64(len(png)))
	req, _ := multipartRequest(t, "pixel.png", "image/png", png)
	w := httptest.NewRecorder()
	uploadRouter(h).ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var response struct {
		URL string `json:"url"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	parts := strings.Split(strings.TrimRight(response.URL, "/"), "/")
	require.GreaterOrEqual(t, len(parts), 2)
	assetID, err := uuid.Parse(parts[len(parts)-2])
	require.NoError(t, err)
	require.Equal(t, "asset.png", parts[len(parts)-1])
	require.NotContains(t, response.URL, "?")
	files, err := filepath.Glob(filepath.Join(h.dataDir, "*", "object"))
	require.NoError(t, err)
	require.Len(t, files, 1)

	serve := func(method string, expired bool) *httptest.ResponseRecorder {
		expires := time.Now().Add(time.Hour)
		if expired {
			expires = time.Now().Add(-time.Hour)
		}
		mock.ExpectQuery("SELECT storage_backend,storage_key,original_filename,mime_type,size_bytes,expires_at").
			WithArgs(assetID).
			WillReturnRows(sqlmock.NewRows([]string{"storage_backend", "storage_key", "original_filename", "mime_type", "size_bytes", "expires_at"}).
				AddRow("local", files[0], "pixel.png", "image/png", len(png), expires))
		if !expired {
			mock.ExpectExec("UPDATE temporary_assets SET last_accessed_at").
				WithArgs(assetID).
				WillReturnResult(sqlmock.NewResult(0, 1))
		}
		r := gin.New()
		r.Handle(method, "/media/:id/:filename", h.ServeCleanTemporaryAsset)
		out := httptest.NewRecorder()
		request := httptest.NewRequest(method, "/media/"+assetID.String()+"/asset.png", nil)
		if method == http.MethodGet && !expired {
			request.Header.Set("Range", "bytes=1-3")
		}
		r.ServeHTTP(out, request)
		return out
	}

	rangeResponse := serve(http.MethodGet, false)
	require.Equal(t, http.StatusPartialContent, rangeResponse.Code)
	require.Equal(t, png[1:4], rangeResponse.Body.Bytes())
	headResponse := serve(http.MethodHead, false)
	require.Equal(t, http.StatusOK, headResponse.Code)
	require.Empty(t, headResponse.Body.Bytes())
	require.Equal(t, strconv.Itoa(len(png)), headResponse.Header().Get("Content-Length"))
	expiredResponse := serve(http.MethodGet, true)
	require.Equal(t, http.StatusNotFound, expiredResponse.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTemporaryAssetUploadRejectsSpoofedActiveContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	req, _ := multipartRequest(t, "attack.png", "image/png", []byte("<html><script>alert(1)</script></html>"))
	w := httptest.NewRecorder()
	uploadRouter(h).ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Equal(t, "unsupported_media", responseCode(w))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTemporaryAssetUploadRejectsSVGActiveContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		filename    string
		contentType string
	}{
		{name: "declared svg", filename: "attack.svg", contentType: "image/svg+xml"},
		{name: "svg disguised as png", filename: "attack.png", contentType: "image/png"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h, mock := newAgentHandlerMock(t)
			req, _ := multipartRequest(t, test.filename, test.contentType, []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))
			w := httptest.NewRecorder()
			uploadRouter(h).ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
			require.Equal(t, "unsupported_media", responseCode(w))
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestTemporaryAssetUploadDoesNotUseFilenameAsStoragePath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	png := onePixelPNG(t)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\),COALESCE\\(SUM\\(size_bytes\\),0\\)").
		WithArgs(int64(2), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count", "bytes"}).AddRow(int64(0), int64(0)))
	mock.ExpectExec("INSERT INTO temporary_assets").
		WithArgs(sqlmock.AnyArg(), int64(1), int64(2), sqlmock.AnyArg(), sha256HexArgument{}, "local", sqlmock.AnyArg(), "escape.png", "image", "image/png", int64(len(png)), sha256HexArgument{}, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	req, _ := multipartRequest(t, "../../../../escape.png", "image/png", png)
	w := httptest.NewRecorder()
	uploadRouter(h).ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	files, err := filepath.Glob(filepath.Join(h.dataDir, "*", "object"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	_, err = os.Stat(filepath.Join(filepath.Dir(h.dataDir), "escape.png"))
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTemporaryAssetUnknownTokenReturnsOpaqueNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	const guessedToken = "guessed-public-token"
	mock.ExpectQuery("SELECT storage_backend,storage_key,original_filename,mime_type,size_bytes,expires_at").
		WithArgs(hashToken(guessedToken)).
		WillReturnError(sql.ErrNoRows)
	r := gin.New()
	r.GET("/temporary-assets/:token", h.ServeTemporaryAsset)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/temporary-assets/"+guessedToken, nil))
	require.Equal(t, http.StatusNotFound, w.Code)
	require.Empty(t, w.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTemporaryAssetUploadEnforcesRollingQuota(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\),COALESCE\\(SUM\\(size_bytes\\),0\\)").
		WithArgs(int64(2), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count", "bytes"}).AddRow(int64(100), int64(1024)))
	png := onePixelPNG(t)
	req, _ := multipartRequest(t, "pixel.png", "image/png", png)
	w := httptest.NewRecorder()
	uploadRouter(h).ServeHTTP(w, req)
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.Equal(t, "temporary_asset_quota_exceeded", responseCode(w))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTemporaryAssetS3UploadAndRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	store := newMemoryObjectStore()
	h.objectStore = store
	png := onePixelPNG(t)
	expectTemporaryAssetInsert(mock, "s3", int64(len(png)))
	req, _ := multipartRequest(t, "pixel.png", "image/png", png)
	w := httptest.NewRecorder()
	uploadRouter(h).ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	require.Len(t, store.objects, 1)
	var storageKey string
	for key := range store.objects {
		storageKey = key
	}
	require.True(t, strings.HasPrefix(storageKey, "yingzo-agent-assets/"))

	var response struct {
		URL string `json:"url"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	parts := strings.Split(strings.TrimRight(response.URL, "/"), "/")
	assetID, err := uuid.Parse(parts[len(parts)-2])
	require.NoError(t, err)
	require.Equal(t, "asset.png", parts[len(parts)-1])
	mock.ExpectQuery("SELECT storage_backend,storage_key,original_filename,mime_type,size_bytes,expires_at").
		WithArgs(assetID).
		WillReturnRows(sqlmock.NewRows([]string{"storage_backend", "storage_key", "original_filename", "mime_type", "size_bytes", "expires_at"}).
			AddRow("s3", storageKey, "pixel.png", "image/png", len(png), time.Now().Add(time.Hour)))
	mock.ExpectExec("UPDATE temporary_assets SET last_accessed_at").
		WithArgs(assetID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	r := gin.New()
	r.GET("/media/:id/:filename", h.ServeCleanTemporaryAsset)
	get := httptest.NewRecorder()
	getRequest := httptest.NewRequest(http.MethodGet, "/media/"+assetID.String()+"/asset.png", nil)
	getRequest.Header.Set("Range", "bytes=4-7")
	r.ServeHTTP(get, getRequest)
	require.Equal(t, http.StatusPartialContent, get.Code)
	require.Equal(t, png[4:8], get.Body.Bytes())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCleanupExpiredClaimsRowsAndDeletesBothBackends(t *testing.T) {
	h, mock := newAgentHandlerMock(t)
	store := newMemoryObjectStore()
	h.objectStore = store
	store.objects["yingzo-agent-assets/expired"] = []byte("data")
	localDir := t.TempDir()
	localFile := filepath.Join(localDir, "object")
	require.NoError(t, os.WriteFile(localFile, []byte("data"), 0600))
	localID, s3ID := uuid.New(), uuid.New()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id,storage_backend,storage_key FROM temporary_assets").
		WillReturnRows(sqlmock.NewRows([]string{"id", "storage_backend", "storage_key"}).
			AddRow(localID, "local", localFile).
			AddRow(s3ID, "s3", "yingzo-agent-assets/expired"))
	mock.ExpectExec("UPDATE temporary_assets SET deleted_at").WithArgs(localID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE temporary_assets SET deleted_at").WithArgs(s3ID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectExec("DELETE FROM agent_generation_quotes").WillReturnResult(sqlmock.NewResult(0, 0))

	n, err := h.CleanupExpired(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(2), n)
	_, err = os.Stat(localDir)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.Equal(t, []string{"yingzo-agent-assets/expired"}, store.deleted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTemporaryAssetEnforcesCredentialOwnership(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	id := uuid.New()
	now := time.Now().UTC()
	mock.ExpectQuery("SELECT original_filename,media_type,mime_type,size_bytes,sha256,metadata,created_at,expires_at,deleted_at").
		WithArgs(id, int64(1), int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"original_filename", "media_type", "mime_type", "size_bytes", "sha256", "metadata", "created_at", "expires_at", "deleted_at"}).
			AddRow("pixel.png", "image", "image/png", 8, strings.Repeat("a", 64), []byte(`{"width":1,"height":1,"probe":"go-image"}`), now, now.Add(time.Hour), nil))

	r := gin.New()
	r.GET("/assets/:id", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 2, UserID: 1})
		h.GetTemporaryAsset(c)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/assets/"+id.String(), nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"active":true`)
	require.Contains(t, w.Body.String(), `"probe":"go-image"`)
	require.NotContains(t, w.Body.String(), "public_token")
	require.NotContains(t, w.Body.String(), "storage_key")
	require.NoError(t, mock.ExpectationsWereMet())
}
