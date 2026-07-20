package handler

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
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
	yingzoDistributionSchema3  = 3
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

type yingzoArtifactRequirement struct {
	ArtifactKind    string `json:"artifact_kind"`
	Target          string `json:"target"`
	OS              string `json:"os"`
	Arch            string `json:"arch"`
	Format          string `json:"format"`
	ContentType     string `json:"content_type"`
	PackageFilename string `json:"package_filename"`
}

type yingzoReleaseListItem struct {
	*yingzoRelease
	RequiredArtifacts []yingzoArtifactRequirement `json:"required_artifacts"`
	ArtifactCount     int                         `json:"artifact_count"`
}

// yingzoBatchArtifactError describes an individual file that could not be
// accepted.  Batch uploads intentionally report every bad file in one
// response so an administrator does not have to retry the upload one file at
// a time.
type yingzoBatchArtifactError struct {
	Filename string `json:"filename"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

type yingzoBatchSkippedArtifact struct {
	Filename string `json:"filename"`
	Reason   string `json:"reason"`
}

type yingzoBatchUploadResponse struct {
	Uploaded      []*yingzoReleaseArtifact     `json:"uploaded"`
	Skipped       []yingzoBatchSkippedArtifact `json:"skipped_duplicates"`
	Ignored       []string                     `json:"ignored_files"`
	Missing       []string                     `json:"missing_artifacts"`
	Complete      bool                         `json:"complete"`
	ExpectedCount int                          `json:"expected_count"`
	ReceivedCount int                          `json:"received_count"`
	Errors        []yingzoBatchArtifactError   `json:"errors,omitempty"`
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
	SchemaVersion             int                            `json:"schema_version"`
	DistributionSchemaVersion *int                           `json:"distribution_schema_version,omitempty"`
	Product                   string                         `json:"product"`
	Version                   string                         `json:"version"`
	Channel                   string                         `json:"channel"`
	RuntimeProtocol           int                            `json:"runtime_protocol"`
	CompleteArtifactMatrix    bool                           `json:"complete_artifact_matrix"`
	PublicSigningRequired     bool                           `json:"public_signing_required"`
	StableEligible            bool                           `json:"stable_eligible"`
	NativeSigning             yingzoNativeSigning            `json:"native_signing"`
	Artifacts                 []yingzoSignedManifestArtifact `json:"artifacts"`
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

// yingzoArtifactSpecsForSchema returns the immutable artifact contract for a
// distribution schema. Keep old schemas byte-for-byte compatible: installed
// clients may still request a schema-2 release after schema 3 is introduced.
func yingzoArtifactSpecsForSchema(schemaVersion int, version string) []yingzoArtifactSpec {
	if schemaVersion == yingzoDistributionSchema3 {
		return []yingzoArtifactSpec{
			{ArtifactKind: "host_package", Target: "openai", OS: "macos", Arch: "any", Format: "tar.gz", ContentType: "application/gzip", Filename: "yingzo-openai-macos-" + version + ".tar.gz"},
			{ArtifactKind: "host_package", Target: "openai", OS: "windows", Arch: "x64", Format: "zip", ContentType: "application/zip", Filename: "yingzo-openai-windows-x64-" + version + ".zip"},
			{ArtifactKind: "host_package", Target: "claude-code", OS: "macos", Arch: "any", Format: "zip", ContentType: "application/zip", Filename: "yingzo-claude-code-macos-" + version + ".zip"},
			{ArtifactKind: "host_package", Target: "claude-code", OS: "windows", Arch: "x64", Format: "zip", ContentType: "application/zip", Filename: "yingzo-claude-code-windows-x64-" + version + ".zip"},
			{ArtifactKind: "host_package", Target: "claude-desktop", OS: "any", Arch: "any", Format: "mcpb", ContentType: "application/zip", Filename: "yingzo-claude-desktop-" + version + ".mcpb"},
			{ArtifactKind: "runtime_installer", Target: "runtime", OS: "macos", Arch: "arm64", Format: "dmg", ContentType: "application/x-apple-diskimage", Filename: "yingzo-runtime-macos-arm64-" + version + ".dmg"},
			{ArtifactKind: "runtime_installer", Target: "runtime", OS: "windows", Arch: "x64", Format: "exe", ContentType: "application/vnd.microsoft.portable-executable", Filename: "yingzo-runtime-windows-x64-" + version + "-setup.exe"},
		}
	}
	if schemaVersion != yingzoDistributionSchema2 {
		return nil
	}
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

// yingzoV2ArtifactSpecs is retained as a named compatibility helper for
// schema-2 tests and callers. It must never silently switch to the current
// release matrix.
func yingzoV2ArtifactSpecs(version string) []yingzoArtifactSpec {
	return yingzoArtifactSpecsForSchema(yingzoDistributionSchema2, version)
}

func yingzoV3ArtifactSpecs(version string) []yingzoArtifactSpec {
	return yingzoArtifactSpecsForSchema(yingzoDistributionSchema3, version)
}

func yingzoArtifactRequirements(schemaVersion int, version string) []yingzoArtifactRequirement {
	specs := yingzoArtifactSpecsForSchema(schemaVersion, version)
	requirements := make([]yingzoArtifactRequirement, 0, len(specs))
	for _, spec := range specs {
		requirements = append(requirements, yingzoArtifactRequirement{
			ArtifactKind:    spec.ArtifactKind,
			Target:          spec.Target,
			OS:              spec.OS,
			Arch:            spec.Arch,
			Format:          spec.Format,
			ContentType:     spec.ContentType,
			PackageFilename: spec.Filename,
		})
	}
	return requirements
}

func yingzoDistributionSchemaSupported(schemaVersion int) bool {
	return schemaVersion == yingzoDistributionSchema2 || schemaVersion == yingzoDistributionSchema3
}

func findYingzoArtifactSpec(schemaVersion int, version, artifactKind, target, targetOS, arch string) (yingzoArtifactSpec, bool) {
	for _, spec := range yingzoArtifactSpecsForSchema(schemaVersion, version) {
		if spec.ArtifactKind == artifactKind && spec.Target == target && spec.OS == targetOS && spec.Arch == arch {
			return spec, true
		}
	}
	return yingzoArtifactSpec{}, false
}

func findYingzoV2Spec(version, artifactKind, target, targetOS, arch string) (yingzoArtifactSpec, bool) {
	return findYingzoArtifactSpec(yingzoDistributionSchema2, version, artifactKind, target, targetOS, arch)
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
				"message": "Claude Desktop requires a schema 2 or 3 release with an MCPB artifact",
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
		downloadURL, ok := descriptor["download_url"].(string)
		if !ok || downloadURL == "" {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "download_ticket_unavailable"}})
			return
		}
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
	if osErr != nil || archErr != nil || !yingzoPlatformSupportedForSchema(release.DistributionSchemaVersion, targetOS, arch) {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "unsupported_platform", "message": yingzoSupportedPlatformMessage(release.DistributionSchemaVersion)}})
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
	hostURL, ok := hostDescriptor["download_url"].(string)
	if !ok || hostURL == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "download_ticket_unavailable"}})
		return
	}
	runtimeHelperURI := ""
	if runtimeDescriptor != nil && runtimeResolution == "probe" {
		installerURL, _ := runtimeDescriptor["download_url"].(string)
		runtimeHelperURI = yingzoRuntimeEnsureURI(release, installerURL)
	}
	c.JSON(http.StatusOK, gin.H{
		"host": input.Host, "host_family": hostFamily, "channel": channel,
		"os": targetOS, "arch": arch, "version": release.Version, "signature": release.Signature,
		"stable_eligible": release.StableEligible, "native_signing": yingzoNativeSigningSummary(release),
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
		requestedOS := normalizeYingzoOSTrusted(ticket.RequestedOS)
		requestedArch := normalizeYingzoArchTrusted(ticket.RequestedArch)
		return artifact.Target == "runtime" && yingzoPlatformSupportedForSchema(release.DistributionSchemaVersion, requestedOS, requestedArch) && artifact.OS == requestedOS && artifact.Arch == requestedArch
	}
	requestedOS := normalizeYingzoOSTrusted(ticket.RequestedOS)
	requestedArch := normalizeYingzoArchTrusted(ticket.RequestedArch)
	if !yingzoPlatformSupportedForSchema(release.DistributionSchemaVersion, requestedOS, requestedArch) {
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
	items := make([]yingzoReleaseListItem, 0)
	for rows.Next() {
		release, scanErr := scanYingzoRelease(rows)
		if scanErr != nil || h.loadYingzoArtifacts(c, release) != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
			return
		}
		requiredArtifacts := yingzoArtifactRequirements(release.DistributionSchemaVersion, release.Version)
		items = append(items, yingzoReleaseListItem{
			yingzoRelease:     release,
			RequiredArtifacts: requiredArtifacts,
			ArtifactCount:     len(requiredArtifacts),
		})
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
	if c.ShouldBindJSON(&input) != nil || !yingzoVersionPattern.MatchString(strings.TrimSpace(input.Version)) || input.DistributionSchemaVersion != yingzoDistributionSchema3 || input.RuntimeProtocol <= 0 {
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
	// Stable/prerelease is an operator-selected channel, not a signing verdict.
	// A published prerelease may be promoted later without rebuilding its files.
	stableEligible := true
	_, err = h.db.ExecContext(c, `INSERT INTO yingzo_releases(id,version,status,distribution_schema_version,channel,stable_eligible,runtime_protocol,compatibility,signature,min_codex_version,min_claude_version,release_notes,created_by) VALUES($1,$2,'draft',$3,$4,$5,$6,$7::jsonb,NULL,$8,$9,$10,$11)`,
		releaseID, strings.TrimSpace(input.Version), input.DistributionSchemaVersion, channel, stableEligible, input.RuntimeProtocol, string(compatibilityJSON), strings.TrimSpace(input.MinCodexVersion), strings.TrimSpace(input.MinClaudeVersion), strings.TrimSpace(input.ReleaseNotes), subject.UserID)
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
		c.JSON(http.StatusCreated, gin.H{"id": releaseID, "version": input.Version, "status": "draft", "channel": channel, "distribution_schema_version": input.DistributionSchemaVersion})
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

// UploadYingzoReleaseArtifactsBatch accepts all installable files for a
// release in one multipart request. The package filename is the artifact
// contract, so callers do not have to duplicate target/platform form fields.
func (h *AgentHandler) UploadYingzoReleaseArtifactsBatch(c *gin.Context) {
	releaseID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "yingzo_release_not_found"}})
		return
	}
	if !strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "multipart/form-data") {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "multipart_form_required"}})
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
	if release.Status != "draft" || !yingzoDistributionSchemaSupported(release.DistributionSchemaVersion) {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "release_state_invalid"}})
		return
	}

	// Keep the request finite while allowing every package in the current
	// matrix to be uploaded together. The body is streamed; it is never held in
	// memory by this handler.
	specs := yingzoArtifactSpecsForSchema(release.DistributionSchemaVersion, release.Version)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, int64(len(specs))*yingzoReleaseMaxBytes+(16<<20))
	multipartReader, err := c.Request.MultipartReader()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_multipart"}})
		return
	}
	specByFilename := make(map[string]yingzoArtifactSpec, len(specs))
	for _, spec := range specs {
		spec.RuntimeProtocol = release.RuntimeProtocol
		specByFilename[spec.Filename] = spec
	}

	result := yingzoBatchUploadResponse{
		Uploaded: make([]*yingzoReleaseArtifact, 0, len(specs)),
		Skipped:  make([]yingzoBatchSkippedArtifact, 0),
		Ignored:  make([]string, 0),
		Errors:   make([]yingzoBatchArtifactError, 0),
	}
	staged := make([]*yingzoReleaseArtifact, 0, len(specs))
	// A batch retry may contain a newer build for a slot that already exists in
	// the draft. Keep the old row/file until the new row is committed, then
	// remove the old file. This makes retrying a whole folder safe and avoids
	// forcing the operator back into per-slot replacement controls.
	replacements := make(map[string]*yingzoReleaseArtifact, len(specs))
	oldStorageKeys := make([]string, 0, len(specs))
	committed := false
	defer func() {
		if committed {
			return
		}
		for _, artifact := range staged {
			h.removeYingzoStorageFile(releaseID, artifact.StorageKey)
		}
	}()

	seen := make(map[string]*yingzoReleaseArtifact, len(specs))
	for {
		part, nextErr := multipartReader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			result.Errors = append(result.Errors, yingzoBatchArtifactError{Code: "invalid_multipart", Message: "could not read multipart upload"})
			break
		}
		// Browsers normally send only File.name, but a hand-crafted multipart
		// request may contain either slash style. Normalize both before applying
		// the exact filename contract; storage never uses the client path.
		filename := filepath.Base(strings.ReplaceAll(strings.TrimSpace(part.FileName()), `\`, "/"))
		if filename == "." || filename == "" {
			_, _ = io.Copy(io.Discard, part)
			continue
		}
		if isYingzoReleaseMetadataFilename(filename) {
			if _, discardErr := io.Copy(io.Discard, part); discardErr != nil {
				result.Errors = append(result.Errors, yingzoBatchArtifactError{Filename: filename, Code: "package_upload_failed", Message: "could not read metadata file"})
			} else {
				result.Ignored = append(result.Ignored, filename)
			}
			continue
		}
		spec, ok := specByFilename[filename]
		if !ok {
			_, _ = io.Copy(io.Discard, part)
			result.Errors = append(result.Errors, yingzoBatchArtifactError{Filename: filename, Code: "invalid_package_filename", Message: "package filename is not part of this release matrix"})
			continue
		}

		artifact, storeErr := h.storeYingzoV2Artifact(part, filename, release, spec)
		if storeErr != nil {
			var uploadErr *yingzoUploadError
			if errors.As(storeErr, &uploadErr) {
				result.Errors = append(result.Errors, yingzoBatchArtifactError{Filename: filename, Code: uploadErr.code, Message: uploadErr.message})
			} else {
				result.Errors = append(result.Errors, yingzoBatchArtifactError{Filename: filename, Code: "storage_error", Message: "could not store package"})
			}
			continue
		}
		staged = append(staged, artifact)

		if previous, exists := seen[spec.key()]; exists {
			if previous.SHA256 == artifact.SHA256 && previous.SizeBytes == artifact.SizeBytes {
				h.removeYingzoStorageFile(releaseID, artifact.StorageKey)
				staged = staged[:len(staged)-1]
				result.Skipped = append(result.Skipped, yingzoBatchSkippedArtifact{Filename: filename, Reason: "duplicate_content"})
			} else {
				result.Errors = append(result.Errors, yingzoBatchArtifactError{Filename: filename, Code: "duplicate_artifact_conflict", Message: "the same artifact slot was uploaded with different content"})
			}
			continue
		}
		seen[spec.key()] = artifact

		existing := yingzoArtifactForTuple(release, spec.Target, spec.OS, spec.Arch)
		if existing != nil {
			if existing.PackageFilename == filename && existing.SHA256 == artifact.SHA256 && existing.SizeBytes == artifact.SizeBytes {
				h.removeYingzoStorageFile(releaseID, artifact.StorageKey)
				staged = staged[:len(staged)-1]
				delete(seen, spec.key())
				result.Skipped = append(result.Skipped, yingzoBatchSkippedArtifact{Filename: filename, Reason: "already_uploaded"})
			} else {
				replacements[spec.key()] = existing
			}
		}
	}

	if len(staged) == 0 && len(result.Errors) == 0 {
		result.Errors = append(result.Errors, yingzoBatchArtifactError{Code: "artifact_file_required", Message: "at least one Yingzo package file is required"})
	}
	if len(result.Errors) > 0 {
		c.JSON(http.StatusBadRequest, yingzoBatchResponsePayload(result, release, specs, staged))
		return
	}

	if len(staged) > 0 {
		if _, err = tx.ExecContext(c, `UPDATE yingzo_releases SET signature=NULL,updated_at=NOW() WHERE id=$1`, release.ID); err == nil {
			_, err = tx.ExecContext(c, `UPDATE yingzo_release_artifacts SET signature_status='unverified',updated_at=NOW() WHERE release_id=$1 AND NOT (os='windows' OR target='claude-desktop')`, release.ID)
		}
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}, "batch": yingzoBatchResponsePayload(result, release, specs, staged)})
		return
	}
	for _, artifact := range staged {
		existing := replacements[artifact.key()]
		if existing != nil {
			_, err = tx.ExecContext(c, `UPDATE yingzo_release_artifacts SET package_filename=$2,storage_backend=$3,storage_key=$4,size_bytes=$5,sha256=$6,content_type=$7,format=$8,runtime_protocol=$9,validation_status='validated',signature_status=$10,validated_at=NOW(),updated_at=NOW() WHERE id=$1 AND release_id=$11`, existing.ID, artifact.PackageFilename, artifact.StorageBackend, artifact.StorageKey, artifact.SizeBytes, artifact.SHA256, artifact.ContentType, artifact.Format, artifact.RuntimeProtocol, artifact.SignatureStatus, artifact.ReleaseID)
			if err == nil {
				artifact.ID = existing.ID
				oldStorageKeys = append(oldStorageKeys, existing.StorageKey)
			}
		} else {
			_, err = tx.ExecContext(c, `INSERT INTO yingzo_release_artifacts(id,release_id,artifact_kind,target,os,arch,format,content_type,runtime_protocol,validation_status,signature_status,validated_at,package_filename,storage_backend,storage_key,size_bytes,sha256) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'validated',$10,NOW(),$11,$12,$13,$14,$15)`, artifact.ID, artifact.ReleaseID, artifact.ArtifactKind, artifact.Target, artifact.OS, artifact.Arch, artifact.Format, artifact.ContentType, artifact.RuntimeProtocol, artifact.SignatureStatus, artifact.PackageFilename, artifact.StorageBackend, artifact.StorageKey, artifact.SizeBytes, artifact.SHA256)
		}
		if err != nil {
			var pqErr *pq.Error
			if errors.As(err, &pqErr) && pqErr.Code == "23505" {
				result.Errors = append(result.Errors, yingzoBatchArtifactError{Filename: artifact.PackageFilename, Code: "release_artifact_exists", Message: "this artifact slot already exists"})
			} else {
				result.Errors = append(result.Errors, yingzoBatchArtifactError{Filename: artifact.PackageFilename, Code: "database_error", Message: "could not register package"})
			}
			c.JSON(http.StatusConflict, yingzoBatchResponsePayload(result, release, specs, staged))
			return
		}
	}
	if err := tx.Commit(); err != nil {
		// Commit errors are ambiguous. Keep files so a committed row can never
		// point at a path that this request already deleted.
		committed = true
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}, "batch": yingzoBatchResponsePayload(result, release, specs, staged)})
		return
	}
	committed = true
	for _, storageKey := range oldStorageKeys {
		h.removeYingzoStorageFile(releaseID, storageKey)
	}
	result.Uploaded = append(result.Uploaded, staged...)
	c.JSON(http.StatusOK, yingzoBatchResponsePayload(result, release, specs, staged))
}

// isYingzoReleaseMetadataFilename identifies release-sidecar files that are
// not installable artifacts. The check is intentionally conservative: an
// unknown package-like filename is an error, never silently dropped.
func isYingzoReleaseMetadataFilename(filename string) bool {
	name := strings.ToLower(filepath.Base(strings.TrimSpace(filename)))
	if name == "" {
		return false
	}
	for _, marker := range []string{"checksum", "sha256", "sha512", "sha1", "sbom", "signature", "manifest", "signing-proof"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	if strings.HasPrefix(name, "yingzo-release-") && strings.HasSuffix(name, ".json") {
		return true
	}
	if name == ".ds_store" || name == "packages.json" || name == "release.json" || name == "license" || name == "notice" {
		return true
	}
	// Release folders often contain human-readable notes and machine-readable
	// build metadata. None of these can be an installable package in the
	// schema-2/3 matrix, so they are safe to ignore when a whole directory is
	// selected in the admin UI.
	for _, suffix := range []string{".json", ".txt", ".md", ".yaml", ".yml", ".toml", ".xml", ".plist"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	for _, suffix := range []string{".sig", ".asc", ".sha256", ".sha512", ".sha1", ".md5", ".pem"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	if name == "checksums.txt" || name == "sha256sums.txt" || name == "sha512sums.txt" || name == "packages.json" {
		return true
	}
	// Release manifests and signing proofs are useful evidence, but they are
	// not installable artifacts and may be selected when a whole release folder
	// is chosen in the browser.
	return strings.HasPrefix(name, "yingzo-release-") || strings.HasPrefix(name, "signing-proof-") || strings.HasPrefix(name, "host-package-proof-") || strings.HasPrefix(name, "yingzo-sbom-")
}

func yingzoBatchResponsePayload(result yingzoBatchUploadResponse, release *yingzoRelease, specs []yingzoArtifactSpec, staged []*yingzoReleaseArtifact) yingzoBatchUploadResponse {
	payload := result
	payload.ExpectedCount = len(specs)
	present := make(map[string]struct{}, len(release.Artifacts)+len(staged))
	for key := range release.Artifacts {
		present[key] = struct{}{}
	}
	for _, artifact := range staged {
		present[artifact.key()] = struct{}{}
	}
	payload.ReceivedCount = len(present)
	payload.Missing = make([]string, 0)
	for _, spec := range specs {
		if _, ok := present[spec.key()]; !ok {
			payload.Missing = append(payload.Missing, spec.Filename)
		}
	}
	payload.Complete = len(payload.Missing) == 0
	return payload
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
	if release.Status != "draft" || !yingzoDistributionSchemaSupported(release.DistributionSchemaVersion) {
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
	if release.Status != "draft" || !yingzoDistributionSchemaSupported(release.DistributionSchemaVersion) {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "release_state_invalid"}})
		return
	}
	spec, ok := findYingzoArtifactSpec(release.DistributionSchemaVersion, release.Version, artifactKind, target, targetOS, arch)
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
	if _, err = tx.ExecContext(c, `UPDATE yingzo_releases SET signature=NULL,updated_at=NOW() WHERE id=$1`, release.ID); err == nil {
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
	if release.Status != "draft" || !yingzoDistributionSchemaSupported(release.DistributionSchemaVersion) {
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
		_, err = tx.ExecContext(c, `UPDATE yingzo_releases SET signature=NULL,updated_at=NOW() WHERE id=$1`, releaseID)
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
	manifestDistributionSchema := yingzoDistributionSchema2
	if manifest.DistributionSchemaVersion != nil {
		manifestDistributionSchema = *manifest.DistributionSchemaVersion
	}
	if !yingzoDistributionSchemaSupported(release.DistributionSchemaVersion) || manifestDistributionSchema != release.DistributionSchemaVersion {
		return &yingzoUploadError{status: http.StatusUnprocessableEntity, code: "release_manifest_mismatch", message: "release manifest distribution schema does not match the draft"}
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
	expectedSpecs := yingzoArtifactSpecsForSchema(release.DistributionSchemaVersion, release.Version)
	if len(expectedSpecs) == 0 {
		return &yingzoUploadError{status: http.StatusConflict, code: "release_state_invalid", message: "unsupported Yingzo distribution schema"}
	}
	if len(manifest.Artifacts) != len(expectedSpecs) || len(release.Artifacts) != len(expectedSpecs) {
		return &yingzoUploadError{status: http.StatusConflict, code: "release_artifacts_incomplete", message: fmt.Sprintf("release proof requires the exact %d-artifact matrix", len(expectedSpecs))}
	}
	seen := make(map[string]bool, len(expectedSpecs))
	windowsUnsignedDetected := false
	for _, signed := range manifest.Artifacts {
		spec, ok := findYingzoArtifactSpec(release.DistributionSchemaVersion, release.Version, signed.ArtifactKind, signed.Target, signed.OS, signed.Arch)
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
	if err := tx.QueryRowContext(c, `SELECT status,channel,distribution_schema_version,stable_eligible,runtime_protocol,version,signature FROM yingzo_releases WHERE id=$1 FOR UPDATE`, id).Scan(&status, &channel, &schemaVersion, &stableEligible, &runtimeProtocol, &version, &releaseSignature); err != nil || status != "published" || channel != "prerelease" || !yingzoDistributionSchemaSupported(schemaVersion) {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "release_state_invalid"}})
		return
	}
	if err = validateYingzoPublishMatrix(c, tx, id, version, runtimeProtocol, schemaVersion); err != nil {
		var uploadErr *yingzoUploadError
		if errors.As(err, &uploadErr) {
			c.JSON(uploadErr.status, gin.H{"error": gin.H{"code": uploadErr.code, "message": uploadErr.message}})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		}
		return
	}
	if _, err = tx.ExecContext(c, `UPDATE yingzo_releases SET status='superseded',updated_at=NOW() WHERE status='published' AND channel='stable' AND id<>$1`, id); err == nil {
		_, err = tx.ExecContext(c, `UPDATE yingzo_releases SET channel='stable',stable_eligible=TRUE,published_at=NOW(),published_by=$2,updated_at=NOW() WHERE id=$1 AND status='published' AND channel='prerelease'`, id, subject.UserID)
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
	} else if yingzoDistributionSchemaSupported(schemaVersion) {
		err = validateYingzoPublishMatrix(c, tx, id, version, runtimeProtocol, schemaVersion)
		if err != nil {
			var uploadErr *yingzoUploadError
			if errors.As(err, &uploadErr) {
				c.JSON(uploadErr.status, gin.H{"error": gin.H{"code": uploadErr.code, "message": uploadErr.message}})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
			}
			return
		}
	} else {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "release_state_invalid"}})
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

func validateYingzoPublishMatrix(ctx context.Context, tx *sql.Tx, releaseID uuid.UUID, version string, runtimeProtocol, schemaVersion int) error {
	expectedSpecs := yingzoArtifactSpecsForSchema(schemaVersion, version)
	if len(expectedSpecs) == 0 {
		return &yingzoUploadError{status: http.StatusConflict, code: "release_state_invalid", message: "unsupported Yingzo distribution schema"}
	}
	rows, err := tx.QueryContext(ctx, `SELECT artifact_kind,target,os,arch,runtime_protocol,validation_status,signature_status,package_filename,storage_key,size_bytes,sha256 FROM yingzo_release_artifacts WHERE release_id=$1`, releaseID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	found := map[string]bool{}
	for rows.Next() {
		var kind, target, targetOS, arch, validationStatus, signatureStatus, packageFilename, storageKey, expectedSHA string
		var artifactProtocol int
		var expectedSize int64
		if err := rows.Scan(&kind, &target, &targetOS, &arch, &artifactProtocol, &validationStatus, &signatureStatus, &packageFilename, &storageKey, &expectedSize, &expectedSHA); err != nil {
			return err
		}
		spec, ok := findYingzoArtifactSpec(schemaVersion, version, kind, target, targetOS, arch)
		if !ok || packageFilename != spec.Filename || artifactProtocol != runtimeProtocol || validationStatus != "validated" {
			return &yingzoUploadError{status: http.StatusConflict, code: "release_artifacts_invalid", message: "every release artifact must match the release filename, platform slot, and protocol"}
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
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(found) != len(expectedSpecs) {
		return &yingzoUploadError{status: http.StatusConflict, code: "release_artifacts_incomplete", message: fmt.Sprintf("all %d release artifacts are required before publishing", len(expectedSpecs))}
	}
	for _, spec := range expectedSpecs {
		if !found[spec.key()] {
			return &yingzoUploadError{status: http.StatusConflict, code: "release_artifacts_incomplete", message: fmt.Sprintf("all %d release artifacts are required before publishing", len(expectedSpecs))}
		}
	}
	return nil
}

func validateYingzoV2PublishMatrix(ctx context.Context, tx *sql.Tx, releaseID uuid.UUID, version string, runtimeProtocol int) error {
	return validateYingzoPublishMatrix(ctx, tx, releaseID, version, runtimeProtocol, yingzoDistributionSchema2)
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
	if status == "draft" && yingzoDistributionSchemaSupported(schemaVersion) {
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

// PurgeYingzoRelease permanently removes a release that has never been
// published. This is intentionally separate from DisableYingzoRelease:
// published history must remain available for audit and rollback, while an
// empty or abandoned draft must not permanently reserve its version number.
func (h *AgentHandler) PurgeYingzoRelease(c *gin.Context) {
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
	var publishedAt sql.NullTime
	if err := tx.QueryRowContext(c, `SELECT status,published_at FROM yingzo_releases WHERE id=$1 FOR UPDATE`, id).Scan(&status, &publishedAt); errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "yingzo_release_not_found"}})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	if publishedAt.Valid || (status != "draft" && status != "disabled") {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{
			"code":    "release_purge_not_allowed",
			"message": "only a draft or an unpublished disabled release can be permanently deleted",
		}})
		return
	}

	artifactRows, err := tx.QueryContext(c, `DELETE FROM yingzo_release_artifacts WHERE release_id=$1 RETURNING storage_key`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	for artifactRows.Next() {
		// The whole release directory is removed after commit. We still consume
		// the RETURNING rows so PostgreSQL can finish the statement cleanly.
		var ignoredStorageKey string
		if scanErr := artifactRows.Scan(&ignoredStorageKey); scanErr != nil {
			_ = artifactRows.Close()
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
			return
		}
	}
	if err := artifactRows.Err(); err != nil {
		_ = artifactRows.Close()
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	if err := artifactRows.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	if _, err = tx.ExecContext(c, `DELETE FROM yingzo_releases WHERE id=$1`, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}
	if err := tx.Commit(); err != nil {
		// Do not remove files when the transaction outcome is ambiguous.
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "database_error"}})
		return
	}

	// A draft can contain failed uploads and orphaned temporary files, so
	// remove the complete per-release directory after the DB commit.
	h.removeYingzoReleaseDirectory(id)
	c.JSON(http.StatusOK, gin.H{"deleted": true, "id": id})
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
	if !yingzoPlatformSupportedForSchema(release.DistributionSchemaVersion, targetOS, arch) {
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

func yingzoPlatformSupportedForSchema(schemaVersion int, targetOS, arch string) bool {
	if schemaVersion == yingzoDistributionSchema3 {
		return (targetOS == "macos" && arch == "arm64") || (targetOS == "windows" && arch == "x64")
	}
	if schemaVersion == yingzoDistributionSchema2 {
		return (targetOS == "macos" && (arch == "arm64" || arch == "x64")) || (targetOS == "windows" && arch == "x64")
	}
	return false
}

func yingzoSupportedPlatformMessage(schemaVersion int) string {
	if schemaVersion == yingzoDistributionSchema3 {
		return "supported platforms are macos/arm64 and windows/x64"
	}
	return "supported platforms are macos/arm64, macos/x64, and windows/x64"
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
	requiredArtifacts := yingzoArtifactRequirements(release.DistributionSchemaVersion, release.Version)
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
		"required_artifacts": requiredArtifacts, "artifact_count": len(requiredArtifacts),
		"signature_status":  map[bool]string{true: "signed", false: "unsigned"}[release.Signature != ""],
		"native_signing":    yingzoNativeSigningSummary(release),
		"min_codex_version": release.MinCodexVersion, "min_claude_version": release.MinClaudeVersion,
		"release_notes": release.ReleaseNotes, "published_at": release.PublishedAt,
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

func yingzoV2InstallPrompt(host string, release *yingzoRelease, hostURL string, runtimeDescriptor gin.H, runtimeResolution string) string {
	message := fmt.Sprintf("请安装或升级 Yingzo（影作）到版本 %s。宿主安装包：%s。保留现有 Yingzo 认证和项目数据，安装后验证 Runtime 协议 %d 与 MCP 连接。", release.Version, hostURL, release.RuntimeProtocol)
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

func yingzoWindowsBearingSpec(spec yingzoArtifactSpec) bool {
	return spec.OS == "windows" || spec.Target == "claude-desktop"
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
		if h.db != nil {
			var releaseExists bool
			if queryErr := h.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM yingzo_releases WHERE id=$1)`, releaseID).Scan(&releaseExists); queryErr == nil && !releaseExists {
				// A just-created directory can belong to a request whose DB
				// transaction has not committed yet. Wait until every entry is
				// older than the normal cleanup window before removing it wholesale.
				allStale := true
				for _, entry := range entries {
					info, infoErr := entry.Info()
					if infoErr != nil || !info.ModTime().Before(olderThan) {
						allStale = false
						break
					}
				}
				if allStale {
					if removeErr := os.RemoveAll(releaseDir); removeErr == nil {
						removed++
					}
					continue
				}
			}
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
	// Only an explicitly disabled, never-published release is considered
	// abandoned by the background worker. Drafts may be intentionally paused;
	// permanent deletion of a draft remains an explicit administrator action.
	rows, err := h.db.QueryContext(ctx, `SELECT id FROM yingzo_releases WHERE status='disabled' AND published_at IS NULL AND updated_at < $1`, olderThan)
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
		var publishedAt sql.NullTime
		var updatedAt time.Time
		if lockErr := tx.QueryRowContext(ctx, `SELECT status,published_at,updated_at FROM yingzo_releases WHERE id=$1 FOR UPDATE`, id).Scan(&status, &publishedAt, &updatedAt); lockErr != nil || publishedAt.Valid || status != "disabled" || !updatedAt.Before(olderThan) {
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
		if _, deleteErr := tx.ExecContext(ctx, `DELETE FROM yingzo_releases WHERE id=$1`, id); deleteErr != nil {
			_ = tx.Rollback()
			return removed, deleteErr
		}
		if commitErr := tx.Commit(); commitErr != nil {
			// Preserve files after an ambiguous commit; orphan reconciliation will
			// remove them only after confirming that no DB row references them.
			continue
		}
		// The row is gone and no published history can reference this directory.
		// Remove failed uploads and temporary files along with registered artifacts.
		h.removeYingzoReleaseDirectory(id)
		removed += len(storageKeys)
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
