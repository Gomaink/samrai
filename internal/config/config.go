package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAddress           = ":8080"
	defaultDataDir           = "./data"
	defaultLogLevel          = "info"
	defaultShutdownTimeout   = 10 * time.Second
	defaultMaxDBConnections  = 4
	defaultSessionDuration   = 30 * 24 * time.Hour
	defaultArgon2MemoryMiB   = 64
	defaultArgon2Iterations  = 3
	defaultArgon2Parallelism = 2
	defaultMaxUploadMiB      = 2048
	defaultMaxArchiveMiB     = 8192
	defaultMaxPageMiB        = 128
	defaultMaxArchiveEntries = 10000
	defaultMaxPages          = 5000
	defaultImageWorkers      = 2
	defaultImageCacheMiB     = 5120
	defaultImageQuality      = 82
	defaultImageMaxWidth     = 3840
	defaultCacheCleanup      = time.Hour
	defaultOCRWorkers        = 1
	defaultOCRTimeout        = 90 * time.Second
	minimumArgon2MemoryMiB   = 19
	maximumArgon2MemoryMiB   = 1024
	maximumArgon2Iterations  = 10
	maximumArgon2Parallelism = 16
)

type Config struct {
	Address           string
	DataDir           string
	DatabasePath      string
	LogPath           string
	LogLevel          string
	ShutdownTimeout   time.Duration
	MaxDBConnections  int
	SessionDuration   time.Duration
	CookieSecure      bool
	Argon2MemoryKiB   uint32
	Argon2Iterations  uint32
	Argon2Parallelism uint8
	MaxUploadBytes    int64
	MaxArchiveBytes   int64
	MaxPageBytes      int64
	MaxArchiveEntries int
	MaxPages          int
	ImageWorkers      int
	ImageCacheBytes   int64
	ImageQuality      int
	ImageMaxWidth     int
	CacheCleanup      time.Duration
	TesseractPath     string
	OCRWorkers        int
	OCRTimeout        time.Duration
	SevenZipPath      string
	LSARPath          string
	UNARPath          string
}

