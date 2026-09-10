package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	settingKeyFileStorageConfig = "file_storage_config"
	fileStorageSchemaVersion    = 1
	defaultFileRetentionHours   = 24
	defaultFileDailyMaxCount    = int64(100)
	defaultFileDailyMaxBytes    = int64(2 << 30)
)

type FileStorageConfig struct {
	SchemaVersion  int            `json:"schema_version"`
	Backend        string         `json:"backend"`
	PublicBaseURL  string         `json:"public_base_url"`
	RetentionHours int            `json:"retention_hours"`
	DailyMaxCount  int64          `json:"daily_max_count"`
	DailyMaxBytes  int64          `json:"daily_max_bytes"`
	S3             BackupS3Config `json:"s3"`
}

type FileStorageUsage struct {
	ActiveFiles         int64 `json:"active_files"`
	ActiveBytes         int64 `json:"active_bytes"`
	LocalFiles          int64 `json:"local_files"`
	S3Files             int64 `json:"s3_files"`
	ExpiringWithin1Hour int64 `json:"expiring_within_1_hour"`
}

type FileStorageSettings struct {
	FileStorageConfig
	Source                    string           `json:"source"`
	LocalPath                 string           `json:"local_path"`
	SecretAccessKeyConfigured bool             `json:"secret_access_key_configured"`
	Usage                     FileStorageUsage `json:"usage"`
}

type FileStorageRuntime struct {
	Config FileStorageConfig
	Store  BackupObjectStore
}

type FileStorageService struct {
	db           *sql.DB
	settingRepo  SettingRepository
	encryptor    SecretEncryptor
	storeFactory BackupObjectStoreFactory
	localPath    string

	storeMu          sync.Mutex
	storeFingerprint string
	store            BackupObjectStore
}

func NewFileStorageService(
	db *sql.DB,
	settingRepo SettingRepository,
	encryptor SecretEncryptor,
	storeFactory BackupObjectStoreFactory,
	cfg *config.Config,
) *FileStorageService {
	localPath := filepath.Join(cfg.Pricing.DataDir, "agent-assets")
	_ = os.MkdirAll(localPath, 0700)
	return &FileStorageService{
		db:           db,
		settingRepo:  settingRepo,
		encryptor:    encryptor,
		storeFactory: storeFactory,
		localPath:    localPath,
	}
}

func (s *FileStorageService) LocalPath() string {
	return s.localPath
}

func (s *FileStorageService) GetSettings(ctx context.Context) (*FileStorageSettings, error) {
	cfg, source, err := s.loadEffectiveConfig(ctx)
	if err != nil {
		return nil, err
	}
	secretConfigured := strings.TrimSpace(cfg.S3.SecretAccessKey) != ""
	cfg.S3.SecretAccessKey = ""
	usage, err := s.Usage(ctx)
	if err != nil {
		return nil, err
	}
	return &FileStorageSettings{
		FileStorageConfig:         cfg,
		Source:                    source,
		LocalPath:                 s.localPath,
		SecretAccessKeyConfigured: secretConfigured,
		Usage:                     usage,
	}, nil
}

func (s *FileStorageService) UpdateSettings(ctx context.Context, input FileStorageConfig) (*FileStorageSettings, error) {
	current, _, _ := s.loadEffectiveConfig(ctx)
	if strings.TrimSpace(input.S3.SecretAccessKey) == "" {
		input.S3.SecretAccessKey = current.S3.SecretAccessKey
	}
	cfg, err := normalizeFileStorageConfig(input)
	if err != nil {
		return nil, infraerrors.BadRequest("FILE_STORAGE_CONFIG_INVALID", err.Error())
	}
	if err := s.testConfig(ctx, cfg); err != nil {
		return nil, infraerrors.BadRequest("FILE_STORAGE_CONNECTION_FAILED", err.Error())
	}

	stored := cfg
	if stored.S3.SecretAccessKey != "" {
		if s.encryptor == nil {
			return nil, errors.New("file storage secret encryptor is unavailable")
		}
		stored.S3.SecretAccessKey, err = s.encryptor.Encrypt(stored.S3.SecretAccessKey)
		if err != nil {
			return nil, fmt.Errorf("encrypt file storage secret: %w", err)
		}
	}
	payload, err := json.Marshal(stored)
	if err != nil {
		return nil, fmt.Errorf("marshal file storage config: %w", err)
	}
	if s.settingRepo == nil {
		return nil, errors.New("file storage setting repository is unavailable")
	}
	if err := s.settingRepo.Set(ctx, settingKeyFileStorageConfig, string(payload)); err != nil {
		return nil, fmt.Errorf("save file storage config: %w", err)
	}
	s.invalidateStore()
	return s.GetSettings(ctx)
}

