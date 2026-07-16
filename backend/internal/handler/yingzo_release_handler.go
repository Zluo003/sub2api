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
)

const (
	yingzoPublicOriginSetting = "yingzo_public_origin"
	yingzoTicketTTL           = 10 * time.Minute
	yingzoReleaseMaxBytes     = int64(512 << 20)
)

var yingzoVersionPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+-]{0,63}$`)

type yingzoRelease struct {
	ID               uuid.UUID  `json:"id"`
	Version          string     `json:"version"`
	Status           string     `json:"status"`
	PackageFilename  string     `json:"package_filename"`
	SizeBytes        int64      `json:"size_bytes"`
	SHA256           string     `json:"sha256"`
	Signature        string     `json:"signature,omitempty"`
	MinCodexVersion  string     `json:"min_codex_version"`
	MinClaudeVersion string     `json:"min_claude_version"`
	ReleaseNotes     string     `json:"release_notes,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	PublishedAt      *time.Time `json:"published_at,omitempty"`
	UpdatedAt        time.Time  `json:"updated_at"`
	StorageKey       string     `json:"-"`
}

type yingzoInstallTicket struct {
	ReleaseID uuid.UUID `json:"release_id"`
	UserID    int64     `json:"user_id"`
	Host      string    `json:"host"`
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
	if c.ShouldBindJSON(&input) != nil || (input.Host != "codex" && input.Host != "claude-code") {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_host", "message": "host must be codex or claude-code"}})
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
	if h.yingzoTicketStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "download_ticket_unavailable"}})
		return
	}
	ticket, err := randomToken(24)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "token_generation_failed"}})
		return
	}
	payload, _ := json.Marshal(yingzoInstallTicket{ReleaseID: release.ID, UserID: subject.UserID, Host: input.Host})
	if err := h.yingzoTicketStore.Store(c.Request.Context(), ticket, payload, yingzoTicketTTL); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "download_ticket_unavailable"}})
		return
	}
	origin, err := h.publicOrigin(c)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "yingzo_origin_invalid"}})
		return
	}
	downloadURL := fmt.Sprintf("%s/api/v1/agent/plugin/download/%s/%s", origin, url.PathEscape(ticket), url.PathEscape(release.PackageFilename))
	expiresAt := time.Now().Add(yingzoTicketTTL).UTC()
	c.JSON(http.StatusOK, gin.H{
		"host":         input.Host,
		"version":      release.Version,
		"signature":    release.Signature,
		"download_url": downloadURL,
		"expires_at":   expiresAt,
		"prompt":       yingzoInstallPrompt(input.Host, release, downloadURL),
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
	if err != nil || release.Status != "published" || c.Param("filename") != release.PackageFilename {
		c.Status(http.StatusNotFound)
		return
	}
	file, err := os.Open(release.StorageKey)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer func() { _ = file.Close() }()
	c.Header("Content-Type", "application/gzip")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", release.PackageFilename))
	c.Header("X-Checksum-SHA256", release.SHA256)
	http.ServeContent(c.Writer, c.Request, release.PackageFilename, release.UpdatedAt, file)
}

func (h *AgentHandler) ListYingzoReleases(c *gin.Context) {
	rows, err := h.db.QueryContext(c, `SELECT id,version,status,package_filename,storage_key,size_bytes,sha256,signature,min_codex_version,min_claude_version,release_notes,created_at,published_at,updated_at FROM yingzo_releases ORDER BY created_at DESC`)
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
		items = append(items, release)
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *AgentHandler) UploadYingzoRelease(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "unauthorized"}})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, yingzoReleaseMaxBytes+(2<<20))
	file, header, err := c.Request.FormFile("package")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "package_required"}})
		return
	}
	defer func() { _ = file.Close() }()
	version := strings.TrimSpace(c.PostForm("version"))
	if !yingzoVersionPattern.MatchString(version) {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_version"}})
		return
	}
	packageFilename, err := validateYingzoPackageFilename(header.Filename, version)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_package_filename", "message": err.Error()}})
		return
	}
	id := uuid.New()
	releaseDir := filepath.Join(h.dataDir, "releases")
	if err := os.MkdirAll(releaseDir, 0700); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "storage_error"}})
		return
	}
	temporary := filepath.Join(releaseDir, id.String()+".tmp")
	target := filepath.Join(releaseDir, id.String()+".tar.gz")
	out, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "storage_error"}})
		return
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(out, hash), io.LimitReader(file, yingzoReleaseMaxBytes+1))
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil || size <= 0 || size > yingzoReleaseMaxBytes {
		_ = os.Remove(temporary)
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "package_upload_failed"}})
		return
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	if err := os.Rename(temporary, target); err != nil {
		_ = os.Remove(temporary)
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "storage_error"}})
		return
	}
	if err := validateYingzoArchive(target, packageFilename); err != nil {
		_ = os.Remove(target)
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": gin.H{"code": "invalid_release_archive", "message": err.Error()}})
		return
	}
	_, err = h.db.ExecContext(c, `INSERT INTO yingzo_releases(id,version,status,package_filename,storage_backend,storage_key,size_bytes,sha256,signature,min_codex_version,min_claude_version,release_notes,created_by) VALUES($1,$2,'draft',$3,'local',$4,$5,$6,$7,$8,$9,$10,$11)`, id, version, packageFilename, target, size, digest, strings.TrimSpace(c.PostForm("signature")), strings.TrimSpace(c.PostForm("min_codex_version")), strings.TrimSpace(c.PostForm("min_claude_version")), strings.TrimSpace(c.PostForm("release_notes")), subject.UserID)
	if err != nil {
		_ = os.Remove(target)
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "release_create_failed"}})
		return
	}
	release, _ := h.getYingzoRelease(c, id)
	c.JSON(http.StatusCreated, release)
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
	if _, err = tx.ExecContext(c, `UPDATE yingzo_releases SET status='superseded',updated_at=NOW() WHERE status='published' AND id<>$1`, id); err == nil {
		_, err = tx.ExecContext(c, `UPDATE yingzo_releases SET status='published',published_at=NOW(),published_by=$2,updated_at=NOW() WHERE id=$1`, id, subject.UserID)
	}
	if err != nil || tx.Commit() != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	release, _ := h.getYingzoRelease(c, id)
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
	row := h.db.QueryRowContext(ctx, `SELECT id,version,status,package_filename,storage_key,size_bytes,sha256,signature,min_codex_version,min_claude_version,release_notes,created_at,published_at,updated_at FROM yingzo_releases WHERE status='published' ORDER BY published_at DESC LIMIT 1`)
	return scanYingzoRelease(row)
}

