package handler

import (
	"bytes"
	"context"
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

func TestYingzoV3ArtifactMatrixRemovesMacOSIntelRuntime(t *testing.T) {
	specs := yingzoV3ArtifactSpecs("0.3.0")
	require.Len(t, specs, 7)
	filenames := make([]string, 0, len(specs))
	for _, spec := range specs {
		filenames = append(filenames, spec.Filename)
	}
	require.ElementsMatch(t, []string{
		"yingzo-openai-macos-0.3.0.tar.gz",
		"yingzo-openai-windows-x64-0.3.0.zip",
		"yingzo-claude-code-macos-0.3.0.zip",
		"yingzo-claude-code-windows-x64-0.3.0.zip",
		"yingzo-claude-desktop-0.3.0.mcpb",
		"yingzo-runtime-macos-arm64-0.3.0.dmg",
		"yingzo-runtime-windows-x64-0.3.0-setup.exe",
	}, filenames)
	_, ok := findYingzoArtifactSpec(3, "0.3.0", "runtime_installer", "runtime", "macos", "x64")
	require.False(t, ok)
	require.True(t, yingzoPlatformSupportedForSchema(3, "macos", "arm64"))
	require.False(t, yingzoPlatformSupportedForSchema(3, "macos", "x64"))
	require.True(t, yingzoPlatformSupportedForSchema(2, "macos", "x64"))
}

func TestYingzoSignedManifestSchemaDefaultsToV2AndRequiresV3Marker(t *testing.T) {
	releaseV2 := &yingzoRelease{Version: "0.3.0", DistributionSchemaVersion: 2, Channel: "prerelease", RuntimeProtocol: 1}
	manifest := yingzoSignedReleaseManifest{
		SchemaVersion: 2, Product: "yingzo", Version: "0.3.0", Channel: "prerelease", RuntimeProtocol: 1,
		CompleteArtifactMatrix: true, NativeSigning: yingzoNativeSigning{
			MacOS: yingzoNativeSigningPlatform{Status: "verified"}, Windows: yingzoNativeSigningPlatform{Status: "unsigned"},
		},
	}
	// The schema-2 manifest format predates the explicit distribution marker.
	// Its omitted marker must remain valid for already-issued proofs.
	err := validateYingzoSignedManifest(releaseV2, manifest)
	var uploadErr *yingzoUploadError
	require.ErrorAs(t, err, &uploadErr)
	require.NotEqual(t, "release_manifest_mismatch", uploadErr.code, "the matrix check should be reached after the legacy marker default")

	schema3 := 3
	releaseV3 := &yingzoRelease{Version: "0.3.0", DistributionSchemaVersion: 3, Channel: "prerelease", RuntimeProtocol: 1}
	manifest.DistributionSchemaVersion = &schema3
	err = validateYingzoSignedManifest(releaseV3, manifest)
	require.ErrorAs(t, err, &uploadErr)
	require.NotEqual(t, "release_manifest_mismatch", uploadErr.code, "the matrix check should be reached for an explicitly marked schema 3 proof")
	manifest.DistributionSchemaVersion = nil
	err = validateYingzoSignedManifest(releaseV3, manifest)
	require.ErrorAs(t, err, &uploadErr)
	require.Equal(t, "release_manifest_mismatch", uploadErr.code)
}

func TestCreateYingzoV2DraftIsRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)

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

	require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"code":"invalid_release_draft"`)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateYingzoV3DraftPersistsRequestedDistributionSchema(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	mock.ExpectExec("INSERT INTO yingzo_releases").
		WithArgs(sqlmock.AnyArg(), "0.3.0", 3, "prerelease", true, 1, sqlmock.AnyArg(), "0.128.0", "0.0.0", "schema 3", int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT .* FROM yingzo_releases WHERE id=\\$1").WillReturnError(context.Canceled)

	router := gin.New()
	router.POST("/releases", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
		h.UploadYingzoRelease(c)
	})
	request := httptest.NewRequest(http.MethodPost, "/releases", strings.NewReader(`{
			"version":"0.3.0","channel":"prerelease","distribution_schema_version":3,
			"runtime_protocol":1,"compatibility":{"project_schema":1},
			"min_codex_version":"0.128.0","min_claude_version":"0.0.0","release_notes":"schema 3"
	}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"distribution_schema_version":3`)
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

	archive := writeOpaqueArtifact(t, "yingzo-openai-macos-0.3.0.tar.gz")
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

func TestUploadWindowsArtifactStoresTheSelectedPackageWithoutSignatureInspection(t *testing.T) {
	for _, tc := range []struct {
		name       string
		channel    string
		wantStatus int
	}{
		{name: "prerelease", channel: "prerelease", wantStatus: http.StatusCreated},
		{name: "stable", channel: "stable", wantStatus: http.StatusCreated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			h, mock := newAgentHandlerMock(t)
			releaseID := uuid.New()
			now := time.Now().UTC()
			mock.ExpectBegin()
			expectV2Release(t, mock, releaseID, "draft", tc.channel, now, nil)
			mock.ExpectExec("UPDATE yingzo_releases SET signature=NULL").WithArgs(releaseID).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("UPDATE yingzo_release_artifacts SET signature_status='unverified'").WithArgs(releaseID).WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectExec("INSERT INTO yingzo_release_artifacts").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()

			spec := mustYingzoV2Spec(t, "host_package", "openai", "windows", "x64")
			archive := writeOpaqueArtifact(t, spec.Filename)
			body := &bytes.Buffer{}
			writer := multipart.NewWriter(body)
			require.NoError(t, writer.WriteField("artifact_kind", spec.ArtifactKind))
			require.NoError(t, writer.WriteField("target", spec.Target))
			require.NoError(t, writer.WriteField("os", spec.OS))
			require.NoError(t, writer.WriteField("arch", spec.Arch))
			require.NoError(t, writer.WriteField("runtime_protocol", "1"))
			writeYingzoMultipartFile(t, writer, "file", archive)
			require.NoError(t, writer.Close())

			router := gin.New()
			router.POST("/releases/:id/artifacts", h.UploadYingzoReleaseArtifact)
			request := httptest.NewRequest(http.MethodPost, "/releases/"+releaseID.String()+"/artifacts", body)
			request.Header.Set("Content-Type", writer.FormDataContentType())
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			require.Equal(t, tc.wantStatus, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), `"signature_status":"unverified"`)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestUploadYingzoV2ArtifactRechecksDraftStateUnderReleaseLock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	releaseID := uuid.New()
	now := time.Now().UTC()
	mock.ExpectBegin()
	expectV2Release(t, mock, releaseID, "published", "prerelease", now, nil)
	mock.ExpectRollback()

	archive := writeOpaqueArtifact(t, "yingzo-openai-macos-0.3.0.tar.gz")
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

func TestStoreYingzoV2ArtifactRejectsWrongFilename(t *testing.T) {
	h, _ := newAgentHandlerMock(t)
	release := &yingzoRelease{ID: uuid.New(), RuntimeProtocol: 1}
	spec := mustYingzoV2Spec(t, "host_package", "openai", "macos", "any")

	_, err := h.storeYingzoV2Artifact(strings.NewReader("opaque build output"), "wrong-name.tar.gz", release, spec)
	var uploadErr *yingzoUploadError
	require.ErrorAs(t, err, &uploadErr)
	require.Equal(t, "invalid_package_filename", uploadErr.code)
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

func TestPublishYingzoV2AcceptsExactFileMatrix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	releaseID := uuid.New()
	storage := t.TempDir()
	rows := v2YingzoPublishRows(t, releaseID, storage, "unverified")

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status,channel,distribution_schema_version,stable_eligible,runtime_protocol,version,signature FROM yingzo_releases WHERE id=\\$1 FOR UPDATE").WithArgs(releaseID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "channel", "distribution_schema_version", "stable_eligible", "runtime_protocol", "version", "signature"}).AddRow("draft", "prerelease", 2, true, 1, "0.3.0", ""))
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

func TestYingzoV2PublishMatrixRejectsMissingOrModifiedStoredArtifact(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(t *testing.T, storage string)
		code   string
	}{
		{
			name: "missing file",
			mutate: func(t *testing.T, storage string) {
				require.NoError(t, os.Remove(filepath.Join(storage, yingzoV2ArtifactSpecs("0.3.0")[0].Filename)))
			},
			code: "release_artifact_file_missing",
		},
		{
			name: "modified file",
			mutate: func(t *testing.T, storage string) {
				file := filepath.Join(storage, yingzoV2ArtifactSpecs("0.3.0")[0].Filename)
				opened, err := os.OpenFile(file, os.O_WRONLY, 0600)
				require.NoError(t, err)
				_, err = opened.WriteAt([]byte("X"), 0)
				require.NoError(t, err)
				require.NoError(t, opened.Close())
			},
			code: "release_artifact_integrity_failed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, mock := newAgentHandlerMock(t)
			releaseID := uuid.New()
			storage := t.TempDir()
			rows := v2YingzoPublishRows(t, releaseID, storage, "unverified")
			tc.mutate(t, storage)
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT artifact_kind,target,os,arch,runtime_protocol,validation_status,signature_status,package_filename,storage_key,size_bytes,sha256 FROM yingzo_release_artifacts WHERE release_id=\\$1").WithArgs(releaseID).WillReturnRows(rows)
			tx, err := h.db.BeginTx(context.Background(), nil)
			require.NoError(t, err)
			defer func() { _ = tx.Rollback() }()

			err = validateYingzoV2PublishMatrix(context.Background(), tx, releaseID, "0.3.0", 1)
			var uploadErr *yingzoUploadError
			require.ErrorAs(t, err, &uploadErr)
			require.Equal(t, tc.code, uploadErr.code)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestPublishYingzoV2IgnoresSignatureStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	releaseID := uuid.New()
	rows := v2YingzoPublishRows(t, releaseID, t.TempDir(), "unverified")

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status,channel,distribution_schema_version,stable_eligible,runtime_protocol,version,signature FROM yingzo_releases WHERE id=\\$1 FOR UPDATE").WithArgs(releaseID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "channel", "distribution_schema_version", "stable_eligible", "runtime_protocol", "version", "signature"}).AddRow("draft", "prerelease", 2, true, 1, "0.3.0", ""))
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
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPublishYingzoV2DoesNotRequireSignatureProof(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	releaseID := uuid.New()
	storage := t.TempDir()
	rows := v2YingzoPublishRows(t, releaseID, storage, "unverified")
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status,channel,distribution_schema_version,stable_eligible,runtime_protocol,version,signature FROM yingzo_releases WHERE id=\\$1 FOR UPDATE").WithArgs(releaseID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "channel", "distribution_schema_version", "stable_eligible", "runtime_protocol", "version", "signature"}).AddRow("draft", "prerelease", 2, true, 1, "0.3.0", ""))
	mock.ExpectQuery("SELECT artifact_kind,target,os,arch,runtime_protocol,validation_status,signature_status,package_filename,storage_key,size_bytes,sha256 FROM yingzo_release_artifacts WHERE release_id=\\$1").WithArgs(releaseID).
		WillReturnRows(rows)
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
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPublishStableYingzoV2IgnoresOptionalReleaseProof(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	releaseID := uuid.New()
	storage := t.TempDir()
	rows := v2YingzoPublishRows(t, releaseID, storage, "unverified")
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status,channel,distribution_schema_version,stable_eligible,runtime_protocol,version,signature FROM yingzo_releases WHERE id=\\$1 FOR UPDATE").WithArgs(releaseID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "channel", "distribution_schema_version", "stable_eligible", "runtime_protocol", "version", "signature"}).AddRow("draft", "stable", 2, true, 1, "0.3.0", "legacy-proof-is-ignored"))
	mock.ExpectQuery("SELECT artifact_kind,target,os,arch,runtime_protocol,validation_status,signature_status,package_filename,storage_key,size_bytes,sha256 FROM yingzo_release_artifacts WHERE release_id=\\$1").WithArgs(releaseID).
		WillReturnRows(rows)
	mock.ExpectExec("UPDATE yingzo_releases SET status='superseded'").WithArgs("stable", releaseID).WillReturnResult(sqlmock.NewResult(0, 1))
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
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPromoteYingzoV2AllowsPreviouslyIneligiblePrerelease(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	releaseID := uuid.New()
	storage := t.TempDir()
	rows := v2YingzoPublishRows(t, releaseID, storage, "unverified")
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status,channel,distribution_schema_version,stable_eligible,runtime_protocol,version,signature FROM yingzo_releases WHERE id=\\$1 FOR UPDATE").WithArgs(releaseID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "channel", "distribution_schema_version", "stable_eligible", "runtime_protocol", "version", "signature"}).AddRow("published", "prerelease", 2, false, 1, "0.3.0", ""))
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

func TestRollbackPromotedYingzoV2AllowsPreviouslyIneligibleRelease(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	releaseID := uuid.New()
	storage := t.TempDir()
	rows := v2YingzoPublishRows(t, releaseID, storage, "unverified")
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status,channel,distribution_schema_version,stable_eligible,runtime_protocol,version,signature FROM yingzo_releases WHERE id=\\$1 FOR UPDATE").WithArgs(releaseID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "channel", "distribution_schema_version", "stable_eligible", "runtime_protocol", "version", "signature"}).AddRow("superseded", "stable", 2, false, 1, "0.3.0", ""))
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

func TestYingzoV2PersistentVolumeMaintenance(t *testing.T) {
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
	t.Run("orphaned release directories are reconciled after the row is gone", func(t *testing.T) {
		t.Setenv(yingzoReleaseStorageEnv, t.TempDir())
		h, mock := newAgentHandlerMock(t)
		releaseID := uuid.New()
		dir, err := h.yingzoReleaseDirectory(releaseID)
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(dir, 0700))
		orphan := filepath.Join(dir, "artifact.zip")
		require.NoError(t, os.WriteFile(orphan, []byte("orphan"), 0600))
		old := time.Now().Add(-48 * time.Hour)
		require.NoError(t, os.Chtimes(orphan, old, old))
		mock.ExpectQuery("SELECT EXISTS\\(SELECT 1 FROM yingzo_releases WHERE id=\\$1\\)").
			WithArgs(releaseID).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		removed, err := h.CleanupYingzoReleaseTemporaryFiles(time.Now().Add(-24 * time.Hour))
		require.NoError(t, err)
		require.Equal(t, 1, removed)
		_, err = os.Stat(dir)
		require.True(t, os.IsNotExist(err))
		require.NoError(t, mock.ExpectationsWereMet())
	})
	t.Run("abandoned disabled artifacts and row are deleted atomically", func(t *testing.T) {
		h, mock := newAgentHandlerMock(t)
		releaseID := uuid.New()
		releaseDir, err := h.yingzoReleaseDirectory(releaseID)
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(releaseDir, 0700))
		artifactPath := filepath.Join(releaseDir, "artifact.zip")
		require.NoError(t, os.WriteFile(artifactPath, []byte("artifact"), 0600))
		olderThan := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
		updatedAt := olderThan.Add(-time.Hour)
		mock.ExpectQuery("SELECT id FROM yingzo_releases WHERE status='disabled' AND published_at IS NULL").
			WithArgs(olderThan).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(releaseID.String()))
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT status,published_at,updated_at FROM yingzo_releases WHERE id=\\$1 FOR UPDATE").
			WithArgs(releaseID).WillReturnRows(sqlmock.NewRows([]string{"status", "published_at", "updated_at"}).AddRow("disabled", nil, updatedAt))
		mock.ExpectQuery("DELETE FROM yingzo_release_artifacts WHERE release_id=\\$1 RETURNING storage_key").
			WithArgs(releaseID).WillReturnRows(sqlmock.NewRows([]string{"storage_key"}).AddRow(artifactPath))
		mock.ExpectExec("DELETE FROM yingzo_releases WHERE id=\\$1").WithArgs(releaseID).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		removed, err := h.CleanupYingzoAbandonedDraftArtifacts(context.Background(), olderThan)
		require.NoError(t, err)
		require.Equal(t, 1, removed)
		_, err = os.Stat(artifactPath)
		require.True(t, os.IsNotExist(err))
		require.NoError(t, mock.ExpectationsWereMet())
	})
	t.Run("published disabled releases are excluded from abandoned draft cleanup", func(t *testing.T) {
		h, mock := newAgentHandlerMock(t)
		olderThan := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
		mock.ExpectQuery("SELECT id FROM yingzo_releases WHERE status='disabled' AND published_at IS NULL").
			WithArgs(olderThan).WillReturnRows(sqlmock.NewRows([]string{"id"}))

		removed, err := h.CleanupYingzoAbandonedDraftArtifacts(context.Background(), olderThan)
		require.NoError(t, err)
		require.Zero(t, removed)
		require.NoError(t, mock.ExpectationsWereMet())
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

func TestPurgeYingzoReleaseFreesOnlyNeverPublishedVersions(t *testing.T) {
	t.Run("draft row and files are permanently removed after commit", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		h, mock := newAgentHandlerMock(t)
		releaseID := uuid.New()
		releaseDir, err := h.yingzoReleaseDirectory(releaseID)
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(releaseDir, 0700))
		artifactPath := filepath.Join(releaseDir, "artifact.zip")
		require.NoError(t, os.WriteFile(artifactPath, []byte("artifact"), 0600))

		mock.ExpectBegin()
		mock.ExpectQuery("SELECT status,published_at FROM yingzo_releases WHERE id=\\$1 FOR UPDATE").WithArgs(releaseID).
			WillReturnRows(sqlmock.NewRows([]string{"status", "published_at"}).AddRow("draft", nil))
		mock.ExpectQuery("DELETE FROM yingzo_release_artifacts WHERE release_id=\\$1 RETURNING storage_key").WithArgs(releaseID).
			WillReturnRows(sqlmock.NewRows([]string{"storage_key"}).AddRow(artifactPath))
		mock.ExpectExec("DELETE FROM yingzo_releases WHERE id=\\$1").WithArgs(releaseID).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		router := gin.New()
		router.DELETE("/releases/:id/purge", h.PurgeYingzoRelease)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/releases/"+releaseID.String()+"/purge", nil))

		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		require.Contains(t, response.Body.String(), `"deleted":true`)
		_, err = os.Stat(releaseDir)
		require.True(t, os.IsNotExist(err))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("unpublished disabled row can be purged", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		h, mock := newAgentHandlerMock(t)
		releaseID := uuid.New()

		mock.ExpectBegin()
		mock.ExpectQuery("SELECT status,published_at FROM yingzo_releases WHERE id=\\$1 FOR UPDATE").WithArgs(releaseID).
			WillReturnRows(sqlmock.NewRows([]string{"status", "published_at"}).AddRow("disabled", nil))
		mock.ExpectQuery("DELETE FROM yingzo_release_artifacts WHERE release_id=\\$1 RETURNING storage_key").WithArgs(releaseID).
			WillReturnRows(sqlmock.NewRows([]string{"storage_key"}))
		mock.ExpectExec("DELETE FROM yingzo_releases WHERE id=\\$1").WithArgs(releaseID).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		router := gin.New()
		router.DELETE("/releases/:id/purge", h.PurgeYingzoRelease)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/releases/"+releaseID.String()+"/purge", nil))

		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("published history cannot be purged", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		h, mock := newAgentHandlerMock(t)
		releaseID := uuid.New()
		publishedAt := time.Now().UTC()

		mock.ExpectBegin()
		mock.ExpectQuery("SELECT status,published_at FROM yingzo_releases WHERE id=\\$1 FOR UPDATE").WithArgs(releaseID).
			WillReturnRows(sqlmock.NewRows([]string{"status", "published_at"}).AddRow("disabled", publishedAt))
		mock.ExpectRollback()

		router := gin.New()
		router.DELETE("/releases/:id/purge", h.PurgeYingzoRelease)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/releases/"+releaseID.String()+"/purge", nil))

		require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
		require.Contains(t, response.Body.String(), `"code":"release_purge_not_allowed"`)
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
	return v2YingzoReleaseRowsWithEligibility(id, status, channel, true, now, publishedAt)
}

func v2YingzoReleaseRowsWithEligibility(id uuid.UUID, status, channel string, stableEligible bool, now time.Time, publishedAt any) *sqlmock.Rows {
	return sqlmock.NewRows(strings.Split(yingzoReleaseColumns, ",")).
		AddRow(id, "0.3.0", status, 2, channel, stableEligible, 1, []byte(`{"project_schema":1}`), nil, "0.128.0", "0.0.0", "preview", now, publishedAt, now)
}

func v2YingzoArtifactRows(releaseID uuid.UUID, now time.Time, version, storageRoot string) *sqlmock.Rows {
	rows := sqlmock.NewRows(strings.Split(yingzoArtifactColumns, ","))
	for _, spec := range yingzoV2ArtifactSpecs(version) {
		rows.AddRow(uuid.New(), releaseID, nil, spec.ArtifactKind, spec.Target, spec.OS, spec.Arch, spec.Format, spec.ContentType, 1,
			"validated", "unverified", now, spec.Filename, "local", filepath.Join(storageRoot, spec.Filename), int64(100), strings.Repeat("a", 64), now, now)
	}
	return rows
}

func v2YingzoPublishRows(t *testing.T, releaseID uuid.UUID, storageRoot, signatureStatus string) *sqlmock.Rows {
	t.Helper()
	rows := sqlmock.NewRows([]string{"artifact_kind", "target", "os", "arch", "runtime_protocol", "validation_status", "signature_status", "package_filename", "storage_key", "size_bytes", "sha256"})
	for _, spec := range yingzoV2ArtifactSpecs("0.3.0") {
		storageKey := filepath.Join(storageRoot, spec.Filename)
		writeV2ArtifactFixture(t, storageKey, spec)
		sha, err := yingzoFileSHA256(storageKey)
		require.NoError(t, err)
		info, statErr := os.Stat(storageKey)
		require.NoError(t, statErr)
		rows.AddRow(spec.ArtifactKind, spec.Target, spec.OS, spec.Arch, 1, "validated", signatureStatus, spec.Filename, storageKey, info.Size(), sha)
	}
	return rows
}

func v3YingzoPublishRows(t *testing.T, releaseID uuid.UUID, storageRoot, signatureStatus string) *sqlmock.Rows {
	t.Helper()
	rows := sqlmock.NewRows([]string{"artifact_kind", "target", "os", "arch", "runtime_protocol", "validation_status", "signature_status", "package_filename", "storage_key", "size_bytes", "sha256"})
	for _, spec := range yingzoV3ArtifactSpecs("0.3.0") {
		storageKey := filepath.Join(storageRoot, spec.Filename)
		writeV2ArtifactFixture(t, storageKey, spec)
		sha, err := yingzoFileSHA256(storageKey)
		require.NoError(t, err)
		info, statErr := os.Stat(storageKey)
		require.NoError(t, statErr)
		rows.AddRow(spec.ArtifactKind, spec.Target, spec.OS, spec.Arch, 1, "validated", signatureStatus, spec.Filename, storageKey, info.Size(), sha)
	}
	return rows
}

func TestYingzoV3PublishMatrixAcceptsSevenArtifacts(t *testing.T) {
	h, mock := newAgentHandlerMock(t)
	releaseID := uuid.New()
	rows := v3YingzoPublishRows(t, releaseID, t.TempDir(), "unverified")
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT artifact_kind,target,os,arch,runtime_protocol,validation_status,signature_status,package_filename,storage_key,size_bytes,sha256 FROM yingzo_release_artifacts WHERE release_id=\\$1").
		WithArgs(releaseID).WillReturnRows(rows)
	tx, err := h.db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	require.NoError(t, validateYingzoPublishMatrix(context.Background(), tx, releaseID, "0.3.0", 1, 3))
	mock.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
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
	require.NoError(t, os.WriteFile(target, []byte("opaque build output for "+spec.Filename), 0600))
}

func writeOpaqueArtifact(t *testing.T, filename string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), filename)
	require.NoError(t, os.WriteFile(path, []byte("opaque build output"), 0600))
	return path
}
