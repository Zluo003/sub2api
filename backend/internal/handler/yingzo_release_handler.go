package handler

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"debug/pe"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

const (
	yingzoPublicOriginSetting  = "yingzo_public_origin"
	yingzoReleaseStorageEnv    = "YINGZO_RELEASE_STORAGE_DIR"
	yingzoReleasePublicKeyEnv  = "YINGZO_RELEASE_PUBLIC_KEY"
	yingzoReleasePublicKeysEnv = "YINGZO_RELEASE_PUBLIC_KEYS"
	yingzoTicketTTL            = 10 * time.Minute
	yingzoReleaseMaxBytes      = int64(512 << 20)
	yingzoExpandedArchiveMax   = int64(2 << 30)
	yingzoReleaseManifestMax   = int64(2 << 20)
	yingzoDistributionSchema2  = 2
)

const yingzoReleaseColumns = `id,version,status,distribution_schema_version,channel,stable_eligible,runtime_protocol,compatibility,signature,min_codex_version,min_claude_version,release_notes,created_at,published_at,updated_at`
const yingzoArtifactColumns = `id,release_id,host_family,artifact_kind,target,os,arch,format,content_type,runtime_protocol,validation_status,signature_status,validated_at,package_filename,storage_backend,storage_key,size_bytes,sha256,created_at,updated_at`

var yingzoVersionPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+-]{0,63}$`)

type yingzoRelease struct {
	ID                        uuid.UUID                         `json:"id"`
	Version                   string                            `json:"version"`
	Status                    string                            `json:"status"`
	DistributionSchemaVersion int                               `json:"distribution_schema_version"`
	Channel                   string                            `json:"channel"`
	StableEligible            bool                              `json:"stable_eligible"`
	RuntimeProtocol           int                               `json:"runtime_protocol"`
	Compatibility             json.RawMessage                   `json:"compatibility"`
	Signature                 string                            `json:"signature,omitempty"`
	MinCodexVersion           string                            `json:"min_codex_version"`
	MinClaudeVersion          string                            `json:"min_claude_version"`
	ReleaseNotes              string                            `json:"release_notes,omitempty"`
	CreatedAt                 time.Time                         `json:"created_at"`
	PublishedAt               *time.Time                        `json:"published_at,omitempty"`
	UpdatedAt                 time.Time                         `json:"updated_at"`
	Artifacts                 map[string]*yingzoReleaseArtifact `json:"artifacts"`
}

type yingzoReleaseArtifact struct {
	ID               uuid.UUID  `json:"id"`
	ReleaseID        uuid.UUID  `json:"release_id"`
	HostFamily       string     `json:"host_family,omitempty"`
	ArtifactKind     string     `json:"artifact_kind"`
	Target           string     `json:"target"`
	OS               string     `json:"os"`
	Arch             string     `json:"arch"`
	Format           string     `json:"format"`
	ContentType      string     `json:"content_type"`
	RuntimeProtocol  int        `json:"runtime_protocol"`
	ValidationStatus string     `json:"validation_status"`
	SignatureStatus  string     `json:"signature_status"`
	ValidatedAt      *time.Time `json:"validated_at,omitempty"`
	PackageFilename  string     `json:"package_filename"`
	StorageBackend   string     `json:"storage_backend"`
	StorageKey       string     `json:"-"`
	SizeBytes        int64      `json:"size_bytes"`
	SHA256           string     `json:"sha256"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type yingzoInstallTicket struct {
	ReleaseID     uuid.UUID `json:"release_id"`
	ArtifactID    uuid.UUID `json:"artifact_id"`
	UserID        int64     `json:"user_id"`
	Host          string    `json:"host"`
	HostFamily    string    `json:"host_family"`
	Channel       string    `json:"channel"`
	ArtifactKind  string    `json:"artifact_kind"`
	Target        string    `json:"target"`
	OS            string    `json:"os"`
	Arch          string    `json:"arch"`
	RequestedOS   string    `json:"requested_os"`
	RequestedArch string    `json:"requested_arch"`
}

type yingzoArtifactSpec struct {
	ArtifactKind    string
	Target          string
	OS              string
	Arch            string
	Format          string
	ContentType     string
	Filename        string
	RuntimeProtocol int
}

type yingzoReleaseProofEnvelope struct {
	Algorithm       string `json:"algorithm"`
	KeyID           string `json:"key_id"`
	ManifestBase64  string `json:"manifest_base64"`
	SignatureBase64 string `json:"signature_base64"`
	VerifiedAt      string `json:"verified_at"`
}

type yingzoReleaseProofInput struct {
	Algorithm       string `json:"algorithm" binding:"required"`
	KeyID           string `json:"key_id" binding:"required"`
	ManifestBase64  string `json:"manifest_base64" binding:"required"`
	SignatureBase64 string `json:"signature_base64" binding:"required"`
}

type yingzoSignedReleaseManifest struct {
	SchemaVersion          int                            `json:"schema_version"`
	Product                string                         `json:"product"`
	Version                string                         `json:"version"`
	Channel                string                         `json:"channel"`
	RuntimeProtocol        int                            `json:"runtime_protocol"`
	CompleteArtifactMatrix bool                           `json:"complete_artifact_matrix"`
	PublicSigningRequired  bool                           `json:"public_signing_required"`
	StableEligible         bool                           `json:"stable_eligible"`
	NativeSigning          yingzoNativeSigning            `json:"native_signing"`
	Artifacts              []yingzoSignedManifestArtifact `json:"artifacts"`
}

type yingzoNativeSigning struct {
	MacOS   yingzoNativeSigningPlatform `json:"macos"`
	Windows yingzoNativeSigningPlatform `json:"windows"`
}

type yingzoNativeSigningPlatform struct {
	Status string `json:"status"`
}

type yingzoSignedManifestArtifact struct {
	Filename        string `json:"filename"`
	ArtifactKind    string `json:"artifact_kind"`
	Target          string `json:"target"`
	OS              string `json:"os"`
	Arch            string `json:"arch"`
	Format          string `json:"format"`
	ContentType     string `json:"content_type"`
	Bytes           int64  `json:"bytes"`
	SHA256          string `json:"sha256"`
	RuntimeProtocol int    `json:"runtime_protocol"`
}

func (s yingzoArtifactSpec) key() string {
	return strings.Join([]string{s.Target, s.OS, s.Arch}, ":")
}

func (a *yingzoReleaseArtifact) key() string {
	if a.Target != "" {
		return strings.Join([]string{a.Target, a.OS, a.Arch}, ":")
	}
	return a.HostFamily
}

func yingzoV2ArtifactSpecs(version string) []yingzoArtifactSpec {
	return []yingzoArtifactSpec{
		{ArtifactKind: "host_package", Target: "openai", OS: "macos", Arch: "any", Format: "tar.gz", ContentType: "application/gzip", Filename: "yingzo-openai-macos-" + version + ".tar.gz"},
		{ArtifactKind: "host_package", Target: "openai", OS: "windows", Arch: "x64", Format: "zip", ContentType: "application/zip", Filename: "yingzo-openai-windows-x64-" + version + ".zip"},
		{ArtifactKind: "host_package", Target: "claude-code", OS: "macos", Arch: "any", Format: "zip", ContentType: "application/zip", Filename: "yingzo-claude-code-macos-" + version + ".zip"},
		{ArtifactKind: "host_package", Target: "claude-code", OS: "windows", Arch: "x64", Format: "zip", ContentType: "application/zip", Filename: "yingzo-claude-code-windows-x64-" + version + ".zip"},
		{ArtifactKind: "host_package", Target: "claude-desktop", OS: "any", Arch: "any", Format: "mcpb", ContentType: "application/zip", Filename: "yingzo-claude-desktop-" + version + ".mcpb"},
		{ArtifactKind: "runtime_installer", Target: "runtime", OS: "macos", Arch: "arm64", Format: "dmg", ContentType: "application/x-apple-diskimage", Filename: "yingzo-runtime-macos-arm64-" + version + ".dmg"},
		{ArtifactKind: "runtime_installer", Target: "runtime", OS: "macos", Arch: "x64", Format: "dmg", ContentType: "application/x-apple-diskimage", Filename: "yingzo-runtime-macos-x64-" + version + ".dmg"},
		{ArtifactKind: "runtime_installer", Target: "runtime", OS: "windows", Arch: "x64", Format: "exe", ContentType: "application/vnd.microsoft.portable-executable", Filename: "yingzo-runtime-windows-x64-" + version + "-setup.exe"},
	}
}

func findYingzoV2Spec(version, artifactKind, target, targetOS, arch string) (yingzoArtifactSpec, bool) {
	for _, spec := range yingzoV2ArtifactSpecs(version) {
		if spec.ArtifactKind == artifactKind && spec.Target == target && spec.OS == targetOS && spec.Arch == arch {
			return spec, true
		}
	}
	return yingzoArtifactSpec{}, false
}

func (h *AgentHandler) GetYingzoDiscovery(c *gin.Context) {
	origin, err := h.publicOrigin(c)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "yingzo_origin_invalid"}})
		return
	}
	release, err := h.latestPublishedYingzoRelease(c, "stable")
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	response := gin.H{
		"schema_version":    2,
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
	channel, err := normalizeYingzoChannel(c.Query("channel"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_channel", "message": err.Error()}})
		return
	}
	release, err := h.latestPublishedYingzoRelease(c, channel)
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

type yingzoInstallRequest struct {
	Host                     string `json:"host" binding:"required"`
	OS                       string `json:"os"`
	Arch                     string `json:"arch"`
	Channel                  string `json:"channel"`
	InstalledRuntimeVersion  string `json:"installed_runtime_version"`
	InstalledRuntimeProtocol int    `json:"installed_runtime_protocol"`
	RuntimeCapability        string `json:"runtime_capability"`
}

func (h *AgentHandler) CreateYingzoInstallInstructions(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "unauthorized"}})
		return
	}
	var input yingzoInstallRequest
	if c.ShouldBindJSON(&input) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request"}})
		return
	}
	hostFamily, hostErr := yingzoHostFamily(input.Host)
	if hostErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_host", "message": hostErr.Error()}})
		return
	}
	channel, err := normalizeYingzoChannel(input.Channel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_channel", "message": err.Error()}})
		return
	}
	release, err := h.latestPublishedYingzoRelease(c, channel)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "yingzo_release_not_found"}})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
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

	if release.DistributionSchemaVersion == 1 {
		if input.Host == "claude-chat" {
			c.JSON(http.StatusConflict, gin.H{"error": gin.H{
				"code":    "host_not_supported_by_release",
				"message": "Claude Desktop requires a schema 2 release with an MCPB artifact",
			}})
			return
		}
		hostArtifact := yingzoArtifactForFamily(release, hostFamily)
		if hostArtifact == nil {
			c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "release_artifact_unavailable"}})
			return
		}
		descriptor, ticketErr := h.createYingzoDownloadDescriptor(c, origin, release, hostArtifact, subject.UserID, input.Host, hostFamily, channel, "any", "any")
		if ticketErr != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "download_ticket_unavailable"}})
			return
		}
		downloadURL := descriptor["download_url"].(string)
		c.JSON(http.StatusOK, gin.H{
			"host": input.Host, "host_family": hostFamily, "channel": channel,
			"version": release.Version, "signature": release.Signature, "stable_eligible": release.StableEligible,
			"download_url": downloadURL, "expires_at": descriptor["expires_at"],
			"host_package": descriptor, "runtime_installer": nil, "runtime_installer_required": false,
			"runtime_protocol": 0, "prompt": yingzoInstallPrompt(input.Host, release, hostArtifact, downloadURL),
		})
		return
	}

	targetOS, osErr := normalizeYingzoOS(input.OS)
	arch, archErr := normalizeYingzoArch(input.Arch)
	if osErr != nil || archErr != nil || !yingzoPlatformSupported(targetOS, arch) {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "unsupported_platform", "message": "supported platforms are macos/arm64, macos/x64, and windows/x64"}})
		return
	}
	hostArtifact := yingzoHostArtifactForPlatform(release, input.Host, targetOS, arch)
	if hostArtifact == nil {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "release_artifact_unavailable"}})
		return
	}
	runtimeResolution, resolutionErr := resolveYingzoRuntimeCapability(input, release)
	if resolutionErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_runtime_capability", "message": resolutionErr.Error()}})
		return
	}
	runtimeRequired := runtimeResolution == "required"
	var runtimeArtifact *yingzoReleaseArtifact
	if runtimeResolution != "compatible" {
		runtimeArtifact = yingzoArtifactForTuple(release, "runtime", targetOS, arch)
		if runtimeArtifact == nil {
			c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "runtime_artifact_unavailable"}})
			return
		}
	}
	hostDescriptor, ticketErr := h.createYingzoDownloadDescriptor(c, origin, release, hostArtifact, subject.UserID, input.Host, hostFamily, channel, targetOS, arch)
	if ticketErr != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "download_ticket_unavailable"}})
		return
	}
	var runtimeDescriptor gin.H
	if runtimeArtifact != nil {
		runtimeDescriptor, ticketErr = h.createYingzoDownloadDescriptor(c, origin, release, runtimeArtifact, subject.UserID, input.Host, hostFamily, channel, targetOS, arch)
		if ticketErr != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "download_ticket_unavailable"}})
			return
		}
	}
	hostURL := hostDescriptor["download_url"].(string)
	runtimeHelperURI := ""
	if runtimeDescriptor != nil && runtimeResolution == "probe" {
		installerURL, _ := runtimeDescriptor["download_url"].(string)
		runtimeHelperURI = yingzoRuntimeEnsureURI(release, installerURL)
	}
	c.JSON(http.StatusOK, gin.H{
		"host": input.Host, "host_family": hostFamily, "channel": channel,
		"os": targetOS, "arch": arch, "version": release.Version, "signature": release.Signature,
		"stable_eligible": release.StableEligible, "native_signing": yingzoNativeSigningSummary(release), "warning": yingzoReleaseWarning(release),
		"runtime_protocol": release.RuntimeProtocol, "download_url": hostURL,
		"expires_at": hostDescriptor["expires_at"], "host_package": hostDescriptor,
		"runtime_installer": runtimeDescriptor, "runtime_installer_required": runtimeRequired,
		"runtime_resolution": runtimeResolution, "runtime_helper_uri": runtimeHelperURI,
		"prompt": yingzoV2InstallPrompt(input.Host, release, hostURL, runtimeDescriptor, runtimeResolution),
	})
}