func (h *AgentHandler) getYingzoRelease(ctx context.Context, id uuid.UUID) (*yingzoRelease, error) {
	row := h.db.QueryRowContext(ctx, `SELECT id,version,status,package_filename,storage_key,size_bytes,sha256,signature,min_codex_version,min_claude_version,release_notes,created_at,published_at,updated_at FROM yingzo_releases WHERE id=$1`, id)
	return scanYingzoRelease(row)
}

type yingzoReleaseScanner interface{ Scan(dest ...any) error }

func scanYingzoRelease(scanner yingzoReleaseScanner) (*yingzoRelease, error) {
	var release yingzoRelease
	var signature, minCodex, minClaude, notes sql.NullString
	var published sql.NullTime
	err := scanner.Scan(&release.ID, &release.Version, &release.Status, &release.PackageFilename, &release.StorageKey, &release.SizeBytes, &release.SHA256, &signature, &minCodex, &minClaude, &notes, &release.CreatedAt, &published, &release.UpdatedAt)
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
	return &release, nil
}

func publicYingzoRelease(release *yingzoRelease) gin.H {
	return gin.H{
		"version":            release.Version,
		"size_bytes":         release.SizeBytes,
		"sha256":             release.SHA256,
		"checksum_status":    "verified",
		"signature_status":   map[bool]string{true: "signed", false: "unsigned"}[release.Signature != ""],
		"min_codex_version":  release.MinCodexVersion,
		"min_claude_version": release.MinClaudeVersion,
		"release_notes":      release.ReleaseNotes,
		"published_at":       release.PublishedAt,
	}
}

func yingzoInstallPrompt(host string, release *yingzoRelease, downloadURL string) string {
	archiveRoot := strings.TrimSuffix(release.PackageFilename, ".tar.gz")
	hostReleaseRoot := fmt.Sprintf("~/.yingzo/releases/%s/%s", host, release.Version)
	common := fmt.Sprintf(`请安装或升级 Yingzo（影作）私有插件到版本 %s。下载地址：%s
请把安装包下载到临时文件，解压到 %s，保留现有 ~/.yingzo/auth.json，禁止把凭证写入项目、日志或安装目录。安装完成后验证插件和 MCP 工具，并告诉我结果。`, release.Version, downloadURL, hostReleaseRoot)
	if release.Signature != "" {
		common += "\n发行签名：" + release.Signature + "。同时校验签名；失败立即停止。"
	}
	if host == "codex" {
		return common + fmt.Sprintf(`
这是 Codex 专用安装指令，只执行本段列出的 Codex 命令，也不要修改 ~/.claude/plugins。Codex 会把插件安装到自己的 ~/.codex/plugins 缓存。解压后的 marketplace 目录是 %s/%s/marketplace。若旧版本存在，先运行 codex plugin remove yingzo@yingzo-private --json，再运行 codex plugin marketplace remove yingzo-private --json。然后运行 codex plugin marketplace add <上述 marketplace 绝对路径> --json，最后运行 codex plugin add yingzo@yingzo-private --json。不要要求用户手动打开终端。安装后验证插件清单，并提示用户新建 Codex 任务以加载新工具。`, hostReleaseRoot, archiveRoot)
	}
	return common + fmt.Sprintf(`
这是 Claude Code 专用安装指令，只执行本段列出的 Claude Code 命令，也不要修改 ~/.codex/plugins。Claude Code 会把插件安装到自己的 ~/.claude/plugins 缓存。解压后的 marketplace 目录是 %s/%s/marketplace。若旧版本存在，先运行 claude plugin uninstall yingzo@yingzo-private --scope user --keep-data，再运行 claude plugin marketplace remove yingzo-private --scope user。然后运行 claude plugin marketplace add <上述 marketplace 绝对路径> --scope user，最后运行 claude plugin install yingzo@yingzo-private --scope user。不要要求用户手动打开终端。安装后验证插件清单，并提示用户新建 Claude Code 会话以加载新工具。`, hostReleaseRoot, archiveRoot)
}

func validateYingzoPackageFilename(filename, version string) (string, error) {
	filename = filepath.Base(strings.TrimSpace(filename))
	stable := "yingzo-private-" + version + ".tar.gz"
	legacy := "yingzo-private-beta-" + version + ".tar.gz"
	if filename != stable && filename != legacy {
		return "", fmt.Errorf("package filename must be %s (or legacy %s)", stable, legacy)
	}
	return filename, nil
}

func validateYingzoArchive(filename, packageFilename string) error {
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
	required := map[string]bool{
		root + ".agents/plugins/marketplace.json":         false,
		root + ".claude-plugin/marketplace.json":          false,
		root + "plugins/yingzo/.codex-plugin/plugin.json": false,
		root + "plugins/yingzo/distribution.json":         false,
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