func (s *FileStorageService) TestSettings(ctx context.Context, input FileStorageConfig) error {
	current, _, _ := s.loadEffectiveConfig(ctx)
	if strings.TrimSpace(input.S3.SecretAccessKey) == "" {
		input.S3.SecretAccessKey = current.S3.SecretAccessKey
	}
	cfg, err := normalizeFileStorageConfig(input)
	if err != nil {
		return infraerrors.BadRequest("FILE_STORAGE_CONFIG_INVALID", err.Error())
	}
	if err := s.testConfig(ctx, cfg); err != nil {
		return infraerrors.BadRequest("FILE_STORAGE_CONNECTION_FAILED", err.Error())
	}
	return nil
}

func (s *FileStorageService) Runtime(ctx context.Context) (*FileStorageRuntime, error) {
	cfg, _, err := s.loadEffectiveConfig(ctx)
	if err != nil {
		return nil, err
	}
	var store BackupObjectStore
	if cfg.S3.IsConfigured() {
		store, err = s.storeForConfig(ctx, cfg.S3)
		if err != nil {
			return nil, err
		}
	}
	return &FileStorageRuntime{Config: cfg, Store: store}, nil
}

func (s *FileStorageService) EffectivePublicBaseURL(ctx context.Context, fallback string) (string, error) {
	cfg, _, err := s.loadEffectiveConfig(ctx)
	if err != nil {
		return "", err
	}
	if cfg.PublicBaseURL != "" {
		return cfg.PublicBaseURL, nil
	}
	return normalizeFileStoragePublicBaseURL(fallback)
}