func (h *AgentHandler) createYingzoDownloadDescriptor(c *gin.Context, origin string, release *yingzoRelease, artifact *yingzoReleaseArtifact, userID int64, host, hostFamily, channel, requestedOS, requestedArch string) (gin.H, error) {
	ticket, err := randomToken(24)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(yingzoInstallTicket{
		ReleaseID: release.ID, ArtifactID: artifact.ID, UserID: userID, Host: host,
		HostFamily: hostFamily, Channel: channel, ArtifactKind: artifact.ArtifactKind,
		Target: artifact.Target, OS: artifact.OS, Arch: artifact.Arch, RequestedOS: requestedOS, RequestedArch: requestedArch,
	})
	if err != nil {
		return nil, err
	}
	if err := h.yingzoTicketStore.Store(c.Request.Context(), ticket, payload, yingzoTicketTTL); err != nil {
		return nil, err
	}
	expiresAt := time.Now().Add(yingzoTicketTTL).UTC()
	downloadURL := fmt.Sprintf("%s/api/v1/agent/plugin/download/%s/%s", origin, url.PathEscape(ticket), url.PathEscape(artifact.PackageFilename))
	return gin.H{
		"artifact_id": artifact.ID, "artifact_kind": artifact.ArtifactKind, "target": artifact.Target,
		"os": artifact.OS, "arch": artifact.Arch, "filename": artifact.PackageFilename,
		"requested_os": requestedOS, "requested_arch": requestedArch,
		"content_type": artifact.ContentType, "size_bytes": artifact.SizeBytes, "signature_status": artifact.SignatureStatus,
		"download_url": downloadURL, "expires_at": expiresAt, "authorization_model": "short_ttl_bearer_url",
	}, nil
}

func (h *AgentHandler) DownloadYingzoRelease(c *gin.Context) {
	if h.yingzoTicketStore == nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	raw, err := h.yingzoTicketStore.Get(c.Request.Context(), c.Param("ticket"))
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
	ticketChannel := ticket.Channel
	if ticketChannel == "" {
		ticketChannel = "stable"
	}
	if err != nil || release.Status != "published" || release.Channel != ticketChannel {
		c.Status(http.StatusNotFound)
		return
	}
	artifact := yingzoArtifactByID(release, ticket.ArtifactID)
	if artifact == nil || c.Param("filename") != artifact.PackageFilename {
		c.Status(http.StatusNotFound)
		return
	}
	if !yingzoTicketMatchesArtifact(ticket, release, artifact) {
		c.Status(http.StatusNotFound)
		return
	}
	file, err := os.Open(artifact.StorageKey)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != artifact.SizeBytes {
		c.Status(http.StatusNotFound)
		return
	}
	contentType := artifact.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", artifact.PackageFilename))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, no-store")
	http.ServeContent(c.Writer, c.Request, artifact.PackageFilename, artifact.UpdatedAt, file)
}

func yingzoTicketMatchesArtifact(ticket yingzoInstallTicket, release *yingzoRelease, artifact *yingzoReleaseArtifact) bool {
	family, err := yingzoHostFamily(ticket.Host)
	if err != nil || family != ticket.HostFamily {
		return false
	}
	if release.DistributionSchemaVersion == 1 {
		return artifact.HostFamily == family || artifact.HostFamily == "combined"
	}
	if ticket.ArtifactKind != artifact.ArtifactKind || ticket.Target != artifact.Target || ticket.OS != artifact.OS || ticket.Arch != artifact.Arch {
		return false
	}
	if artifact.ArtifactKind == "runtime_installer" {
		return artifact.Target == "runtime" && artifact.OS == ticket.RequestedOS && artifact.Arch == ticket.RequestedArch
	}
	requestedOS := normalizeYingzoOSTrusted(ticket.RequestedOS)
	requestedArch := normalizeYingzoArchTrusted(ticket.RequestedArch)
	if !yingzoPlatformSupported(requestedOS, requestedArch) {
		return false
	}
	expected := yingzoHostArtifactForPlatform(release, ticket.Host, requestedOS, requestedArch)
	return expected != nil && expected.ID == artifact.ID
}

func (h *AgentHandler) ListYingzoReleases(c *gin.Context) {
	rows, err := h.db.QueryContext(c, `SELECT `+yingzoReleaseColumns+` FROM yingzo_releases ORDER BY created_at DESC`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	defer func() { _ = rows.Close() }()
	items := make([]*yingzoRelease, 0)
	for rows.Next() {
		release, scanErr := scanYingzoRelease(rows)
		if scanErr != nil || h.loadYingzoArtifacts(c, release) != nil {
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

type yingzoReleaseDraftInput struct {
	Version                   string         `json:"version" binding:"required"`
	Channel                   string         `json:"channel" binding:"required"`
	DistributionSchemaVersion int            `json:"distribution_schema_version" binding:"required"`
	RuntimeProtocol           int            `json:"runtime_protocol" binding:"required"`
	Compatibility             map[string]any `json:"compatibility"`
	MinCodexVersion           string         `json:"min_codex_version"`
	MinClaudeVersion          string         `json:"min_claude_version"`
	ReleaseNotes              string         `json:"release_notes"`
}

func (h *AgentHandler) UploadYingzoRelease(c *gin.Context) {
	if strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "application/json") {
		h.createYingzoReleaseDraft(c)
		return
	}
	h.uploadLegacyYingzoRelease(c)
}

func (h *AgentHandler) createYingzoReleaseDraft(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "unauthorized"}})
		return
	}
	var input yingzoReleaseDraftInput
	if c.ShouldBindJSON(&input) != nil || !yingzoVersionPattern.MatchString(strings.TrimSpace(input.Version)) || input.DistributionSchemaVersion != yingzoDistributionSchema2 || input.RuntimeProtocol <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_release_draft"}})
		return
	}
	channel, err := normalizeYingzoChannel(input.Channel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_channel", "message": err.Error()}})
		return
	}
	compatibility := input.Compatibility
	if compatibility == nil {
		compatibility = map[string]any{}
	}
	compatibilityJSON, err := json.Marshal(compatibility)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_compatibility"}})
		return
	}
	releaseID := uuid.New()
	_, err = h.db.ExecContext(c, `INSERT INTO yingzo_releases(id,version,status,distribution_schema_version,channel,stable_eligible,runtime_protocol,compatibility,signature,min_codex_version,min_claude_version,release_notes,created_by) VALUES($1,$2,'draft',2,$3,FALSE,$4,$5::jsonb,NULL,$6,$7,$8,$9)`,
		releaseID, strings.TrimSpace(input.Version), channel, input.RuntimeProtocol, string(compatibilityJSON), strings.TrimSpace(input.MinCodexVersion), strings.TrimSpace(input.MinClaudeVersion), strings.TrimSpace(input.ReleaseNotes), subject.UserID)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "yingzo_release_version_exists"}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	release, err := h.getYingzoRelease(c, releaseID)
	if err != nil {
		c.JSON(http.StatusCreated, gin.H{"id": releaseID, "version": input.Version, "status": "draft", "channel": channel, "distribution_schema_version": 2})
		return
	}
	c.JSON(http.StatusCreated, release)
}

func (h *AgentHandler) uploadLegacyYingzoRelease(c *gin.Context) {
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
	openAIArtifact, err := h.storeLegacyYingzoArtifact(openAIFile, openAIHeader.Filename, releaseID, version, "openai")
	if err != nil {
		h.removeYingzoReleaseDirectory(releaseID)
		writeYingzoUploadError(c, err)
		return
	}
	claudeArtifact, err := h.storeLegacyYingzoArtifact(claudeFile, claudeHeader.Filename, releaseID, version, "claude")
	if err != nil {
		h.removeYingzoReleaseDirectory(releaseID)
		writeYingzoUploadError(c, err)
		return
	}
	tx, err := h.db.BeginTx(c, &sql.TxOptions{})
	if err != nil {
		h.removeYingzoReleaseDirectory(releaseID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(c, `INSERT INTO yingzo_releases(id,version,status,signature,min_codex_version,min_claude_version,release_notes,created_by) VALUES($1,$2,'draft',$3,$4,$5,$6,$7)`, releaseID, version, strings.TrimSpace(c.PostForm("signature")), strings.TrimSpace(c.PostForm("min_codex_version")), strings.TrimSpace(c.PostForm("min_claude_version")), strings.TrimSpace(c.PostForm("release_notes")), subject.UserID)
	if err == nil {
		for _, artifact := range []*yingzoReleaseArtifact{openAIArtifact, claudeArtifact} {
			_, err = tx.ExecContext(c, `INSERT INTO yingzo_release_artifacts(id,release_id,host_family,artifact_kind,target,os,arch,format,content_type,runtime_protocol,validation_status,signature_status,validated_at,package_filename,storage_backend,storage_key,size_bytes,sha256) VALUES($1,$2,$3,'host_package',$3,'any','any','tar.gz','application/gzip',0,'validated','unverified',NOW(),$4,$5,$6,$7,$8)`, artifact.ID, artifact.ReleaseID, artifact.HostFamily, artifact.PackageFilename, artifact.StorageBackend, artifact.StorageKey, artifact.SizeBytes, artifact.SHA256)
			if err != nil {
				break
			}
		}
	}
	if err != nil {
		h.removeYingzoReleaseDirectory(releaseID)
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "yingzo_release_version_exists", "message": fmt.Sprintf("Yingzo version %s already exists", version)}})
			return
		}
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "release_create_failed"}})
		return
	}
	if err := tx.Commit(); err != nil {
		// A failed Commit response is not proof that PostgreSQL rolled back.
		// Preserve files so a committed row never points at deleted storage.
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

func (h *AgentHandler) UploadYingzoReleaseArtifact(c *gin.Context) {
	h.saveYingzoReleaseArtifact(c, false)
}

func (h *AgentHandler) ReplaceYingzoReleaseArtifact(c *gin.Context) {
	h.saveYingzoReleaseArtifact(c, true)
}

func (h *AgentHandler) VerifyYingzoReleaseProof(c *gin.Context) {
	releaseID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "yingzo_release_not_found"}})
		return
	}
	var input yingzoReleaseProofInput
	if c.ShouldBindJSON(&input) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_release_proof"}})
		return
	}
	tx, err := h.db.BeginTx(c, &sql.TxOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	defer func() { _ = tx.Rollback() }()
	release, err := getYingzoReleaseFrom(c, tx, releaseID, true)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "yingzo_release_not_found"}})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	if release.Status != "draft" || release.DistributionSchemaVersion != yingzoDistributionSchema2 {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "release_state_invalid"}})
		return
	}
	envelope, manifest, verifyErr := verifyYingzoReleaseProof(release, input)
	if verifyErr != nil {
		var uploadErr *yingzoUploadError
		if errors.As(verifyErr, &uploadErr) {
			c.JSON(uploadErr.status, gin.H{"error": gin.H{"code": uploadErr.code, "message": uploadErr.message}})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "release_proof_verification_failed"}})
		}
		return
	}
	envelopeJSON, _ := json.Marshal(envelope)
	if _, err = tx.ExecContext(c, `UPDATE yingzo_releases SET signature=$2,stable_eligible=$3,updated_at=NOW() WHERE id=$1 AND status='draft'`, releaseID, string(envelopeJSON), manifest.StableEligible); err == nil {
		// The signed manifest attests the macOS release pipeline. Windows-bearing
		// artifacts retain the Authenticode status detected from their PE payloads.
		_, err = tx.ExecContext(c, `UPDATE yingzo_release_artifacts SET signature_status='verified',validated_at=NOW(),updated_at=NOW() WHERE release_id=$1 AND NOT (os='windows' OR target='claude-desktop')`, releaseID)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"verified": true, "key_id": envelope.KeyID, "verified_at": envelope.VerifiedAt, "stable_eligible": manifest.StableEligible, "native_signing": manifest.NativeSigning})
}

