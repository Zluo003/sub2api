package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

var yingzoUploadSessionTestColumns = []string{
	"id", "client_upload_id", "release_id", "created_by", "artifact_kind", "target", "os", "arch",
	"format", "content_type", "runtime_protocol", "package_filename", "total_bytes", "received_bytes",
	"expected_sha256", "temp_storage_key", "last_chunk_offset", "last_chunk_size", "last_chunk_sha256",
	"status", "completed_artifact_id", "expires_at", "created_at", "updated_at",
}

func yingzoUploadSessionRows(session *yingzoUploadSession) *sqlmock.Rows {
	return sqlmock.NewRows(yingzoUploadSessionTestColumns).AddRow(
		session.ID, session.ClientUploadID, session.ReleaseID, session.CreatedBy,
		session.ArtifactKind, session.Target, session.OS, session.Arch, session.Format,
		session.ContentType, session.RuntimeProtocol, session.PackageFilename,
		session.TotalBytes, session.ReceivedBytes, session.ExpectedSHA256,
		session.TempStorageKey, session.LastChunkOffset, session.LastChunkSize,
		session.LastChunkSHA256, session.Status, func() string {
			if session.CompletedArtifactID == uuid.Nil {
				return ""
			}
			return session.CompletedArtifactID.String()
		}(), session.ExpiresAt,
		session.CreatedAt, session.UpdatedAt,
	)
}

func newYingzoChunkRouter(method, path string, handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
		c.Next()
	})
	switch method {
	case http.MethodPost:
		router.POST(path, handler)
	case http.MethodPut:
		router.PUT(path, handler)
	case http.MethodGet:
		router.GET(path, handler)
	case http.MethodDelete:
		router.DELETE(path, handler)
	}
	return router
}

func expectYingzoDraftLock(mock sqlmock.Sqlmock, releaseID uuid.UUID) {
	mock.ExpectQuery("SELECT version,status,distribution_schema_version,runtime_protocol FROM yingzo_releases").
		WithArgs(releaseID).
		WillReturnRows(sqlmock.NewRows([]string{"version", "status", "distribution_schema_version", "runtime_protocol"}).AddRow("0.3.0", "draft", 3, 1))
}