func (s *FileStorageService) Usage(ctx context.Context) (FileStorageUsage, error) {
	if s.db == nil {
		return FileStorageUsage{}, nil
	}
	var usage FileStorageUsage
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(size_bytes),0),
			COUNT(*) FILTER (WHERE storage_backend='local'),
			COUNT(*) FILTER (WHERE storage_backend='s3'),
			COUNT(*) FILTER (WHERE expires_at<=NOW()+INTERVAL '1 hour')
		FROM temporary_assets
		WHERE deleted_at IS NULL AND expires_at>NOW()
	`).Scan(&usage.ActiveFiles, &usage.ActiveBytes, &usage.LocalFiles, &usage.S3Files, &usage.ExpiringWithin1Hour)
	return usage, err
}

func (s *FileStorageService) loadEffectiveConfig(ctx context.Context) (FileStorageConfig, string, error) {
	if s.settingRepo != nil {
		raw, err := s.settingRepo.GetValue(ctx, settingKeyFileStorageConfig)
		if err == nil && strings.TrimSpace(raw) != "" {
			var stored FileStorageConfig
			if err := json.Unmarshal([]byte(raw), &stored); err != nil {
				return FileStorageConfig{}, "", fmt.Errorf("decode file storage config: %w", err)
			}
			if stored.S3.SecretAccessKey != "" {
				if s.encryptor == nil {
					return FileStorageConfig{}, "", errors.New("file storage secret encryptor is unavailable")
				}
				stored.S3.SecretAccessKey, err = s.encryptor.Decrypt(stored.S3.SecretAccessKey)
				if err != nil {
					return FileStorageConfig{}, "", fmt.Errorf("decrypt file storage secret: %w", err)
				}
			}
			normalized, err := normalizeFileStorageConfig(stored)
			if err != nil {
				return FileStorageConfig{}, "", fmt.Errorf("stored file storage config is invalid: %w", err)
			}
			return normalized, "database", nil
		}
	}
	cfg, source := fileStorageConfigFromEnvironment()
	normalized, err := normalizeFileStorageConfig(cfg)
	return normalized, source, err
}

func fileStorageConfigFromEnvironment() (FileStorageConfig, string) {
	cfg := defaultFileStorageConfig()
	source := "default"
	read := func(keys ...string) string {
		for _, key := range keys {
			if value := strings.TrimSpace(os.Getenv(key)); value != "" {
				source = "environment"
				return value
			}
		}
		return ""
	}
	cfg.PublicBaseURL = read("FILE_SERVICE_PUBLIC_BASE_URL", "AGENT_ASSETS_PUBLIC_BASE_URL")
	if value := read("FILE_SERVICE_RETENTION_HOURS"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			cfg.RetentionHours = parsed
		}
	}
	if value := read("FILE_SERVICE_DAILY_MAX_COUNT", "AGENT_ASSETS_DAILY_MAX_COUNT"); value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			cfg.DailyMaxCount = parsed
		}
	}
	if value := read("FILE_SERVICE_DAILY_MAX_BYTES", "AGENT_ASSETS_DAILY_MAX_BYTES"); value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			cfg.DailyMaxBytes = parsed
		}
	}
	cfg.S3 = BackupS3Config{
		Endpoint:        read("FILE_SERVICE_S3_ENDPOINT", "AGENT_ASSETS_S3_ENDPOINT"),
		Region:          read("FILE_SERVICE_S3_REGION", "AGENT_ASSETS_S3_REGION"),
		Bucket:          read("FILE_SERVICE_S3_BUCKET", "AGENT_ASSETS_S3_BUCKET"),
		AccessKeyID:     read("FILE_SERVICE_S3_ACCESS_KEY_ID", "AGENT_ASSETS_S3_ACCESS_KEY_ID"),
		SecretAccessKey: read("FILE_SERVICE_S3_SECRET_ACCESS_KEY", "AGENT_ASSETS_S3_SECRET_ACCESS_KEY"),
		Prefix:          read("FILE_SERVICE_S3_PREFIX"),
		ForcePathStyle:  strings.EqualFold(read("FILE_SERVICE_S3_FORCE_PATH_STYLE", "AGENT_ASSETS_S3_FORCE_PATH_STYLE"), "true"),
	}
	if cfg.S3.Bucket != "" {
		cfg.Backend = "s3"
	}
	return cfg, source
}

func defaultFileStorageConfig() FileStorageConfig {
	return FileStorageConfig{
		SchemaVersion:  fileStorageSchemaVersion,
		Backend:        "local",
		RetentionHours: defaultFileRetentionHours,
		DailyMaxCount:  defaultFileDailyMaxCount,
		DailyMaxBytes:  defaultFileDailyMaxBytes,
		S3: BackupS3Config{
			Region: "auto",
			Prefix: "model-assets/",
		},
	}
}

func normalizeFileStorageConfig(input FileStorageConfig) (FileStorageConfig, error) {
	if input.SchemaVersion == 0 {
		input.SchemaVersion = fileStorageSchemaVersion
	}
	if input.SchemaVersion != fileStorageSchemaVersion {
		return FileStorageConfig{}, fmt.Errorf("unsupported schema_version %d", input.SchemaVersion)
	}
	input.Backend = strings.ToLower(strings.TrimSpace(input.Backend))
	if input.Backend == "" {
		input.Backend = "local"
	}
	if input.Backend != "local" && input.Backend != "s3" {
		return FileStorageConfig{}, errors.New("backend must be local or s3")
	}
	if strings.TrimSpace(input.PublicBaseURL) != "" {
		normalized, err := normalizeFileStoragePublicBaseURL(input.PublicBaseURL)
		if err != nil {
			return FileStorageConfig{}, err
		}
		input.PublicBaseURL = normalized
	}
	if input.RetentionHours < 1 || input.RetentionHours > 720 {
		return FileStorageConfig{}, errors.New("retention_hours must be between 1 and 720")
	}
	if input.DailyMaxCount < 1 || input.DailyMaxCount > 1_000_000 {
		return FileStorageConfig{}, errors.New("daily_max_count must be between 1 and 1000000")
	}
	if input.DailyMaxBytes < 1 || input.DailyMaxBytes > 1<<50 {
		return FileStorageConfig{}, errors.New("daily_max_bytes is outside the supported range")
	}
	input.S3.Endpoint = strings.TrimSpace(input.S3.Endpoint)
	input.S3.Region = strings.TrimSpace(input.S3.Region)
	if input.S3.Region == "" {
		input.S3.Region = "auto"
	}
	input.S3.Bucket = strings.TrimSpace(input.S3.Bucket)
	input.S3.AccessKeyID = strings.TrimSpace(input.S3.AccessKeyID)
	input.S3.SecretAccessKey = strings.TrimSpace(input.S3.SecretAccessKey)
	prefix, err := normalizeFileStoragePrefix(input.S3.Prefix)
	if err != nil {
		return FileStorageConfig{}, err
	}
	input.S3.Prefix = prefix
	if input.S3.Endpoint != "" {
		parsed, err := url.Parse(input.S3.Endpoint)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
			return FileStorageConfig{}, errors.New("S3 endpoint must contain only an HTTP(S) scheme and host")
		}
		input.S3.Endpoint = strings.TrimRight(parsed.String(), "/")
	}
	if input.Backend == "s3" && !input.S3.IsConfigured() {
		return FileStorageConfig{}, errors.New("S3 backend requires bucket, access key ID, and secret access key")
	}
	return input, nil
}

func normalizeFileStoragePublicBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", errors.New("public_base_url must contain only an HTTP(S) scheme and host")
	}
	if parsed.Scheme != "https" && parsed.Hostname() != "localhost" && parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "::1" {
		return "", errors.New("public_base_url must use HTTPS outside local development")
	}
	parsed.Path = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func normalizeFileStoragePrefix(raw string) (string, error) {
	prefix := strings.Trim(strings.TrimSpace(raw), "/")
	if prefix == "" {
		return "model-assets/", nil
	}
	for _, segment := range strings.Split(prefix, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", errors.New("S3 prefix contains an unsafe path segment")
		}
	}
	return prefix + "/", nil
}

func (s *FileStorageService) testConfig(ctx context.Context, cfg FileStorageConfig) error {
	if cfg.Backend == "local" {
		if err := os.MkdirAll(s.localPath, 0700); err != nil {
			return fmt.Errorf("create local storage directory: %w", err)
		}
		file, err := os.CreateTemp(s.localPath, ".file-service-test-")
		if err != nil {
			return fmt.Errorf("write local storage directory: %w", err)
		}
		name := file.Name()
		if closeErr := file.Close(); closeErr != nil {
			_ = os.Remove(name)
			return fmt.Errorf("close local storage test file: %w", closeErr)
		}
		return os.Remove(name)
	}
	if s.storeFactory == nil {
		return errors.New("file storage object store factory is unavailable")
	}
	store, err := s.storeFactory(ctx, &cfg.S3)
	if err != nil {
		return fmt.Errorf("initialize S3 storage: %w", err)
	}
	if err := store.HeadBucket(ctx); err != nil {
		return err
	}
	return nil
}

func (s *FileStorageService) storeForConfig(ctx context.Context, cfg BackupS3Config) (BackupObjectStore, error) {
	fingerprintPayload, _ := json.Marshal(cfg)
	digest := sha256.Sum256(fingerprintPayload)
	fingerprint := hex.EncodeToString(digest[:])
	s.storeMu.Lock()
	defer s.storeMu.Unlock()
	if s.store != nil && s.storeFingerprint == fingerprint {
		return s.store, nil
	}
	if s.storeFactory == nil {
		return nil, errors.New("file storage object store factory is unavailable")
	}
	store, err := s.storeFactory(ctx, &cfg)
	if err != nil {
		return nil, fmt.Errorf("initialize file storage S3 client: %w", err)
	}
	s.store = store
	s.storeFingerprint = fingerprint
	return store, nil
}

func (s *FileStorageService) invalidateStore() {
	s.storeMu.Lock()
	s.store = nil
	s.storeFingerprint = ""
	s.storeMu.Unlock()
}