func (h *AgentHandler) saveYingzoReleaseArtifact(c *gin.Context, replace bool) {
	releaseID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "yingzo_release_not_found"}})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, yingzoReleaseMaxBytes+(2<<20))
	artifactKind := strings.TrimSpace(c.PostForm("artifact_kind"))
	target := strings.TrimSpace(c.PostForm("target"))
	targetOS := strings.TrimSpace(c.PostForm("os"))
	arch := strings.TrimSpace(c.PostForm("arch"))
	runtimeProtocol, parseErr := parsePositiveInt(c.PostForm("runtime_protocol"))
	if parseErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "runtime_protocol_mismatch"}})
		return
	}
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "artifact_file_required"}})
		return
	}
	defer func() { _ = file.Close() }()

	var artifactID uuid.UUID
	if replace {
		artifactID, err = uuid.Parse(c.Param("artifact_id"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "release_artifact_not_found"}})
			return
		}
	}
	tx, err := h.db.BeginTx(c, &sql.TxOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	defer func() { _ = tx.Rollback() }()
	release, err := getYingzoReleaseFrom(c, tx, releaseID, true)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "yingzo_release_not_found"}})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	if release.Status != "draft" || release.DistributionSchemaVersion != 2 {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "release_state_invalid"}})
		return
	}
	spec, ok := findYingzoV2Spec(release.Version, artifactKind, target, targetOS, arch)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_artifact_target"}})
		return
	}
	if runtimeProtocol != release.RuntimeProtocol {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "runtime_protocol_mismatch"}})
		return
	}
	spec.RuntimeProtocol = release.RuntimeProtocol
	var existing *yingzoReleaseArtifact
	if replace {
		existing = yingzoArtifactByID(release, artifactID)
		if existing == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "release_artifact_not_found"}})
			return
		}
		if existing.ArtifactKind != artifactKind || existing.Target != target || existing.OS != targetOS || existing.Arch != arch {
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "artifact_dimensions_immutable"}})
			return
		}
	} else if yingzoArtifactForTuple(release, target, targetOS, arch) != nil {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "release_artifact_exists"}})
		return
	}

	artifact, err := h.storeYingzoV2Artifact(file, header.Filename, release, spec)
	if err != nil {
		writeYingzoUploadError(c, err)
		return
	}
	if _, err = tx.ExecContext(c, `UPDATE yingzo_releases SET signature=NULL,stable_eligible=FALSE,updated_at=NOW() WHERE id=$1`, release.ID); err == nil {
		// Invalidating the proof removes its macOS attestation. PE inspection is
		// independent, so preserve the status of Windows-bearing artifacts.
		_, err = tx.ExecContext(c, `UPDATE yingzo_release_artifacts SET signature_status='unverified',updated_at=NOW() WHERE release_id=$1 AND NOT (os='windows' OR target='claude-desktop')`, release.ID)
	}
	if err == nil && replace {
		_, err = tx.ExecContext(c, `UPDATE yingzo_release_artifacts SET package_filename=$2,storage_backend=$3,storage_key=$4,size_bytes=$5,sha256=$6,content_type=$7,format=$8,runtime_protocol=$9,validation_status='validated',signature_status=$10,validated_at=NOW(),updated_at=NOW() WHERE id=$1 AND release_id=$11`, existing.ID, artifact.PackageFilename, artifact.StorageBackend, artifact.StorageKey, artifact.SizeBytes, artifact.SHA256, artifact.ContentType, artifact.Format, artifact.RuntimeProtocol, artifact.SignatureStatus, release.ID)
	} else if err == nil {
		_, err = tx.ExecContext(c, `INSERT INTO yingzo_release_artifacts(id,release_id,artifact_kind,target,os,arch,format,content_type,runtime_protocol,validation_status,signature_status,validated_at,package_filename,storage_backend,storage_key,size_bytes,sha256) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'validated',$10,NOW(),$11,$12,$13,$14,$15)`, artifact.ID, artifact.ReleaseID, artifact.ArtifactKind, artifact.Target, artifact.OS, artifact.Arch, artifact.Format, artifact.ContentType, artifact.RuntimeProtocol, artifact.SignatureStatus, artifact.PackageFilename, artifact.StorageBackend, artifact.StorageKey, artifact.SizeBytes, artifact.SHA256)
	}
	if err != nil {
		h.removeYingzoStorageFile(release.ID, artifact.StorageKey)
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "release_artifact_exists"}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	if err := tx.Commit(); err != nil {
		// Commit errors are ambiguous: PostgreSQL may have committed before the
		// connection failed. Keep the new file and let reconciliation decide.
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	if replace {
		h.removeYingzoStorageFile(release.ID, existing.StorageKey)
		if refreshed, refreshErr := h.getYingzoRelease(c, release.ID); refreshErr == nil {
			c.JSON(http.StatusOK, yingzoArtifactByID(refreshed, existing.ID))
			return
		}
		artifact.ID = existing.ID
		c.JSON(http.StatusOK, artifact)
		return
	}
	c.JSON(http.StatusCreated, artifact)
}

func (h *AgentHandler) DeleteYingzoReleaseArtifact(c *gin.Context) {
	releaseID, err := uuid.Parse(c.Param("id"))
	artifactID, artifactErr := uuid.Parse(c.Param("artifact_id"))
	if err != nil || artifactErr != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "release_artifact_not_found"}})
		return
	}
	tx, err := h.db.BeginTx(c, &sql.TxOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	defer func() { _ = tx.Rollback() }()
	release, err := getYingzoReleaseFrom(c, tx, releaseID, true)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "yingzo_release_not_found"}})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	if release.Status != "draft" || release.DistributionSchemaVersion != 2 {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "release_state_invalid"}})
		return
	}
	artifact := yingzoArtifactByID(release, artifactID)
	if artifact == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "release_artifact_not_found"}})
		return
	}
	result, err := tx.ExecContext(c, `DELETE FROM yingzo_release_artifacts WHERE id=$1 AND release_id=$2`, artifactID, releaseID)
	if err == nil {
		_, err = tx.ExecContext(c, `UPDATE yingzo_releases SET signature=NULL,stable_eligible=FALSE,updated_at=NOW() WHERE id=$1`, releaseID)
	}
	if err == nil {
		_, err = tx.ExecContext(c, `UPDATE yingzo_release_artifacts SET signature_status='unverified',updated_at=NOW() WHERE release_id=$1 AND NOT (os='windows' OR target='claude-desktop')`, releaseID)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "release_artifact_not_found"}})
		return
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	h.removeYingzoStorageFile(releaseID, artifact.StorageKey)
	c.JSON(http.StatusOK, gin.H{"deleted": true})
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

func (h *AgentHandler) storeLegacyYingzoArtifact(reader io.Reader, originalFilename string, releaseID uuid.UUID, version, hostFamily string) (*yingzoReleaseArtifact, error) {
	packageFilename, err := validateYingzoPackageFilename(originalFilename, version, hostFamily)
	if err != nil {
		return nil, &yingzoUploadError{status: http.StatusBadRequest, code: "invalid_package_filename", message: err.Error()}
	}
	artifact := &yingzoReleaseArtifact{ID: uuid.New(), ReleaseID: releaseID, HostFamily: hostFamily, ArtifactKind: "host_package", Target: hostFamily, OS: "any", Arch: "any", Format: "tar.gz", ContentType: "application/gzip", PackageFilename: packageFilename, StorageBackend: "local", ValidationStatus: "validated", SignatureStatus: "unverified"}
	if err := h.writeYingzoArtifact(reader, artifact); err != nil {
		return nil, err
	}
	if err := validateYingzoArchive(artifact.StorageKey, packageFilename, hostFamily); err != nil {
		h.removeYingzoStorageFile(releaseID, artifact.StorageKey)
		return nil, &yingzoUploadError{status: http.StatusUnprocessableEntity, code: "invalid_release_archive", message: err.Error()}
	}
	return artifact, nil
}

func (h *AgentHandler) storeYingzoV2Artifact(reader io.Reader, originalFilename string, release *yingzoRelease, spec yingzoArtifactSpec) (*yingzoReleaseArtifact, error) {
	packageFilename := filepath.Base(strings.TrimSpace(originalFilename))
	if packageFilename != spec.Filename {
		return nil, &yingzoUploadError{status: http.StatusBadRequest, code: "invalid_package_filename", message: "package filename must be " + spec.Filename}
	}
	artifact := &yingzoReleaseArtifact{ID: uuid.New(), ReleaseID: release.ID, ArtifactKind: spec.ArtifactKind, Target: spec.Target, OS: spec.OS, Arch: spec.Arch, Format: spec.Format, ContentType: spec.ContentType, RuntimeProtocol: release.RuntimeProtocol, ValidationStatus: "validated", SignatureStatus: "unverified", PackageFilename: packageFilename, StorageBackend: "local"}
	if err := h.writeYingzoArtifact(reader, artifact); err != nil {
		return nil, err
	}
	windowsSigned, err := inspectYingzoV2Artifact(artifact.StorageKey, spec)
	if err != nil {
		h.removeYingzoStorageFile(release.ID, artifact.StorageKey)
		return nil, &yingzoUploadError{status: http.StatusUnprocessableEntity, code: "invalid_release_artifact", message: err.Error()}
	}
	if yingzoWindowsBearingSpec(spec) && windowsSigned {
		artifact.SignatureStatus = "verified"
	}
	if release.Channel == "stable" && yingzoWindowsBearingSpec(spec) && !windowsSigned {
		h.removeYingzoStorageFile(release.ID, artifact.StorageKey)
		return nil, &yingzoUploadError{status: http.StatusUnprocessableEntity, code: "unsigned_windows_artifact", message: "stable releases require Authenticode-signed Windows artifacts"}
	}
	now := time.Now().UTC()
	artifact.ValidatedAt = &now
	return artifact, nil
}