func TestInitYingzoChunkUploadHasNoLogicalPackageCap(t *testing.T) {
	h, mock := newAgentHandlerMock(t)
	releaseID := uuid.New()
	logicalSize := int64(1 << 40)
	body, err := json.Marshal(gin.H{
		"client_upload_id": "client-019f-upload", "artifact_kind": "host_package", "target": "openai",
		"os": "windows", "arch": "x64", "runtime_protocol": 1,
		"package_filename": "yingzo-openai-windows-x64-0.3.0.zip", "total_bytes": logicalSize,
	})
	require.NoError(t, err)

	mock.ExpectBegin()
	expectYingzoDraftLock(mock, releaseID)
	mock.ExpectQuery("SELECT .* FROM yingzo_release_upload_sessions WHERE release_id").
		WithArgs(releaseID, "openai", "windows", "x64").
		WillReturnRows(sqlmock.NewRows(yingzoUploadSessionTestColumns))
	mock.ExpectExec("INSERT INTO yingzo_release_upload_sessions").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	router := newYingzoChunkRouter(http.MethodPost, "/releases/:id/artifact-uploads", h.InitYingzoReleaseArtifactUpload)
	request := httptest.NewRequest(http.MethodPost, "/releases/"+releaseID.String()+"/artifact-uploads", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	var payload struct {
		UploadID   string `json:"upload_id"`
		TotalBytes int64  `json:"total_bytes"`
		ChunkSize  int64  `json:"chunk_size"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	require.NotEmpty(t, payload.UploadID)
	require.Equal(t, logicalSize, payload.TotalBytes)
	require.Equal(t, yingzoReleaseChunkBytes, payload.ChunkSize)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInitYingzoChunkUploadRejectsFilenamePath(t *testing.T) {
	h, mock := newAgentHandlerMock(t)
	releaseID := uuid.New()
	body := strings.NewReader(`{"client_upload_id":"client-019f-upload","artifact_kind":"host_package","target":"openai","os":"windows","arch":"x64","runtime_protocol":1,"package_filename":"../../yingzo-openai-windows-x64-0.3.0.zip","total_bytes":1}`)
	router := newYingzoChunkRouter(http.MethodPost, "/releases/:id/artifact-uploads", h.InitYingzoReleaseArtifactUpload)
	request := httptest.NewRequest(http.MethodPost, "/releases/"+releaseID.String()+"/artifact-uploads", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Equal(t, "invalid_package_filename", responseCode(response))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPutYingzoChunkPersistsBytesAndAcceptsIdempotentRetry(t *testing.T) {
	h, mock := newAgentHandlerMock(t)
	releaseID, uploadID := uuid.New(), uuid.New()
	partPath, err := h.yingzoUploadPartPath(releaseID, uploadID)
	require.NoError(t, err)
	now := time.Now().UTC()
	chunk := []byte("reliable-chunk")
	session := &yingzoUploadSession{
		ID: uploadID, ClientUploadID: "client-019f-upload", ReleaseID: releaseID, CreatedBy: 7,
		ArtifactKind: "host_package", Target: "openai", OS: "windows", Arch: "x64", Format: "zip",
		ContentType: "application/zip", RuntimeProtocol: 1, PackageFilename: "yingzo-openai-windows-x64-0.3.0.zip",
		TotalBytes: int64(len(chunk)), TempStorageKey: partPath, LastChunkOffset: -1, LastChunkSize: -1,
		Status: "active", ExpiresAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now,
	}
	// Simulate a crash after the file append but before the DB offset commit.
	// The retry must truncate these ahead bytes to the authoritative DB offset.
	require.NoError(t, os.MkdirAll(filepath.Dir(partPath), 0700))
	require.NoError(t, os.WriteFile(partPath, []byte("uncommitted-ahead-bytes"), 0600))

	mock.ExpectBegin()
	expectYingzoDraftLock(mock, releaseID)
	mock.ExpectQuery("SELECT .* FROM yingzo_release_upload_sessions WHERE id").
		WithArgs(uploadID, releaseID, int64(7)).WillReturnRows(yingzoUploadSessionRows(session))
	mock.ExpectExec("UPDATE yingzo_release_upload_sessions SET received_bytes").
		WithArgs(uploadID, int64(len(chunk)), int64(0), int64(len(chunk)), sha256Hex(chunk), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	router := newYingzoChunkRouter(http.MethodPut, "/releases/:id/artifact-uploads/:upload_id", h.PutYingzoReleaseArtifactUploadChunk)
	request := httptest.NewRequest(http.MethodPut, "/releases/"+releaseID.String()+"/artifact-uploads/"+uploadID.String(), bytes.NewReader(chunk))
	request.Header.Set("Upload-Offset", "0")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	stored, err := os.ReadFile(partPath)
	require.NoError(t, err)
	require.Equal(t, chunk, stored)

	session.ReceivedBytes = int64(len(chunk))
	session.LastChunkOffset = 0
	session.LastChunkSize = int64(len(chunk))
	session.LastChunkSHA256 = sha256Hex(chunk)
	mock.ExpectBegin()
	expectYingzoDraftLock(mock, releaseID)
	mock.ExpectQuery("SELECT .* FROM yingzo_release_upload_sessions WHERE id").
		WithArgs(uploadID, releaseID, int64(7)).WillReturnRows(yingzoUploadSessionRows(session))
	mock.ExpectExec("UPDATE yingzo_release_upload_sessions SET expires_at").
		WithArgs(uploadID, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	retry := httptest.NewRequest(http.MethodPut, "/releases/"+releaseID.String()+"/artifact-uploads/"+uploadID.String(), bytes.NewReader(chunk))
	retry.Header.Set("Upload-Offset", "0")
	retryResponse := httptest.NewRecorder()
	router.ServeHTTP(retryResponse, retry)
	require.Equal(t, http.StatusOK, retryResponse.Code, retryResponse.Body.String())
	require.Contains(t, retryResponse.Body.String(), `"replayed":true`)
	stored, err = os.ReadFile(partPath)
	require.NoError(t, err)
	require.Equal(t, chunk, stored)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPutYingzoChunkReturnsAuthoritativeOffsetConflict(t *testing.T) {
	h, mock := newAgentHandlerMock(t)
	releaseID, uploadID := uuid.New(), uuid.New()
	now := time.Now().UTC()
	session := &yingzoUploadSession{
		ID: uploadID, ClientUploadID: "client-019f-upload", ReleaseID: releaseID, CreatedBy: 7,
		ArtifactKind: "host_package", Target: "openai", OS: "windows", Arch: "x64", Format: "zip",
		ContentType: "application/zip", RuntimeProtocol: 1, PackageFilename: "yingzo-openai-windows-x64-0.3.0.zip",
		TotalBytes: 64, ReceivedBytes: 16, TempStorageKey: filepath.Join(t.TempDir(), "part"),
		LastChunkOffset: 8, LastChunkSize: 8, LastChunkSHA256: strings.Repeat("a", 64),
		Status: "active", ExpiresAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now,
	}
	mock.ExpectBegin()
	expectYingzoDraftLock(mock, releaseID)
	mock.ExpectQuery("SELECT .* FROM yingzo_release_upload_sessions WHERE id").
		WithArgs(uploadID, releaseID, int64(7)).WillReturnRows(yingzoUploadSessionRows(session))
	mock.ExpectRollback()

	router := newYingzoChunkRouter(http.MethodPut, "/releases/:id/artifact-uploads/:upload_id", h.PutYingzoReleaseArtifactUploadChunk)
	request := httptest.NewRequest(http.MethodPut, "/releases/"+releaseID.String()+"/artifact-uploads/"+uploadID.String(), strings.NewReader("different"))
	request.Header.Set("Upload-Offset", "8")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
	require.Equal(t, "upload_offset_conflict", responseCode(response))
	require.Contains(t, response.Body.String(), `"expected_offset":16`)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPutYingzoChunkLimitsOnlyTheRequestBody(t *testing.T) {
	h, mock := newAgentHandlerMock(t)
	releaseID, uploadID := uuid.New(), uuid.New()
	router := newYingzoChunkRouter(http.MethodPut, "/releases/:id/artifact-uploads/:upload_id", h.PutYingzoReleaseArtifactUploadChunk)
	request := httptest.NewRequest(http.MethodPut, "/releases/"+releaseID.String()+"/artifact-uploads/"+uploadID.String(), strings.NewReader("x"))
	request.Header.Set("Upload-Offset", "0")
	request.ContentLength = yingzoReleaseChunkBytes + 1
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
	require.Equal(t, "upload_chunk_too_large", responseCode(response))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestYingzoUploadTotalBytesRejectsNegativeAndConflictingIntegers(t *testing.T) {
	_, ok := yingzoUploadTotalBytes(yingzoUploadInitInput{TotalBytes: -1})
	require.False(t, ok)
	_, ok = yingzoUploadTotalBytes(yingzoUploadInitInput{TotalBytes: 10, SizeBytes: 11})
	require.False(t, ok)
	total, ok := yingzoUploadTotalBytes(yingzoUploadInitInput{TotalBytes: 1 << 62})
	require.True(t, ok)
	require.Equal(t, int64(1<<62), total)
}

func TestCompleteYingzoChunkUploadHashesAndCreatesArtifact(t *testing.T) {
	h, mock := newAgentHandlerMock(t)
	releaseID, uploadID := uuid.New(), uuid.New()
	partPath, err := h.yingzoUploadPartPath(releaseID, uploadID)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(partPath), 0700))
	contents := []byte("complete package bytes")
	require.NoError(t, os.WriteFile(partPath, contents, 0600))
	now := time.Now().UTC()
	session := &yingzoUploadSession{
		ID: uploadID, ClientUploadID: "client-019f-upload", ReleaseID: releaseID, CreatedBy: 7,
		ArtifactKind: "host_package", Target: "openai", OS: "windows", Arch: "x64", Format: "zip",
		ContentType: "application/zip", RuntimeProtocol: 1, PackageFilename: "yingzo-openai-windows-x64-0.3.0.zip",
		TotalBytes: int64(len(contents)), ReceivedBytes: int64(len(contents)), ExpectedSHA256: sha256Hex(contents),
		TempStorageKey: partPath, LastChunkOffset: 0, LastChunkSize: int64(len(contents)), LastChunkSHA256: sha256Hex(contents),
		Status: "active", ExpiresAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now,
	}

	mock.ExpectBegin()
	expectYingzoDraftLock(mock, releaseID)
	mock.ExpectQuery("SELECT .* FROM yingzo_release_upload_sessions WHERE id").
		WithArgs(uploadID, releaseID, int64(7)).WillReturnRows(yingzoUploadSessionRows(session))
	mock.ExpectExec("UPDATE yingzo_release_upload_sessions SET status='finalizing'").
		WithArgs(uploadID, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	finalizing := *session
	finalizing.Status = "finalizing"
	mock.ExpectBegin()
	expectYingzoDraftLock(mock, releaseID)
	mock.ExpectQuery("SELECT .* FROM yingzo_release_upload_sessions WHERE id").
		WithArgs(uploadID, releaseID, int64(7)).WillReturnRows(yingzoUploadSessionRows(&finalizing))
	mock.ExpectQuery("SELECT id,storage_key FROM yingzo_release_artifacts").
		WithArgs(releaseID, "openai", "windows", "x64").WillReturnRows(sqlmock.NewRows([]string{"id", "storage_key"}))
	mock.ExpectExec("UPDATE yingzo_releases SET signature=NULL").WithArgs(releaseID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE yingzo_release_artifacts SET signature_status='unverified'").WithArgs(releaseID).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO yingzo_release_artifacts").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE yingzo_release_upload_sessions SET status='completed'").
		WithArgs(uploadID, sqlmock.AnyArg(), sqlmock.AnyArg(), releaseID, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	router := newYingzoChunkRouter(http.MethodPost, "/releases/:id/artifact-uploads/:upload_id/complete", h.CompleteYingzoReleaseArtifactUpload)
	request := httptest.NewRequest(http.MethodPost, "/releases/"+releaseID.String()+"/artifact-uploads/"+uploadID.String()+"/complete", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), sha256Hex(contents))
	var completedArtifact yingzoReleaseArtifact
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &completedArtifact))
	require.NotEqual(t, uuid.Nil, completedArtifact.ID)
	_, err = os.Stat(partPath)
	require.True(t, os.IsNotExist(err))
	finalFiles, err := filepath.Glob(filepath.Join(filepath.Dir(filepath.Dir(partPath)), "*.zip"))
	require.NoError(t, err)
	require.Len(t, finalFiles, 1)
	stored, err := os.ReadFile(finalFiles[0])
	require.NoError(t, err)
	require.Equal(t, contents, stored)

	completedSession := finalizing
	completedSession.Status = "completed"
	completedSession.CompletedArtifactID = completedArtifact.ID
	completedSession.ExpiresAt = now.Add(time.Hour)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT version,status,distribution_schema_version,runtime_protocol FROM yingzo_releases").
		WithArgs(releaseID).
		WillReturnRows(sqlmock.NewRows([]string{"version", "status", "distribution_schema_version", "runtime_protocol"}).AddRow("0.3.0", "published", 3, 1))
	mock.ExpectQuery("SELECT .* FROM yingzo_release_upload_sessions WHERE id").
		WithArgs(uploadID, releaseID, int64(7)).WillReturnRows(yingzoUploadSessionRows(&completedSession))
	mock.ExpectRollback()
	mock.ExpectQuery("SELECT .* FROM yingzo_releases WHERE id").WithArgs(releaseID).
		WillReturnRows(v3YingzoReleaseRowsForUpload(releaseID, "published", "prerelease", now))
	mock.ExpectQuery("SELECT .* FROM yingzo_release_artifacts WHERE release_id").WithArgs(releaseID).
		WillReturnRows(sqlmock.NewRows(strings.Split(yingzoArtifactColumns, ",")).AddRow(
			completedArtifact.ID, releaseID, nil, session.ArtifactKind, session.Target, session.OS, session.Arch,
			session.Format, session.ContentType, session.RuntimeProtocol, "validated", "unverified", now,
			session.PackageFilename, "local", finalFiles[0], session.TotalBytes, sha256Hex(contents), now, now,
		))
	replay := httptest.NewRequest(http.MethodPost, "/releases/"+releaseID.String()+"/artifact-uploads/"+uploadID.String()+"/complete", nil)
	replayResponse := httptest.NewRecorder()
	router.ServeHTTP(replayResponse, replay)
	require.Equal(t, http.StatusOK, replayResponse.Code, replayResponse.Body.String())
	require.Contains(t, replayResponse.Body.String(), completedArtifact.ID.String())

	mock.ExpectQuery("SELECT .* FROM yingzo_release_upload_sessions WHERE id").
		WithArgs(uploadID, releaseID, int64(7)).WillReturnRows(yingzoUploadSessionRows(&completedSession))
	mock.ExpectQuery("SELECT .* FROM yingzo_releases WHERE id").WithArgs(releaseID).
		WillReturnRows(v3YingzoReleaseRowsForUpload(releaseID, "published", "prerelease", now))
	mock.ExpectQuery("SELECT .* FROM yingzo_release_artifacts WHERE release_id").WithArgs(releaseID).
		WillReturnRows(sqlmock.NewRows(strings.Split(yingzoArtifactColumns, ",")).AddRow(
			completedArtifact.ID, releaseID, nil, session.ArtifactKind, session.Target, session.OS, session.Arch,
			session.Format, session.ContentType, session.RuntimeProtocol, "validated", "unverified", now,
			session.PackageFilename, "local", finalFiles[0], session.TotalBytes, sha256Hex(contents), now, now,
		))
	statusRouter := newYingzoChunkRouter(http.MethodGet, "/releases/:id/artifact-uploads/:upload_id", h.GetYingzoReleaseArtifactUpload)
	statusRequest := httptest.NewRequest(http.MethodGet, "/releases/"+releaseID.String()+"/artifact-uploads/"+uploadID.String(), nil)
	statusResponse := httptest.NewRecorder()
	statusRouter.ServeHTTP(statusResponse, statusRequest)
	require.Equal(t, http.StatusOK, statusResponse.Code, statusResponse.Body.String())
	require.Contains(t, statusResponse.Body.String(), `"artifact":`)
	require.Contains(t, statusResponse.Body.String(), completedArtifact.ID.String())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCleanupExpiredYingzoChunkUploadRemovesPartAndSession(t *testing.T) {
	h, mock := newAgentHandlerMock(t)
	releaseID, uploadID := uuid.New(), uuid.New()
	partPath, err := h.yingzoUploadPartPath(releaseID, uploadID)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(partPath), 0700))
	require.NoError(t, os.WriteFile(partPath, []byte("stale"), 0600))
	now := time.Now().UTC()
	session := &yingzoUploadSession{
		ID: uploadID, ClientUploadID: "client-019f-upload", ReleaseID: releaseID, CreatedBy: 7,
		ArtifactKind: "host_package", Target: "openai", OS: "windows", Arch: "x64", Format: "zip",
		ContentType: "application/zip", RuntimeProtocol: 1, PackageFilename: "yingzo-openai-windows-x64-0.3.0.zip",
		TotalBytes: 10, ReceivedBytes: 5, TempStorageKey: partPath, LastChunkOffset: 0, LastChunkSize: 5,
		LastChunkSHA256: sha256Hex([]byte("stale")), Status: "active", ExpiresAt: now.Add(-time.Minute), CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour),
	}
	mock.ExpectQuery("SELECT id FROM yingzo_release_upload_sessions WHERE expires_at").WithArgs(now).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uploadID))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM yingzo_release_upload_sessions WHERE id").WithArgs(uploadID).
		WillReturnRows(yingzoUploadSessionRows(session))
	mock.ExpectExec("UPDATE yingzo_release_upload_sessions SET status='expired'").WithArgs(uploadID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectExec("DELETE FROM yingzo_release_upload_sessions WHERE id").WithArgs(uploadID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	removed, err := h.CleanupExpiredYingzoArtifactUploads(context.Background(), now)
	require.NoError(t, err)
	require.Equal(t, 1, removed)
	_, err = os.Stat(partPath)
	require.True(t, os.IsNotExist(err))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCompleteYingzoChunkUploadAtomicallyReplacesSlot(t *testing.T) {
	h, mock := newAgentHandlerMock(t)
	releaseID, uploadID, existingArtifactID := uuid.New(), uuid.New(), uuid.New()
	partPath, err := h.yingzoUploadPartPath(releaseID, uploadID)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(partPath), 0700))
	contents := []byte("replacement package")
	require.NoError(t, os.WriteFile(partPath, contents, 0600))
	oldStorage := filepath.Join(filepath.Dir(filepath.Dir(partPath)), "old.zip")
	require.NoError(t, os.WriteFile(oldStorage, []byte("old"), 0600))
	now := time.Now().UTC()
	session := &yingzoUploadSession{
		ID: uploadID, ClientUploadID: "client-replacement", ReleaseID: releaseID, CreatedBy: 7,
		ArtifactKind: "host_package", Target: "openai", OS: "windows", Arch: "x64", Format: "zip",
		ContentType: "application/zip", RuntimeProtocol: 1, PackageFilename: "yingzo-openai-windows-x64-0.3.0.zip",
		TotalBytes: int64(len(contents)), ReceivedBytes: int64(len(contents)), ExpectedSHA256: sha256Hex(contents),
		TempStorageKey: partPath, LastChunkOffset: 0, LastChunkSize: int64(len(contents)), LastChunkSHA256: sha256Hex(contents),
		Status: "active", ExpiresAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now,
	}
	mock.ExpectBegin()
	expectYingzoDraftLock(mock, releaseID)
	mock.ExpectQuery("SELECT .* FROM yingzo_release_upload_sessions WHERE id").
		WithArgs(uploadID, releaseID, int64(7)).WillReturnRows(yingzoUploadSessionRows(session))
	mock.ExpectExec("UPDATE yingzo_release_upload_sessions SET status='finalizing'").
		WithArgs(uploadID, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	finalizing := *session
	finalizing.Status = "finalizing"
	mock.ExpectBegin()
	expectYingzoDraftLock(mock, releaseID)
	mock.ExpectQuery("SELECT .* FROM yingzo_release_upload_sessions WHERE id").
		WithArgs(uploadID, releaseID, int64(7)).WillReturnRows(yingzoUploadSessionRows(&finalizing))
	mock.ExpectQuery("SELECT id,storage_key FROM yingzo_release_artifacts").
		WithArgs(releaseID, "openai", "windows", "x64").
		WillReturnRows(sqlmock.NewRows([]string{"id", "storage_key"}).AddRow(existingArtifactID, oldStorage))
	mock.ExpectExec("UPDATE yingzo_releases SET signature=NULL").WithArgs(releaseID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE yingzo_release_artifacts SET signature_status='unverified'").WithArgs(releaseID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE yingzo_release_artifacts SET artifact_kind").
		WithArgs(existingArtifactID, session.ArtifactKind, session.PackageFilename, sqlmock.AnyArg(), session.TotalBytes, sha256Hex(contents), session.ContentType, session.Format, session.RuntimeProtocol, releaseID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE yingzo_release_upload_sessions SET status='completed'").
		WithArgs(uploadID, existingArtifactID, sqlmock.AnyArg(), releaseID, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	router := newYingzoChunkRouter(http.MethodPost, "/releases/:id/artifact-uploads/:upload_id/complete", h.CompleteYingzoReleaseArtifactUpload)
	request := httptest.NewRequest(http.MethodPost, "/releases/"+releaseID.String()+"/artifact-uploads/"+uploadID.String()+"/complete", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), existingArtifactID.String())
	_, err = os.Stat(oldStorage)
	require.True(t, os.IsNotExist(err), "old slot bytes must be removed only after replacement commits")
	require.NoError(t, mock.ExpectationsWereMet())
}
