package handler

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

const (
	yingzoPublicOriginSetting = "yingzo_public_origin"
	yingzoTicketTTL           = 10 * time.Minute
	yingzoReleaseMaxBytes     = int64(512 << 20)
)

var yingzoVersionPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+-]{0,63}$`)

type yingzoRelease struct {
	ID               uuid.UUID                         `json:"id"`
	Version          string                            `json:"version"`
	Status           string                            `json:"status"`
	Signature        string                            `json:"signature,omitempty"`
	MinCodexVersion  string                            `json:"min_codex_version"`
	MinClaudeVersion string                            `json:"min_claude_version"`
	ReleaseNotes     string                            `json:"release_notes,omitempty"`
	CreatedAt        time.Time                         `json:"created_at"`
	PublishedAt      *time.Time                        `json:"published_at,omitempty"`
	UpdatedAt        time.Time                         `json:"updated_at"`
	Artifacts        map[string]*yingzoReleaseArtifact `json:"artifacts"`
}

type yingzoReleaseArtifact struct {
	ID              uuid.UUID `json:"id"`
	ReleaseID       uuid.UUID `json:"release_id"`
	HostFamily      string    `json:"host_family"`
	PackageFilename string    `json:"package_filename"`
	StorageBackend  string    `json:"storage_backend"`
	StorageKey      string    `json:"-"`
	SizeBytes       int64     `json:"size_bytes"`
	SHA256          string    `json:"sha256"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type yingzoInstallTicket struct {
	ReleaseID  uuid.UUID `json:"release_id"`
	ArtifactID uuid.UUID `json:"artifact_id"`
	UserID     int64     `json:"user_id"`
	Host       string    `json:"host"`
	HostFamily string    `json:"host_family"`
}

func (h *AgentHandler) GetYingzoDiscovery(c *gin.Context) {
	origin, err := h.publicOrigin(c)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "yingzo_origin_invalid"}})
		return
	}
	release, err := h.latestPublishedYingzoRelease(c)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	response := gin.H{
		"schema_version":    1,
		"product":           "yingzo",
		"name":              "Yingzo（影作）",
		"origin":            origin,
		"product_url":       origin + "/yingzo",
		"authorization_url": origin + "/agent/authorize",
		"api": gin.H{
			"latest_release":       origin + "/api/v1/agent/plugin/releases/latest",
			"install_instructions": origin + "/api/v1/agent/plugin/install-instructions",
		},
	}
	if release != nil {
		response["latest"] = publicYingzoRelease(release)
	} else {
		response["latest"] = nil
	}
	c.JSON(http.StatusOK, response)
}

func (h *AgentHandler) GetLatestYingzoRelease(c *gin.Context) {
	release, err := h.latestPublishedYingzoRelease(c)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "yingzo_release_not_found"}})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	c.JSON(http.StatusOK, publicYingzoRelease(release))
}

func (h *AgentHandler) CreateYingzoInstallInstructions(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "unauthorized"}})
		return
	}
	var input struct {
		Host string `json:"host" binding:"required"`
	}
	if c.ShouldBindJSON(&input) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_host", "message": "host must be chatgpt-work, codex, claude-cowork, or claude-code"}})
		return
	}
	hostFamily, hostErr := yingzoHostFamily(input.Host)
	if hostErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_host", "message": hostErr.Error()}})
		return
	}
	release, err := h.latestPublishedYingzoRelease(c)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "yingzo_release_not_found"}})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	artifact := yingzoArtifactForFamily(release, hostFamily)
	if artifact == nil {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "release_artifact_unavailable"}})
		return
	}
	origin, err := h.publicOrigin(c)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "yingzo_origin_invalid"}})
		return
	}
	if h.yingzoTicketStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "download_ticket_unavailable"}})
		return
	}
	ticket, err := randomToken(24)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "token_generation_failed"}})
		return
	}
	payload, _ := json.Marshal(yingzoInstallTicket{ReleaseID: release.ID, ArtifactID: artifact.ID, UserID: subject.UserID, Host: input.Host, HostFamily: hostFamily})
	if err := h.yingzoTicketStore.Store(c.Request.Context(), ticket, payload, yingzoTicketTTL); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "download_ticket_unavailable"}})
		return
	}
	downloadURL := fmt.Sprintf("%s/api/v1/agent/plugin/download/%s/%s", origin, url.PathEscape(ticket), url.PathEscape(artifact.PackageFilename))
	expiresAt := time.Now().Add(yingzoTicketTTL).UTC()
	c.JSON(http.StatusOK, gin.H{
		"host":         input.Host,
		"host_family":  hostFamily,
		"version":      release.Version,
		"signature":    release.Signature,
		"download_url": downloadURL,
		"expires_at":   expiresAt,
		"prompt":       yingzoInstallPrompt(input.Host, release, artifact, downloadURL),
	})
}