func (h *AgentHandler) writeYingzoArtifact(reader io.Reader, artifact *yingzoReleaseArtifact) error {
	releaseDir, err := h.yingzoReleaseDirectory(artifact.ReleaseID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(releaseDir, 0700); err != nil {
		return err
	}
	extension := ".bin"
	if artifact.Format != "" {
		extension = "." + artifact.Format
	}
	temporary := filepath.Join(releaseDir, artifact.ID.String()+".tmp")
	target := filepath.Join(releaseDir, artifact.ID.String()+extension)
	out, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(out, hash), io.LimitReader(reader, yingzoReleaseMaxBytes+1))
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil || size <= 0 || size > yingzoReleaseMaxBytes {
		_ = os.Remove(temporary)
		return &yingzoUploadError{status: http.StatusBadRequest, code: "package_upload_failed", message: "package is empty, incomplete, or exceeds 512 MB"}
	}
	if err := os.Rename(temporary, target); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	artifact.StorageKey = target
	artifact.SizeBytes = size
	artifact.SHA256 = hex.EncodeToString(hash.Sum(nil))
	return nil
}

func (h *AgentHandler) PublishYingzoRelease(c *gin.Context)  { h.setPublishedYingzoRelease(c, false) }
func (h *AgentHandler) RollbackYingzoRelease(c *gin.Context) { h.setPublishedYingzoRelease(c, true) }

func verifyYingzoReleaseProof(release *yingzoRelease, input yingzoReleaseProofInput) (*yingzoReleaseProofEnvelope, *yingzoSignedReleaseManifest, error) {
	if strings.TrimSpace(input.Algorithm) != "Ed25519" {
		return nil, nil, &yingzoUploadError{status: http.StatusUnprocessableEntity, code: "release_proof_invalid", message: "release proof algorithm must be Ed25519"}
	}
	keys, err := loadYingzoReleasePublicKeys()
	if err != nil {
		return nil, nil, &yingzoUploadError{status: http.StatusServiceUnavailable, code: "release_proof_key_unavailable", message: err.Error()}
	}
	keyID := strings.TrimSpace(input.KeyID)
	publicKey, ok := keys[keyID]
	if !ok {
		return nil, nil, &yingzoUploadError{status: http.StatusUnprocessableEntity, code: "release_proof_key_unknown", message: "release proof key_id is not configured"}
	}
	manifestBytes, err := decodeYingzoBase64(input.ManifestBase64)
	if err != nil || len(manifestBytes) == 0 || int64(len(manifestBytes)) > yingzoReleaseManifestMax {
		return nil, nil, &yingzoUploadError{status: http.StatusBadRequest, code: "invalid_release_manifest", message: "release manifest must be valid base64 and no larger than 2 MB"}
	}
	signature, err := decodeYingzoBase64(input.SignatureBase64)
	if err != nil || len(signature) != ed25519.SignatureSize || !ed25519.Verify(publicKey, manifestBytes, signature) {
		return nil, nil, &yingzoUploadError{status: http.StatusUnprocessableEntity, code: "release_proof_invalid", message: "Ed25519 release proof verification failed"}
	}
	var manifest yingzoSignedReleaseManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, nil, &yingzoUploadError{status: http.StatusBadRequest, code: "invalid_release_manifest", message: "release manifest is not valid JSON"}
	}
	if err := validateYingzoSignedManifest(release, manifest); err != nil {
		return nil, nil, err
	}
	return &yingzoReleaseProofEnvelope{
		Algorithm: "Ed25519", KeyID: keyID, ManifestBase64: base64.StdEncoding.EncodeToString(manifestBytes),
		SignatureBase64: base64.StdEncoding.EncodeToString(signature), VerifiedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, &manifest, nil
}

func validateYingzoSignedManifest(release *yingzoRelease, manifest yingzoSignedReleaseManifest) error {
	if release == nil || manifest.SchemaVersion != 2 || manifest.Product != "yingzo" || manifest.Version != release.Version || manifest.RuntimeProtocol != release.RuntimeProtocol || !manifest.CompleteArtifactMatrix {
		return &yingzoUploadError{status: http.StatusUnprocessableEntity, code: "release_manifest_mismatch", message: "release manifest identity, protocol, or signing policy does not match the draft"}
	}
	if manifest.Channel != "prerelease" && manifest.Channel != "stable" {
		return &yingzoUploadError{status: http.StatusUnprocessableEntity, code: "release_manifest_mismatch", message: "public release manifest must identify a prerelease or stable build"}
	}
	if release == nil || manifest.Channel != release.Channel {
		return &yingzoUploadError{status: http.StatusUnprocessableEntity, code: "release_manifest_mismatch", message: "release manifest channel does not match the draft"}
	}
	macOSSigning := strings.TrimSpace(manifest.NativeSigning.MacOS.Status)
	windowsSigning := strings.TrimSpace(manifest.NativeSigning.Windows.Status)
	if macOSSigning != "verified" || (windowsSigning != "verified" && windowsSigning != "unsigned") {
		return &yingzoUploadError{status: http.StatusUnprocessableEntity, code: "release_native_signing_invalid", message: "macOS signing must be verified and Windows signing must be verified or unsigned"}
	}
	fullySigned := macOSSigning == "verified" && windowsSigning == "verified"
	if manifest.StableEligible != fullySigned || manifest.PublicSigningRequired != fullySigned {
		return &yingzoUploadError{status: http.StatusUnprocessableEntity, code: "release_manifest_mismatch", message: "stable_eligible and public_signing_required must match the native signing status"}
	}
	if !fullySigned && release.Channel != "prerelease" {
		return &yingzoUploadError{status: http.StatusConflict, code: "release_not_stable_eligible", message: "unsigned Windows artifacts are allowed only in prerelease releases"}
	}
	expectedSpecs := yingzoV2ArtifactSpecs(release.Version)
	if len(manifest.Artifacts) != len(expectedSpecs) || len(release.Artifacts) != len(expectedSpecs) {
		return &yingzoUploadError{status: http.StatusConflict, code: "release_artifacts_incomplete", message: "release proof requires the exact eight-artifact matrix"}
	}
	seen := make(map[string]bool, len(expectedSpecs))
	windowsUnsignedDetected := false
	for _, signed := range manifest.Artifacts {
		spec, ok := findYingzoV2Spec(release.Version, signed.ArtifactKind, signed.Target, signed.OS, signed.Arch)
		if !ok || seen[spec.key()] {
			return &yingzoUploadError{status: http.StatusUnprocessableEntity, code: "release_manifest_mismatch", message: "release manifest contains an unknown or duplicate artifact"}
		}
		seen[spec.key()] = true
		artifact := yingzoArtifactForTuple(release, signed.Target, signed.OS, signed.Arch)
		if artifact == nil || signed.Filename != spec.Filename || signed.Format != spec.Format || signed.ContentType != spec.ContentType || signed.RuntimeProtocol != release.RuntimeProtocol || signed.Bytes != artifact.SizeBytes || !strings.EqualFold(signed.SHA256, artifact.SHA256) {
			return &yingzoUploadError{status: http.StatusUnprocessableEntity, code: "release_manifest_mismatch", message: "release manifest artifact metadata does not match the uploaded file"}
		}
		if yingzoWindowsBearingSpec(spec) {
			if artifact.SignatureStatus == "failed" {
				return &yingzoUploadError{status: http.StatusUnprocessableEntity, code: "release_native_signing_invalid", message: "a Windows-bearing artifact has a failed native signature"}
			}
			if artifact.SignatureStatus != "verified" {
				windowsUnsignedDetected = true
			}
		}
		info, statErr := os.Stat(artifact.StorageKey)
		if statErr != nil || !info.Mode().IsRegular() || info.Size() != artifact.SizeBytes {
			return &yingzoUploadError{status: http.StatusConflict, code: "release_artifact_file_missing", message: "a release artifact file is missing from local storage"}
		}
		actualSHA, hashErr := yingzoFileSHA256(artifact.StorageKey)
		if hashErr != nil || !strings.EqualFold(actualSHA, artifact.SHA256) {
			return &yingzoUploadError{status: http.StatusConflict, code: "release_artifact_integrity_failed", message: "a release artifact no longer matches its uploaded checksum"}
		}
	}
	if (windowsSigning == "unsigned") != windowsUnsignedDetected {
		return &yingzoUploadError{status: http.StatusUnprocessableEntity, code: "release_manifest_mismatch", message: "Windows native signing status does not match the uploaded artifacts"}
	}
	return nil
}

func loadYingzoReleasePublicKeys() (map[string]ed25519.PublicKey, error) {
	rawKeys := map[string]string{}
	if raw := strings.TrimSpace(os.Getenv(yingzoReleasePublicKeysEnv)); raw != "" {
		if err := json.Unmarshal([]byte(raw), &rawKeys); err != nil {
			return nil, errors.New("YINGZO_RELEASE_PUBLIC_KEYS must be a JSON object of key_id to base64 Ed25519 public key")
		}
	}
	if raw := strings.TrimSpace(os.Getenv(yingzoReleasePublicKeyEnv)); raw != "" {
		if _, exists := rawKeys["default"]; !exists {
			rawKeys["default"] = raw
		}
	}
	if len(rawKeys) == 0 {
		return nil, errors.New("no Yingzo release verification public key is configured")
	}
	keys := make(map[string]ed25519.PublicKey, len(rawKeys))
	for keyID, encoded := range rawKeys {
		keyID = strings.TrimSpace(keyID)
		decoded, err := decodeYingzoBase64(encoded)
		if keyID == "" || err != nil || len(decoded) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("invalid Ed25519 public key for key_id %q", keyID)
		}
		keys[keyID] = ed25519.PublicKey(append([]byte(nil), decoded...))
	}
	return keys, nil
}

func decodeYingzoBase64(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if decoded, err := encoding.DecodeString(raw); err == nil {
			return decoded, nil
		}
	}
	return nil, errors.New("invalid base64")
}

func (h *AgentHandler) PromoteYingzoRelease(c *gin.Context) {
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
	var status, channel, version string
	var schemaVersion, runtimeProtocol int
	var stableEligible bool
	var releaseSignature sql.NullString
	if err := tx.QueryRowContext(c, `SELECT status,channel,distribution_schema_version,stable_eligible,runtime_protocol,version,signature FROM yingzo_releases WHERE id=$1 FOR UPDATE`, id).Scan(&status, &channel, &schemaVersion, &stableEligible, &runtimeProtocol, &version, &releaseSignature); err != nil || status != "published" || channel != "prerelease" || schemaVersion != 2 {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "release_state_invalid"}})
		return
	}
	if !stableEligible {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "release_not_stable_eligible", "message": "unsigned Windows prereleases cannot be promoted; publish a new fully signed version"}})
		return
	}
	if err = validateYingzoV2PublishMatrix(c, tx, id, version, channel, stableEligible, runtimeProtocol, releaseSignature.String, false); err != nil {
		var uploadErr *yingzoUploadError
		if errors.As(err, &uploadErr) {
			c.JSON(uploadErr.status, gin.H{"error": gin.H{"code": uploadErr.code, "message": uploadErr.message}})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		}
		return
	}
	if _, err = tx.ExecContext(c, `UPDATE yingzo_releases SET status='superseded',updated_at=NOW() WHERE status='published' AND channel='stable' AND id<>$1`, id); err == nil {
		_, err = tx.ExecContext(c, `UPDATE yingzo_releases SET channel='stable',published_at=NOW(),published_by=$2,updated_at=NOW() WHERE id=$1 AND status='published' AND channel='prerelease'`, id, subject.UserID)
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
		c.JSON(http.StatusOK, gin.H{"id": id, "status": "published", "channel": "stable"})
		return
	}
	c.JSON(http.StatusOK, release)
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
	var status, channel, version string
	var schemaVersion, runtimeProtocol int
	var stableEligible bool
	var releaseSignature sql.NullString
	if err = tx.QueryRowContext(c, `SELECT status,channel,distribution_schema_version,stable_eligible,runtime_protocol,version,signature FROM yingzo_releases WHERE id=$1 FOR UPDATE`, id).Scan(&status, &channel, &schemaVersion, &stableEligible, &runtimeProtocol, &version, &releaseSignature); err != nil || status == "disabled" || (!rollback && status != "draft") || (rollback && status != "superseded") {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "release_state_invalid"}})
		return
	}
	if schemaVersion == 1 {
		legacyRows, queryErr := tx.QueryContext(c, `SELECT host_family,storage_key,size_bytes,sha256 FROM yingzo_release_artifacts WHERE release_id=$1`, id)
		if queryErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
			return
		}
		families := map[string]bool{}
		legacyValid := true
		for legacyRows.Next() {
			var family, storageKey, expectedSHA string
			var expectedSize int64
			if scanErr := legacyRows.Scan(&family, &storageKey, &expectedSize, &expectedSHA); scanErr != nil {
				legacyValid = false
				break
			}
			info, statErr := os.Stat(storageKey)
			actualSHA, hashErr := yingzoFileSHA256(storageKey)
			if statErr != nil || !info.Mode().IsRegular() || info.Size() != expectedSize || hashErr != nil || !strings.EqualFold(actualSHA, expectedSHA) {
				legacyValid = false
				break
			}
			families[family] = true
		}
		rowsErr := legacyRows.Err()
		_ = legacyRows.Close()
		if rowsErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
			return
		}
		if !legacyValid || (!families["combined"] && (!families["openai"] || !families["claude"])) {
			c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "release_artifacts_incomplete"}})
			return
		}
	} else if channel == "stable" && !stableEligible {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "release_not_stable_eligible", "message": "stable publication requires fully signed native artifacts"}})
		return
	} else if err = validateYingzoV2PublishMatrix(c, tx, id, version, channel, stableEligible, runtimeProtocol, releaseSignature.String, rollback && channel == "stable"); err != nil {
		var uploadErr *yingzoUploadError
		if errors.As(err, &uploadErr) {
			c.JSON(uploadErr.status, gin.H{"error": gin.H{"code": uploadErr.code, "message": uploadErr.message}})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		}
		return
	}
	if _, err = tx.ExecContext(c, `UPDATE yingzo_releases SET status='superseded',updated_at=NOW() WHERE status='published' AND channel=$1 AND id<>$2`, channel, id); err == nil {
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
		c.JSON(http.StatusOK, gin.H{"id": id, "status": "published", "channel": channel})
		return
	}
	c.JSON(http.StatusOK, release)
}

