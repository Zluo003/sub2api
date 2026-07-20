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
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

type memoryYingzoTicketStore struct {
	payloads map[string][]byte
}

func newMemoryYingzoTicketStore() *memoryYingzoTicketStore {
	return &memoryYingzoTicketStore{payloads: map[string][]byte{}}
}

func (s *memoryYingzoTicketStore) Store(_ context.Context, ticket string, payload []byte, _ time.Duration) error {
	s.payloads[ticket] = append([]byte(nil), payload...)
	return nil
}

func (s *memoryYingzoTicketStore) Get(_ context.Context, ticket string) ([]byte, error) {
	payload, ok := s.payloads[ticket]
	if !ok {
		return nil, context.Canceled
	}
	return append([]byte(nil), payload...), nil
}

func TestValidateYingzoOrigin(t *testing.T) {
	require.Equal(t, "https://api-key.cc", mustYingzoOrigin(t, "https://api-key.cc/"))
	require.Equal(t, "http://127.0.0.1:8080", mustYingzoOrigin(t, "http://127.0.0.1:8080"))
	_, err := validateYingzoOrigin("http://example.com")
	require.ErrorContains(t, err, "HTTPS")
	_, err = validateYingzoOrigin("https://example.com/path")
	require.Error(t, err)
}

func TestTemporaryAssetPublicURLUsesCleanExtensionPath(t *testing.T) {
	id := uuid.New()
	got := temporaryAssetPublicURL("https://api-key.cc", id, "image/png")
	require.Equal(t, "https://api-key.cc/media/"+id.String()+"/asset.png", got)
	require.NotContains(t, got, "?")
	require.Equal(t, ".mp4", canonicalMediaExtension("video/mp4"))
	require.Equal(t, ".mp3", canonicalMediaExtension("audio/mpeg"))
}

func TestYingzoInstallPromptUsesVerifiedHostCommands(t *testing.T) {
	release := &yingzoRelease{Version: "0.2.0"}
	openAI := &yingzoReleaseArtifact{HostFamily: "openai", PackageFilename: "yingzo-openai-0.2.0.tar.gz", SHA256: strings.Repeat("a", 64)}
	claudeArtifact := &yingzoReleaseArtifact{HostFamily: "claude", PackageFilename: "yingzo-claude-0.2.0.tar.gz", SHA256: strings.Repeat("b", 64)}
	codex := yingzoInstallPrompt("codex", release, openAI, "https://api-key.cc/download/package.tar.gz")
	require.Contains(t, codex, "codex plugin marketplace add")
	require.Contains(t, codex, "codex plugin add yingzo@yingzo-private --json")
	require.Contains(t, codex, "保留现有 ~/.yingzo/auth.json")
	require.Contains(t, codex, "~/.yingzo/releases/openai/0.2.0/yingzo-openai-0.2.0/marketplace")
	require.Contains(t, codex, "~/.codex/plugins")
	require.Contains(t, codex, "不要修改 ~/.claude/plugins")
	require.NotContains(t, codex, "private-beta")
	require.NotContains(t, codex, "SHA-256")
	require.NotContains(t, codex, "claude plugin")
	chatGPT := yingzoInstallPrompt("chatgpt-work", release, openAI, "https://api-key.cc/download/package.tar.gz")
	require.Contains(t, chatGPT, "ChatGPT Work")
	require.Contains(t, chatGPT, "codex plugin")
	claude := yingzoInstallPrompt("claude-code", release, claudeArtifact, "https://api-key.cc/download/package.tar.gz")
	require.Contains(t, claude, "claude plugin marketplace add")
	require.Contains(t, claude, "claude plugin install yingzo@yingzo-private --scope user")
	require.Contains(t, claude, "~/.yingzo/releases/claude/0.2.0/yingzo-claude-0.2.0/marketplace")
	require.Contains(t, claude, "~/.claude/plugins")
	require.Contains(t, claude, "不要修改 ~/.codex/plugins")
	require.NotContains(t, claude, "private-beta")
	require.NotContains(t, claude, "SHA-256")
	require.NotContains(t, claude, "codex plugin")
	cowork := yingzoInstallPrompt("claude-cowork", release, claudeArtifact, "https://api-key.cc/download/package.tar.gz")
	require.Contains(t, cowork, "Claude Cowork")
}