func (h *AgentHandler) DownloadYingzoRelease(c *gin.Context) {
	if h.yingzoTicketStore == nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	raw, err := h.yingzoTicketStore.Consume(c.Request.Context(), c.Param("ticket"))
	if errors.Is(err, service.ErrYingzoDownloadTicketNotFound) {
		c.Status(http.StatusNotFound)
		return
	}
	if err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	var ticket yingzoInstallTicket
	if json.Unmarshal(raw, &ticket) != nil {
		c.Status(http.StatusNotFound)
		return
	}
	release, err := h.getYingzoRelease(c, ticket.ReleaseID)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	artifact := yingzoArtifactByID(release, ticket.ArtifactID)
	if release.Status != "published" || artifact == nil || c.Param("filename") != artifact.PackageFilename {
		c.Status(http.StatusNotFound)
		return
	}
	if family, familyErr := yingzoHostFamily(ticket.Host); familyErr != nil || family != ticket.HostFamily || (artifact.HostFamily != family && artifact.HostFamily != "combined") {
		c.Status(http.StatusNotFound)
		return
	}
	file, err := os.Open(artifact.StorageKey)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer func() { _ = file.Close() }()
	c.Header("Content-Type", "application/gzip")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", artifact.PackageFilename))
	http.ServeContent(c.Writer, c.Request, artifact.PackageFilename, artifact.UpdatedAt, file)
}