func Load() (Config, error) {
	dataDir := envOrDefault("SAMRAI_DATA_DIR", defaultDataDir)
	absoluteDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return Config{}, fmt.Errorf("resolve data directory: %w", err)
	}

	shutdownTimeout, err := durationEnv("SAMRAI_SHUTDOWN_TIMEOUT", defaultShutdownTimeout)
	if err != nil {
		return Config{}, err
	}

	maxDBConnections, err := intEnv("SAMRAI_MAX_DB_CONNECTIONS", defaultMaxDBConnections)
	if err != nil {
		return Config{}, err
	}
	if maxDBConnections < 1 {
		return Config{}, errors.New("SAMRAI_MAX_DB_CONNECTIONS must be at least 1")
	}

	sessionDuration, err := durationEnv("SAMRAI_SESSION_DURATION", defaultSessionDuration)
	if err != nil {
		return Config{}, err
	}

	cookieSecure, err := boolEnv("SAMRAI_COOKIE_SECURE", false)
	if err != nil {
		return Config{}, err
	}

	argon2MemoryMiB, err := intEnv("SAMRAI_ARGON2_MEMORY_MIB", defaultArgon2MemoryMiB)
	if err != nil {
		return Config{}, err
	}
	if argon2MemoryMiB < minimumArgon2MemoryMiB || argon2MemoryMiB > maximumArgon2MemoryMiB {
		return Config{}, fmt.Errorf("SAMRAI_ARGON2_MEMORY_MIB must be between %d and %d", minimumArgon2MemoryMiB, maximumArgon2MemoryMiB)
	}

	argon2Iterations, err := intEnv("SAMRAI_ARGON2_ITERATIONS", defaultArgon2Iterations)
	if err != nil {
		return Config{}, err
	}
	if argon2Iterations < 1 || argon2Iterations > maximumArgon2Iterations {
		return Config{}, fmt.Errorf("SAMRAI_ARGON2_ITERATIONS must be between 1 and %d", maximumArgon2Iterations)
	}

	argon2Parallelism, err := intEnv("SAMRAI_ARGON2_PARALLELISM", defaultArgon2Parallelism)
	if err != nil {
		return Config{}, err
	}
	if argon2Parallelism < 1 || argon2Parallelism > maximumArgon2Parallelism {
		return Config{}, fmt.Errorf("SAMRAI_ARGON2_PARALLELISM must be between 1 and %d", maximumArgon2Parallelism)
	}

	maxUploadMiB, err := positiveIntEnv("SAMRAI_MAX_UPLOAD_MIB", defaultMaxUploadMiB)
	if err != nil {
		return Config{}, err
	}
	maxArchiveMiB, err := positiveIntEnv("SAMRAI_MAX_ARCHIVE_MIB", defaultMaxArchiveMiB)
	if err != nil {
		return Config{}, err
	}
	maxPageMiB, err := positiveIntEnv("SAMRAI_MAX_PAGE_MIB", defaultMaxPageMiB)
	if err != nil {
		return Config{}, err
	}
	maxArchiveEntries, err := positiveIntEnv("SAMRAI_MAX_ARCHIVE_ENTRIES", defaultMaxArchiveEntries)
	if err != nil {
		return Config{}, err
	}
	maxPages, err := positiveIntEnv("SAMRAI_MAX_PAGES", defaultMaxPages)
	if err != nil {
		return Config{}, err
	}
	imageWorkers, err := positiveIntEnv("SAMRAI_IMAGE_WORKERS", defaultImageWorkers)
	if err != nil {
		return Config{}, err
	}
	if imageWorkers > 16 {
		return Config{}, errors.New("SAMRAI_IMAGE_WORKERS must not exceed 16")
	}
	imageCacheMiB, err := positiveIntEnv("SAMRAI_IMAGE_CACHE_MIB", defaultImageCacheMiB)
	if err != nil {
		return Config{}, err
	}
	imageQuality, err := positiveIntEnv("SAMRAI_IMAGE_QUALITY", defaultImageQuality)
	if err != nil {
		return Config{}, err
	}
	if imageQuality > 100 {
		return Config{}, errors.New("SAMRAI_IMAGE_QUALITY must be between 1 and 100")
	}
	imageMaxWidth, err := positiveIntEnv("SAMRAI_IMAGE_MAX_WIDTH", defaultImageMaxWidth)
	if err != nil {
		return Config{}, err
	}
	if imageMaxWidth < 320 || imageMaxWidth > 8192 {
		return Config{}, errors.New("SAMRAI_IMAGE_MAX_WIDTH must be between 320 and 8192")
	}
	cacheCleanup, err := durationEnv("SAMRAI_CACHE_CLEANUP_INTERVAL", defaultCacheCleanup)
	if err != nil {
		return Config{}, err
	}

	ocrWorkers, err := positiveIntEnv("SAMRAI_OCR_WORKERS", defaultOCRWorkers)
	if err != nil {
		return Config{}, err
	}
	if ocrWorkers > 4 {
		return Config{}, errors.New("SAMRAI_OCR_WORKERS must not exceed 4")
	}
	ocrTimeout, err := durationEnv("SAMRAI_OCR_TIMEOUT", defaultOCRTimeout)
	if err != nil {
		return Config{}, err
	}

	logLevel := strings.ToLower(envOrDefault("SAMRAI_LOG_LEVEL", defaultLogLevel))
	switch logLevel {
	case "debug", "info", "warn", "error":
	default:
		return Config{}, fmt.Errorf("invalid SAMRAI_LOG_LEVEL %q", logLevel)
	}

	databasePath := compatibleDataFile(absoluteDataDir, "config", "samrai.db", "pageturner.db")
	logPath := compatibleDataFile(absoluteDataDir, "logs", "samrai.log", "pageturner.log")

	return Config{
		Address:           envOrDefault("SAMRAI_ADDRESS", defaultAddress),
		DataDir:           absoluteDataDir,
		DatabasePath:      databasePath,
		LogPath:           logPath,
		LogLevel:          logLevel,
		ShutdownTimeout:   shutdownTimeout,
		MaxDBConnections:  maxDBConnections,
		SessionDuration:   sessionDuration,
		CookieSecure:      cookieSecure,
		Argon2MemoryKiB:   uint32(argon2MemoryMiB * 1024),
		Argon2Iterations:  uint32(argon2Iterations),
		Argon2Parallelism: uint8(argon2Parallelism),
		MaxUploadBytes:    int64(maxUploadMiB) * 1024 * 1024,
		MaxArchiveBytes:   int64(maxArchiveMiB) * 1024 * 1024,
		MaxPageBytes:      int64(maxPageMiB) * 1024 * 1024,
		MaxArchiveEntries: maxArchiveEntries,
		MaxPages:          maxPages,
		ImageWorkers:      imageWorkers,
		ImageCacheBytes:   int64(imageCacheMiB) * 1024 * 1024,
		ImageQuality:      imageQuality,
		ImageMaxWidth:     imageMaxWidth,
		CacheCleanup:      cacheCleanup,
		TesseractPath:     envValue("SAMRAI_TESSERACT_PATH"),
		OCRWorkers:        ocrWorkers,
		OCRTimeout:        ocrTimeout,
		SevenZipPath:      envValue("SAMRAI_7ZIP_PATH"),
		LSARPath:          envValue("SAMRAI_LSAR_PATH"),
		UNARPath:          envValue("SAMRAI_UNAR_PATH"),
	}, nil
}