func TestLegacyYingzoPackageFilenameContract(t *testing.T) {
	version := "0.2.0"
	filename, err := validateYingzoPackageFilename("yingzo-openai-"+version+".tar.gz", version, "openai")
	require.NoError(t, err)
	require.Equal(t, "yingzo-openai-"+version+".tar.gz", filename)
	_, err = validateYingzoPackageFilename("not-yingzo.tar.gz", version, "openai")
	require.Error(t, err)
}

func TestYingzoLegacyBetaArchiveRemainsCompatible(t *testing.T) {
	version := "0.1.2"
	packageFilename := "yingzo-private-beta-" + version + ".tar.gz"
	filename, err := validateYingzoPackageFilename(packageFilename, version, "combined")
	require.NoError(t, err)
	require.Equal(t, packageFilename, filename)
	require.Contains(t, yingzoInstallPrompt("codex", &yingzoRelease{Version: version}, &yingzoReleaseArtifact{
		HostFamily: "combined", PackageFilename: packageFilename, SHA256: strings.Repeat("b", 64),
	}, "https://api-key.cc/download/legacy.tar.gz"), "yingzo-private-beta-0.1.2/marketplace")
}

func TestValidateYingzoPackageFilename(t *testing.T) {
	stable, err := validateYingzoPackageFilename("yingzo-openai-0.2.0.tar.gz", "0.2.0", "openai")
	require.NoError(t, err)
	require.Equal(t, "yingzo-openai-0.2.0.tar.gz", stable)
	claude, err := validateYingzoPackageFilename("yingzo-claude-0.2.0.tar.gz", "0.2.0", "claude")
	require.NoError(t, err)
	require.Equal(t, "yingzo-claude-0.2.0.tar.gz", claude)

	legacy, err := validateYingzoPackageFilename("yingzo-private-beta-0.1.2.tar.gz", "0.1.2", "combined")
	require.NoError(t, err)
	require.Equal(t, "yingzo-private-beta-0.1.2.tar.gz", legacy)

	_, err = validateYingzoPackageFilename("sub2api-linux-amd64.tar.gz", "0.2.0", "openai")
	require.Error(t, err)
}

func TestYingzoHostFamiliesAndLegacyArtifactFallback(t *testing.T) {
	for host, expected := range map[string]string{
		"chatgpt-work": "openai", "codex": "openai", "claude-cowork": "claude", "claude-code": "claude",
	} {
		family, err := yingzoHostFamily(host)
		require.NoError(t, err)
		require.Equal(t, expected, family)
	}
	_, err := yingzoHostFamily("mobile")
	require.Error(t, err)
	combined := &yingzoReleaseArtifact{ID: uuid.New(), HostFamily: "combined"}
	release := &yingzoRelease{Artifacts: map[string]*yingzoReleaseArtifact{"combined": combined}}
	require.Same(t, combined, yingzoArtifactForFamily(release, "openai"))
	require.Same(t, combined, yingzoArtifactForFamily(release, "claude"))
}

func TestPublicYingzoReleaseExposesBothHostArtifactsWithoutChecksums(t *testing.T) {
	release := &yingzoRelease{
		Version: "0.2.0",
		Artifacts: map[string]*yingzoReleaseArtifact{
			"openai": {HostFamily: "openai", PackageFilename: "yingzo-openai-0.2.0.tar.gz", SizeBytes: 10, SHA256: strings.Repeat("a", 64)},
			"claude": {HostFamily: "claude", PackageFilename: "yingzo-claude-0.2.0.tar.gz", SizeBytes: 20, SHA256: strings.Repeat("b", 64)},
		},
	}
	public := publicYingzoRelease(release)
	require.Equal(t, int64(30), public["size_bytes"])
	artifacts, ok := public["artifacts"].(gin.H)
	require.True(t, ok)
	require.Contains(t, artifacts, "openai")
	require.Contains(t, artifacts, "claude")
	require.NotContains(t, public, "sha256")
	openAIArtifact, ok := artifacts["openai"].(gin.H)
	require.True(t, ok)
	require.NotContains(t, openAIArtifact, "sha256")
}