func validateYingzoV2PublishMatrix(ctx context.Context, tx *sql.Tx, releaseID uuid.UUID, version, channel string, stableEligible bool, runtimeProtocol int, releaseSignature string, allowPromotedPrereleaseProof bool) error {
	rows, err := tx.QueryContext(ctx, `SELECT artifact_kind,target,os,arch,runtime_protocol,validation_status,signature_status,package_filename,storage_key,size_bytes,sha256 FROM yingzo_release_artifacts WHERE release_id=$1`, releaseID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	found := map[string]bool{}
	proofRelease := &yingzoRelease{ID: releaseID, Version: version, Channel: channel, StableEligible: stableEligible, RuntimeProtocol: runtimeProtocol, Signature: releaseSignature, Artifacts: map[string]*yingzoReleaseArtifact{}}
	for rows.Next() {
		var kind, target, targetOS, arch, validationStatus, signatureStatus, packageFilename, storageKey, expectedSHA string
		var artifactProtocol int
		var expectedSize int64
		if err := rows.Scan(&kind, &target, &targetOS, &arch, &artifactProtocol, &validationStatus, &signatureStatus, &packageFilename, &storageKey, &expectedSize, &expectedSHA); err != nil {
			return err
		}
		spec, ok := findYingzoV2Spec(version, kind, target, targetOS, arch)
		validSignatureStatus := signatureStatus == "verified" || (!stableEligible && channel == "prerelease" && yingzoWindowsBearingSpec(spec) && signatureStatus == "unverified")
		if !ok || packageFilename != spec.Filename || artifactProtocol != runtimeProtocol || validationStatus != "validated" || !validSignatureStatus {
			return &yingzoUploadError{status: http.StatusConflict, code: "release_artifacts_invalid", message: "every v2 artifact must match the release matrix, protocol, validation, and signature requirements"}
		}
		info, statErr := os.Stat(storageKey)
		if statErr != nil || !info.Mode().IsRegular() || info.Size() != expectedSize {
			return &yingzoUploadError{status: http.StatusConflict, code: "release_artifact_file_missing", message: "a release artifact file is missing from local storage"}
		}
		actualSHA, hashErr := yingzoFileSHA256(storageKey)
		if hashErr != nil || !strings.EqualFold(actualSHA, expectedSHA) {
			return &yingzoUploadError{status: http.StatusConflict, code: "release_artifact_integrity_failed", message: "a release artifact no longer matches its uploaded checksum"}
		}
		found[spec.key()] = true
		proofRelease.Artifacts[spec.key()] = &yingzoReleaseArtifact{
			ReleaseID: releaseID, ArtifactKind: kind, Target: target, OS: targetOS, Arch: arch,
			Format: spec.Format, ContentType: spec.ContentType, RuntimeProtocol: artifactProtocol,
			ValidationStatus: validationStatus, SignatureStatus: signatureStatus, PackageFilename: packageFilename,
			StorageBackend: "local", StorageKey: storageKey, SizeBytes: expectedSize, SHA256: expectedSHA,
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(found) != len(yingzoV2ArtifactSpecs(version)) {
		return &yingzoUploadError{status: http.StatusConflict, code: "release_artifacts_incomplete", message: "all eight release artifacts are required before publishing"}
	}
	for _, spec := range yingzoV2ArtifactSpecs(version) {
		if !found[spec.key()] {
			return &yingzoUploadError{status: http.StatusConflict, code: "release_artifacts_incomplete", message: "all eight release artifacts are required before publishing"}
		}
	}
	var envelope yingzoReleaseProofEnvelope
	if json.Unmarshal([]byte(releaseSignature), &envelope) != nil {
		return &yingzoUploadError{status: http.StatusConflict, code: "release_proof_required", message: "a server-verified signed release manifest is required before publishing"}
	}
	// Promotion deliberately keeps the original signed prerelease manifest so
	// the exact published bytes can move to stable without being rebuilt. A
	// later stable rollback may therefore revalidate that prerelease proof. This
	// exception is enabled only by the superseded stable rollback path; draft
	// publication and promotion continue to require an exact channel match.
	if allowPromotedPrereleaseProof {
		manifestBytes, decodeErr := decodeYingzoBase64(envelope.ManifestBase64)
		var manifest yingzoSignedReleaseManifest
		if decodeErr == nil && json.Unmarshal(manifestBytes, &manifest) == nil && manifest.Channel == "prerelease" {
			proofRelease.Channel = "prerelease"
		}
	}
	_, manifest, err := verifyYingzoReleaseProof(proofRelease, yingzoReleaseProofInput{Algorithm: envelope.Algorithm, KeyID: envelope.KeyID, ManifestBase64: envelope.ManifestBase64, SignatureBase64: envelope.SignatureBase64})
	if err != nil {
		return err
	}
	if manifest.StableEligible != stableEligible {
		return &yingzoUploadError{status: http.StatusConflict, code: "release_manifest_mismatch", message: "release stable eligibility no longer matches its signed proof"}
	}
	return nil
}

func (h *AgentHandler) DisableYingzoRelease(c *gin.Context) {
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
	var schemaVersion int
	if err := tx.QueryRowContext(c, `SELECT status,distribution_schema_version FROM yingzo_releases WHERE id=$1 FOR UPDATE`, id).Scan(&status, &schemaVersion); errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "yingzo_release_not_found"}})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	if status == "disabled" {
		c.JSON(http.StatusOK, gin.H{"disabled": false})
		return
	}
	storageKeys := make([]string, 0)
	if status == "draft" && schemaVersion == yingzoDistributionSchema2 {
		rows, queryErr := tx.QueryContext(c, `DELETE FROM yingzo_release_artifacts WHERE release_id=$1 RETURNING storage_key`, id)
		if queryErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
			return
		}
		for rows.Next() {
			var storageKey string
			if scanErr := rows.Scan(&storageKey); scanErr != nil {
				_ = rows.Close()
				c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
				return
			}
			storageKeys = append(storageKeys, storageKey)
		}
		rowsErr := rows.Err()
		_ = rows.Close()
		if rowsErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
			return
		}
	}
	if _, err = tx.ExecContext(c, `UPDATE yingzo_releases SET status='disabled',signature=NULL,updated_at=NOW() WHERE id=$1 AND status<>'disabled'`, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	if err := tx.Commit(); err != nil {
		// Commit results can be ambiguous. Preserve files until reconciliation can
		// prove that no database row references them.
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	for _, storageKey := range storageKeys {
		h.removeYingzoStorageFile(id, storageKey)
	}
	c.JSON(http.StatusOK, gin.H{"disabled": true})
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
	storage, storageErr := h.yingzoReleaseStorageRoot()
	if storageErr != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "yingzo_release_storage_invalid", "message": storageErr.Error()}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"public_origin": configured, "effective_origin": effective, "release_storage": storage})
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

func (h *AgentHandler) latestPublishedYingzoRelease(ctx context.Context, channel string) (*yingzoRelease, error) {
	row := h.db.QueryRowContext(ctx, `SELECT `+yingzoReleaseColumns+` FROM yingzo_releases WHERE status='published' AND channel=$1 ORDER BY published_at DESC LIMIT 1`, channel)
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
	return getYingzoReleaseFrom(ctx, h.db, id, false)
}

type yingzoReleaseQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func getYingzoReleaseFrom(ctx context.Context, querier yingzoReleaseQuerier, id uuid.UUID, forUpdate bool) (*yingzoRelease, error) {
	query := `SELECT ` + yingzoReleaseColumns + ` FROM yingzo_releases WHERE id=$1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	row := querier.QueryRowContext(ctx, query, id)
	release, err := scanYingzoRelease(row)
	if err != nil {
		return nil, err
	}
	if err := loadYingzoArtifactsFrom(ctx, querier, release); err != nil {
		return nil, err
	}
	return release, nil
}

type yingzoReleaseScanner interface{ Scan(dest ...any) error }

func scanYingzoRelease(scanner yingzoReleaseScanner) (*yingzoRelease, error) {
	var release yingzoRelease
	var compatibility []byte
	var signature, minCodex, minClaude, notes sql.NullString
	var published sql.NullTime
	err := scanner.Scan(&release.ID, &release.Version, &release.Status, &release.DistributionSchemaVersion, &release.Channel, &release.StableEligible, &release.RuntimeProtocol, &compatibility, &signature, &minCodex, &minClaude, &notes, &release.CreatedAt, &published, &release.UpdatedAt)
	if err != nil {
		return nil, err
	}
	release.Compatibility = append(json.RawMessage(nil), compatibility...)
	if len(release.Compatibility) == 0 {
		release.Compatibility = json.RawMessage(`{}`)
	}
	release.Signature, release.MinCodexVersion, release.MinClaudeVersion, release.ReleaseNotes = signature.String, minCodex.String, minClaude.String, notes.String
	if published.Valid {
		release.PublishedAt = &published.Time
	}
	release.Artifacts = map[string]*yingzoReleaseArtifact{}
	return &release, nil
}

func (h *AgentHandler) loadYingzoArtifacts(ctx context.Context, release *yingzoRelease) error {
	return loadYingzoArtifactsFrom(ctx, h.db, release)
}

func loadYingzoArtifactsFrom(ctx context.Context, querier yingzoReleaseQuerier, release *yingzoRelease) error {
	rows, err := querier.QueryContext(ctx, `SELECT `+yingzoArtifactColumns+` FROM yingzo_release_artifacts WHERE release_id=$1 ORDER BY target,os,arch`, release.ID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	release.Artifacts = map[string]*yingzoReleaseArtifact{}
	for rows.Next() {
		var artifact yingzoReleaseArtifact
		var hostFamily sql.NullString
		var validated sql.NullTime
		if err := rows.Scan(&artifact.ID, &artifact.ReleaseID, &hostFamily, &artifact.ArtifactKind, &artifact.Target, &artifact.OS, &artifact.Arch, &artifact.Format, &artifact.ContentType, &artifact.RuntimeProtocol, &artifact.ValidationStatus, &artifact.SignatureStatus, &validated, &artifact.PackageFilename, &artifact.StorageBackend, &artifact.StorageKey, &artifact.SizeBytes, &artifact.SHA256, &artifact.CreatedAt, &artifact.UpdatedAt); err != nil {
			return err
		}
		artifact.HostFamily = hostFamily.String
		if validated.Valid {
			artifact.ValidatedAt = &validated.Time
		}
		release.Artifacts[artifact.key()] = &artifact
	}
	return rows.Err()
}

func yingzoArtifactForFamily(release *yingzoRelease, family string) *yingzoReleaseArtifact {
	if release == nil {
		return nil
	}
	for _, artifact := range release.Artifacts {
		if artifact.HostFamily == family {
			return artifact
		}
	}
	for _, artifact := range release.Artifacts {
		if artifact.HostFamily == "combined" {
			return artifact
		}
	}
	return nil
}

func yingzoArtifactForTuple(release *yingzoRelease, target, targetOS, arch string) *yingzoReleaseArtifact {
	if release == nil {
		return nil
	}
	return release.Artifacts[strings.Join([]string{target, targetOS, arch}, ":")]
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

func yingzoHostArtifactForPlatform(release *yingzoRelease, host, targetOS, arch string) *yingzoReleaseArtifact {
	if release == nil {
		return nil
	}
	if release.DistributionSchemaVersion == 1 {
		family, _ := yingzoHostFamily(host)
		return yingzoArtifactForFamily(release, family)
	}
	var target, artifactOS, artifactArch string
	switch host {
	case "chatgpt-work", "codex":
		target = "openai"
	case "claude-code":
		target = "claude-code"
	case "claude-chat", "claude-cowork":
		return yingzoArtifactForTuple(release, "claude-desktop", "any", "any")
	default:
		return nil
	}
	artifactOS, artifactArch = targetOS, arch
	if targetOS == "macos" {
		artifactArch = "any"
	}
	return yingzoArtifactForTuple(release, target, artifactOS, artifactArch)
}

func yingzoHostFamily(host string) (string, error) {
	switch host {
	case "chatgpt-work", "codex":
		return "openai", nil
	case "claude-cowork", "claude-code", "claude-chat":
		return "claude", nil
	default:
		return "", errors.New("host must be chatgpt-work, codex, claude-cowork, claude-chat, or claude-code")
	}
}

func normalizeYingzoChannel(raw string) (string, error) {
	channel := strings.ToLower(strings.TrimSpace(raw))
	if channel == "" {
		return "stable", nil
	}
	if channel != "stable" && channel != "prerelease" {
		return "", errors.New("channel must be stable or prerelease")
	}
	return channel, nil
}

func normalizeYingzoOS(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "macos", "darwin":
		return "macos", nil
	case "windows", "win32":
		return "windows", nil
	default:
		return "", errors.New("unsupported operating system")
	}
}
func normalizeYingzoOSTrusted(raw string) string { value, _ := normalizeYingzoOS(raw); return value }

func normalizeYingzoArch(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "arm64", "aarch64":
		return "arm64", nil
	case "x64", "amd64", "x86_64":
		return "x64", nil
	default:
		return "", errors.New("unsupported architecture")
	}
}
func normalizeYingzoArchTrusted(raw string) string {
	value, _ := normalizeYingzoArch(raw)
	return value
}

func yingzoPlatformSupported(targetOS, arch string) bool {
	return (targetOS == "macos" && (arch == "arm64" || arch == "x64")) || (targetOS == "windows" && arch == "x64")
}

func resolveYingzoRuntimeCapability(input yingzoInstallRequest, release *yingzoRelease) (string, error) {
	capability := strings.ToLower(strings.TrimSpace(input.RuntimeCapability))
	if capability == "" {
		hasReportedRuntime := strings.TrimSpace(input.InstalledRuntimeVersion) != "" || input.InstalledRuntimeProtocol != 0
		if !hasReportedRuntime {
			return "probe", nil
		}
		if strings.TrimSpace(input.InstalledRuntimeVersion) == release.Version && input.InstalledRuntimeProtocol == release.RuntimeProtocol {
			return "compatible", nil
		}
		return "required", nil
	}
	switch capability {
	case "unknown":
		return "probe", nil
	case "missing", "incompatible":
		return "required", nil
	case "compatible":
		if strings.TrimSpace(input.InstalledRuntimeVersion) != release.Version || input.InstalledRuntimeProtocol != release.RuntimeProtocol {
			return "", errors.New("compatible runtime reports must include the exact release version and protocol")
		}
		return "compatible", nil
	default:
		return "", errors.New("runtime_capability must be unknown, missing, incompatible, or compatible")
	}
}

func publicYingzoRelease(release *yingzoRelease) gin.H {
	artifacts := gin.H{}
	items := make([]gin.H, 0, len(release.Artifacts))
	var totalSize int64
	for _, artifact := range release.Artifacts {
		summary := gin.H{"artifact_kind": artifact.ArtifactKind, "target": artifact.Target, "os": artifact.OS, "arch": artifact.Arch, "package_filename": artifact.PackageFilename, "content_type": artifact.ContentType, "size_bytes": artifact.SizeBytes}
		totalSize += artifact.SizeBytes
		items = append(items, summary)
		if release.DistributionSchemaVersion == 1 {
			if artifact.HostFamily == "combined" {
				artifacts["openai"], artifacts["claude"] = summary, summary
			} else {
				artifacts[artifact.HostFamily] = summary
			}
		} else {
			artifacts[artifact.key()] = summary
		}
	}
	response := gin.H{
		"version": release.Version, "distribution_schema_version": release.DistributionSchemaVersion,
		"channel": release.Channel, "stable_eligible": release.StableEligible, "runtime_protocol": release.RuntimeProtocol, "compatibility": release.Compatibility,
		"size_bytes": totalSize, "artifacts": artifacts, "artifact_items": items,
		"signature_status":  map[bool]string{true: "signed", false: "unsigned"}[release.Signature != ""],
		"native_signing":    yingzoNativeSigningSummary(release),
		"min_codex_version": release.MinCodexVersion, "min_claude_version": release.MinClaudeVersion,
		"release_notes": release.ReleaseNotes, "published_at": release.PublishedAt,
	}
	if yingzoUnsignedWindowsPrerelease(release) {
		response["warning"] = yingzoReleaseWarning(release)
	}
	return response
}

func yingzoNativeSigningSummary(release *yingzoRelease) yingzoNativeSigning {
	macOSStatus := "unverified"
	windowsStatus := "unsigned"
	if release != nil && release.DistributionSchemaVersion >= 2 {
		macOSStatus = "verified"
		windowsStatus = "verified"
		for _, artifact := range release.Artifacts {
			if artifact.SignatureStatus == "failed" {
				if artifact.OS == "macos" {
					macOSStatus = "failed"
				}
				if artifact.OS == "windows" || artifact.Target == "claude-desktop" {
					windowsStatus = "failed"
				}
				continue
			}
			if artifact.OS == "macos" && artifact.SignatureStatus != "verified" && macOSStatus != "failed" {
				macOSStatus = "unverified"
			}
			if (artifact.OS == "windows" || artifact.Target == "claude-desktop") && artifact.SignatureStatus != "verified" && windowsStatus != "failed" {
				windowsStatus = "unsigned"
			}
		}
	}
	return yingzoNativeSigning{
		MacOS:   yingzoNativeSigningPlatform{Status: macOSStatus},
		Windows: yingzoNativeSigningPlatform{Status: windowsStatus},
	}
}

func yingzoUnsignedWindowsPrerelease(release *yingzoRelease) bool {
	return release != nil && release.Channel == "prerelease" && !release.StableEligible
}

func yingzoReleaseWarning(release *yingzoRelease) string {
	if !yingzoUnsignedWindowsPrerelease(release) {
		return ""
	}
	return "此预发布版本包含未签名 Windows 产物。Windows 安装时可能触发 Microsoft Defender SmartScreen；该版本不能提升为稳定版。"
}

func yingzoV2InstallPrompt(host string, release *yingzoRelease, hostURL string, runtimeDescriptor gin.H, runtimeResolution string) string {
	message := fmt.Sprintf("请安装或升级 Yingzo（影作）到版本 %s。宿主安装包：%s。保留现有 Yingzo 认证和项目数据，安装后验证 Runtime 协议 %d 与 MCP 连接。", release.Version, hostURL, release.RuntimeProtocol)
	if yingzoUnsignedWindowsPrerelease(release) {
		message += " 注意：这是包含未签名 Windows 产物的预发布版，只用于测试且不能提升为稳定版；在 Windows 安装时 Microsoft Defender SmartScreen 可能显示未知发布者警告。"
	}
	if runtimeDescriptor != nil && runtimeResolution == "required" {
		message += fmt.Sprintf(" 请先安装或升级 Yingzo Runtime：%s。", runtimeDescriptor["download_url"])
	} else if runtimeDescriptor != nil && runtimeResolution == "probe" {
		installerURL, _ := runtimeDescriptor["download_url"].(string)
		probeURI := yingzoRuntimeEnsureURI(release, installerURL)
		message += fmt.Sprintf(" 当前网页无法读取本机 Runtime 状态；先调用 %s 让 Runtime Helper 检测。已兼容时不得重装，缺失或不兼容时才使用安装器：%s。", probeURI, installerURL)
	}
	if host == "codex" || host == "chatgpt-work" {
		return message + " 使用官方 OpenAI/Codex 插件管理器：先记录当前 Yingzo 版本及旧 marketplace 路径；执行 `codex plugin remove yingzo@yingzo-private --json`、`codex plugin marketplace remove yingzo-private --json`，解压宿主包后执行 `codex plugin marketplace add <解压目录>/marketplace --json`、`codex plugin add yingzo@yingzo-private --json`。不得直接复制插件目录，不得删除认证与项目数据。验证版本和 MCP；失败时移除新注册并用已记录的旧 marketplace 路径重新 add/install。"
	}
	if host == "claude-code" {
		return message + " 使用官方 Claude Code 插件管理器：先记录当前 Yingzo 版本及旧 marketplace 路径；执行 `claude plugin uninstall yingzo@yingzo-private --scope user --keep-data`、`claude plugin marketplace remove yingzo-private --scope user`，解压宿主包后执行 `claude plugin marketplace add <解压目录>/marketplace --scope user`、`claude plugin install yingzo@yingzo-private --scope user`。不得直接复制插件目录，不得删除认证与项目数据。验证版本和 MCP；失败时移除新注册并用已记录的旧 marketplace 路径重新 add/install。"
	}
	return message + " 使用 Claude Desktop MCPB 安装流程，并重新打开 Claude 会话加载工具。"
}

func yingzoRuntimeEnsureURI(release *yingzoRelease, installerURL string) string {
	return fmt.Sprintf("yingzo://runtime/ensure?version=%s&protocol=%d&installer_url=%s", url.QueryEscape(release.Version), release.RuntimeProtocol, url.QueryEscape(installerURL))
}

func yingzoInstallPrompt(host string, release *yingzoRelease, artifact *yingzoReleaseArtifact, downloadURL string) string {
	archiveRoot := strings.TrimSuffix(artifact.PackageFilename, ".tar.gz")
	hostFamily, _ := yingzoHostFamily(host)
	hostReleaseRoot := fmt.Sprintf("~/.yingzo/releases/%s/%s", hostFamily, release.Version)
	common := fmt.Sprintf("请安装或升级 Yingzo（影作）私有插件到版本 %s。下载地址：%s\n请把安装包下载到临时文件，解压到 %s，保留现有 ~/.yingzo/auth.json，禁止把凭证写入项目、日志或安装目录。安装完成后验证插件和 MCP 工具，并告诉我结果。", release.Version, downloadURL, hostReleaseRoot)
	if release.Signature != "" {
		common += "\n发行签名：" + release.Signature + "。同时校验签名；失败立即停止。"
	}
	if hostFamily == "openai" {
		return common + fmt.Sprintf("\n这是 OpenAI 宿主专用安装指令，适用于 ChatGPT Work 与 Codex。只执行本段列出的 Codex 插件命令，也不要修改 ~/.claude/plugins。插件安装到独立的 ~/.codex/plugins 缓存。解压后的 marketplace 目录是 %s/%s/marketplace。若旧版本存在，先运行 codex plugin remove yingzo@yingzo-private --json，再运行 codex plugin marketplace remove yingzo-private --json。然后运行 codex plugin marketplace add <上述 marketplace 绝对路径> --json，最后运行 codex plugin add yingzo@yingzo-private --json。不要要求用户手动打开终端。安装后验证插件清单，并提示用户新建任务以加载新工具。", hostReleaseRoot, archiveRoot)
	}
	return common + fmt.Sprintf("\n这是 Claude 宿主专用安装指令，适用于 Claude Cowork 与 Claude Code。只执行本段列出的 Claude 插件命令，也不要修改 ~/.codex/plugins。插件安装到独立的 ~/.claude/plugins 缓存。解压后的 marketplace 目录是 %s/%s/marketplace。若旧版本存在，先运行 claude plugin uninstall yingzo@yingzo-private --scope user --keep-data，再运行 claude plugin marketplace remove yingzo-private --scope user。然后运行 claude plugin marketplace add <上述 marketplace 绝对路径> --scope user，最后运行 claude plugin install yingzo@yingzo-private --scope user。不要要求用户手动打开终端。安装后验证插件清单，并提示用户新建 Claude 会话以加载新工具。", hostReleaseRoot, archiveRoot)
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

func validateYingzoV2Artifact(filename string, spec yingzoArtifactSpec) error {
	switch spec.Format {
	case "tar.gz":
		if err := validateTarArchiveSafety(filename); err != nil {
			return err
		}
		entries, err := yingzoTarEntryNames(filename)
		if err != nil {
			return err
		}
		root, err := findYingzoV2HostRoot(entries, spec)
		if err != nil {
			return err
		}
		return validateYingzoV2HostContents(filename, "tar.gz", root, spec)
	case "zip", "mcpb":
		if err := validateZipArchiveSafety(filename); err != nil {
			return err
		}
		entries, err := yingzoZipEntryNames(filename)
		if err != nil {
			return err
		}
		root, err := findYingzoV2HostRoot(entries, spec)
		if err != nil {
			return err
		}
		return validateYingzoV2HostContents(filename, "zip", root, spec)
	case "exe":
		file, err := pe.Open(filename)
		if err != nil {
			return errors.New("Windows installer is not a valid PE executable")
		}
		defer func() { _ = file.Close() }()
		return validateAMD64PEFile(file)
	case "dmg":
		file, err := os.Open(filename)
		if err != nil {
			return err
		}
		defer func() { _ = file.Close() }()
		info, err := file.Stat()
		if err != nil || info.Size() < 512 {
			return errors.New("macOS installer is not a valid UDIF disk image")
		}
		if _, err := file.Seek(-512, io.SeekEnd); err != nil {
			return err
		}
		trailer := make([]byte, 4)
		if _, err := io.ReadFull(file, trailer); err != nil || string(trailer) != "koly" {
			return errors.New("macOS installer is not a valid UDIF disk image")
		}
		return nil
	default:
		return errors.New("unsupported artifact format")
	}
}

func inspectYingzoV2Artifact(filename string, spec yingzoArtifactSpec) (bool, error) {
	if err := validateYingzoV2Artifact(filename, spec); err != nil {
		return false, err
	}
	if !yingzoWindowsBearingSpec(spec) {
		return false, nil
	}
	if spec.Format == "exe" {
		file, err := pe.Open(filename)
		if err != nil {
			return false, errors.New("Windows installer is not a valid PE executable")
		}
		defer func() { _ = file.Close() }()
		return inspectAMD64PEFile(file)
	}
	archiveFormat := "zip"
	entries, err := yingzoZipEntryNames(filename)
	if err != nil {
		return false, err
	}
	root, err := findYingzoV2HostRoot(entries, spec)
	if err != nil {
		return false, err
	}
	launcherName := "runtime/yingzo-mcp.exe"
	if spec.Target == "claude-desktop" {
		launcherName = "server/yingzo-mcp.exe"
	}
	launcher, err := readYingzoArchiveEntry(filename, archiveFormat, root+launcherName, 64<<20)
	if err != nil {
		return false, err
	}
	return inspectAMD64PE(launcher.Data)
}

func yingzoWindowsBearingSpec(spec yingzoArtifactSpec) bool {
	return spec.OS == "windows" || spec.Target == "claude-desktop"
}

func validateYingzoV2HostLayout(entries []string, spec yingzoArtifactSpec) error {
	_, err := findYingzoV2HostRoot(entries, spec)
	return err
}

func findYingzoV2HostRoot(entries []string, spec yingzoArtifactSpec) (string, error) {
	var required []string
	switch spec.Target {
	case "openai":
		launcher := "runtime/yingzo-mcp"
		if spec.OS == "windows" {
			launcher = "runtime/yingzo-mcp.exe"
		}
		required = []string{".codex-plugin/plugin.json", ".mcp.json", "ui-manifest.json", "runtime/runtime-helper.json", launcher}
	case "claude-code":
		launcher := "runtime/yingzo-mcp"
		if spec.OS == "windows" {
			launcher = "runtime/yingzo-mcp.exe"
		}
		required = []string{".claude-plugin/plugin.json", ".mcp.json", "ui-manifest.json", "runtime/runtime-helper.json", launcher}
	case "claude-desktop":
		required = []string{"manifest.json", "ui-manifest.json", "server/yingzo-mcp", "server/yingzo-mcp.exe"}
	default:
		return "", nil
	}
	entrySet := make(map[string]bool, len(entries))
	for _, entry := range entries {
		entrySet[entry] = true
	}
	marker := required[0]
	for entry := range entrySet {
		if !strings.HasSuffix(entry, marker) {
			continue
		}
		root := strings.TrimSuffix(entry, marker)
		if root != "" && !strings.HasSuffix(root, "/") {
			continue
		}
		complete := true
		for _, requiredEntry := range required {
			if !entrySet[root+requiredEntry] {
				complete = false
				break
			}
		}
		if complete {
			return root, nil
		}
	}
	return "", fmt.Errorf("package is missing the required %s layout", spec.Target)
}

type yingzoArchiveEntryData struct {
	Data []byte
	Mode os.FileMode
}

func validateYingzoV2HostContents(filename, archiveFormat, root string, spec yingzoArtifactSpec) error {
	uiManifest, err := readYingzoArchiveEntry(filename, archiveFormat, root+"ui-manifest.json", 2<<20)
	if err != nil {
		return err
	}
	var ui struct {
		SchemaVersion  int    `json:"schema_version"`
		ProductVersion string `json:"product_version"`
	}
	expectedVersion := strings.TrimSuffix(strings.TrimPrefix(spec.Filename, artifactFilenamePrefix(spec)), artifactFilenameSuffix(spec))
	if json.Unmarshal(uiManifest.Data, &ui) != nil || ui.SchemaVersion != 1 || ui.ProductVersion != expectedVersion {
		return errors.New("ui-manifest.json product_version does not match the release")
	}

	switch spec.Target {
	case "openai", "claude-code":
		if !strings.HasSuffix(root, "marketplace/plugins/yingzo/") {
			return errors.New("host package must contain the self-contained marketplace/plugins/yingzo layout")
		}
		marketplaceRoot := strings.TrimSuffix(root, "plugins/yingzo/")
		marketplaceManifest := ".agents/plugins/marketplace.json"
		if spec.Target == "claude-code" {
			marketplaceManifest = ".claude-plugin/marketplace.json"
		}
		marketplaceEntry, err := readYingzoArchiveEntry(filename, archiveFormat, marketplaceRoot+marketplaceManifest, 2<<20)
		if err != nil || !json.Valid(marketplaceEntry.Data) {
			return errors.New("host package marketplace manifest is missing or invalid")
		}
		pluginPath := ".codex-plugin/plugin.json"
		if spec.Target == "claude-code" {
			pluginPath = ".claude-plugin/plugin.json"
		}
		pluginEntry, err := readYingzoArchiveEntry(filename, archiveFormat, root+pluginPath, 2<<20)
		if err != nil {
			return err
		}
		var plugin struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}
		if json.Unmarshal(pluginEntry.Data, &plugin) != nil || plugin.Name != "yingzo" || plugin.Version != expectedVersion {
			return errors.New("plugin manifest name or version does not match the release")
		}
		helperEntry, err := readYingzoArchiveEntry(filename, archiveFormat, root+"runtime/runtime-helper.json", 2<<20)
		if err != nil {
			return err
		}
		var helper struct {
			SchemaVersion          int    `json:"schema_version"`
			ProductVersion         string `json:"product_version"`
			RuntimeProtocolVersion int    `json:"runtime_protocol_version"`
		}
		if json.Unmarshal(helperEntry.Data, &helper) != nil || helper.SchemaVersion != 1 || helper.ProductVersion != expectedVersion || helper.RuntimeProtocolVersion <= 0 || (spec.RuntimeProtocol > 0 && helper.RuntimeProtocolVersion != spec.RuntimeProtocol) {
			return errors.New("runtime-helper.json version or protocol is invalid")
		}
		mcpEntry, err := readYingzoArchiveEntry(filename, archiveFormat, root+".mcp.json", 2<<20)
		if err != nil {
			return err
		}
		var mcp struct {
			MCPServers map[string]struct {
				Command string `json:"command"`
			} `json:"mcpServers"`
		}
		if json.Unmarshal(mcpEntry.Data, &mcp) != nil || mcp.MCPServers["yingzo"].Command == "" {
			return errors.New(".mcp.json must configure the Yingzo launcher")
		}
		launcherName := "runtime/yingzo-mcp"
		if spec.OS == "windows" {
			launcherName += ".exe"
		}
		if !strings.HasSuffix(strings.ReplaceAll(mcp.MCPServers["yingzo"].Command, "\\", "/"), launcherName) {
			return errors.New(".mcp.json points to the wrong platform launcher")
		}
		launcher, err := readYingzoArchiveEntry(filename, archiveFormat, root+launcherName, 64<<20)
		if err != nil {
			return err
		}
		if spec.OS == "windows" {
			if err := validateAMD64PE(launcher.Data); err != nil {
				return fmt.Errorf("Windows host launcher: %w", err)
			}
		} else if launcher.Mode&0111 == 0 || !bytes.HasPrefix(launcher.Data, []byte("#!")) {
			return errors.New("macOS host launcher must be an executable script")
		}
	case "claude-desktop":
		manifestEntry, err := readYingzoArchiveEntry(filename, archiveFormat, root+"manifest.json", 2<<20)
		if err != nil {
			return err
		}
		var manifest struct {
			Name    string `json:"name"`
			Version string `json:"version"`
			Server  struct {
				EntryPoint string `json:"entry_point"`
				MCPConfig  struct {
					Command           string `json:"command"`
					PlatformOverrides struct {
						Win32 struct {
							Command string `json:"command"`
						} `json:"win32"`
					} `json:"platform_overrides"`
				} `json:"mcp_config"`
			} `json:"server"`
		}
		if json.Unmarshal(manifestEntry.Data, &manifest) != nil || manifest.Name != "yingzo" || manifest.Version != expectedVersion || manifest.Server.EntryPoint != "server/yingzo-mcp" || !strings.HasSuffix(manifest.Server.MCPConfig.Command, "/server/yingzo-mcp") || !strings.HasSuffix(manifest.Server.MCPConfig.PlatformOverrides.Win32.Command, "/server/yingzo-mcp.exe") {
			return errors.New("MCPB manifest version or platform_overrides are invalid")
		}
		macLauncher, err := readYingzoArchiveEntry(filename, archiveFormat, root+"server/yingzo-mcp", 64<<20)
		if err != nil || macLauncher.Mode&0111 == 0 || !bytes.HasPrefix(macLauncher.Data, []byte("#!")) {
			return errors.New("MCPB macOS launcher must be an executable script")
		}
		windowsLauncher, err := readYingzoArchiveEntry(filename, archiveFormat, root+"server/yingzo-mcp.exe", 64<<20)
		if err != nil {
			return err
		}
		if err := validateAMD64PE(windowsLauncher.Data); err != nil {
			return fmt.Errorf("MCPB Windows launcher: %w", err)
		}
	}
	return nil
}

func artifactFilenamePrefix(spec yingzoArtifactSpec) string {
	switch spec.Target {
	case "openai":
		return "yingzo-openai-" + spec.OS + map[bool]string{true: "-x64", false: ""}[spec.OS == "windows"] + "-"
	case "claude-code":
		return "yingzo-claude-code-" + spec.OS + map[bool]string{true: "-x64", false: ""}[spec.OS == "windows"] + "-"
	case "claude-desktop":
		return "yingzo-claude-desktop-"
	default:
		return ""
	}
}

func artifactFilenameSuffix(spec yingzoArtifactSpec) string { return "." + spec.Format }

func readYingzoArchiveEntry(filename, archiveFormat, wanted string, maxBytes int64) (*yingzoArchiveEntryData, error) {
	if archiveFormat == "tar.gz" {
		file, err := os.Open(filename)
		if err != nil {
			return nil, err
		}
		defer func() { _ = file.Close() }()
		gzipReader, err := gzip.NewReader(file)
		if err != nil {
			return nil, err
		}
		defer func() { _ = gzipReader.Close() }()
		reader := tar.NewReader(gzipReader)
		for {
			header, nextErr := reader.Next()
			if errors.Is(nextErr, io.EOF) {
				break
			}
			if nextErr != nil {
				return nil, nextErr
			}
			if normalizeArchivePath(header.Name) != wanted {
				continue
			}
			data, readErr := io.ReadAll(io.LimitReader(reader, maxBytes+1))
			if readErr != nil || int64(len(data)) > maxBytes {
				return nil, errors.New("required archive entry is unreadable or too large")
			}
			return &yingzoArchiveEntryData{Data: data, Mode: os.FileMode(header.Mode)}, nil
		}
	} else {
		reader, err := zip.OpenReader(filename)
		if err != nil {
			return nil, err
		}
		defer func() { _ = reader.Close() }()
		for _, entry := range reader.File {
			if normalizeArchivePath(entry.Name) != wanted {
				continue
			}
			if entry.UncompressedSize64 > uint64(maxBytes) {
				return nil, errors.New("required archive entry is too large")
			}
			content, openErr := entry.Open()
			if openErr != nil {
				return nil, openErr
			}
			data, readErr := io.ReadAll(io.LimitReader(content, maxBytes+1))
			closeErr := content.Close()
			if readErr != nil || closeErr != nil || int64(len(data)) > maxBytes {
				return nil, errors.New("required archive entry is unreadable or too large")
			}
			return &yingzoArchiveEntryData{Data: data, Mode: entry.Mode()}, nil
		}
	}
	return nil, fmt.Errorf("package is missing %s", wanted)
}

func validateAMD64PE(data []byte) error {
	_, err := inspectAMD64PE(data)
	return err
}

func validateAMD64PEFile(file *pe.File) error {
	_, err := inspectAMD64PEFile(file)
	return err
}

func inspectAMD64PE(data []byte) (bool, error) {
	file, err := pe.NewFile(bytes.NewReader(data))
	if err != nil {
		return false, errors.New("binary must be a valid AMD64 PE executable")
	}
	defer func() { _ = file.Close() }()
	return inspectAMD64PEFile(file)
}

func inspectAMD64PEFile(file *pe.File) (bool, error) {
	if file == nil || file.FileHeader.Machine != pe.IMAGE_FILE_MACHINE_AMD64 {
		return false, errors.New("binary must be a valid AMD64 PE executable")
	}
	optional, ok := file.OptionalHeader.(*pe.OptionalHeader64)
	if !ok {
		return false, errors.New("binary must be a valid AMD64 PE executable")
	}
	return len(optional.DataDirectory) > 4 && optional.DataDirectory[4].Size > 0, nil
}

func yingzoTarEntryNames(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, errors.New("package is not a valid gzip archive")
	}
	defer func() { _ = gzipReader.Close() }()
	tarReader := tar.NewReader(gzipReader)
	entries := make([]string, 0)
	for {
		header, nextErr := tarReader.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return nil, errors.New("package tar stream is invalid")
		}
		entries = append(entries, normalizeArchivePath(header.Name))
	}
	return entries, nil
}

func yingzoZipEntryNames(filename string) ([]string, error) {
	reader, err := zip.OpenReader(filename)
	if err != nil {
		return nil, errors.New("package is not a valid ZIP archive")
	}
	defer func() { _ = reader.Close() }()
	entries := make([]string, 0, len(reader.File))
	for _, entry := range reader.File {
		entries = append(entries, normalizeArchivePath(entry.Name))
	}
	return entries, nil
}

func validateTarArchiveSafety(filename string) error {
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
	entries := 0
	var expanded int64
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
		if err := validateArchiveEntry(header.Name, header.Size); err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeReg, tar.TypeRegA, tar.TypeDir, tar.TypeXHeader, tar.TypeXGlobalHeader:
		default:
			return errors.New("package special files and links are not allowed")
		}
		if header.Size > yingzoExpandedArchiveMax-expanded {
			return errors.New("package expands beyond the safety limit")
		}
		expanded += header.Size
	}
	if entries == 0 {
		return errors.New("package archive is empty")
	}
	return nil
}

func validateZipArchiveSafety(filename string) error {
	reader, err := zip.OpenReader(filename)
	if err != nil {
		return errors.New("package is not a valid ZIP archive")
	}
	defer func() { _ = reader.Close() }()
	if len(reader.File) == 0 {
		return errors.New("package archive is empty")
	}
	if len(reader.File) > 20_000 {
		return errors.New("package contains too many entries")
	}
	var expanded int64
	for _, entry := range reader.File {
		if entry.UncompressedSize64 > uint64(yingzoExpandedArchiveMax) {
			return errors.New("package expands beyond the safety limit")
		}
		if err := validateArchiveEntry(entry.Name, int64(entry.UncompressedSize64)); err != nil {
			return err
		}
		if mode := entry.Mode(); !mode.IsRegular() && !mode.IsDir() {
			return errors.New("package special files and links are not allowed")
		}
		if entry.FileInfo().IsDir() {
			continue
		}
		content, openErr := entry.Open()
		if openErr != nil {
			return errors.New("package contains an unreadable ZIP entry")
		}
		readBytes, readErr := io.Copy(io.Discard, io.LimitReader(content, int64(entry.UncompressedSize64)+1))
		closeErr := content.Close()
		if readErr != nil || closeErr != nil || readBytes != int64(entry.UncompressedSize64) {
			return errors.New("package contains an incomplete or corrupt ZIP entry")
		}
		expanded += int64(entry.UncompressedSize64)
		if expanded > yingzoExpandedArchiveMax {
			return errors.New("package expands beyond the safety limit")
		}
	}
	return nil
}

func validateArchiveEntry(name string, size int64) error {
	normalized := strings.ReplaceAll(name, "\\", "/")
	clean := normalizeArchivePath(normalized)
	firstSegment := strings.SplitN(normalized, "/", 2)[0]
	if name == "" || path.IsAbs(normalized) || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(firstSegment, ":") {
		return errors.New("package contains an unsafe path")
	}
	if size < 0 {
		return errors.New("package contains an invalid entry size")
	}
	return nil
}

func normalizeArchivePath(name string) string {
	return path.Clean(strings.ReplaceAll(name, "\\", "/"))
}

func validateYingzoArchive(filename, packageFilename, hostFamily string) error {
	if err := validateTarArchiveSafety(filename); err != nil {
		return err
	}
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return err
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
	for {
		header, nextErr := tarReader.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return errors.New("package tar stream is invalid")
		}
		clean := filepath.ToSlash(filepath.Clean(header.Name))
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

func (h *AgentHandler) yingzoReleaseStorageRoot() (string, error) {
	configured := strings.TrimSpace(os.Getenv(yingzoReleaseStorageEnv))
	if configured == "" {
		return filepath.Join(h.dataDir, "releases"), nil
	}
	if !filepath.IsAbs(configured) {
		return "", errors.New("YINGZO_RELEASE_STORAGE_DIR must be an absolute path")
	}
	return filepath.Clean(configured), nil
}

func (h *AgentHandler) yingzoReleaseDirectory(releaseID uuid.UUID) (string, error) {
	root, err := h.yingzoReleaseStorageRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, releaseID.String()), nil
}

func (h *AgentHandler) removeYingzoStorageFile(releaseID uuid.UUID, storageKey string) {
	releaseDir, err := h.yingzoReleaseDirectory(releaseID)
	if err != nil {
		return
	}
	absolute, err := filepath.Abs(storageKey)
	if err != nil {
		return
	}
	relative, err := filepath.Rel(releaseDir, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return
	}
	_ = os.Remove(absolute)
}

func (h *AgentHandler) removeYingzoReleaseDirectory(releaseID uuid.UUID) {
	dir, err := h.yingzoReleaseDirectory(releaseID)
	if err == nil {
		_ = os.RemoveAll(dir)
	}
}

func (h *AgentHandler) CleanupYingzoReleaseTemporaryFiles(olderThan time.Time) (int, error) {
	root, err := h.yingzoReleaseStorageRoot()
	if err != nil {
		return 0, err
	}
	releases, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, releaseEntry := range releases {
		if !releaseEntry.IsDir() {
			continue
		}
		releaseID, parseErr := uuid.Parse(releaseEntry.Name())
		if parseErr != nil {
			continue
		}
		releaseDir, dirErr := h.yingzoReleaseDirectory(releaseID)
		if dirErr != nil {
			continue
		}
		entries, readErr := os.ReadDir(releaseDir)
		if readErr != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			info, infoErr := entry.Info()
			if infoErr != nil || !info.Mode().IsRegular() || !info.ModTime().Before(olderThan) {
				continue
			}
			candidate := filepath.Join(releaseDir, entry.Name())
			remove := strings.HasSuffix(entry.Name(), ".tmp")
			if !remove && h.db != nil {
				var referenced bool
				if queryErr := h.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM yingzo_release_artifacts WHERE release_id=$1 AND storage_key=$2)`, releaseID, candidate).Scan(&referenced); queryErr == nil {
					remove = !referenced
				}
			}
			if remove {
				if removeErr := os.Remove(candidate); removeErr == nil {
					removed++
				}
			}
		}
	}
	return removed, nil
}

