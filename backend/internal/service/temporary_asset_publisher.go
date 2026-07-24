package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "golang.org/x/image/webp"
)

const maxPublishedGeneratedImageBytes int64 = 30 << 20

// TemporaryAssetOwner binds a generated asset to the API key that paid for it.
type TemporaryAssetOwner struct {
	UserID   int64
	APIKeyID int64
	GroupID  int64
}

// OpenAIImageResultPublisher converts generated image bytes into a short-lived
// HTTP(S) asset. Implementations must not return data URLs.
type OpenAIImageResultPublisher interface {
	PublishGeneratedImage(
		ctx context.Context,
		owner TemporaryAssetOwner,
		fallbackPublicBaseURL string,
		encodedImage string,
		outputFormat string,
	) (string, error)
}

// TemporaryAssetPublisher stores generated images in the same managed storage
// used by Agent reference assets and records their lifecycle in temporary_assets.
type TemporaryAssetPublisher struct {
	db          *sql.DB
	fileStorage *FileStorageService
}

func NewTemporaryAssetPublisher(db *sql.DB, fileStorage *FileStorageService) *TemporaryAssetPublisher {
	return &TemporaryAssetPublisher{db: db, fileStorage: fileStorage}
}

func (p *TemporaryAssetPublisher) PublishGeneratedImage(
	ctx context.Context,
	owner TemporaryAssetOwner,
	fallbackPublicBaseURL string,
	encodedImage string,
	outputFormat string,
) (string, error) {
	if p == nil || p.db == nil || p.fileStorage == nil {
		return "", errors.New("temporary asset publisher is unavailable")
	}
	if owner.UserID <= 0 || owner.APIKeyID <= 0 || owner.GroupID <= 0 {
		return "", errors.New("temporary asset owner is invalid")
	}

	imageBytes, err := decodeGeneratedImageBase64(encodedImage)
	if err != nil {
		return "", err
	}
	mimeType, extension, err := inspectGeneratedImage(imageBytes, outputFormat)
	if err != nil {
		return "", err
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(imageBytes))
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return "", errors.New("generated image cannot be decoded")
	}

	runtime, err := p.fileStorage.Runtime(ctx)
	if err != nil {
		return "", fmt.Errorf("load temporary asset storage: %w", err)
	}
	publicBaseURL, err := p.fileStorage.EffectivePublicBaseURL(ctx, fallbackPublicBaseURL)
	if err != nil {
		return "", fmt.Errorf("resolve temporary asset public URL: %w", err)
	}

	var activeCount, activeBytes int64
	err = p.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(size_bytes), 0)
		FROM temporary_assets
		WHERE api_key_id=$1 AND user_id=$2
			AND created_at>NOW()-INTERVAL '24 hours'
			AND deleted_at IS NULL
	`, owner.APIKeyID, owner.UserID).Scan(&activeCount, &activeBytes)
	if err != nil {
		return "", fmt.Errorf("read temporary asset quota: %w", err)
	}
	imageSize := int64(len(imageBytes))
	if activeCount >= runtime.Config.DailyMaxCount || activeBytes+imageSize > runtime.Config.DailyMaxBytes {
		return "", errors.New("temporary asset quota exceeded")
	}

	id := uuid.New()
	token, err := generatedAssetRandomToken(32)
	if err != nil {
		return "", fmt.Errorf("generate temporary asset token: %w", err)
	}
	metadata, err := json.Marshal(map[string]any{
		"width":  config.Width,
		"height": config.Height,
		"probe":  "go-image",
		"source": "generated",
	})
	if err != nil {
		return "", fmt.Errorf("encode temporary asset metadata: %w", err)
	}

	assetDir := filepath.Join(p.fileStorage.LocalPath(), id.String())
	if err := os.MkdirAll(assetDir, 0o700); err != nil {
		return "", fmt.Errorf("create temporary asset directory: %w", err)
	}
	localPath := filepath.Join(assetDir, "object")
	if err := writeGeneratedAssetAtomically(localPath, imageBytes); err != nil {
		_ = os.RemoveAll(assetDir)
		return "", err
	}

	backend := "local"
	storageKey := localPath
	if runtime.Config.Backend == "s3" {
		if runtime.Store == nil {
			_ = os.RemoveAll(assetDir)
			return "", errors.New("temporary asset object storage is unavailable")
		}
		file, openErr := os.Open(localPath)
		if openErr != nil {
			_ = os.RemoveAll(assetDir)
			return "", fmt.Errorf("open temporary asset for upload: %w", openErr)
		}
		storageKey = runtime.Config.S3.Prefix + id.String()
		_, uploadErr := runtime.Store.Upload(ctx, storageKey, file, mimeType)
		_ = file.Close()
		if uploadErr != nil {
			_ = os.RemoveAll(assetDir)
			return "", fmt.Errorf("upload temporary asset: %w", uploadErr)
		}
		backend = "s3"
		_ = os.RemoveAll(assetDir)
	}

	checksum := sha256.Sum256(imageBytes)
	expiresAt := time.Now().UTC().Add(time.Duration(runtime.Config.RetentionHours) * time.Hour)
	_, err = p.db.ExecContext(ctx, `
		INSERT INTO temporary_assets(
			id,user_id,api_key_id,group_id,public_token_hash,storage_backend,
			storage_key,original_filename,media_type,mime_type,size_bytes,sha256,
			metadata,expires_at
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
	`, id, owner.UserID, owner.APIKeyID, owner.GroupID, generatedAssetHashToken(token),
		backend, storageKey, "generated-image"+extension, "image", mimeType, imageSize,
		hex.EncodeToString(checksum[:]), metadata, expiresAt)
	if err != nil {
		if backend == "s3" {
			_ = runtime.Store.Delete(context.Background(), storageKey)
		} else {
			_ = os.RemoveAll(assetDir)
		}
		return "", fmt.Errorf("record temporary asset: %w", err)
	}

	return strings.TrimRight(publicBaseURL, "/") + "/media/" + id.String() + "/asset" + extension, nil
}

func decodeGeneratedImageBase64(encoded string) ([]byte, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, errors.New("generated image payload is empty")
	}
	maxEncodedBytes := base64.StdEncoding.EncodedLen(int(maxPublishedGeneratedImageBytes))
	if len(encoded) > maxEncodedBytes+8 {
		return nil, errors.New("generated image exceeds the temporary asset size limit")
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(encoded)
	}
	if err != nil {
		return nil, errors.New("generated image payload is not valid base64")
	}
	if len(decoded) == 0 || int64(len(decoded)) > maxPublishedGeneratedImageBytes {
		return nil, errors.New("generated image exceeds the temporary asset size limit")
	}
	return decoded, nil
}

func inspectGeneratedImage(data []byte, outputFormat string) (string, string, error) {
	var mimeType, extension string
	switch {
	case len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}):
		mimeType, extension = "image/png", ".png"
	case len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
		mimeType, extension = "image/jpeg", ".jpg"
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		mimeType, extension = "image/webp", ".webp"
	case len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a"):
		mimeType, extension = "image/gif", ".gif"
	default:
		return "", "", errors.New("generated image has an unsupported media type")
	}

	if declared := strings.TrimSpace(outputFormat); declared != "" {
		expected := openAIImageOutputMIMEType(declared)
		if expected != mimeType {
			return "", "", fmt.Errorf("generated image media type %s does not match output format %s", mimeType, declared)
		}
	}
	return mimeType, extension, nil
}

func generatedAssetRandomToken(size int) (string, error) {
	value := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func generatedAssetHashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func writeGeneratedAssetAtomically(target string, data []byte) error {
	temporary, err := os.OpenFile(target+".tmp", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create temporary asset file: %w", err)
	}
	cleanup := true
	defer func() {
		_ = temporary.Close()
		if cleanup {
			_ = os.Remove(target + ".tmp")
		}
	}()
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write temporary asset file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary asset file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary asset file: %w", err)
	}
	if err := os.Rename(target+".tmp", target); err != nil {
		return fmt.Errorf("publish temporary asset file: %w", err)
	}
	cleanup = false
	return nil
}
