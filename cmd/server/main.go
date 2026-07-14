package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"samrai/internal/auth"
	"samrai/internal/backup"
	"samrai/internal/catalog"
	"samrai/internal/config"
	"samrai/internal/database"
	"samrai/internal/httpserver"
	"samrai/internal/imagecache"
	"samrai/internal/importer"
	"samrai/internal/ocr"
	instancesettings "samrai/internal/settings"
	"samrai/internal/systeminfo"
	"samrai/internal/version"
	"strings"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "healthcheck":
			if e := healthcheck(); e != nil {
				fmt.Fprintln(os.Stderr, e)
				os.Exit(1)
			}
			return
		case "backup":
			if e := runBackup(); e != nil {
				fmt.Fprintln(os.Stderr, e)
				os.Exit(1)
			}
			return
		case "restore":
			if e := runRestore(os.Args[2:]); e != nil {
				fmt.Fprintln(os.Stderr, e)
				os.Exit(1)
			}
			return
		case "version":
			fmt.Printf("samrai %s (%s, %s)\n", version.Version, version.Commit, version.Date)
			return
		}
	}
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	cfg, e := config.Load()
	if e != nil {
		return fmt.Errorf("load configuration: %w", e)
	}
	if e = cfg.EnsureDirectories(); e != nil {
		return e
	}
	logger, logFile, e := newLogger(cfg.LogLevel, cfg.LogPath)
	if e != nil {
		return e
	}
	defer logFile.Close()
	slog.SetDefault(logger)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	started := time.Now()
	db, e := database.Open(ctx, cfg.DatabasePath, cfg.MaxDBConnections)
	if e != nil {
		return e
	}
	defer db.Close()
	if e = database.Migrate(ctx, db); e != nil {
		return e
	}
	storageReport, e := catalog.ReconcileManagedStorage(ctx, db, cfg.DataDir)
	if e != nil {
		return e
	}
	if storageReport.RebasedBooks > 0 || storageReport.RebasedLibraries > 0 || storageReport.MissingBooks > 0 {
		logger.Info("managed storage reconciled",
			"rebased_books", storageReport.RebasedBooks,
			"rebased_libraries", storageReport.RebasedLibraries,
			"missing_books", storageReport.MissingBooks,
		)
	}
	authService, e := auth.NewService(db, auth.Options{SessionDuration: cfg.SessionDuration, PasswordParams: auth.PasswordParams{MemoryKiB: cfg.Argon2MemoryKiB, Iterations: cfg.Argon2Iterations, Parallelism: cfg.Argon2Parallelism, SaltLength: 16, KeyLength: 32}})
	if e != nil {
		return e
	}
	imports, e := importer.NewService(db, cfg.DataDir, importer.Limits{MaxUploadBytes: cfg.MaxUploadBytes, MaxArchiveBytes: cfg.MaxArchiveBytes, MaxPageBytes: cfg.MaxPageBytes, MaxArchiveEntries: cfg.MaxArchiveEntries, MaxPages: cfg.MaxPages, SevenZipPath: cfg.SevenZipPath, LSARPath: cfg.LSARPath, UNARPath: cfg.UNARPath}, logger)
	if e != nil {
		return e
	}
	if e = imports.Start(ctx); e != nil {
		return e
	}
	catalogService := catalog.NewService(db)
	images, e := imagecache.NewService(db, catalogService, imagecache.Options{DataDir: cfg.DataDir, MaxBytes: cfg.ImageCacheBytes, Workers: cfg.ImageWorkers, Quality: cfg.ImageQuality, MaxWidth: cfg.ImageMaxWidth, CleanupInterval: cfg.CacheCleanup}, logger)
	if e != nil {
		return e
	}
	if e = images.Start(ctx); e != nil {
		return e
	}
	ocrService := ocr.NewService(ocr.Options{
		Executable: cfg.TesseractPath,
		Workers:    cfg.OCRWorkers,
		Timeout:    cfg.OCRTimeout,
		TempDir:    filepath.Join(cfg.DataDir, "staging", "ocr"),
	}, logger)
	ocrStatus := ocrService.Status()
	if ocrStatus.Available {
		logger.Info("ocr available", "executable", ocrStatus.Executable, "languages", ocrStatus.Languages, "workers", ocrStatus.Workers)
	} else {
		logger.Warn("ocr unavailable", "message", ocrStatus.Message)
	}
	settingsService := instancesettings.NewService(db)
	backups := backup.NewService(db, cfg.DataDir, version.Version)
	system := systeminfo.NewService(db, systeminfo.Options{DataDir: cfg.DataDir, DatabasePath: cfg.DatabasePath, LogPath: cfg.LogPath, Version: version.Version, Commit: version.Commit, BuildDate: version.Date, StartedAt: started, Limits: systeminfo.Limits{MaxUploadBytes: cfg.MaxUploadBytes, ImageCacheBytes: cfg.ImageCacheBytes, ImageWorkers: cfg.ImageWorkers, ImageQuality: cfg.ImageQuality, ImageMaxWidth: cfg.ImageMaxWidth}})
	server := httpserver.New(httpserver.Config{Address: cfg.Address, CookieSecure: cfg.CookieSecure}, db, authService, imports, catalogService, images, ocrService, settingsService, backups, system, logger)
	errs := make(chan error, 1)
	go func() {
		logger.Info("server started", "address", cfg.Address, "data_dir", cfg.DataDir, "version", version.Version)
		errs <- server.ListenAndServe()
	}()
	var serveErr error
	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case e := <-errs:
		if e != nil && !errors.Is(e, http.ErrServerClosed) {
			serveErr = e
		}
	}
	stop()
	shutdown, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if e = server.Shutdown(shutdown); e != nil {
		return e
	}
	imports.Wait()
	images.Wait()
	logger.Info("server stopped")
	return serveErr
}
func runBackup() error {
	cfg, e := config.Load()
	if e != nil {
		return e
	}
	if e = cfg.EnsureDirectories(); e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
	defer cancel()
	db, e := database.Open(ctx, cfg.DatabasePath, 1)
	if e != nil {
		return e
	}
	defer db.Close()
	if e = database.Migrate(ctx, db); e != nil {
		return e
	}
	path, _, e := backup.NewService(db, cfg.DataDir, version.Version).Create(ctx)
	if e == nil {
		fmt.Println(path)
	}
	return e
}
func runRestore(args []string) error {
	cfg, e := config.Load()
	if e != nil {
		return e
	}
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	file := fs.String("file", "", "backup ZIP")
	data := fs.String("data-dir", cfg.DataDir, "data directory")
	force := fs.Bool("force", false, "replace current data")
	if e = fs.Parse(args); e != nil {
		return errors.New("usage: samrai restore --file backup.zip [--data-dir ./data] [--force]")
	}
	if strings.TrimSpace(*file) == "" {
		return errors.New("provide --file with the backup path")
	}
	if e = backup.Restore(*file, *data, *force); e == nil {
		fmt.Printf("Backup restored to %s.\n", *data)
	}
	return e
}
func newLogger(level, path string) (*slog.Logger, io.Closer, error) {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if e != nil {
		return nil, nil, e
	}
	return slog.New(slog.NewJSONHandler(io.MultiWriter(os.Stdout, f), &slog.HandlerOptions{Level: l})), f, nil
}
func healthcheck() error {
	address := os.Getenv("SAMRAI_ADDRESS")
	if address == "" {
		address = ":8080"
	}
	if strings.HasPrefix(address, ":") {
		address = "127.0.0.1" + address
	}
	client := &http.Client{Timeout: 3 * time.Second}
	r, e := client.Get("http://" + address + "/health")
	if e != nil {
		return e
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return fmt.Errorf("health endpoint returned %s", r.Status)
	}
	return nil
}
