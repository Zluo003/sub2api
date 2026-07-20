package handler

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

const (
	yingzoReleaseChunkBytes = int64(8 << 20)
	yingzoUploadSessionTTL  = 24 * time.Hour
)

var (
	yingzoClientUploadIDPattern    = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._:-]{7,127}$`)
	yingzoSHA256Pattern            = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)
	errYingzoCompletedArtifactGone = errors.New("completed artifact no longer exists")
)

const yingzoUploadSessionColumns = `id,client_upload_id,release_id,created_by,artifact_kind,target,os,arch,format,content_type,runtime_protocol,package_filename,total_bytes,received_bytes,COALESCE(expected_sha256,''),temp_storage_key,COALESCE(last_chunk_offset,-1),COALESCE(last_chunk_size,-1),COALESCE(last_chunk_sha256,''),status,COALESCE(completed_artifact_id::text,''),expires_at,created_at,updated_at`

type yingzoUploadSession struct {
	ID                  uuid.UUID
	ClientUploadID      string
	ReleaseID           uuid.UUID
	CreatedBy           int64
	ArtifactKind        string
	Target              string
	OS                  string
	Arch                string
	Format              string
	ContentType         string
	RuntimeProtocol     int
	PackageFilename     string
	TotalBytes          int64
	ReceivedBytes       int64
	ExpectedSHA256      string
	TempStorageKey      string
	LastChunkOffset     int64
	LastChunkSize       int64
	LastChunkSHA256     string
	Status              string
	CompletedArtifactID uuid.UUID
	ExpiresAt           time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type yingzoUploadSessionScanner interface{ Scan(dest ...any) error }

func scanYingzoUploadSession(scanner yingzoUploadSessionScanner) (*yingzoUploadSession, error) {
	var session yingzoUploadSession
	var completedArtifactID string
	err := scanner.Scan(
		&session.ID, &session.ClientUploadID, &session.ReleaseID, &session.CreatedBy,
		&session.ArtifactKind, &session.Target, &session.OS, &session.Arch,
		&session.Format, &session.ContentType, &session.RuntimeProtocol,
		&session.PackageFilename, &session.TotalBytes, &session.ReceivedBytes,
		&session.ExpectedSHA256, &session.TempStorageKey, &session.LastChunkOffset,
		&session.LastChunkSize, &session.LastChunkSHA256, &session.Status, &completedArtifactID,
		&session.ExpiresAt, &session.CreatedAt, &session.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if completedArtifactID != "" {
		session.CompletedArtifactID, err = uuid.Parse(completedArtifactID)
		if err != nil {
			return nil, err
		}
	}
	return &session, nil
}

type yingzoUploadInitInput struct {
	ClientUploadID  string `json:"client_upload_id"`
	ArtifactKind    string `json:"artifact_kind"`
	Target          string `json:"target"`
	OS              string `json:"os"`
	Arch            string `json:"arch"`
	RuntimeProtocol int    `json:"runtime_protocol"`
	PackageFilename string `json:"package_filename"`
	TotalBytes      int64  `json:"total_bytes"`
	SizeBytes       int64  `json:"size_bytes"`
	SHA256          string `json:"sha256"`
}

type yingzoUploadCompleteInput struct {
	SHA256 string `json:"sha256"`
}

type yingzoUploadReleaseState struct {
	Version         string
	Status          string
	SchemaVersion   int
	RuntimeProtocol int
}

func lockYingzoUploadRelease(ctx context.Context, tx *sql.Tx, releaseID uuid.UUID) (*yingzoUploadReleaseState, error) {
	var state yingzoUploadReleaseState
	err := tx.QueryRowContext(ctx, `SELECT version,status,distribution_schema_version,runtime_protocol FROM yingzo_releases WHERE id=$1 FOR UPDATE`, releaseID).
		Scan(&state.Version, &state.Status, &state.SchemaVersion, &state.RuntimeProtocol)
	return &state, err
}

func writeYingzoUploadAPIError(c *gin.Context, status int, code, message string, extra gin.H) {
	payload := gin.H{"error": gin.H{"code": code, "message": message}}
	for key, value := range extra {
		payload[key] = value
	}
	c.JSON(status, payload)
}

func yingzoUploadTotalBytes(input yingzoUploadInitInput) (int64, bool) {
	if input.TotalBytes < 0 || input.SizeBytes < 0 {
		return 0, false
	}
	if input.TotalBytes > 0 && input.SizeBytes > 0 && input.TotalBytes != input.SizeBytes {
		return 0, false
	}
	if input.TotalBytes > 0 {
		return input.TotalBytes, true
	}
	if input.SizeBytes > 0 {
		return input.SizeBytes, true
	}
	return 0, false
}

func normalizeYingzoExpectedSHA256(raw string) (string, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "", true
	}
	return value, yingzoSHA256Pattern.MatchString(value)
}

func (h *AgentHandler) yingzoUploadPartPath(releaseID, uploadID uuid.UUID) (string, error) {
	releaseDir, err := h.yingzoReleaseDirectory(releaseID)
	if err != nil {
		return "", err
	}
	return filepath.Join(releaseDir, ".uploads", uploadID.String()+".part"), nil
}

func (h *AgentHandler) validateYingzoUploadPartPath(session *yingzoUploadSession) (string, error) {
	if session == nil {
		return "", errors.New("upload session is nil")
	}
	expected, err := h.yingzoUploadPartPath(session.ReleaseID, session.ID)
	if err != nil {
		return "", err
	}
	expectedAbs, err := filepath.Abs(expected)
	if err != nil {
		return "", err
	}
	storedAbs, err := filepath.Abs(session.TempStorageKey)
	if err != nil || filepath.Clean(storedAbs) != filepath.Clean(expectedAbs) {
		return "", errors.New("upload session storage path is invalid")
	}
	return expectedAbs, nil
}

func yingzoUploadSessionPayload(session *yingzoUploadSession, resumed bool) gin.H {
	return gin.H{
		"upload_id":        session.ID,
		"client_upload_id": session.ClientUploadID,
		"offset":           session.ReceivedBytes,
		"total_bytes":      session.TotalBytes,
		"chunk_size":       yingzoReleaseChunkBytes,
		"status":           session.Status,
		"expires_at":       session.ExpiresAt,
		"resumed":          resumed,
	}
}

func (h *AgentHandler) yingzoUploadSessionPayloadWithArtifact(ctx context.Context, session *yingzoUploadSession, resumed bool) (gin.H, error) {
	payload := yingzoUploadSessionPayload(session, resumed)
	if session.Status != "completed" {
		return payload, nil
	}
	if session.CompletedArtifactID == uuid.Nil {
		return nil, errYingzoCompletedArtifactGone
	}
	release, err := h.getYingzoRelease(ctx, session.ReleaseID)
	if err != nil {
		return nil, err
	}
	artifact := yingzoArtifactByID(release, session.CompletedArtifactID)
	if artifact == nil {
		return nil, errYingzoCompletedArtifactGone
	}
	payload["artifact"] = artifact
	return payload, nil
}

func yingzoUploadSessionMatches(session *yingzoUploadSession, input yingzoUploadInitInput, totalBytes int64, expectedSHA string, spec yingzoArtifactSpec, userID int64) bool {
	return session != nil && session.CreatedBy == userID && session.ClientUploadID == input.ClientUploadID &&
		session.ArtifactKind == spec.ArtifactKind && session.Target == spec.Target && session.OS == spec.OS &&
		session.Arch == spec.Arch && session.Format == spec.Format && session.ContentType == spec.ContentType &&
		session.RuntimeProtocol == input.RuntimeProtocol && session.PackageFilename == spec.Filename &&
		session.TotalBytes == totalBytes && session.ExpectedSHA256 == expectedSHA
}

// InitYingzoReleaseArtifactUpload creates or resumes the one durable upload
// session for a release slot. The client id is an idempotency key; upload_id is
// the only locator used by subsequent requests.
func (h *AgentHandler) InitYingzoReleaseArtifactUpload(c *gin.Context) {
	releaseID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeYingzoUploadAPIError(c, http.StatusNotFound, "yingzo_release_not_found", "release not found", nil)
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		writeYingzoUploadAPIError(c, http.StatusUnauthorized, "unauthorized", "administrator identity is required", nil)
		return
	}
	var input yingzoUploadInitInput
	if c.ShouldBindJSON(&input) != nil {
		writeYingzoUploadAPIError(c, http.StatusBadRequest, "invalid_upload_init", "upload metadata must be valid JSON", nil)
		return
	}
	input.ClientUploadID = strings.TrimSpace(input.ClientUploadID)
	rawFilename := strings.TrimSpace(input.PackageFilename)
	normalizedFilename := strings.ReplaceAll(rawFilename, `\`, "/")
	if rawFilename == "" || filepath.Base(normalizedFilename) != normalizedFilename {
		writeYingzoUploadAPIError(c, http.StatusBadRequest, "invalid_package_filename", "package filename must not contain a path", nil)
		return
	}
	input.PackageFilename = normalizedFilename
	if !yingzoClientUploadIDPattern.MatchString(input.ClientUploadID) {
		writeYingzoUploadAPIError(c, http.StatusBadRequest, "invalid_client_upload_id", "client_upload_id must be an opaque 8-128 character token", nil)
		return
	}
	totalBytes, validTotal := yingzoUploadTotalBytes(input)
	expectedSHA, validSHA := normalizeYingzoExpectedSHA256(input.SHA256)
	if !validTotal || !validSHA || input.RuntimeProtocol <= 0 {
		writeYingzoUploadAPIError(c, http.StatusBadRequest, "invalid_upload_metadata", "size, hash, or runtime protocol is invalid", nil)
		return
	}

	tx, err := h.db.BeginTx(c, &sql.TxOptions{})
	if err != nil {
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "could not start upload transaction", nil)
		return
	}
	defer func() { _ = tx.Rollback() }()
	state, err := lockYingzoUploadRelease(c, tx, releaseID)
	if errors.Is(err, sql.ErrNoRows) {
		writeYingzoUploadAPIError(c, http.StatusNotFound, "yingzo_release_not_found", "release not found", nil)
		return
	}
	if err != nil {
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "could not read release", nil)
		return
	}
	if state.Status != "draft" || !yingzoDistributionSchemaSupported(state.SchemaVersion) {
		writeYingzoUploadAPIError(c, http.StatusConflict, "release_state_invalid", "only a supported draft release can accept uploads", nil)
		return
	}
	spec, found := findYingzoArtifactSpec(state.SchemaVersion, state.Version, input.ArtifactKind, input.Target, input.OS, input.Arch)
	if !found || input.PackageFilename != spec.Filename {
		writeYingzoUploadAPIError(c, http.StatusBadRequest, "invalid_package_filename", "package filename or release slot is invalid", nil)
		return
	}
	if input.RuntimeProtocol != state.RuntimeProtocol {
		writeYingzoUploadAPIError(c, http.StatusBadRequest, "runtime_protocol_mismatch", "runtime protocol does not match the release", nil)
		return
	}
	spec.RuntimeProtocol = state.RuntimeProtocol

	row := tx.QueryRowContext(c, `SELECT `+yingzoUploadSessionColumns+` FROM yingzo_release_upload_sessions WHERE release_id=$1 AND target=$2 AND os=$3 AND arch=$4 FOR UPDATE`, releaseID, spec.Target, spec.OS, spec.Arch)
	existing, queryErr := scanYingzoUploadSession(row)
	if queryErr != nil && !errors.Is(queryErr, sql.ErrNoRows) {
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "could not read upload session", nil)
		return
	}
	now := time.Now().UTC()
	if queryErr == nil && (existing.Status == "active" || existing.Status == "finalizing" || (existing.Status == "completed" && existing.CompletedArtifactID != uuid.Nil)) && existing.ExpiresAt.After(now) &&
		yingzoUploadSessionMatches(existing, input, totalBytes, expectedSHA, spec, subject.UserID) {
		if _, err = tx.ExecContext(c, `UPDATE yingzo_release_upload_sessions SET expires_at=$2,updated_at=NOW() WHERE id=$1`, existing.ID, now.Add(yingzoUploadSessionTTL)); err != nil {
			writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "could not resume upload", nil)
			return
		}
		existing.ExpiresAt = now.Add(yingzoUploadSessionTTL)
		if err = tx.Commit(); err != nil {
			writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "could not resume upload", nil)
			return
		}
		payload, payloadErr := h.yingzoUploadSessionPayloadWithArtifact(c, existing, true)
		if payloadErr != nil {
			writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "could not resume completed upload", nil)
			return
		}
		c.JSON(http.StatusOK, payload)
		return
	}
	if queryErr == nil && existing.CreatedBy != subject.UserID && existing.ExpiresAt.After(now) && (existing.Status == "active" || existing.Status == "finalizing") {
		writeYingzoUploadAPIError(c, http.StatusConflict, "upload_slot_busy", "another administrator owns the active upload for this slot", nil)
		return
	}

	var stale *yingzoUploadSession
	if queryErr == nil {
		stale = existing
		if _, err = tx.ExecContext(c, `DELETE FROM yingzo_release_upload_sessions WHERE id=$1`, existing.ID); err != nil {
			writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "could not reset upload session", nil)
			return
		}
	}
	uploadID := uuid.New()
	tempPath, err := h.yingzoUploadPartPath(releaseID, uploadID)
	if err != nil {
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "storage_error", "could not resolve upload storage", nil)
		return
	}
	session := &yingzoUploadSession{
		ID: uploadID, ClientUploadID: input.ClientUploadID, ReleaseID: releaseID, CreatedBy: subject.UserID,
		ArtifactKind: spec.ArtifactKind, Target: spec.Target, OS: spec.OS, Arch: spec.Arch, Format: spec.Format,
		ContentType: spec.ContentType, RuntimeProtocol: state.RuntimeProtocol, PackageFilename: spec.Filename,
		TotalBytes: totalBytes, ExpectedSHA256: expectedSHA, TempStorageKey: tempPath, Status: "active",
		ExpiresAt: now.Add(yingzoUploadSessionTTL), CreatedAt: now, UpdatedAt: now,
		LastChunkOffset: -1, LastChunkSize: -1,
	}
	_, err = tx.ExecContext(c, `INSERT INTO yingzo_release_upload_sessions(id,client_upload_id,release_id,created_by,artifact_kind,target,os,arch,format,content_type,runtime_protocol,package_filename,total_bytes,received_bytes,expected_sha256,temp_storage_key,status,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,0,NULLIF($14,''),$15,'active',$16)`,
		session.ID, session.ClientUploadID, session.ReleaseID, session.CreatedBy, session.ArtifactKind, session.Target,
		session.OS, session.Arch, session.Format, session.ContentType, session.RuntimeProtocol, session.PackageFilename,
		session.TotalBytes, session.ExpectedSHA256, session.TempStorageKey, session.ExpiresAt)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			writeYingzoUploadAPIError(c, http.StatusConflict, "upload_init_conflict", "client_upload_id or release slot is already active", nil)
			return
		}
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "could not create upload session", nil)
		return
	}
	if err = tx.Commit(); err != nil {
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "could not create upload session", nil)
		return
	}
	if stale != nil {
		_ = h.removeYingzoUploadPart(stale)
	}
	c.JSON(http.StatusCreated, yingzoUploadSessionPayload(session, false))
}

func (h *AgentHandler) getOwnedYingzoUploadSession(ctx context.Context, querier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, releaseID, uploadID uuid.UUID, userID int64, forUpdate bool) (*yingzoUploadSession, error) {
	query := `SELECT ` + yingzoUploadSessionColumns + ` FROM yingzo_release_upload_sessions WHERE id=$1 AND release_id=$2 AND created_by=$3`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	return scanYingzoUploadSession(querier.QueryRowContext(ctx, query, uploadID, releaseID, userID))
}

func parseYingzoUploadLocator(c *gin.Context) (uuid.UUID, uuid.UUID, int64, bool) {
	releaseID, releaseErr := uuid.Parse(c.Param("id"))
	uploadID, uploadErr := uuid.Parse(c.Param("upload_id"))
	subject, authenticated := middleware.GetAuthSubjectFromContext(c)
	if releaseErr != nil || uploadErr != nil {
		writeYingzoUploadAPIError(c, http.StatusNotFound, "upload_not_found", "upload session not found", nil)
		return uuid.Nil, uuid.Nil, 0, false
	}
	if !authenticated || subject.UserID <= 0 {
		writeYingzoUploadAPIError(c, http.StatusUnauthorized, "unauthorized", "administrator identity is required", nil)
		return uuid.Nil, uuid.Nil, 0, false
	}
	return releaseID, uploadID, subject.UserID, true
}

func (h *AgentHandler) GetYingzoReleaseArtifactUpload(c *gin.Context) {
	releaseID, uploadID, userID, ok := parseYingzoUploadLocator(c)
	if !ok {
		return
	}
	session, err := h.getOwnedYingzoUploadSession(c, h.db, releaseID, uploadID, userID, false)
	if errors.Is(err, sql.ErrNoRows) {
		writeYingzoUploadAPIError(c, http.StatusNotFound, "upload_not_found", "upload session not found", nil)
		return
	}
	if err != nil {
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "could not read upload session", nil)
		return
	}
	if session.ExpiresAt.Before(time.Now().UTC()) || session.Status == "expired" || session.Status == "aborted" {
		writeYingzoUploadAPIError(c, http.StatusGone, "upload_expired", "upload session has expired", gin.H{"offset": session.ReceivedBytes})
		return
	}
	payload, payloadErr := h.yingzoUploadSessionPayloadWithArtifact(c, session, true)
	if payloadErr != nil {
		if errors.Is(payloadErr, errYingzoCompletedArtifactGone) {
			writeYingzoUploadAPIError(c, http.StatusGone, "completed_artifact_gone", "the completed artifact no longer exists; initialize a new upload", nil)
			return
		}
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "could not read completed upload", nil)
		return
	}
	c.JSON(http.StatusOK, payload)
}

func parseYingzoUploadOffset(raw string) (int64, bool) {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	return value, err == nil && value >= 0
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (h *AgentHandler) PutYingzoReleaseArtifactUploadChunk(c *gin.Context) {
	releaseID, uploadID, userID, ok := parseYingzoUploadLocator(c)
	if !ok {
		return
	}
	offset, validOffset := parseYingzoUploadOffset(c.GetHeader("Upload-Offset"))
	if !validOffset {
		writeYingzoUploadAPIError(c, http.StatusBadRequest, "invalid_upload_offset", "Upload-Offset must be a non-negative int64", nil)
		return
	}
	if c.Request.ContentLength > yingzoReleaseChunkBytes {
		writeYingzoUploadAPIError(c, http.StatusRequestEntityTooLarge, "upload_chunk_too_large", "each chunk must be no larger than 8 MiB", nil)
		return
	}
	chunk, err := io.ReadAll(io.LimitReader(c.Request.Body, yingzoReleaseChunkBytes+1))
	if err != nil {
		writeYingzoUploadAPIError(c, http.StatusBadRequest, "upload_chunk_read_failed", "could not read upload chunk", nil)
		return
	}
	if len(chunk) == 0 {
		writeYingzoUploadAPIError(c, http.StatusBadRequest, "upload_chunk_empty", "upload chunk must not be empty", nil)
		return
	}
	if int64(len(chunk)) > yingzoReleaseChunkBytes {
		writeYingzoUploadAPIError(c, http.StatusRequestEntityTooLarge, "upload_chunk_too_large", "each chunk must be no larger than 8 MiB", nil)
		return
	}
	chunkSHA := sha256Hex(chunk)

	tx, err := h.db.BeginTx(c, &sql.TxOptions{})
	if err != nil {
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "could not start chunk transaction", nil)
		return
	}
	defer func() { _ = tx.Rollback() }()
	state, err := lockYingzoUploadRelease(c, tx, releaseID)
	if errors.Is(err, sql.ErrNoRows) {
		writeYingzoUploadAPIError(c, http.StatusNotFound, "yingzo_release_not_found", "release not found", nil)
		return
	}
	if err != nil {
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "could not lock release", nil)
		return
	}
	if state.Status != "draft" {
		writeYingzoUploadAPIError(c, http.StatusConflict, "release_state_invalid", "published releases are immutable", nil)
		return
	}
	session, err := h.getOwnedYingzoUploadSession(c, tx, releaseID, uploadID, userID, true)
	if errors.Is(err, sql.ErrNoRows) {
		writeYingzoUploadAPIError(c, http.StatusNotFound, "upload_not_found", "upload session not found", nil)
		return
	}
	if err != nil {
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "could not lock upload session", nil)
		return
	}
	now := time.Now().UTC()
	if session.ExpiresAt.Before(now) || session.Status == "expired" || session.Status == "aborted" {
		_, _ = tx.ExecContext(c, `UPDATE yingzo_release_upload_sessions SET status='expired',updated_at=NOW() WHERE id=$1`, session.ID)
		_ = tx.Commit()
		writeYingzoUploadAPIError(c, http.StatusGone, "upload_expired", "upload session has expired", gin.H{"offset": session.ReceivedBytes})
		return
	}
	if session.Status != "active" {
		writeYingzoUploadAPIError(c, http.StatusConflict, "upload_not_writable", "upload is being finalized", gin.H{"expected_offset": session.ReceivedBytes})
		return
	}
	if offset < session.ReceivedBytes && offset == session.LastChunkOffset && int64(len(chunk)) == session.LastChunkSize && chunkSHA == session.LastChunkSHA256 {
		session.ExpiresAt = now.Add(yingzoUploadSessionTTL)
		if _, err = tx.ExecContext(c, `UPDATE yingzo_release_upload_sessions SET expires_at=$2,updated_at=NOW() WHERE id=$1`, session.ID, session.ExpiresAt); err != nil || tx.Commit() != nil {
			writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "could not acknowledge retried chunk", nil)
			return
		}
		c.JSON(http.StatusOK, gin.H{"upload_id": session.ID, "offset": session.ReceivedBytes, "accepted_bytes": 0, "replayed": true, "expires_at": session.ExpiresAt})
		return
	}
	if offset != session.ReceivedBytes {
		writeYingzoUploadAPIError(c, http.StatusConflict, "upload_offset_conflict", "chunk offset does not match the server offset", gin.H{"expected_offset": session.ReceivedBytes})
		return
	}
	chunkSize := int64(len(chunk))
	if offset > session.TotalBytes || chunkSize > session.TotalBytes-offset {
		writeYingzoUploadAPIError(c, http.StatusConflict, "upload_exceeds_total", "chunk exceeds the declared total size", gin.H{"expected_offset": session.ReceivedBytes})
		return
	}
	partPath, err := h.validateYingzoUploadPartPath(session)
	if err != nil {
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "storage_error", "upload storage metadata is invalid", nil)
		return
	}
	if err = os.MkdirAll(filepath.Dir(partPath), 0700); err != nil {
		writeYingzoUploadAPIError(c, http.StatusInsufficientStorage, "storage_unavailable", "could not create upload storage", nil)
		return
	}
	file, err := os.OpenFile(partPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		writeYingzoUploadAPIError(c, http.StatusInsufficientStorage, "storage_unavailable", "could not open upload storage", nil)
		return
	}
	info, statErr := file.Stat()
	if statErr != nil {
		_ = file.Close()
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "storage_error", "could not inspect upload storage", nil)
		return
	}
	if info.Size() < session.ReceivedBytes {
		_ = file.Close()
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "upload_storage_inconsistent", "stored upload is shorter than the committed offset", nil)
		return
	}
	if info.Size() > session.ReceivedBytes {
		if err = file.Truncate(session.ReceivedBytes); err != nil {
			_ = file.Close()
			writeYingzoUploadAPIError(c, http.StatusInsufficientStorage, "storage_unavailable", "could not reconcile upload storage", nil)
			return
		}
	}
	written, writeErr := file.WriteAt(chunk, session.ReceivedBytes)
	if writeErr == nil && written != len(chunk) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		_ = os.Truncate(partPath, session.ReceivedBytes)
		writeYingzoUploadAPIError(c, http.StatusInsufficientStorage, "storage_write_failed", "could not persist upload chunk", nil)
		return
	}
	newOffset := session.ReceivedBytes + chunkSize
	expiresAt := now.Add(yingzoUploadSessionTTL)
	result, err := tx.ExecContext(c, `UPDATE yingzo_release_upload_sessions SET received_bytes=$2,last_chunk_offset=$3,last_chunk_size=$4,last_chunk_sha256=$5,expires_at=$6,updated_at=NOW() WHERE id=$1 AND status='active' AND received_bytes=$3`, session.ID, newOffset, session.ReceivedBytes, chunkSize, chunkSHA, expiresAt)
	if err == nil {
		var affected int64
		affected, err = result.RowsAffected()
		if err == nil && affected != 1 {
			err = errors.New("upload offset changed")
		}
	}
	if err != nil {
		_ = os.Truncate(partPath, session.ReceivedBytes)
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "could not commit upload offset", nil)
		return
	}
	if err = tx.Commit(); err != nil {
		// A commit error is ambiguous. Keep the ahead bytes; the next request will
		// either accept the idempotent retry or truncate back to the DB offset.
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "upload offset commit outcome is unknown; retry the same chunk", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"upload_id": session.ID, "offset": newOffset, "accepted_bytes": chunkSize, "replayed": false, "expires_at": expiresAt})
}

func (h *AgentHandler) resetYingzoFinalizingUpload(ctx context.Context, sessionID uuid.UUID, userID int64) {
	_, _ = h.db.ExecContext(ctx, `UPDATE yingzo_release_upload_sessions SET status='active',expires_at=$3,updated_at=NOW() WHERE id=$1 AND created_by=$2 AND status='finalizing'`, sessionID, userID, time.Now().UTC().Add(yingzoUploadSessionTTL))
}

func linkOrCopyYingzoUpload(source, target string) error {
	if err := os.Link(source, target); err == nil {
		return nil
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	if copyErr == nil {
		copyErr = out.Sync()
	}
	closeErr := out.Close()
	if copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		_ = os.Remove(target)
	}
	return copyErr
}

func (h *AgentHandler) CompleteYingzoReleaseArtifactUpload(c *gin.Context) {
	releaseID, uploadID, userID, ok := parseYingzoUploadLocator(c)
	if !ok {
		return
	}
	var input yingzoUploadCompleteInput
	if c.Request.ContentLength > 0 && c.ShouldBindJSON(&input) != nil {
		writeYingzoUploadAPIError(c, http.StatusBadRequest, "invalid_upload_complete", "completion metadata must be valid JSON", nil)
		return
	}
	completeSHA, validSHA := normalizeYingzoExpectedSHA256(input.SHA256)
	if !validSHA {
		writeYingzoUploadAPIError(c, http.StatusBadRequest, "invalid_upload_hash", "sha256 must contain 64 hexadecimal characters", nil)
		return
	}

	tx, err := h.db.BeginTx(c, &sql.TxOptions{})
	if err != nil {
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "could not start completion", nil)
		return
	}
	state, err := lockYingzoUploadRelease(c, tx, releaseID)
	if errors.Is(err, sql.ErrNoRows) {
		_ = tx.Rollback()
		writeYingzoUploadAPIError(c, http.StatusNotFound, "yingzo_release_not_found", "release not found", nil)
		return
	}
	if err != nil {
		_ = tx.Rollback()
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "could not lock release", nil)
		return
	}
	session, err := h.getOwnedYingzoUploadSession(c, tx, releaseID, uploadID, userID, true)
	if errors.Is(err, sql.ErrNoRows) {
		_ = tx.Rollback()
		writeYingzoUploadAPIError(c, http.StatusNotFound, "upload_not_found", "upload session not found", nil)
		return
	}
	if err != nil {
		_ = tx.Rollback()
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "could not lock upload session", nil)
		return
	}
	if session.Status == "completed" {
		if session.ExpiresAt.Before(time.Now().UTC()) {
			_, _ = tx.ExecContext(c, `DELETE FROM yingzo_release_upload_sessions WHERE id=$1`, session.ID)
			_ = tx.Commit()
			writeYingzoUploadAPIError(c, http.StatusGone, "upload_expired", "completed upload receipt has expired", nil)
			return
		}
		if session.CompletedArtifactID == uuid.Nil {
			_, _ = tx.ExecContext(c, `DELETE FROM yingzo_release_upload_sessions WHERE id=$1`, session.ID)
			_ = tx.Commit()
			writeYingzoUploadAPIError(c, http.StatusGone, "completed_artifact_gone", "the completed artifact no longer exists; initialize a new upload", nil)
			return
		}
		_ = tx.Rollback()
		payload, payloadErr := h.yingzoUploadSessionPayloadWithArtifact(c, session, true)
		if payloadErr != nil {
			if errors.Is(payloadErr, errYingzoCompletedArtifactGone) {
				_, _ = h.db.ExecContext(c, `DELETE FROM yingzo_release_upload_sessions WHERE id=$1`, session.ID)
				writeYingzoUploadAPIError(c, http.StatusGone, "completed_artifact_gone", "the completed artifact no longer exists; initialize a new upload", nil)
				return
			}
			writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "could not recover completed artifact", nil)
			return
		}
		c.JSON(http.StatusOK, payload["artifact"])
		return
	}
	if state.Status != "draft" {
		_ = tx.Rollback()
		writeYingzoUploadAPIError(c, http.StatusConflict, "release_state_invalid", "published releases are immutable", nil)
		return
	}
	if session.ExpiresAt.Before(time.Now().UTC()) || (session.Status != "active" && session.Status != "finalizing") {
		_ = tx.Rollback()
		writeYingzoUploadAPIError(c, http.StatusGone, "upload_expired", "upload session has expired", nil)
		return
	}
	if session.ReceivedBytes != session.TotalBytes {
		_ = tx.Rollback()
		writeYingzoUploadAPIError(c, http.StatusConflict, "upload_incomplete", "all bytes must be uploaded before completion", gin.H{"expected_offset": session.ReceivedBytes, "total_bytes": session.TotalBytes})
		return
	}
	if session.Status == "active" {
		if _, err = tx.ExecContext(c, `UPDATE yingzo_release_upload_sessions SET status='finalizing',expires_at=$2,updated_at=NOW() WHERE id=$1 AND status='active'`, session.ID, time.Now().UTC().Add(yingzoUploadSessionTTL)); err != nil {
			_ = tx.Rollback()
			writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "could not finalize upload session", nil)
			return
		}
	}
	if err = tx.Commit(); err != nil {
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "could not finalize upload session", nil)
		return
	}
	session.Status = "finalizing"

	partPath, err := h.validateYingzoUploadPartPath(session)
	if err != nil {
		h.resetYingzoFinalizingUpload(c, session.ID, userID)
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "storage_error", "upload storage metadata is invalid", nil)
		return
	}
	info, err := os.Stat(partPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() != session.TotalBytes {
		h.resetYingzoFinalizingUpload(c, session.ID, userID)
		writeYingzoUploadAPIError(c, http.StatusConflict, "upload_size_mismatch", "stored upload size does not match the declared size", nil)
		return
	}
	actualSHA, err := yingzoFileSHA256(partPath)
	if err != nil {
		h.resetYingzoFinalizingUpload(c, session.ID, userID)
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "storage_error", "could not hash uploaded package", nil)
		return
	}
	expectedSHA := session.ExpectedSHA256
	if completeSHA != "" {
		if expectedSHA != "" && expectedSHA != completeSHA {
			h.resetYingzoFinalizingUpload(c, session.ID, userID)
			writeYingzoUploadAPIError(c, http.StatusUnprocessableEntity, "upload_hash_mismatch", "completion hash does not match the initialized hash", nil)
			return
		}
		expectedSHA = completeSHA
	}
	if expectedSHA != "" && actualSHA != expectedSHA {
		h.resetYingzoFinalizingUpload(c, session.ID, userID)
		writeYingzoUploadAPIError(c, http.StatusUnprocessableEntity, "upload_hash_mismatch", "uploaded package sha256 does not match", gin.H{"actual_sha256": actualSHA})
		return
	}

	fileID := uuid.New()
	releaseDir, err := h.yingzoReleaseDirectory(releaseID)
	if err != nil {
		h.resetYingzoFinalizingUpload(c, session.ID, userID)
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "storage_error", "could not resolve release storage", nil)
		return
	}
	finalPath := filepath.Join(releaseDir, fileID.String()+"."+session.Format)
	if err = linkOrCopyYingzoUpload(partPath, finalPath); err != nil {
		h.resetYingzoFinalizingUpload(c, session.ID, userID)
		writeYingzoUploadAPIError(c, http.StatusInsufficientStorage, "storage_write_failed", "could not stage completed package", nil)
		return
	}

	tx, err = h.db.BeginTx(c, &sql.TxOptions{})
	if err != nil {
		_ = os.Remove(finalPath)
		h.resetYingzoFinalizingUpload(c, session.ID, userID)
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "could not register completed package", nil)
		return
	}
	state, err = lockYingzoUploadRelease(c, tx, releaseID)
	if err != nil || state.Status != "draft" || state.RuntimeProtocol != session.RuntimeProtocol {
		_ = tx.Rollback()
		_ = os.Remove(finalPath)
		h.resetYingzoFinalizingUpload(c, session.ID, userID)
		writeYingzoUploadAPIError(c, http.StatusConflict, "release_state_invalid", "release changed while the upload was finalized", nil)
		return
	}
	locked, err := h.getOwnedYingzoUploadSession(c, tx, releaseID, uploadID, userID, true)
	if err != nil || locked.Status != "finalizing" || locked.ReceivedBytes != session.TotalBytes {
		_ = tx.Rollback()
		_ = os.Remove(finalPath)
		writeYingzoUploadAPIError(c, http.StatusConflict, "upload_state_changed", "upload session changed while it was finalized", nil)
		return
	}
	var artifactID uuid.UUID
	var oldStorageKey string
	existingErr := tx.QueryRowContext(c, `SELECT id,storage_key FROM yingzo_release_artifacts WHERE release_id=$1 AND target=$2 AND os=$3 AND arch=$4 FOR UPDATE`, releaseID, session.Target, session.OS, session.Arch).Scan(&artifactID, &oldStorageKey)
	if existingErr != nil && !errors.Is(existingErr, sql.ErrNoRows) {
		_ = tx.Rollback()
		_ = os.Remove(finalPath)
		h.resetYingzoFinalizingUpload(c, session.ID, userID)
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "could not inspect artifact slot", nil)
		return
	}
	if errors.Is(existingErr, sql.ErrNoRows) {
		artifactID = uuid.New()
	}
	if _, err = tx.ExecContext(c, `UPDATE yingzo_releases SET signature=NULL,updated_at=NOW() WHERE id=$1 AND status='draft'`, releaseID); err == nil {
		_, err = tx.ExecContext(c, `UPDATE yingzo_release_artifacts SET signature_status='unverified',updated_at=NOW() WHERE release_id=$1 AND NOT (os='windows' OR target='claude-desktop')`, releaseID)
	}
	if err == nil && existingErr == nil {
		_, err = tx.ExecContext(c, `UPDATE yingzo_release_artifacts SET artifact_kind=$2,package_filename=$3,storage_backend='local',storage_key=$4,size_bytes=$5,sha256=$6,content_type=$7,format=$8,runtime_protocol=$9,validation_status='validated',signature_status='unverified',validated_at=NOW(),updated_at=NOW() WHERE id=$1 AND release_id=$10`, artifactID, session.ArtifactKind, session.PackageFilename, finalPath, session.TotalBytes, actualSHA, session.ContentType, session.Format, session.RuntimeProtocol, releaseID)
	} else if err == nil {
		_, err = tx.ExecContext(c, `INSERT INTO yingzo_release_artifacts(id,release_id,artifact_kind,target,os,arch,format,content_type,runtime_protocol,validation_status,signature_status,validated_at,package_filename,storage_backend,storage_key,size_bytes,sha256) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'validated','unverified',NOW(),$10,'local',$11,$12,$13)`, artifactID, releaseID, session.ArtifactKind, session.Target, session.OS, session.Arch, session.Format, session.ContentType, session.RuntimeProtocol, session.PackageFilename, finalPath, session.TotalBytes, actualSHA)
	}
	if err == nil {
		_, err = tx.ExecContext(c, `UPDATE yingzo_release_upload_sessions SET status='completed',completed_artifact_id=$2,expires_at=$3,updated_at=NOW() WHERE id=$1 AND release_id=$4 AND created_by=$5 AND status='finalizing'`, session.ID, artifactID, time.Now().UTC().Add(yingzoUploadSessionTTL), releaseID, userID)
	}
	if err != nil {
		_ = tx.Rollback()
		_ = os.Remove(finalPath)
		h.resetYingzoFinalizingUpload(c, session.ID, userID)
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "could not register completed package", nil)
		return
	}
	if err = tx.Commit(); err != nil {
		// Preserve both links after an ambiguous commit. If PostgreSQL committed,
		// the final artifact remains downloadable; otherwise retry can finalize the
		// retained session and stale-link cleanup will remove the orphan later.
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "completion commit outcome is unknown; query the release before retrying", nil)
		return
	}
	_ = os.Remove(partPath)
	_ = os.Remove(filepath.Dir(partPath))
	if oldStorageKey != "" && oldStorageKey != finalPath {
		h.removeYingzoStorageFile(releaseID, oldStorageKey)
	}
	now := time.Now().UTC()
	artifact := &yingzoReleaseArtifact{
		ID: artifactID, ReleaseID: releaseID, ArtifactKind: session.ArtifactKind, Target: session.Target,
		OS: session.OS, Arch: session.Arch, Format: session.Format, ContentType: session.ContentType,
		RuntimeProtocol: session.RuntimeProtocol, ValidationStatus: "validated", SignatureStatus: "unverified",
		ValidatedAt: &now, PackageFilename: session.PackageFilename, StorageBackend: "local", StorageKey: finalPath,
		SizeBytes: session.TotalBytes, SHA256: actualSHA, CreatedAt: now, UpdatedAt: now,
	}
	c.JSON(http.StatusOK, artifact)
}

func (h *AgentHandler) removeYingzoUploadPart(session *yingzoUploadSession) error {
	path, err := h.validateYingzoUploadPartPath(session)
	if err != nil {
		return err
	}
	if err = os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_ = os.Remove(filepath.Dir(path))
	return nil
}

func (h *AgentHandler) AbortYingzoReleaseArtifactUpload(c *gin.Context) {
	releaseID, uploadID, userID, ok := parseYingzoUploadLocator(c)
	if !ok {
		return
	}
	tx, err := h.db.BeginTx(c, &sql.TxOptions{})
	if err != nil {
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "could not abort upload", nil)
		return
	}
	session, err := h.getOwnedYingzoUploadSession(c, tx, releaseID, uploadID, userID, true)
	if errors.Is(err, sql.ErrNoRows) {
		_ = tx.Rollback()
		writeYingzoUploadAPIError(c, http.StatusNotFound, "upload_not_found", "upload session not found", nil)
		return
	}
	if err != nil {
		_ = tx.Rollback()
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "could not abort upload", nil)
		return
	}
	if session.Status == "completed" {
		_ = tx.Rollback()
		writeYingzoUploadAPIError(c, http.StatusConflict, "upload_already_completed", "completed artifacts cannot be aborted", nil)
		return
	}
	if _, err = tx.ExecContext(c, `UPDATE yingzo_release_upload_sessions SET status='aborted',expires_at=NOW(),updated_at=NOW() WHERE id=$1`, session.ID); err != nil || tx.Commit() != nil {
		writeYingzoUploadAPIError(c, http.StatusInternalServerError, "database_error", "could not abort upload", nil)
		return
	}
	removed := h.removeYingzoUploadPart(session) == nil
	if removed {
		_, _ = h.db.ExecContext(c, `DELETE FROM yingzo_release_upload_sessions WHERE id=$1 AND status='aborted'`, session.ID)
	}
	c.JSON(http.StatusOK, gin.H{"aborted": true, "upload_id": session.ID, "storage_removed": removed})
}

// CleanupExpiredYingzoArtifactUploads expires sessions transactionally, then
// removes their part files. A row is retained when storage removal fails so the
// next worker pass can retry instead of orphaning untracked bytes.
func (h *AgentHandler) CleanupExpiredYingzoArtifactUploads(ctx context.Context, now time.Time) (int, error) {
	if h == nil || h.db == nil {
		return 0, nil
	}
	rows, err := h.db.QueryContext(ctx, `SELECT id FROM yingzo_release_upload_sessions WHERE expires_at < $1 OR status IN ('aborted','expired') ORDER BY expires_at`, now)
	if err != nil {
		return 0, err
	}
	ids := make([]uuid.UUID, 0)
	for rows.Next() {
		var id uuid.UUID
		if scanErr := rows.Scan(&id); scanErr != nil {
			_ = rows.Close()
			return 0, scanErr
		}
		ids = append(ids, id)
	}
	rowsErr := rows.Err()
	_ = rows.Close()
	if rowsErr != nil {
		return 0, rowsErr
	}
	removed := 0
	for _, id := range ids {
		tx, beginErr := h.db.BeginTx(ctx, &sql.TxOptions{})
		if beginErr != nil {
			return removed, beginErr
		}
		session, scanErr := scanYingzoUploadSession(tx.QueryRowContext(ctx, `SELECT `+yingzoUploadSessionColumns+` FROM yingzo_release_upload_sessions WHERE id=$1 FOR UPDATE`, id))
		if errors.Is(scanErr, sql.ErrNoRows) {
			_ = tx.Rollback()
			continue
		}
		if scanErr != nil {
			_ = tx.Rollback()
			return removed, scanErr
		}
		if session.Status != "aborted" && session.Status != "expired" && !session.ExpiresAt.Before(now) {
			_ = tx.Rollback()
			continue
		}
		if _, updateErr := tx.ExecContext(ctx, `UPDATE yingzo_release_upload_sessions SET status='expired',updated_at=NOW() WHERE id=$1`, id); updateErr != nil {
			_ = tx.Rollback()
			return removed, updateErr
		}
		if commitErr := tx.Commit(); commitErr != nil {
			continue
		}
		if removeErr := h.removeYingzoUploadPart(session); removeErr != nil {
			continue
		}
		result, deleteErr := h.db.ExecContext(ctx, `DELETE FROM yingzo_release_upload_sessions WHERE id=$1 AND status='expired'`, id)
		if deleteErr != nil {
			return removed, deleteErr
		}
		if affected, _ := result.RowsAffected(); affected == 1 {
			removed++
		}
	}
	return removed, nil
}

func (h *AgentHandler) cleanupOrphanYingzoUploadParts(ctx context.Context, releaseID uuid.UUID, uploadDir string, olderThan time.Time) (int, error) {
	entries, err := os.ReadDir(uploadDir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".part") {
			continue
		}
		uploadID, parseErr := uuid.Parse(strings.TrimSuffix(entry.Name(), ".part"))
		info, infoErr := entry.Info()
		if parseErr != nil || infoErr != nil || !info.Mode().IsRegular() || !info.ModTime().Before(olderThan) {
			continue
		}
		candidate := filepath.Join(uploadDir, entry.Name())
		var referenced bool
		queryErr := h.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM yingzo_release_upload_sessions WHERE id=$1 AND release_id=$2 AND temp_storage_key=$3)`, uploadID, releaseID, candidate).Scan(&referenced)
		if queryErr != nil {
			return removed, queryErr
		}
		if !referenced {
			if removeErr := os.Remove(candidate); removeErr == nil {
				removed++
			}
		}
	}
	_ = os.Remove(uploadDir)
	return removed, nil
}
