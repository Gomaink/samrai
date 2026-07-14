package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	for _, key := range []string{
		"SAMRAI_ADDRESS",
		"SAMRAI_DATA_DIR",
		"SAMRAI_LOG_LEVEL",
		"SAMRAI_SHUTDOWN_TIMEOUT",
		"SAMRAI_MAX_DB_CONNECTIONS",
		"SAMRAI_SESSION_DURATION",
		"SAMRAI_COOKIE_SECURE",
		"SAMRAI_ARGON2_MEMORY_MIB",
		"SAMRAI_ARGON2_ITERATIONS",
		"SAMRAI_ARGON2_PARALLELISM",
		"SAMRAI_MAX_UPLOAD_MIB",
		"SAMRAI_MAX_ARCHIVE_MIB",
		"SAMRAI_MAX_PAGE_MIB",
		"SAMRAI_MAX_ARCHIVE_ENTRIES",
		"SAMRAI_MAX_PAGES",
		"SAMRAI_IMAGE_WORKERS",
		"SAMRAI_IMAGE_CACHE_MIB",
		"SAMRAI_IMAGE_QUALITY",
		"SAMRAI_IMAGE_MAX_WIDTH",
		"SAMRAI_CACHE_CLEANUP_INTERVAL",
		"SAMRAI_TESSERACT_PATH",
		"SAMRAI_OCR_WORKERS",
		"SAMRAI_OCR_TIMEOUT",
		"SAMRAI_7ZIP_PATH",
		"SAMRAI_LSAR_PATH",
		"SAMRAI_UNAR_PATH",
	} {
		t.Setenv(key, "")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Address != ":8080" {
		t.Fatalf("Address = %q, want %q", cfg.Address, ":8080")
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.ShutdownTimeout != 10*time.Second {
		t.Fatalf("ShutdownTimeout = %v", cfg.ShutdownTimeout)
	}
	if cfg.SessionDuration != 30*24*time.Hour {
		t.Fatalf("SessionDuration = %v", cfg.SessionDuration)
	}
	if cfg.CookieSecure {
		t.Fatal("CookieSecure = true, want false")
	}
	if cfg.Argon2MemoryKiB != 64*1024 || cfg.Argon2Iterations != 3 || cfg.Argon2Parallelism != 2 {
		t.Fatalf("unexpected Argon2 defaults: %d/%d/%d", cfg.Argon2MemoryKiB, cfg.Argon2Iterations, cfg.Argon2Parallelism)
	}
	if cfg.MaxUploadBytes != 2048*1024*1024 || cfg.MaxArchiveBytes != 8192*1024*1024 || cfg.MaxPageBytes != 128*1024*1024 {
		t.Fatalf("unexpected upload defaults: %+v", cfg)
	}
	if cfg.MaxArchiveEntries != 10000 || cfg.MaxPages != 5000 {
		t.Fatalf("unexpected archive defaults: %+v", cfg)
	}
	if cfg.ImageWorkers != 2 || cfg.ImageCacheBytes != 5120*1024*1024 || cfg.ImageQuality != 82 || cfg.ImageMaxWidth != 3840 || cfg.CacheCleanup != time.Hour {
		t.Fatalf("unexpected image cache defaults: %+v", cfg)
	}
	if cfg.TesseractPath != "" || cfg.OCRWorkers != 1 || cfg.OCRTimeout != 90*time.Second {
		t.Fatalf("unexpected OCR defaults: %+v", cfg)
	}
	if filepath.Base(cfg.DatabasePath) != "samrai.db" {
		t.Fatalf("DatabasePath = %q", cfg.DatabasePath)
	}
}

func TestLoadAuthOverrides(t *testing.T) {
	t.Setenv("SAMRAI_SESSION_DURATION", "24h")
	t.Setenv("SAMRAI_COOKIE_SECURE", "true")
	t.Setenv("SAMRAI_ARGON2_MEMORY_MIB", "32")
	t.Setenv("SAMRAI_ARGON2_ITERATIONS", "2")
	t.Setenv("SAMRAI_ARGON2_PARALLELISM", "1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.SessionDuration != 24*time.Hour || !cfg.CookieSecure {
		t.Fatalf("unexpected auth config: %+v", cfg)
	}
	if cfg.Argon2MemoryKiB != 32*1024 || cfg.Argon2Iterations != 2 || cfg.Argon2Parallelism != 1 {
		t.Fatalf("unexpected Argon2 config: %+v", cfg)
	}
}

func TestEnsureDirectories(t *testing.T) {
	dataDir := t.TempDir()
	cfg := Config{DataDir: dataDir}

	if err := cfg.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories() error = %v", err)
	}

	expected := []string{
		"config",
		"library",
		"uploads",
		"staging",
		filepath.Join("cache", "covers"),
		filepath.Join("cache", "thumbnails"),
		filepath.Join("cache", "pages"),
		filepath.Join("staging", "ocr"),
		filepath.Join("staging", "comics"),
		"prepared",
		"logs",
	}

	for _, relative := range expected {
		path := filepath.Join(dataDir, relative)
		if info, err := filepath.Glob(path); err != nil || len(info) != 1 {
			t.Fatalf("expected directory %q to exist", path)
		}
	}
}

func TestLoadUsesLegacyEnvironmentFallback(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("SAMRAI_DATA_DIR", "")
	t.Setenv("PAGETURNER_DATA_DIR", dataDir)
	t.Setenv("SAMRAI_ADDRESS", "")
	t.Setenv("PAGETURNER_ADDRESS", "127.0.0.1:9090")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	absoluteDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != absoluteDataDir || cfg.Address != "127.0.0.1:9090" {
		t.Fatalf("legacy environment was not used: %+v", cfg)
	}
}

func TestLoadPrefersSamraiEnvironment(t *testing.T) {
	t.Setenv("SAMRAI_ADDRESS", "127.0.0.1:8088")
	t.Setenv("PAGETURNER_ADDRESS", "127.0.0.1:9090")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Address != "127.0.0.1:8088" {
		t.Fatalf("Address = %q, want canonical SAMRAI_ADDRESS", cfg.Address)
	}
}

func TestLoadReusesLegacyDatabaseAndLog(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dataDir, "config"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "logs"), 0o750); err != nil {
		t.Fatal(err)
	}
	legacyDatabase := filepath.Join(dataDir, "config", "pageturner.db")
	legacyLog := filepath.Join(dataDir, "logs", "pageturner.log")
	if err := os.WriteFile(legacyDatabase, []byte("legacy-db"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyLog, []byte("legacy-log"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SAMRAI_DATA_DIR", dataDir)
	t.Setenv("PAGETURNER_DATA_DIR", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.DatabasePath != legacyDatabase || cfg.LogPath != legacyLog {
		t.Fatalf("legacy files were not reused: database=%q log=%q", cfg.DatabasePath, cfg.LogPath)
	}
}
