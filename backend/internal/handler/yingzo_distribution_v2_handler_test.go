package handler

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"mime/multipart"
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

func TestYingzoV2ArtifactMatrixIsExact(t *testing.T) {
	specs := yingzoV2ArtifactSpecs("0.3.0")
	require.Len(t, specs, 8)
	filenames := make([]string, 0, len(specs))
	keys := map[string]bool{}
	for _, spec := range specs {
		filenames = append(filenames, spec.Filename)
		require.False(t, keys[spec.key()])
		keys[spec.key()] = true
	}
	require.ElementsMatch(t, []string{
		"yingzo-openai-macos-0.3.0.tar.gz",
		"yingzo-openai-windows-x64-0.3.0.zip",
		"yingzo-claude-code-macos-0.3.0.zip",
		"yingzo-claude-code-windows-x64-0.3.0.zip",
		"yingzo-claude-desktop-0.3.0.mcpb",
		"yingzo-runtime-macos-arm64-0.3.0.dmg",
		"yingzo-runtime-macos-x64-0.3.0.dmg",
		"yingzo-runtime-windows-x64-0.3.0-setup.exe",
	}, filenames)
	_, ok := findYingzoV2Spec("0.3.0", "runtime_installer", "runtime", "windows", "arm64")
	require.False(t, ok)
}