func TestCreateYingzoInstallInstructionsMapsFourHostsToTwoArtifacts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		host           string
		expectedFamily string
		expectedFile   string
	}{
		{host: "chatgpt-work", expectedFamily: "openai", expectedFile: "yingzo-openai-0.2.0.tar.gz"},
		{host: "codex", expectedFamily: "openai", expectedFile: "yingzo-openai-0.2.0.tar.gz"},
		{host: "claude-cowork", expectedFamily: "claude", expectedFile: "yingzo-claude-0.2.0.tar.gz"},
		{host: "claude-code", expectedFamily: "claude", expectedFile: "yingzo-claude-0.2.0.tar.gz"},
	} {
		t.Run(tc.host, func(t *testing.T) {
			h, mock := newAgentHandlerMock(t)
			tickets := newMemoryYingzoTicketStore()
			h.yingzoTicketStore = tickets
			releaseID := uuid.New()
			openAIID := uuid.New()
			claudeID := uuid.New()
			now := time.Now().UTC()
			mock.ExpectQuery("SELECT .* FROM yingzo_releases WHERE status='published' AND channel=\\$1").
				WithArgs("stable").
				WillReturnRows(legacyYingzoReleaseRows(releaseID, "0.2.0", "published", now, now))
			mock.ExpectQuery("SELECT .* FROM yingzo_release_artifacts WHERE release_id=\\$1").
				WithArgs(releaseID).
				WillReturnRows(legacyYingzoArtifactRows(releaseID, now,
					yingzoLegacyArtifactRow{id: openAIID, family: "openai", filename: "yingzo-openai-0.2.0.tar.gz", storageKey: "/tmp/openai.tar.gz", size: 100},
					yingzoLegacyArtifactRow{id: claudeID, family: "claude", filename: "yingzo-claude-0.2.0.tar.gz", storageKey: "/tmp/claude.tar.gz", size: 110}))

			router := gin.New()
			router.POST("/install", func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
				h.CreateYingzoInstallInstructions(c)
			})
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "https://api-key.cc/install", strings.NewReader(`{"host":"`+tc.host+`"}`))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(response, request)

			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			var body struct {
				Host        string `json:"host"`
				HostFamily  string `json:"host_family"`
				DownloadURL string `json:"download_url"`
				Prompt      string `json:"prompt"`
			}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
			require.Equal(t, tc.host, body.Host)
			require.Equal(t, tc.expectedFamily, body.HostFamily)
			require.Contains(t, body.DownloadURL, "/"+tc.expectedFile)
			require.NotContains(t, body.Prompt, "SHA-256")
			require.Len(t, tickets.payloads, 1)
			for _, payload := range tickets.payloads {
				var ticket yingzoInstallTicket
				require.NoError(t, json.Unmarshal(payload, &ticket))
				require.Equal(t, tc.expectedFamily, ticket.HostFamily)
				require.Equal(t, map[string]uuid.UUID{"openai": openAIID, "claude": claudeID}[tc.expectedFamily], ticket.ArtifactID)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestCreateYingzoInstallInstructionsRejectsClaudeDesktopForLegacyRelease(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	tickets := newMemoryYingzoTicketStore()
	h.yingzoTicketStore = tickets
	releaseID := uuid.New()
	now := time.Now().UTC()
	mock.ExpectQuery("SELECT .* FROM yingzo_releases WHERE status='published' AND channel=\\$1").
		WithArgs("stable").
		WillReturnRows(legacyYingzoReleaseRows(releaseID, "0.2.4", "published", now, now))
	mock.ExpectQuery("SELECT .* FROM yingzo_release_artifacts WHERE release_id=\\$1").
		WithArgs(releaseID).
		WillReturnRows(legacyYingzoArtifactRows(releaseID, now,
			yingzoLegacyArtifactRow{id: uuid.New(), family: "claude", filename: "yingzo-claude-0.2.4.tar.gz", storageKey: "/tmp/claude.tar.gz", size: 110}))

	router := gin.New()
	router.POST("/install", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
		h.CreateYingzoInstallInstructions(c)
	})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "https://api-key.cc/install", strings.NewReader(`{"host":"claude-chat"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "host_not_supported_by_release")
	require.Empty(t, tickets.payloads)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDownloadYingzoReleaseTicketIsBoundToArtifactAndHostFamily(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	tickets := newMemoryYingzoTicketStore()
	h.yingzoTicketStore = tickets
	releaseID := uuid.New()
	openAIID := uuid.New()
	claudeID := uuid.New()
	now := time.Now().UTC()
	openAIFile := filepath.Join(t.TempDir(), "openai.tar.gz")
	require.NoError(t, os.WriteFile(openAIFile, []byte("openai-package"), 0600))
	payload, err := json.Marshal(yingzoInstallTicket{ReleaseID: releaseID, ArtifactID: openAIID, Host: "codex", HostFamily: "openai"})
	require.NoError(t, err)
	tickets.payloads["ticket"] = payload
	mock.ExpectQuery("SELECT .* FROM yingzo_releases WHERE id=\\$1").
		WithArgs(releaseID).
		WillReturnRows(legacyYingzoReleaseRows(releaseID, "0.2.0", "published", now, now))
	mock.ExpectQuery("SELECT .* FROM yingzo_release_artifacts WHERE release_id=\\$1").
		WithArgs(releaseID).
		WillReturnRows(legacyYingzoArtifactRows(releaseID, now,
			yingzoLegacyArtifactRow{id: openAIID, family: "openai", filename: "yingzo-openai-0.2.0.tar.gz", storageKey: openAIFile, size: 14},
			yingzoLegacyArtifactRow{id: claudeID, family: "claude", filename: "yingzo-claude-0.2.0.tar.gz", storageKey: "/tmp/claude.tar.gz", size: 15}))

	router := gin.New()
	router.GET("/download/:ticket/:filename", h.DownloadYingzoRelease)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/download/ticket/yingzo-openai-0.2.0.tar.gz", nil))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, "openai-package", response.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDownloadYingzoReleaseReturnsNotFoundWhenReleaseLookupFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	tickets := newMemoryYingzoTicketStore()
	h.yingzoTicketStore = tickets
	releaseID := uuid.New()
	payload, err := json.Marshal(yingzoInstallTicket{
		ReleaseID: releaseID, ArtifactID: uuid.New(), Host: "codex", HostFamily: "openai",
	})
	require.NoError(t, err)
	tickets.payloads["ticket"] = payload
	mock.ExpectQuery("SELECT .* FROM yingzo_releases WHERE id=\\$1").
		WithArgs(releaseID).
		WillReturnError(context.Canceled)

	router := gin.New()
	router.GET("/download/:ticket/:filename", h.DownloadYingzoRelease)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/download/ticket/yingzo-openai-0.2.0.tar.gz", nil))

	require.Equal(t, http.StatusNotFound, response.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListYingzoReleasesReturnsDatabaseErrorAfterIterationFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	now := time.Now()
	rows := legacyYingzoReleaseRows(uuid.New(), "0.2.0", "draft", now, time.Time{}).
		RowError(0, context.Canceled)
	mock.ExpectQuery("SELECT .* FROM yingzo_releases ORDER BY created_at DESC").
		WillReturnRows(rows)

	router := gin.New()
	router.GET("/releases", h.ListYingzoReleases)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/releases", nil))

	require.Equal(t, http.StatusInternalServerError, response.Code)
	require.Equal(t, "database_error", responseCode(response))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUploadYingzoReleaseCleansArtifactsWhenTransactionCannotStart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	openAIArchive := filepath.Join(t.TempDir(), "yingzo-openai-0.2.0.tar.gz")
	claudeArchive := filepath.Join(t.TempDir(), "yingzo-claude-0.2.0.tar.gz")
	writeYingzoHostArchive(t, openAIArchive, "openai")
	writeYingzoHostArchive(t, claudeArchive, "claude")
	mock.ExpectBegin().WillReturnError(context.Canceled)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	require.NoError(t, writer.WriteField("version", "0.2.0"))
	writeYingzoMultipartFile(t, writer, "openai_package", openAIArchive)
	writeYingzoMultipartFile(t, writer, "claude_package", claudeArchive)
	require.NoError(t, writer.Close())

	router := gin.New()
	router.POST("/releases", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
		h.UploadYingzoRelease(c)
	})
	request := httptest.NewRequest(http.MethodPost, "/releases", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusInternalServerError, response.Code, response.Body.String())
	require.Equal(t, "database_error", responseCode(response))
	entries, err := os.ReadDir(filepath.Join(h.dataDir, "releases"))
	if !os.IsNotExist(err) {
		require.NoError(t, err)
		require.Empty(t, entries)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUploadYingzoReleaseReportsDuplicateVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	openAIArchive := filepath.Join(t.TempDir(), "yingzo-openai-0.2.1.tar.gz")
	claudeArchive := filepath.Join(t.TempDir(), "yingzo-claude-0.2.1.tar.gz")
	writeYingzoHostArchive(t, openAIArchive, "openai")
	writeYingzoHostArchive(t, claudeArchive, "claude")
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO yingzo_releases").WillReturnError(&pq.Error{Code: "23505"})
	mock.ExpectRollback()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	require.NoError(t, writer.WriteField("version", "0.2.1"))
	writeYingzoMultipartFile(t, writer, "openai_package", openAIArchive)
	writeYingzoMultipartFile(t, writer, "claude_package", claudeArchive)
	require.NoError(t, writer.Close())

	router := gin.New()
	router.POST("/releases", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
		h.UploadYingzoRelease(c)
	})
	request := httptest.NewRequest(http.MethodPost, "/releases", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
	require.Equal(t, "yingzo_release_version_exists", responseCode(response))
	require.Contains(t, response.Body.String(), "0.2.1")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPublishYingzoReleaseRejectsIncompleteDualArtifacts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mock := newAgentHandlerMock(t)
	releaseID := uuid.New()
	storage := t.TempDir()
	openAIFile := filepath.Join(storage, "openai.tar.gz")
	require.NoError(t, os.WriteFile(openAIFile, []byte("openai"), 0600))
	openAISHA, err := yingzoFileSHA256(openAIFile)
	require.NoError(t, err)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status,channel,distribution_schema_version,stable_eligible,runtime_protocol,version,signature FROM yingzo_releases WHERE id=\\$1 FOR UPDATE").
		WithArgs(releaseID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "channel", "distribution_schema_version", "stable_eligible", "runtime_protocol", "version", "signature"}).AddRow("draft", "stable", 1, true, 0, "0.2.0", nil))
	mock.ExpectQuery("SELECT host_family,storage_key,size_bytes,sha256 FROM yingzo_release_artifacts WHERE release_id=\\$1").
		WithArgs(releaseID).
		WillReturnRows(sqlmock.NewRows([]string{"host_family", "storage_key", "size_bytes", "sha256"}).AddRow("openai", openAIFile, int64(6), openAISHA))
	mock.ExpectRollback()

	router := gin.New()
	router.POST("/releases/:id/publish", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
		h.PublishYingzoRelease(c)
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/releases/"+releaseID.String()+"/publish", bytes.NewReader(nil)))
	require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
	require.Equal(t, "release_artifacts_incomplete", responseCode(response))
	require.NoError(t, mock.ExpectationsWereMet())
}

type yingzoLegacyArtifactRow struct {
	id         uuid.UUID
	family     string
	filename   string
	storageKey string
	size       int64
}

func legacyYingzoReleaseRows(id uuid.UUID, version, status string, now time.Time, publishedAt any) *sqlmock.Rows {
	return sqlmock.NewRows(strings.Split(yingzoReleaseColumns, ",")).
		AddRow(id, version, status, 1, "stable", true, 0, []byte(`{}`), nil, nil, nil, nil, now, publishedAt, now)
}

func legacyYingzoArtifactRows(releaseID uuid.UUID, now time.Time, artifacts ...yingzoLegacyArtifactRow) *sqlmock.Rows {
	rows := sqlmock.NewRows(strings.Split(yingzoArtifactColumns, ","))
	for _, artifact := range artifacts {
		rows.AddRow(
			artifact.id, releaseID, artifact.family, "host_package", artifact.family, "any", "any", "tar.gz", "application/gzip", 0,
			"validated", "unverified", now, artifact.filename, "local", artifact.storageKey, artifact.size, strings.Repeat("a", 64), now, now,
		)
	}
	return rows
}

func mustYingzoOrigin(t *testing.T, raw string) string {
	t.Helper()
	value, err := validateYingzoOrigin(raw)
	require.NoError(t, err)
	return value
}

func writeYingzoHostArchive(t *testing.T, target, hostFamily string) {
	t.Helper()
	require.NoError(t, os.WriteFile(target, []byte("opaque "+hostFamily+" package"), 0600))
}

func writeYingzoMultipartFile(t *testing.T, writer *multipart.Writer, field, filename string) {
	t.Helper()
	part, err := writer.CreateFormFile(field, filepath.Base(filename))
	require.NoError(t, err)
	contents, err := os.ReadFile(filename)
	require.NoError(t, err)
	_, err = part.Write(contents)
	require.NoError(t, err)
}