func (cfg Config) EnsureDirectories() error {
	directories := []string{
		filepath.Join(cfg.DataDir, "config"),
		filepath.Join(cfg.DataDir, "library"),
		filepath.Join(cfg.DataDir, "uploads"),
		filepath.Join(cfg.DataDir, "staging"),
		filepath.Join(cfg.DataDir, "cache", "covers"),
		filepath.Join(cfg.DataDir, "cache", "thumbnails"),
		filepath.Join(cfg.DataDir, "cache", "pages"),
		filepath.Join(cfg.DataDir, "logs"),
		filepath.Join(cfg.DataDir, "backups"),
		filepath.Join(cfg.DataDir, "staging", "ocr"),
		filepath.Join(cfg.DataDir, "staging", "comics"),
		filepath.Join(cfg.DataDir, "prepared"),
	}

	for _, directory := range directories {
		if err := os.MkdirAll(directory, 0o750); err != nil {
			return fmt.Errorf("create directory %q: %w", directory, err)
		}
	}

	return nil
}

func compatibleDataFile(dataDir, subdir, currentName, legacyName string) string {
	current := filepath.Join(dataDir, subdir, currentName)
	legacy := filepath.Join(dataDir, subdir, legacyName)
	if _, err := os.Stat(current); err == nil {
		return current
	}
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return current
}

func legacyEnvKey(key string) string {
	if strings.HasPrefix(key, "SAMRAI_") {
		return "PAGETURNER_" + strings.TrimPrefix(key, "SAMRAI_")
	}
	return ""
}

func envValue(key string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	if legacy := legacyEnvKey(key); legacy != "" {
		return strings.TrimSpace(os.Getenv(legacy))
	}
	return ""
}

func envOrDefault(key, fallback string) string {
	if value := envValue(key); value != "" {
		return value
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	value := envValue(key)
	if value == "" {
		return fallback, nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", key)
	}
	return parsed, nil
}

func intEnv(key string, fallback int) (int, error) {
	value := envValue(key)
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}

func boolEnv(key string, fallback bool) (bool, error) {
	value := envValue(key)
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}

func positiveIntEnv(key string, fallback int) (int, error) {
	value, err := intEnv(key, fallback)
	if err != nil {
		return 0, err
	}
	if value < 1 {
		return 0, fmt.Errorf("%s must be at least 1", key)
	}
	return value, nil
}