func (h *AgentHandler) CleanupYingzoAbandonedDraftArtifacts(ctx context.Context, olderThan time.Time) (int, error) {
	if h.db == nil {
		return 0, nil
	}
	rows, err := h.db.QueryContext(ctx, `SELECT id FROM yingzo_releases WHERE status IN ('draft','disabled') AND updated_at < $1`, olderThan)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	ids := make([]uuid.UUID, 0)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	removed := 0
	for _, id := range ids {
		tx, beginErr := h.db.BeginTx(ctx, &sql.TxOptions{})
		if beginErr != nil {
			return removed, beginErr
		}
		var status string
		var updatedAt time.Time
		if lockErr := tx.QueryRowContext(ctx, `SELECT status,updated_at FROM yingzo_releases WHERE id=$1 FOR UPDATE`, id).Scan(&status, &updatedAt); lockErr != nil || (status != "draft" && status != "disabled") || !updatedAt.Before(olderThan) {
			_ = tx.Rollback()
			continue
		}
		artifactRows, queryErr := tx.QueryContext(ctx, `DELETE FROM yingzo_release_artifacts WHERE release_id=$1 RETURNING storage_key`, id)
		if queryErr != nil {
			_ = tx.Rollback()
			return removed, queryErr
		}
		storageKeys := make([]string, 0)
		for artifactRows.Next() {
			var storageKey string
			if scanErr := artifactRows.Scan(&storageKey); scanErr != nil {
				_ = artifactRows.Close()
				_ = tx.Rollback()
				return removed, scanErr
			}
			storageKeys = append(storageKeys, storageKey)
		}
		rowsErr := artifactRows.Err()
		_ = artifactRows.Close()
		if rowsErr != nil {
			_ = tx.Rollback()
			return removed, rowsErr
		}
		if _, updateErr := tx.ExecContext(ctx, `UPDATE yingzo_releases SET signature=NULL,updated_at=NOW() WHERE id=$1`, id); updateErr != nil {
			_ = tx.Rollback()
			return removed, updateErr
		}
		if commitErr := tx.Commit(); commitErr != nil {
			// Preserve files after an ambiguous commit; orphan reconciliation will
			// remove them only after confirming that no DB row references them.
			continue
		}
		for _, storageKey := range storageKeys {
			h.removeYingzoStorageFile(id, storageKey)
			removed++
		}
	}
	return removed, nil
}

func parsePositiveInt(raw string) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return 0, errors.New("value must be a positive integer")
	}
	return value, nil
}

func yingzoFileSHA256(filename string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