func (h *AgentHandler) ListYingzoReleases(c *gin.Context) {
	rows, err := h.db.QueryContext(c, `SELECT id,version,status,signature,min_codex_version,min_claude_version,release_notes,created_at,published_at,updated_at FROM yingzo_releases ORDER BY created_at DESC`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	defer func() { _ = rows.Close() }()
	items := make([]*yingzoRelease, 0)
	for rows.Next() {
		release, scanErr := scanYingzoRelease(rows)
		if scanErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
			return
		}
		if err := h.loadYingzoArtifacts(c, release); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
			return
		}
		items = append(items, release)
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *AgentHandler) UploadYingzoRelease(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "unauthorized"}})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2*yingzoReleaseMaxBytes+(4<<20))
	version := strings.TrimSpace(c.PostForm("version"))
	if !yingzoVersionPattern.MatchString(version) {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_version"}})
		return
	}
	releaseID := uuid.New()
	releaseDir := filepath.Join(h.dataDir, "releases", releaseID.String())
	cleanup := func() { _ = os.RemoveAll(releaseDir) }
	openAIFile, openAIHeader, err := c.Request.FormFile("openai_package")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "openai_package_required"}})
		return
	}
	defer func() { _ = openAIFile.Close() }()
	claudeFile, claudeHeader, err := c.Request.FormFile("claude_package")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "claude_package_required"}})
		return
	}
	defer func() { _ = claudeFile.Close() }()

	openAIArtifact, err := h.storeYingzoReleaseArtifact(openAIFile, openAIHeader.Filename, releaseID, version, "openai")
	if err != nil {
		cleanup()
		writeYingzoUploadError(c, err)
		return
	}
	claudeArtifact, err := h.storeYingzoReleaseArtifact(claudeFile, claudeHeader.Filename, releaseID, version, "claude")
	if err != nil {
		cleanup()
		writeYingzoUploadError(c, err)
		return
	}
	tx, err := h.db.BeginTx(c, &sql.TxOptions{})
	if err != nil {
		cleanup()
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(c, `INSERT INTO yingzo_releases(id,version,status,signature,min_codex_version,min_claude_version,release_notes,created_by) VALUES($1,$2,'draft',$3,$4,$5,$6,$7)`, releaseID, version, strings.TrimSpace(c.PostForm("signature")), strings.TrimSpace(c.PostForm("min_codex_version")), strings.TrimSpace(c.PostForm("min_claude_version")), strings.TrimSpace(c.PostForm("release_notes")), subject.UserID)
	if err == nil {
		for _, artifact := range []*yingzoReleaseArtifact{openAIArtifact, claudeArtifact} {
			_, err = tx.ExecContext(c, `INSERT INTO yingzo_release_artifacts(id,release_id,host_family,package_filename,storage_backend,storage_key,size_bytes,sha256) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, artifact.ID, artifact.ReleaseID, artifact.HostFamily, artifact.PackageFilename, artifact.StorageBackend, artifact.StorageKey, artifact.SizeBytes, artifact.SHA256)
			if err != nil {
				break
			}
		}
	}
	if err != nil {
		cleanup()
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			c.JSON(http.StatusConflict, gin.H{"error": gin.H{
				"code":    "yingzo_release_version_exists",
				"message": fmt.Sprintf("Yingzo version %s already exists", version),
			}})
			return
		}
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "release_create_failed", "message": "failed to create Yingzo release"}})
		return
	}
	if err := tx.Commit(); err != nil {
		cleanup()
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	release, err := h.getYingzoRelease(c, releaseID)
	if err != nil {
		c.JSON(http.StatusCreated, gin.H{"id": releaseID, "version": version, "status": "draft"})
		return
	}
	c.JSON(http.StatusCreated, release)
}

type yingzoUploadError struct {
	status  int
	code    string
	message string
}

func (e *yingzoUploadError) Error() string { return e.message }

func writeYingzoUploadError(c *gin.Context, err error) {
	var uploadErr *yingzoUploadError
	if errors.As(err, &uploadErr) {
		c.JSON(uploadErr.status, gin.H{"error": gin.H{"code": uploadErr.code, "message": uploadErr.message}})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "storage_error"}})
}

func (h *AgentHandler) storeYingzoReleaseArtifact(reader io.Reader, originalFilename string, releaseID uuid.UUID, version, hostFamily string) (*yingzoReleaseArtifact, error) {
	packageFilename, err := validateYingzoPackageFilename(originalFilename, version, hostFamily)
	if err != nil {
		return nil, &yingzoUploadError{status: http.StatusBadRequest, code: "invalid_package_filename", message: err.Error()}
	}
	artifact := &yingzoReleaseArtifact{
		ID:              uuid.New(),
		ReleaseID:       releaseID,
		HostFamily:      hostFamily,
		PackageFilename: packageFilename,
		StorageBackend:  "local",
	}
	releaseDir := filepath.Join(h.dataDir, "releases", releaseID.String())
	if err := os.MkdirAll(releaseDir, 0700); err != nil {
		return nil, err
	}
	temporary := filepath.Join(releaseDir, artifact.ID.String()+".tmp")
	target := filepath.Join(releaseDir, artifact.ID.String()+".tar.gz")
	out, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(out, hash), io.LimitReader(reader, yingzoReleaseMaxBytes+1))
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil || size <= 0 || size > yingzoReleaseMaxBytes {
		_ = os.Remove(temporary)
		return nil, &yingzoUploadError{status: http.StatusBadRequest, code: "package_upload_failed", message: "package is empty, incomplete, or exceeds 512 MB"}
	}
	if err := os.Rename(temporary, target); err != nil {
		_ = os.Remove(temporary)
		return nil, err
	}
	if err := validateYingzoArchive(target, packageFilename, hostFamily); err != nil {
		_ = os.Remove(target)
		return nil, &yingzoUploadError{status: http.StatusUnprocessableEntity, code: "invalid_release_archive", message: err.Error()}
	}
	artifact.StorageKey = target
	artifact.SizeBytes = size
	artifact.SHA256 = hex.EncodeToString(hash.Sum(nil))
	return artifact, nil
}

func (h *AgentHandler) PublishYingzoRelease(c *gin.Context) {
	h.setPublishedYingzoRelease(c, false)
}

func (h *AgentHandler) RollbackYingzoRelease(c *gin.Context) {
	h.setPublishedYingzoRelease(c, true)
}

func (h *AgentHandler) setPublishedYingzoRelease(c *gin.Context, rollback bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "unauthorized"}})
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "yingzo_release_not_found"}})
		return
	}
	tx, err := h.db.BeginTx(c, &sql.TxOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	defer func() { _ = tx.Rollback() }()
	var status string
	if err = tx.QueryRowContext(c, `SELECT status FROM yingzo_releases WHERE id=$1 FOR UPDATE`, id).Scan(&status); err != nil || status == "disabled" || (!rollback && status != "draft") || (rollback && status != "superseded") {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "release_state_invalid"}})
		return
	}
	var combinedCount, openAICount, claudeCount int
	if err = tx.QueryRowContext(c, `SELECT COUNT(*) FILTER (WHERE host_family='combined'),COUNT(*) FILTER (WHERE host_family='openai'),COUNT(*) FILTER (WHERE host_family='claude') FROM yingzo_release_artifacts WHERE release_id=$1`, id).Scan(&combinedCount, &openAICount, &claudeCount); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	if combinedCount == 0 && (openAICount == 0 || claudeCount == 0) {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "release_artifacts_incomplete", "message": "both OpenAI and Claude packages are required before publishing"}})
		return
	}
	if _, err = tx.ExecContext(c, `UPDATE yingzo_releases SET status='superseded',updated_at=NOW() WHERE status='published' AND id<>$1`, id); err == nil {
		_, err = tx.ExecContext(c, `UPDATE yingzo_releases SET status='published',published_at=NOW(),published_by=$2,updated_at=NOW() WHERE id=$1`, id, subject.UserID)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	release, err := h.getYingzoRelease(c, id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"id": id, "status": "published"})
		return
	}
	c.JSON(http.StatusOK, release)
}

func (h *AgentHandler) DisableYingzoRelease(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "yingzo_release_not_found"}})
		return
	}
	result, err := h.db.ExecContext(c, `UPDATE yingzo_releases SET status='disabled',updated_at=NOW() WHERE id=$1 AND status<>'disabled'`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	rows, _ := result.RowsAffected()
	c.JSON(http.StatusOK, gin.H{"disabled": rows == 1})
}

func (h *AgentHandler) GetYingzoSettings(c *gin.Context) {
	configured := ""
	if h.settingRepo != nil {
		configured, _ = h.settingRepo.GetValue(c, yingzoPublicOriginSetting)
	}
	effective, err := h.publicOrigin(c)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "yingzo_origin_invalid"}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"public_origin": configured, "effective_origin": effective, "release_storage": filepath.Join(h.dataDir, "releases")})
}

func (h *AgentHandler) UpdateYingzoSettings(c *gin.Context) {
	var input struct {
		PublicOrigin string `json:"public_origin"`
	}
	if c.ShouldBindJSON(&input) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request"}})
		return
	}
	origin := strings.TrimSpace(input.PublicOrigin)
	if origin != "" {
		var err error
		origin, err = validateYingzoOrigin(origin)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_public_origin", "message": err.Error()}})
			return
		}
	}
	if h.settingRepo == nil || h.settingRepo.Set(c, yingzoPublicOriginSetting, origin) != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	h.GetYingzoSettings(c)
}

func (h *AgentHandler) publicOrigin(c *gin.Context) (string, error) {
	if h.settingRepo != nil {
		if configured, err := h.settingRepo.GetValue(c, yingzoPublicOriginSetting); err == nil && strings.TrimSpace(configured) != "" {
			return validateYingzoOrigin(configured)
		}
	}
	return requestPublicOrigin(c)
}

func requestPublicOrigin(c *gin.Context) (string, error) {
	scheme := "https"
	if c.Request.TLS == nil {
		scheme = "http"
	}
	if forwarded := strings.TrimSpace(strings.Split(c.GetHeader("X-Forwarded-Proto"), ",")[0]); forwarded == "http" || forwarded == "https" {
		scheme = forwarded
	}
	return validateYingzoOrigin(scheme + "://" + c.Request.Host)
}

func validateYingzoOrigin(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", errors.New("public origin must contain only an HTTP(S) scheme and host")
	}
	if parsed.Scheme != "https" && parsed.Hostname() != "localhost" && parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "::1" {
		return "", errors.New("public origin must use HTTPS outside local development")
	}
	parsed.Path = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func (h *AgentHandler) latestPublishedYingzoRelease(ctx context.Context) (*yingzoRelease, error) {
	row := h.db.QueryRowContext(ctx, `SELECT id,version,status,signature,min_codex_version,min_claude_version,release_notes,created_at,published_at,updated_at FROM yingzo_releases WHERE status='published' ORDER BY published_at DESC LIMIT 1`)
	release, err := scanYingzoRelease(row)
	if err != nil {
		return nil, err
	}
	if err := h.loadYingzoArtifacts(ctx, release); err != nil {
		return nil, err
	}
	return release, nil
}

func (h *AgentHandler) getYingzoRelease(ctx context.Context, id uuid.UUID) (*yingzoRelease, error) {
	row := h.db.QueryRowContext(ctx, `SELECT id,version,status,signature,min_codex_version,min_claude_version,release_notes,created_at,published_at,updated_at FROM yingzo_releases WHERE id=$1`, id)
	release, err := scanYingzoRelease(row)
	if err != nil {
		return nil, err
	}
	if err := h.loadYingzoArtifacts(ctx, release); err != nil {
		return nil, err
	}
	return release, nil
}

type yingzoReleaseScanner interface{ Scan(dest ...any) error }

func scanYingzoRelease(scanner yingzoReleaseScanner) (*yingzoRelease, error) {
	var release yingzoRelease
	var signature, minCodex, minClaude, notes sql.NullString
	var published sql.NullTime
	err := scanner.Scan(&release.ID, &release.Version, &release.Status, &signature, &minCodex, &minClaude, &notes, &release.CreatedAt, &published, &release.UpdatedAt)
	if err != nil {
		return nil, err
	}
	release.Signature = signature.String
	release.MinCodexVersion = minCodex.String
	release.MinClaudeVersion = minClaude.String
	release.ReleaseNotes = notes.String
	if published.Valid {
		release.PublishedAt = &published.Time
	}
	release.Artifacts = map[string]*yingzoReleaseArtifact{}
	return &release, nil
}

func (h *AgentHandler) loadYingzoArtifacts(ctx context.Context, release *yingzoRelease) error {
	rows, err := h.db.QueryContext(ctx, `SELECT id,release_id,host_family,package_filename,storage_backend,storage_key,size_bytes,sha256,created_at,updated_at FROM yingzo_release_artifacts WHERE release_id=$1 ORDER BY host_family`, release.ID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	release.Artifacts = map[string]*yingzoReleaseArtifact{}
	for rows.Next() {
		var artifact yingzoReleaseArtifact
		if err := rows.Scan(&artifact.ID, &artifact.ReleaseID, &artifact.HostFamily, &artifact.PackageFilename, &artifact.StorageBackend, &artifact.StorageKey, &artifact.SizeBytes, &artifact.SHA256, &artifact.CreatedAt, &artifact.UpdatedAt); err != nil {
			return err
		}
		release.Artifacts[artifact.HostFamily] = &artifact
	}
	return rows.Err()
}

func yingzoArtifactForFamily(release *yingzoRelease, family string) *yingzoReleaseArtifact {
	if release == nil {
		return nil
	}
	if artifact := release.Artifacts[family]; artifact != nil {
		return artifact
	}
	return release.Artifacts["combined"]
}

func yingzoArtifactByID(release *yingzoRelease, artifactID uuid.UUID) *yingzoReleaseArtifact {
	if release == nil {
		return nil
	}
	for _, artifact := range release.Artifacts {
		if artifact.ID == artifactID {
			return artifact
		}
	}
	return nil
}

func yingzoHostFamily(host string) (string, error) {
	switch host {
	case "chatgpt-work", "codex":
		return "openai", nil
	case "claude-cowork", "claude-code":
		return "claude", nil
	default:
		return "", errors.New("host must be chatgpt-work, codex, claude-cowork, or claude-code")
	}
}

func publicYingzoRelease(release *yingzoRelease) gin.H {
	artifacts := gin.H{}
	var totalSize int64
	for family, artifact := range release.Artifacts {
		summary := gin.H{
			"host_family":      artifact.HostFamily,
			"package_filename": artifact.PackageFilename,
			"size_bytes":       artifact.SizeBytes,
		}
		totalSize += artifact.SizeBytes
		if family == "combined" {
			artifacts["openai"] = summary
			artifacts["claude"] = summary
			continue
		}
		artifacts[family] = summary
	}
	return gin.H{
		"version":            release.Version,
		"size_bytes":         totalSize,
		"artifacts":          artifacts,
		"signature_status":   map[bool]string{true: "signed", false: "unsigned"}[release.Signature != ""],
		"min_codex_version":  release.MinCodexVersion,
		"min_claude_version": release.MinClaudeVersion,
		"release_notes":      release.ReleaseNotes,
		"published_at":       release.PublishedAt,
	}
}

func yingzoInstallPrompt(host string, release *yingzoRelease, artifact *yingzoReleaseArtifact, downloadURL string) string {
	archiveRoot := strings.TrimSuffix(artifact.PackageFilename, ".tar.gz")
	hostFamily, _ := yingzoHostFamily(host)
	hostReleaseRoot := fmt.Sprintf("~/.yingzo/releases/%s/%s", hostFamily, release.Version)
	common := fmt.Sprintf(`请安装或升级 Yingzo（影作）私有插件到版本 %s。下载地址：%s
请把安装包下载到临时文件，解压到 %s，保留现有 ~/.yingzo/auth.json，禁止把凭证写入项目、日志或安装目录。安装完成后验证插件和 MCP 工具，并告诉我结果。`, release.Version, downloadURL, hostReleaseRoot)
	if release.Signature != "" {
		common += "\n发行签名：" + release.Signature + "。同时校验签名；失败立即停止。"
	}
	if hostFamily == "openai" {
		return common + fmt.Sprintf(`
这是 OpenAI 宿主专用安装指令，适用于 ChatGPT Work 与 Codex。只执行本段列出的 Codex 插件命令，也不要修改 ~/.claude/plugins。插件安装到独立的 ~/.codex/plugins 缓存。解压后的 marketplace 目录是 %s/%s/marketplace。若旧版本存在，先运行 codex plugin remove yingzo@yingzo-private --json，再运行 codex plugin marketplace remove yingzo-private --json。然后运行 codex plugin marketplace add <上述 marketplace 绝对路径> --json，最后运行 codex plugin add yingzo@yingzo-private --json。不要要求用户手动打开终端。安装后验证插件清单，并提示用户新建任务以加载新工具。`, hostReleaseRoot, archiveRoot)
	}
	return common + fmt.Sprintf(`
这是 Claude 宿主专用安装指令，适用于 Claude Cowork 与 Claude Code。只执行本段列出的 Claude 插件命令，也不要修改 ~/.codex/plugins。插件安装到独立的 ~/.claude/plugins 缓存。解压后的 marketplace 目录是 %s/%s/marketplace。若旧版本存在，先运行 claude plugin uninstall yingzo@yingzo-private --scope user --keep-data，再运行 claude plugin marketplace remove yingzo-private --scope user。然后运行 claude plugin marketplace add <上述 marketplace 绝对路径> --scope user，最后运行 claude plugin install yingzo@yingzo-private --scope user。不要要求用户手动打开终端。安装后验证插件清单，并提示用户新建 Claude 会话以加载新工具。`, hostReleaseRoot, archiveRoot)
}

func validateYingzoPackageFilename(filename, version, hostFamily string) (string, error) {
	filename = filepath.Base(strings.TrimSpace(filename))
	var allowed []string
	switch hostFamily {
	case "openai":
		allowed = []string{"yingzo-openai-" + version + ".tar.gz"}
	case "claude":
		allowed = []string{"yingzo-claude-" + version + ".tar.gz"}
	case "combined":
		allowed = []string{"yingzo-private-" + version + ".tar.gz", "yingzo-private-beta-" + version + ".tar.gz"}
	default:
		return "", errors.New("unsupported host family")
	}
	for _, candidate := range allowed {
		if filename == candidate {
			return filename, nil
		}
	}
	return "", fmt.Errorf("package filename must be %s", strings.Join(allowed, " or "))
}

func validateYingzoArchive(filename, packageFilename, hostFamily string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return errors.New("package is not a valid gzip archive")
	}
	defer func() { _ = gzipReader.Close() }()
	tarReader := tar.NewReader(gzipReader)
	root := strings.TrimSuffix(packageFilename, ".tar.gz") + "/marketplace/"
	required := map[string]bool{root + "plugins/yingzo/distribution.json": false}
	switch hostFamily {
	case "openai":
		required[root+".agents/plugins/marketplace.json"] = false
		required[root+"plugins/yingzo/.codex-plugin/plugin.json"] = false
		required[root+"plugins/yingzo/apps/review/dist/index.html"] = false
	case "claude":
		required[root+".claude-plugin/marketplace.json"] = false
		required[root+"plugins/yingzo/.claude-plugin/plugin.json"] = false
		required[root+"plugins/yingzo/apps/review/dist/index.html"] = false
	case "combined":
		required[root+".agents/plugins/marketplace.json"] = false
		required[root+".claude-plugin/marketplace.json"] = false
		required[root+"plugins/yingzo/.codex-plugin/plugin.json"] = false
	default:
		return errors.New("unsupported host family")
	}
	entries := 0
	var expandedBytes int64
	for {
		header, nextErr := tarReader.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return errors.New("package tar stream is invalid")
		}
		entries++
		if entries > 20_000 {
			return errors.New("package contains too many entries")
		}
		name := filepath.ToSlash(header.Name)
		clean := filepath.ToSlash(filepath.Clean(name))
		if name == "" || strings.HasPrefix(name, "/") || clean == ".." || strings.HasPrefix(clean, "../") {
			return errors.New("package contains an unsafe path")
		}
		if header.Typeflag == tar.TypeSymlink || header.Typeflag == tar.TypeLink {
			return errors.New("package links are not allowed")
		}
		if header.Size < 0 {
			return errors.New("package contains an invalid entry size")
		}
		expandedBytes += header.Size
		if expandedBytes > 2<<30 {
			return errors.New("package expands beyond the safety limit")
		}
		if _, ok := required[clean]; ok {
			required[clean] = true
		}
	}
	for name, found := range required {
		if !found {
			return fmt.Errorf("package is missing %s", strings.TrimPrefix(name, root))
		}
	}
	return nil
}