func TestCreateYingzoV2DraftDoesNotRequireArtifacts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	mock.ExpectExec("INSERT INTO yingzo_releases").
		WithArgs(sqlmock.AnyArg(), "0.3.0", "prerelease", 1, sqlmock.AnyArg(), "0.128.0", "0.0.0", "preview", int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT .* FROM yingzo_releases WHERE id=\\$1").WillReturnError(context.Canceled)

	router := gin.New()
	router.POST("/releases", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
		h.UploadYingzoRelease(c)
	})
	request := httptest.NewRequest(http.MethodPost, "/releases", strings.NewReader(`{
			"version":"0.3.0","channel":"prerelease","distribution_schema_version":2,
			"runtime_protocol":1,"compatibility":{"project_schema":1},
			"signature":"frontend-must-not-be-trusted",
			"min_codex_version":"0.128.0","min_claude_version":"0.0.0","release_notes":"preview"
	}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"distribution_schema_version":2`)
	require.Contains(t, response.Body.String(), `"channel":"prerelease"`)
	require.NotContains(t, response.Body.String(), "frontend-must-not-be-trusted")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUploadYingzoV2ArtifactStoresValidatedLocalFile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	releaseID := uuid.New()
	now := time.Now().UTC()
	mock.ExpectBegin()
	expectV2Release(t, mock, releaseID, "draft", "prerelease", now, nil)
	mock.ExpectExec("UPDATE yingzo_releases SET signature=NULL").WithArgs(releaseID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE yingzo_release_artifacts SET signature_status='unverified'").WithArgs(releaseID).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO yingzo_release_artifacts").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	archive := filepath.Join(t.TempDir(), "yingzo-openai-macos-0.3.0.tar.gz")
	writeV2HostArchive(t, archive, mustYingzoV2Spec(t, "host_package", "openai", "macos", "any"))
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	require.NoError(t, writer.WriteField("artifact_kind", "host_package"))
	require.NoError(t, writer.WriteField("target", "openai"))
	require.NoError(t, writer.WriteField("os", "macos"))
	require.NoError(t, writer.WriteField("arch", "any"))
	require.NoError(t, writer.WriteField("runtime_protocol", "1"))
	require.NoError(t, writer.WriteField("signature_status", "verified"))
	writeYingzoMultipartFile(t, writer, "file", archive)
	require.NoError(t, writer.Close())

	router := gin.New()
	router.POST("/releases/:id/artifacts", h.UploadYingzoReleaseArtifact)
	request := httptest.NewRequest(http.MethodPost, "/releases/"+releaseID.String()+"/artifacts", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"signature_status":"unverified"`)
	releaseDir, err := h.yingzoReleaseDirectory(releaseID)
	require.NoError(t, err)
	entries, err := os.ReadDir(releaseDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.True(t, strings.HasSuffix(entries[0].Name(), ".tar.gz"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUploadYingzoV2ArtifactRechecksDraftStateUnderReleaseLock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	releaseID := uuid.New()
	now := time.Now().UTC()
	mock.ExpectBegin()
	expectV2Release(t, mock, releaseID, "published", "prerelease", now, nil)
	mock.ExpectRollback()

	archive := filepath.Join(t.TempDir(), "yingzo-openai-macos-0.3.0.tar.gz")
	writeSimpleTarGzip(t, archive)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for key, value := range map[string]string{
		"artifact_kind": "host_package", "target": "openai", "os": "macos", "arch": "any", "runtime_protocol": "1", "signature_status": "verified",
	} {
		require.NoError(t, writer.WriteField(key, value))
	}
	writeYingzoMultipartFile(t, writer, "file", archive)
	require.NoError(t, writer.Close())

	router := gin.New()
	router.POST("/releases/:id/artifacts", h.UploadYingzoReleaseArtifact)
	request := httptest.NewRequest(http.MethodPost, "/releases/"+releaseID.String()+"/artifacts", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
	require.Equal(t, "release_state_invalid", responseCode(response))
	releaseDir, err := h.yingzoReleaseDirectory(releaseID)
	require.NoError(t, err)
	_, err = os.Stat(releaseDir)
	require.True(t, os.IsNotExist(err), "a state race must not leave an uploaded file")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestYingzoV2InstallReturnsHostAndRuntimeTickets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	tickets := newMemoryYingzoTicketStore()
	h.yingzoTicketStore = tickets
	releaseID := uuid.New()
	now := time.Now().UTC()
	mock.ExpectQuery("SELECT .* FROM yingzo_releases WHERE status='published' AND channel=\\$1").
		WithArgs("prerelease").
		WillReturnRows(v2YingzoReleaseRows(releaseID, "published", "prerelease", now, now))
	mock.ExpectQuery("SELECT .* FROM yingzo_release_artifacts WHERE release_id=\\$1").
		WithArgs(releaseID).
		WillReturnRows(v2YingzoArtifactRows(releaseID, now, "0.3.0", "/tmp"))

	router := gin.New()
	router.POST("/install", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
		h.CreateYingzoInstallInstructions(c)
	})
	request := httptest.NewRequest(http.MethodPost, "https://api-key.cc/install", strings.NewReader(`{
			"host":"codex","os":"darwin","arch":"aarch64","channel":"prerelease","runtime_capability":"unknown"
		}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Equal(t, false, body["runtime_installer_required"])
	require.Equal(t, "macos", body["os"])
	require.Equal(t, "arm64", body["arch"])
	require.NotNil(t, body["host_package"])
	require.NotNil(t, body["runtime_installer"])
	require.Equal(t, "probe", body["runtime_resolution"])
	require.Contains(t, body["runtime_helper_uri"], "yingzo://runtime/ensure?version=0.3.0&protocol=1&installer_url=")
	require.Len(t, tickets.payloads, 2)
	seenKinds := map[string]bool{}
	for _, raw := range tickets.payloads {
		var ticket yingzoInstallTicket
		require.NoError(t, json.Unmarshal(raw, &ticket))
		require.Equal(t, int64(42), ticket.UserID)
		require.Equal(t, "prerelease", ticket.Channel)
		seenKinds[ticket.ArtifactKind] = true
	}
	require.True(t, seenKinds["host_package"])
	require.True(t, seenKinds["runtime_installer"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestYingzoDownloadTicketSupportsHeadRangeAndRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	tickets := newMemoryYingzoTicketStore()
	h.yingzoTicketStore = tickets
	releaseID := uuid.New()
	artifactID := uuid.New()
	now := time.Now().UTC()
	file := filepath.Join(t.TempDir(), "yingzo-openai-windows-x64-0.3.0.zip")
	require.NoError(t, os.WriteFile(file, []byte("0123456789"), 0600))
	ticketPayload, err := json.Marshal(yingzoInstallTicket{
		ReleaseID: releaseID, ArtifactID: artifactID, Host: "codex", HostFamily: "openai",
		Channel: "prerelease", ArtifactKind: "host_package", Target: "openai", OS: "windows", Arch: "x64", RequestedOS: "windows", RequestedArch: "x64",
	})
	require.NoError(t, err)
	tickets.payloads["ticket"] = ticketPayload

	for range 3 {
		mock.ExpectQuery("SELECT .* FROM yingzo_releases WHERE id=\\$1").WithArgs(releaseID).
			WillReturnRows(v2YingzoReleaseRows(releaseID, "published", "prerelease", now, now))
		rows := sqlmock.NewRows(strings.Split(yingzoArtifactColumns, ",")).AddRow(
			artifactID, releaseID, nil, "host_package", "openai", "windows", "x64", "zip", "application/zip", 1,
			"validated", "verified", now, filepath.Base(file), "local", file, int64(10), strings.Repeat("a", 64), now, now,
		)
		mock.ExpectQuery("SELECT .* FROM yingzo_release_artifacts WHERE release_id=\\$1").WithArgs(releaseID).WillReturnRows(rows)
	}

	router := gin.New()
	router.GET("/download/:ticket/:filename", h.DownloadYingzoRelease)
	router.HEAD("/download/:ticket/:filename", h.DownloadYingzoRelease)
	path := "/download/ticket/" + filepath.Base(file)

	head := httptest.NewRecorder()
	router.ServeHTTP(head, httptest.NewRequest(http.MethodHead, path, nil))
	require.Equal(t, http.StatusOK, head.Code)
	require.Empty(t, head.Body.String())
	require.Equal(t, "application/zip", head.Header().Get("Content-Type"))

	rangeResponse := httptest.NewRecorder()
	rangeRequest := httptest.NewRequest(http.MethodGet, path, nil)
	rangeRequest.Header.Set("Range", "bytes=2-5")
	router.ServeHTTP(rangeResponse, rangeRequest)
	require.Equal(t, http.StatusPartialContent, rangeResponse.Code)
	require.Equal(t, "2345", rangeResponse.Body.String())

	retry := httptest.NewRecorder()
	router.ServeHTTP(retry, httptest.NewRequest(http.MethodGet, path, nil))
	require.Equal(t, http.StatusOK, retry.Code)
	require.Equal(t, "0123456789", retry.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPublishYingzoV2RequiresAndAcceptsExactSignedMatrix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	releaseID := uuid.New()
	storage := t.TempDir()
	rows, proof := v2YingzoPublishRows(t, releaseID, storage, "verified")

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status,channel,distribution_schema_version,runtime_protocol,version,signature FROM yingzo_releases WHERE id=\\$1 FOR UPDATE").WithArgs(releaseID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "channel", "distribution_schema_version", "runtime_protocol", "version", "signature"}).AddRow("draft", "prerelease", 2, 1, "0.3.0", proof))
	mock.ExpectQuery("SELECT artifact_kind,target,os,arch,runtime_protocol,validation_status,signature_status,package_filename,storage_key,size_bytes,sha256 FROM yingzo_release_artifacts WHERE release_id=\\$1").WithArgs(releaseID).WillReturnRows(rows)
	mock.ExpectExec("UPDATE yingzo_releases SET status='superseded'").WithArgs("prerelease", releaseID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE yingzo_releases SET status='published'").WithArgs(releaseID, int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT .* FROM yingzo_releases WHERE id=\\$1").WithArgs(releaseID).WillReturnError(context.Canceled)

	router := gin.New()
	router.POST("/releases/:id/publish", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
		h.PublishYingzoRelease(c)
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/releases/"+releaseID.String()+"/publish", nil))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"channel":"prerelease"`)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPublishYingzoV2RejectsUnverifiedArtifact(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	releaseID := uuid.New()
	storage := t.TempDir()
	rows, _ := v2YingzoPublishRows(t, releaseID, storage, "unverified")
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status,channel,distribution_schema_version,runtime_protocol,version,signature FROM yingzo_releases WHERE id=\\$1 FOR UPDATE").WithArgs(releaseID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "channel", "distribution_schema_version", "runtime_protocol", "version", "signature"}).AddRow("draft", "prerelease", 2, 1, "0.3.0", ""))
	mock.ExpectQuery("SELECT artifact_kind,target,os,arch,runtime_protocol,validation_status,signature_status,package_filename,storage_key,size_bytes,sha256 FROM yingzo_release_artifacts WHERE release_id=\\$1").WithArgs(releaseID).
		WillReturnRows(rows)
	mock.ExpectRollback()

	router := gin.New()
	router.POST("/releases/:id/publish", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
		h.PublishYingzoRelease(c)
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/releases/"+releaseID.String()+"/publish", nil))
	require.Equal(t, http.StatusConflict, response.Code)
	require.Equal(t, "release_artifacts_invalid", responseCode(response))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPublishStableYingzoV2RejectsPrereleaseProof(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	releaseID := uuid.New()
	storage := t.TempDir()
	rows, proof := v2YingzoPublishRows(t, releaseID, storage, "verified")
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status,channel,distribution_schema_version,runtime_protocol,version,signature FROM yingzo_releases WHERE id=\\$1 FOR UPDATE").WithArgs(releaseID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "channel", "distribution_schema_version", "runtime_protocol", "version", "signature"}).AddRow("draft", "stable", 2, 1, "0.3.0", proof))
	mock.ExpectQuery("SELECT artifact_kind,target,os,arch,runtime_protocol,validation_status,signature_status,package_filename,storage_key,size_bytes,sha256 FROM yingzo_release_artifacts WHERE release_id=\\$1").WithArgs(releaseID).
		WillReturnRows(rows)
	mock.ExpectRollback()

	router := gin.New()
	router.POST("/releases/:id/publish", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
		h.PublishYingzoRelease(c)
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/releases/"+releaseID.String()+"/publish", nil))
	require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Body.String())
	require.Equal(t, "release_manifest_mismatch", responseCode(response))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPromoteYingzoV2RevalidatesArtifactsAndMovesSameReleaseToStable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	releaseID := uuid.New()
	storage := t.TempDir()
	rows, proof := v2YingzoPublishRows(t, releaseID, storage, "verified")
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status,channel,distribution_schema_version,runtime_protocol,version,signature FROM yingzo_releases WHERE id=\\$1 FOR UPDATE").WithArgs(releaseID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "channel", "distribution_schema_version", "runtime_protocol", "version", "signature"}).AddRow("published", "prerelease", 2, 1, "0.3.0", proof))
	mock.ExpectQuery("SELECT artifact_kind,target,os,arch,runtime_protocol,validation_status,signature_status,package_filename,storage_key,size_bytes,sha256 FROM yingzo_release_artifacts WHERE release_id=\\$1").WithArgs(releaseID).
		WillReturnRows(rows)
	mock.ExpectExec("UPDATE yingzo_releases SET status='superseded'").WithArgs(releaseID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE yingzo_releases SET channel='stable'").WithArgs(releaseID, int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT .* FROM yingzo_releases WHERE id=\\$1").WithArgs(releaseID).WillReturnError(context.Canceled)

	router := gin.New()
	router.POST("/releases/:id/promote", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
		h.PromoteYingzoRelease(c)
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/releases/"+releaseID.String()+"/promote", nil))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"channel":"stable"`)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRollbackPromotedYingzoV2AcceptsOriginalPrereleaseProof(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	releaseID := uuid.New()
	storage := t.TempDir()
	rows, proof := v2YingzoPublishRows(t, releaseID, storage, "verified")
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status,channel,distribution_schema_version,runtime_protocol,version,signature FROM yingzo_releases WHERE id=\\$1 FOR UPDATE").WithArgs(releaseID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "channel", "distribution_schema_version", "runtime_protocol", "version", "signature"}).AddRow("superseded", "stable", 2, 1, "0.3.0", proof))
	mock.ExpectQuery("SELECT artifact_kind,target,os,arch,runtime_protocol,validation_status,signature_status,package_filename,storage_key,size_bytes,sha256 FROM yingzo_release_artifacts WHERE release_id=\\$1").WithArgs(releaseID).
		WillReturnRows(rows)
	mock.ExpectExec("UPDATE yingzo_releases SET status='superseded'").WithArgs("stable", releaseID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE yingzo_releases SET status='published'").WithArgs(releaseID, int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT .* FROM yingzo_releases WHERE id=\\$1").WithArgs(releaseID).WillReturnError(context.Canceled)

	router := gin.New()
	router.POST("/releases/:id/rollback", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
		h.RollbackYingzoRelease(c)
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/releases/"+releaseID.String()+"/rollback", nil))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"channel":"stable"`)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteYingzoV2ArtifactCommitsBeforeRemovingFile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	releaseID := uuid.New()
	artifactID := uuid.New()
	now := time.Now().UTC()
	releaseDir, err := h.yingzoReleaseDirectory(releaseID)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(releaseDir, 0700))
	file := filepath.Join(releaseDir, artifactID.String()+".tar.gz")
	require.NoError(t, os.WriteFile(file, []byte("archive"), 0600))
	artifactRows := sqlmock.NewRows(strings.Split(yingzoArtifactColumns, ",")).AddRow(
		artifactID, releaseID, nil, "host_package", "openai", "macos", "any", "tar.gz", "application/gzip", 1,
		"validated", "verified", now, "yingzo-openai-macos-0.3.0.tar.gz", "local", file, int64(7), strings.Repeat("a", 64), now, now,
	)
	mock.ExpectBegin()
	expectV2Release(t, mock, releaseID, "draft", "prerelease", now, artifactRows)
	mock.ExpectExec("DELETE FROM yingzo_release_artifacts").WithArgs(artifactID, releaseID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE yingzo_releases SET signature=NULL").WithArgs(releaseID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE yingzo_release_artifacts SET signature_status='unverified'").WithArgs(releaseID).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	router := gin.New()
	router.DELETE("/releases/:id/artifacts/:artifact_id", h.DeleteYingzoReleaseArtifact)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/releases/"+releaseID.String()+"/artifacts/"+artifactID.String(), nil))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	_, err = os.Stat(file)
	require.True(t, os.IsNotExist(err))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestYingzoV2BinaryValidationAndPersistentVolume(t *testing.T) {
	t.Run("zip", func(t *testing.T) {
		spec := mustYingzoV2Spec(t, "host_package", "openai", "windows", "x64")
		path := filepath.Join(t.TempDir(), spec.Filename)
		writeV2ArtifactFixture(t, path, spec)
		require.NoError(t, validateYingzoV2Artifact(path, spec))
	})
	t.Run("exe", func(t *testing.T) {
		spec := mustYingzoV2Spec(t, "runtime_installer", "runtime", "windows", "x64")
		path := filepath.Join(t.TempDir(), spec.Filename)
		writeV2ArtifactFixture(t, path, spec)
		require.NoError(t, validateYingzoV2Artifact(path, spec))
	})
	t.Run("dmg", func(t *testing.T) {
		spec := mustYingzoV2Spec(t, "runtime_installer", "runtime", "macos", "arm64")
		path := filepath.Join(t.TempDir(), spec.Filename)
		writeV2ArtifactFixture(t, path, spec)
		require.NoError(t, validateYingzoV2Artifact(path, spec))
	})
	t.Run("portable archive traversal is rejected", func(t *testing.T) {
		require.ErrorContains(t, validateArchiveEntry(`..\escape.exe`, 10), "unsafe path")
		require.ErrorContains(t, validateArchiveEntry(`C:\escape.exe`, 10), "unsafe path")
	})
	t.Run("configured volume must be absolute", func(t *testing.T) {
		t.Setenv(yingzoReleaseStorageEnv, "relative/releases")
		h := &AgentHandler{dataDir: t.TempDir()}
		_, err := h.yingzoReleaseStorageRoot()
		require.ErrorContains(t, err, "absolute")
	})
	t.Run("stale upload temporaries are removed without touching artifacts", func(t *testing.T) {
		t.Setenv(yingzoReleaseStorageEnv, t.TempDir())
		h := &AgentHandler{dataDir: t.TempDir()}
		releaseID := uuid.New()
		dir, err := h.yingzoReleaseDirectory(releaseID)
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(dir, 0700))
		stale := filepath.Join(dir, "stale.tmp")
		artifact := filepath.Join(dir, "artifact.zip")
		require.NoError(t, os.WriteFile(stale, []byte("partial"), 0600))
		require.NoError(t, os.WriteFile(artifact, []byte("keep"), 0600))
		old := time.Now().Add(-48 * time.Hour)
		require.NoError(t, os.Chtimes(stale, old, old))
		removed, err := h.CleanupYingzoReleaseTemporaryFiles(time.Now().Add(-24 * time.Hour))
		require.NoError(t, err)
		require.Equal(t, 1, removed)
		_, err = os.Stat(stale)
		require.True(t, os.IsNotExist(err))
		_, err = os.Stat(artifact)
		require.NoError(t, err)
	})
	t.Run("abandoned draft artifacts are deleted atomically before file removal", func(t *testing.T) {
		h, mock := newAgentHandlerMock(t)
		releaseID := uuid.New()
		releaseDir, err := h.yingzoReleaseDirectory(releaseID)
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(releaseDir, 0700))
		artifactPath := filepath.Join(releaseDir, "artifact.zip")
		require.NoError(t, os.WriteFile(artifactPath, []byte("artifact"), 0600))
		olderThan := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
		updatedAt := olderThan.Add(-time.Hour)
		mock.ExpectQuery("SELECT id FROM yingzo_releases WHERE status IN").
			WithArgs(olderThan).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(releaseID.String()))
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT status,updated_at FROM yingzo_releases WHERE id=\\$1 FOR UPDATE").
			WithArgs(releaseID).WillReturnRows(sqlmock.NewRows([]string{"status", "updated_at"}).AddRow("draft", updatedAt))
		mock.ExpectQuery("DELETE FROM yingzo_release_artifacts WHERE release_id=\\$1 RETURNING storage_key").
			WithArgs(releaseID).WillReturnRows(sqlmock.NewRows([]string{"storage_key"}).AddRow(artifactPath))
		mock.ExpectExec("UPDATE yingzo_releases SET signature=NULL").WithArgs(releaseID).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		removed, err := h.CleanupYingzoAbandonedDraftArtifacts(context.Background(), olderThan)
		require.NoError(t, err)
		require.Equal(t, 1, removed)
		_, err = os.Stat(artifactPath)
		require.True(t, os.IsNotExist(err))
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestYingzoV2HostArchivesRequireTargetSpecificLayout(t *testing.T) {
	tests := []struct {
		name string
		spec yingzoArtifactSpec
	}{
		{name: "openai macos", spec: mustYingzoV2Spec(t, "host_package", "openai", "macos", "any")},
		{name: "openai windows", spec: mustYingzoV2Spec(t, "host_package", "openai", "windows", "x64")},
		{name: "claude code macos", spec: mustYingzoV2Spec(t, "host_package", "claude-code", "macos", "any")},
		{name: "claude code windows", spec: mustYingzoV2Spec(t, "host_package", "claude-code", "windows", "x64")},
		{name: "claude desktop mcpb", spec: mustYingzoV2Spec(t, "host_package", "claude-desktop", "any", "any")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			archive := filepath.Join(t.TempDir(), test.spec.Filename)
			writeV2HostArchive(t, archive, test.spec)
			require.NoError(t, validateYingzoV2Artifact(archive, test.spec))
		})
	}

	t.Run("wrong host archive", func(t *testing.T) {
		archive := filepath.Join(t.TempDir(), "wrong.tar.gz")
		writeTarGzipEntries(t, archive, map[string]string{
			"yingzo/.claude-plugin/plugin.json": "{}", "yingzo/.mcp.json": "{}",
			"yingzo/ui-manifest.json": "{}", "yingzo/runtime/yingzo-mcp": "launcher",
		})
		err := validateYingzoV2Artifact(archive, yingzoArtifactSpec{Target: "openai", OS: "macos", Format: "tar.gz"})
		require.ErrorContains(t, err, "required openai layout")
	})

	t.Run("required files cannot be split across roots", func(t *testing.T) {
		archive := filepath.Join(t.TempDir(), "split.zip")
		writeZipEntries(t, archive, map[string]string{
			"one/.codex-plugin/plugin.json": "{}", "two/.mcp.json": "{}",
			"two/ui-manifest.json": "{}", "two/runtime/yingzo-mcp.exe": "launcher",
		})
		err := validateYingzoV2Artifact(archive, yingzoArtifactSpec{Target: "openai", OS: "windows", Format: "zip"})
		require.ErrorContains(t, err, "required openai layout")
	})
}

func TestDisableYingzoDraftRemovesArtifactsButPreservesPublishedFiles(t *testing.T) {
	t.Run("draft files are removed after commit", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		h, mock := newAgentHandlerMock(t)
		releaseID := uuid.New()
		releaseDir, err := h.yingzoReleaseDirectory(releaseID)
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(releaseDir, 0700))
		file := filepath.Join(releaseDir, "artifact.zip")
		require.NoError(t, os.WriteFile(file, []byte("artifact"), 0600))

		mock.ExpectBegin()
		mock.ExpectQuery("SELECT status,distribution_schema_version FROM yingzo_releases WHERE id=\\$1 FOR UPDATE").WithArgs(releaseID).
			WillReturnRows(sqlmock.NewRows([]string{"status", "distribution_schema_version"}).AddRow("draft", 2))
		mock.ExpectQuery("DELETE FROM yingzo_release_artifacts WHERE release_id=\\$1 RETURNING storage_key").WithArgs(releaseID).
			WillReturnRows(sqlmock.NewRows([]string{"storage_key"}).AddRow(file))
		mock.ExpectExec("UPDATE yingzo_releases SET status='disabled'").WithArgs(releaseID).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		router := gin.New()
		router.DELETE("/releases/:id", h.DisableYingzoRelease)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/releases/"+releaseID.String(), nil))
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		_, err = os.Stat(file)
		require.True(t, os.IsNotExist(err))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("published files remain available for rollback", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		h, mock := newAgentHandlerMock(t)
		releaseID := uuid.New()
		releaseDir, err := h.yingzoReleaseDirectory(releaseID)
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(releaseDir, 0700))
		file := filepath.Join(releaseDir, "artifact.zip")
		require.NoError(t, os.WriteFile(file, []byte("artifact"), 0600))

		mock.ExpectBegin()
		mock.ExpectQuery("SELECT status,distribution_schema_version FROM yingzo_releases WHERE id=\\$1 FOR UPDATE").WithArgs(releaseID).
			WillReturnRows(sqlmock.NewRows([]string{"status", "distribution_schema_version"}).AddRow("published", 2))
		mock.ExpectExec("UPDATE yingzo_releases SET status='disabled'").WithArgs(releaseID).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		router := gin.New()
		router.DELETE("/releases/:id", h.DisableYingzoRelease)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/releases/"+releaseID.String(), nil))
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		_, err = os.Stat(file)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestResolveYingzoRuntimeCapability(t *testing.T) {
	release := &yingzoRelease{Version: "0.3.0", RuntimeProtocol: 1}
	for _, tc := range []struct {
		name, capability, version string
		protocol                  int
		want                      string
	}{
		{name: "unknown probes", capability: "unknown", want: "probe"},
		{name: "missing requires installer", capability: "missing", want: "required"},
		{name: "incompatible requires installer", capability: "incompatible", want: "required"},
		{name: "exact runtime is compatible", capability: "compatible", version: "0.3.0", protocol: 1, want: "compatible"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveYingzoRuntimeCapability(yingzoInstallRequest{RuntimeCapability: tc.capability, InstalledRuntimeVersion: tc.version, InstalledRuntimeProtocol: tc.protocol}, release)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
	_, err := resolveYingzoRuntimeCapability(yingzoInstallRequest{RuntimeCapability: "compatible", InstalledRuntimeVersion: "0.2.4", InstalledRuntimeProtocol: 0}, release)
	require.Error(t, err)
}

func TestVerifyYingzoReleaseProofBindsExactManifestBytes(t *testing.T) {
	t.Run("valid proof is accepted", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		h, mock := newAgentHandlerMock(t)
		releaseID := uuid.New()
		rows, proof := v2YingzoProofRows(t, releaseID, t.TempDir(), "verified")
		now := time.Now().UTC()
		mock.ExpectBegin()
		expectV2Release(t, mock, releaseID, "draft", "prerelease", now, rows)
		mock.ExpectExec("UPDATE yingzo_releases SET signature").WithArgs(releaseID, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("UPDATE yingzo_release_artifacts SET signature_status='verified'").WithArgs(releaseID).WillReturnResult(sqlmock.NewResult(0, 8))
		mock.ExpectCommit()

		router := gin.New()
		router.PUT("/releases/:id/proof", h.VerifyYingzoReleaseProof)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/releases/"+releaseID.String()+"/proof", strings.NewReader(proof)))
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		require.Contains(t, response.Body.String(), `"verified":true`)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("tampered manifest is rejected before writes", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		h, mock := newAgentHandlerMock(t)
		releaseID := uuid.New()
		rows, proof := v2YingzoProofRows(t, releaseID, t.TempDir(), "verified")
		var envelope yingzoReleaseProofEnvelope
		require.NoError(t, json.Unmarshal([]byte(proof), &envelope))
		envelope.ManifestBase64 = base64.StdEncoding.EncodeToString([]byte(`{"tampered":true}`))
		tampered, err := json.Marshal(envelope)
		require.NoError(t, err)
		now := time.Now().UTC()
		mock.ExpectBegin()
		expectV2Release(t, mock, releaseID, "draft", "prerelease", now, rows)
		mock.ExpectRollback()

		router := gin.New()
		router.PUT("/releases/:id/proof", h.VerifyYingzoReleaseProof)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/releases/"+releaseID.String()+"/proof", bytes.NewReader(tampered)))
		require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Body.String())
		require.Equal(t, "release_proof_invalid", responseCode(response))
		require.NoError(t, mock.ExpectationsWereMet())
	})
	t.Run("non-Ed25519 algorithms are rejected", func(t *testing.T) {
		_, err := verifyYingzoReleaseProof(nil, yingzoReleaseProofInput{Algorithm: "RSA-PSS"})
		require.ErrorContains(t, err, "algorithm must be Ed25519")
	})
	t.Run("manifest channel is bound to the release", func(t *testing.T) {
		err := validateYingzoSignedManifest(
			&yingzoRelease{Version: "0.3.0", Channel: "stable", RuntimeProtocol: 1},
			yingzoSignedReleaseManifest{SchemaVersion: 2, Product: "yingzo", Version: "0.3.0", Channel: "prerelease", RuntimeProtocol: 1, CompleteArtifactMatrix: true, PublicSigningRequired: true},
		)
		require.ErrorContains(t, err, "channel does not match")
	})
}

func expectV2Release(t *testing.T, mock sqlmock.Sqlmock, releaseID uuid.UUID, status, channel string, now time.Time, artifacts *sqlmock.Rows) {
	t.Helper()
	mock.ExpectQuery("SELECT .* FROM yingzo_releases WHERE id=\\$1").WithArgs(releaseID).
		WillReturnRows(v2YingzoReleaseRows(releaseID, status, channel, now, nil))
	if artifacts == nil {
		artifacts = sqlmock.NewRows(strings.Split(yingzoArtifactColumns, ","))
	}
	mock.ExpectQuery("SELECT .* FROM yingzo_release_artifacts WHERE release_id=\\$1").WithArgs(releaseID).WillReturnRows(artifacts)
}

func v2YingzoReleaseRows(id uuid.UUID, status, channel string, now time.Time, publishedAt any) *sqlmock.Rows {
	return sqlmock.NewRows(strings.Split(yingzoReleaseColumns, ",")).
		AddRow(id, "0.3.0", status, 2, channel, 1, []byte(`{"project_schema":1}`), nil, "0.128.0", "0.0.0", "preview", now, publishedAt, now)
}

func v2YingzoArtifactRows(releaseID uuid.UUID, now time.Time, version, storageRoot string) *sqlmock.Rows {
	rows := sqlmock.NewRows(strings.Split(yingzoArtifactColumns, ","))
	for _, spec := range yingzoV2ArtifactSpecs(version) {
		rows.AddRow(uuid.New(), releaseID, nil, spec.ArtifactKind, spec.Target, spec.OS, spec.Arch, spec.Format, spec.ContentType, 1,
			"validated", "verified", now, spec.Filename, "local", filepath.Join(storageRoot, spec.Filename), int64(100), strings.Repeat("a", 64), now, now)
	}
	return rows
}

func v2YingzoPublishRows(t *testing.T, releaseID uuid.UUID, storageRoot, signatureStatus string) (*sqlmock.Rows, string) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	publicKeys, err := json.Marshal(map[string]string{"test-release": base64.StdEncoding.EncodeToString(publicKey)})
	require.NoError(t, err)
	t.Setenv(yingzoReleasePublicKeysEnv, string(publicKeys))
	rows := sqlmock.NewRows([]string{"artifact_kind", "target", "os", "arch", "runtime_protocol", "validation_status", "signature_status", "package_filename", "storage_key", "size_bytes", "sha256"})
	manifestArtifacts := make([]yingzoSignedManifestArtifact, 0, len(yingzoV2ArtifactSpecs("0.3.0")))
	for _, spec := range yingzoV2ArtifactSpecs("0.3.0") {
		storageKey := filepath.Join(storageRoot, spec.Filename)
		writeV2ArtifactFixture(t, storageKey, spec)
		sha, err := yingzoFileSHA256(storageKey)
		require.NoError(t, err)
		info, statErr := os.Stat(storageKey)
		require.NoError(t, statErr)
		rows.AddRow(spec.ArtifactKind, spec.Target, spec.OS, spec.Arch, 1, "validated", signatureStatus, spec.Filename, storageKey, info.Size(), sha)
		manifestArtifacts = append(manifestArtifacts, yingzoSignedManifestArtifact{
			Filename: spec.Filename, ArtifactKind: spec.ArtifactKind, Target: spec.Target, OS: spec.OS, Arch: spec.Arch,
			Format: spec.Format, ContentType: spec.ContentType, Bytes: info.Size(), SHA256: sha, RuntimeProtocol: 1,
		})
	}
	manifestBytes, err := json.Marshal(yingzoSignedReleaseManifest{
		SchemaVersion: 2, Product: "yingzo", Version: "0.3.0", Channel: "prerelease", RuntimeProtocol: 1,
		CompleteArtifactMatrix: true, PublicSigningRequired: true, Artifacts: manifestArtifacts,
	})
	require.NoError(t, err)
	manifestBytes = append(manifestBytes, '\n')
	signature := ed25519.Sign(privateKey, manifestBytes)
	envelopeBytes, err := json.Marshal(yingzoReleaseProofEnvelope{
		Algorithm: "Ed25519", KeyID: "test-release", ManifestBase64: base64.StdEncoding.EncodeToString(manifestBytes),
		SignatureBase64: base64.StdEncoding.EncodeToString(signature), VerifiedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
	require.NoError(t, err)
	return rows, string(envelopeBytes)
}

// The proof endpoint loads the canonical persisted artifact row rather than
// the narrow publish projection above. Keep the two query contracts explicit
// so a fixture cannot accidentally hide a scan or schema regression.
func v2YingzoProofRows(t *testing.T, releaseID uuid.UUID, storageRoot, signatureStatus string) (*sqlmock.Rows, string) {
	t.Helper()
	_, proof := v2YingzoPublishRows(t, releaseID, storageRoot, signatureStatus)
	now := time.Now().UTC()
	rows := sqlmock.NewRows(strings.Split(yingzoArtifactColumns, ","))
	for _, spec := range yingzoV2ArtifactSpecs("0.3.0") {
		storageKey := filepath.Join(storageRoot, spec.Filename)
		sha, err := yingzoFileSHA256(storageKey)
		require.NoError(t, err)
		info, err := os.Stat(storageKey)
		require.NoError(t, err)
		rows.AddRow(uuid.New(), releaseID, nil, spec.ArtifactKind, spec.Target, spec.OS, spec.Arch, spec.Format, spec.ContentType, 1,
			"validated", signatureStatus, now, spec.Filename, "local", storageKey, info.Size(), sha, now, now)
	}
	return rows, proof
}

func mustYingzoV2Spec(t *testing.T, kind, target, operatingSystem, arch string) yingzoArtifactSpec {
	t.Helper()
	spec, ok := findYingzoV2Spec("0.3.0", kind, target, operatingSystem, arch)
	require.True(t, ok)
	spec.RuntimeProtocol = 1
	return spec
}

func writeV2ArtifactFixture(t *testing.T, target string, spec yingzoArtifactSpec) {
	t.Helper()
	switch spec.Format {
	case "tar.gz":
		writeV2HostArchive(t, target, spec)
	case "zip", "mcpb":
		writeV2HostArchive(t, target, spec)
	case "exe":
		writeMinimalAMD64PE(t, target)
	case "dmg":
		data := make([]byte, 1024)
		copy(data[len(data)-512:], []byte("koly"))
		require.NoError(t, os.WriteFile(target, data, 0600))
	default:
		t.Fatalf("unsupported fixture format %s", spec.Format)
	}
}

func writeV2HostArchive(t *testing.T, target string, spec yingzoArtifactSpec) {
	t.Helper()
	version := "0.3.0"
	entries := map[string]string{}
	if spec.Target == "claude-desktop" {
		root := "outer/"
		entries[root+"manifest.json"] = `{"name":"yingzo","version":"` + version + `","server":{"entry_point":"server/yingzo-mcp","mcp_config":{"command":"${__dirname}/server/yingzo-mcp","platform_overrides":{"win32":{"command":"${__dirname}/server/yingzo-mcp.exe"}}}}}`
		entries[root+"ui-manifest.json"] = `{"schema_version":1,"product_version":"` + version + `"}`
		entries[root+"server/yingzo-mcp"] = "#!/bin/sh\n"
		entries[root+"server/yingzo-mcp.exe"] = string(minimalAMD64PEBytes())
	} else {
		root := "outer/marketplace/plugins/yingzo/"
		entries[root+"ui-manifest.json"] = `{"schema_version":1,"product_version":"` + version + `"}`
		entries[root+"runtime/runtime-helper.json"] = `{"schema_version":1,"product_version":"` + version + `","runtime_protocol_version":1}`
		entries[root+".mcp.json"] = `{"mcpServers":{"yingzo":{"command":"${__dirname}/runtime/` + map[bool]string{true: "yingzo-mcp.exe", false: "yingzo-mcp"}[spec.OS == "windows"] + `"}}}`
		if spec.Target == "openai" {
			entries["outer/marketplace/.agents/plugins/marketplace.json"] = `{"name":"yingzo-private","plugins":[{"name":"yingzo"}]}`
			entries[root+".codex-plugin/plugin.json"] = `{"name":"yingzo","version":"` + version + `"}`
		} else {
			entries["outer/marketplace/.claude-plugin/marketplace.json"] = `{"name":"yingzo-private","plugins":[{"name":"yingzo"}]}`
			entries[root+".claude-plugin/plugin.json"] = `{"name":"yingzo","version":"` + version + `"}`
		}
		launcher := root + "runtime/yingzo-mcp"
		if spec.OS == "windows" {
			launcher += ".exe"
			entries[launcher] = string(minimalAMD64PEBytes())
		} else {
			entries[launcher] = "#!/bin/sh\n"
		}
	}
	if spec.Format == "tar.gz" {
		writeTarGzipEntries(t, target, entries)
	} else {
		writeZipEntries(t, target, entries)
	}
}

func writeSimpleTarGzip(t *testing.T, target string) {
	t.Helper()
	writeTarGzipEntries(t, target, map[string]string{
		"yingzo/.codex-plugin/plugin.json": "{}",
		"yingzo/.mcp.json":                 "{}",
		"yingzo/ui-manifest.json":          "{}",
		"yingzo/runtime/yingzo-mcp":        "launcher",
	})
}

func writeTarGzipEntries(t *testing.T, target string, entries map[string]string) {
	t.Helper()
	file, err := os.Create(target)
	require.NoError(t, err)
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, content := range entries {
		payload := []byte(content)
		mode := int64(0600)
		if strings.HasSuffix(name, "/yingzo-mcp") && !strings.HasSuffix(name, ".exe") {
			mode = 0755
		}
		require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(payload)), Typeflag: tar.TypeReg}))
		_, err = tarWriter.Write(payload)
		require.NoError(t, err)
	}
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())
	require.NoError(t, file.Close())
}

func writeSimpleZip(t *testing.T, target string) {
	t.Helper()
	writeZipEntries(t, target, map[string]string{"yingzo/plugin.json": "payload"})
}

func writeZipEntries(t *testing.T, target string, entries map[string]string) {
	t.Helper()
	file, err := os.Create(target)
	require.NoError(t, err)
	writer := zip.NewWriter(file)
	for name, content := range entries {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(0600)
		if strings.HasSuffix(name, "/yingzo-mcp") && !strings.HasSuffix(name, ".exe") {
			header.SetMode(0755)
		}
		entry, createErr := writer.CreateHeader(header)
		require.NoError(t, createErr)
		_, writeErr := entry.Write([]byte(content))
		require.NoError(t, writeErr)
	}
	require.NoError(t, writer.Close())
	require.NoError(t, file.Close())
}

func writeMinimalAMD64PE(t *testing.T, target string) {
	t.Helper()
	data := minimalAMD64PEBytes()
	require.NoError(t, os.WriteFile(target, data, 0600))
}

func minimalAMD64PEBytes() []byte {
	data := make([]byte, 0x4000)
	copy(data, []byte("MZ"))
	binary.LittleEndian.PutUint32(data[0x3c:0x40], 0x80)
	copy(data[0x80:0x84], []byte{'P', 'E', 0, 0})
	binary.LittleEndian.PutUint16(data[0x84:0x86], 0x8664)
	binary.LittleEndian.PutUint16(data[0x94:0x96], 0xf0)
	optional := 0x98
	binary.LittleEndian.PutUint16(data[optional:optional+2], 0x20b)
	binary.LittleEndian.PutUint32(data[optional+108:optional+112], 16)
	// IMAGE_DIRECTORY_ENTRY_SECURITY is a file-offset based certificate table.
	binary.LittleEndian.PutUint32(data[optional+112+4*8:optional+112+4*8+4], 0x3000)
	binary.LittleEndian.PutUint32(data[optional+112+4*8+4:optional+112+4*8+8], 8)
	return data
}
