package httpserver

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"samrai/internal/auth"
	"samrai/internal/backup"
	"samrai/internal/catalog"
	"samrai/internal/imagecache"
	"samrai/internal/importer"
	"samrai/internal/ocr"
	instancesettings "samrai/internal/settings"
	"samrai/internal/systeminfo"
	"samrai/internal/version"
	"samrai/internal/webui"
)

type Config struct {
	Address      string
	CookieSecure bool
}

type Server struct {
	db           *sql.DB
	auth         *auth.Service
	importer     *importer.Service
	catalog      *catalog.Service
	images       *imagecache.Service
	ocr          *ocr.Service
	settings     *instancesettings.Service
	backups      *backup.Service
	system       *systeminfo.Service
	logger       *slog.Logger
	startedAt    time.Time
	cookieSecure bool
	loginLimiter *loginLimiter
	http         *http.Server
}

type healthResponse struct {
	Status   string `json:"status"`
	Service  string `json:"service"`
	Version  string `json:"version"`
	Commit   string `json:"commit"`
	Database string `json:"database"`
	Time     string `json:"time"`
}

func New(config Config, db *sql.DB, authService *auth.Service, importService *importer.Service, catalogService *catalog.Service, imageService *imagecache.Service, ocrService *ocr.Service, settingsService *instancesettings.Service, backupService *backup.Service, systemService *systeminfo.Service, logger *slog.Logger) *Server {
	server := &Server{
		db:           db,
		auth:         authService,
		importer:     importService,
		catalog:      catalogService,
		images:       imageService,
		ocr:          ocrService,
		settings:     settingsService,
		backups:      backupService,
		system:       systemService,
		logger:       logger,
		startedAt:    time.Now(),
		cookieSecure: config.CookieSecure,
		loginLimiter: newLoginLimiter(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", server.health)
	mux.HandleFunc("GET /api/v1", server.apiRoot)
	mux.HandleFunc("GET /api/v1/auth/status", server.authStatus)
	mux.HandleFunc("POST /api/v1/auth/setup", server.setup)
	mux.HandleFunc("POST /api/v1/auth/login", server.login)
	mux.Handle("GET /api/v1/auth/me", server.requireAuthentication(http.HandlerFunc(server.me)))
	mux.Handle("PUT /api/v1/auth/password", server.requireAuthentication(http.HandlerFunc(server.changeOwnPassword)))
	mux.HandleFunc("POST /api/v1/auth/logout", server.logout)
	mux.Handle("POST /api/v1/uploads", server.requireAuthentication(server.requireAdmin(http.HandlerFunc(server.uploadBook))))
	mux.Handle("GET /api/v1/jobs", server.requireAuthentication(server.requireAdmin(http.HandlerFunc(server.listJobs))))
	mux.Handle("GET /api/v1/dashboard", server.requireAuthentication(http.HandlerFunc(server.dashboard)))
	mux.Handle("GET /api/v1/favorites", server.requireAuthentication(http.HandlerFunc(server.favorites)))
	mux.Handle("PUT /api/v1/books/{bookID}/favorite", server.requireAuthentication(http.HandlerFunc(server.favoriteBook)))
	mux.Handle("DELETE /api/v1/books/{bookID}/favorite", server.requireAuthentication(http.HandlerFunc(server.favoriteBook)))
	mux.Handle("PUT /api/v1/series/{seriesID}/favorite", server.requireAuthentication(http.HandlerFunc(server.favoriteSeries)))
	mux.Handle("DELETE /api/v1/series/{seriesID}/favorite", server.requireAuthentication(http.HandlerFunc(server.favoriteSeries)))
	mux.Handle("GET /api/v1/history", server.requireAuthentication(http.HandlerFunc(server.readingHistory)))
	mux.Handle("DELETE /api/v1/history", server.requireAuthentication(http.HandlerFunc(server.clearReadingHistory)))
	mux.Handle("POST /api/v1/reading-sessions/end", server.requireAuthentication(http.HandlerFunc(server.endReadingSession)))
	mux.Handle("GET /api/v1/annotations", server.requireAuthentication(http.HandlerFunc(server.allAnnotations)))
	mux.Handle("GET /api/v1/annotations/export", server.requireAuthentication(http.HandlerFunc(server.exportAllAnnotations)))
	mux.Handle("GET /api/v1/books", server.requireAuthentication(http.HandlerFunc(server.listBooks)))
	mux.Handle("GET /api/v1/series", server.requireAuthentication(http.HandlerFunc(server.listSeries)))
	mux.Handle("GET /api/v1/series/{seriesID}", server.requireAuthentication(http.HandlerFunc(server.getSeries)))
	mux.Handle("GET /api/v1/books/{bookID}", server.requireAuthentication(http.HandlerFunc(server.getBook)))
	mux.Handle("PATCH /api/v1/books/{bookID}", server.requireAuthentication(server.requireAdmin(http.HandlerFunc(server.updateBook))))
	mux.Handle("DELETE /api/v1/books/{bookID}", server.requireAuthentication(server.requireAdmin(http.HandlerFunc(server.deleteBook))))
	mux.Handle("GET /api/v1/books/{bookID}/cover", server.requireAuthentication(http.HandlerFunc(server.bookCover)))
	mux.Handle("GET /api/v1/books/{bookID}/pages/{pageNumber}/image", server.requireAuthentication(http.HandlerFunc(server.bookPage)))
	mux.Handle("GET /api/v1/books/{bookID}/file", server.requireAuthentication(http.HandlerFunc(server.bookFile)))
	mux.Handle("GET /api/v1/books/{bookID}/epub", server.requireAuthentication(http.HandlerFunc(server.getEPUBPublication)))
	mux.Handle("GET /api/v1/books/{bookID}/epub/content/{resource...}", server.requireAuthentication(http.HandlerFunc(server.epubResource)))
	mux.Handle("GET /api/v1/books/{bookID}/epub/search", server.requireAuthentication(http.HandlerFunc(server.searchEPUB)))
	mux.Handle("GET /api/v1/books/{bookID}/epub/annotations", server.requireAuthentication(http.HandlerFunc(server.listEPUBAnnotations)))
	mux.Handle("POST /api/v1/books/{bookID}/epub/annotations", server.requireAuthentication(http.HandlerFunc(server.createEPUBAnnotation)))
	mux.Handle("GET /api/v1/books/{bookID}/epub/annotations/export", server.requireAuthentication(http.HandlerFunc(server.exportEPUBAnnotations)))
	mux.Handle("PATCH /api/v1/epub-annotations/{annotationID}", server.requireAuthentication(http.HandlerFunc(server.updateEPUBAnnotation)))
	mux.Handle("DELETE /api/v1/epub-annotations/{annotationID}", server.requireAuthentication(http.HandlerFunc(server.deleteEPUBAnnotation)))
	mux.Handle("PUT /api/v1/books/{bookID}/pdf/metadata", server.requireAuthentication(http.HandlerFunc(server.updatePDFMetadata)))
	mux.Handle("PUT /api/v1/books/{bookID}/pdf/analysis", server.requireAuthentication(http.HandlerFunc(server.savePDFAnalysis)))
	mux.Handle("PUT /api/v1/books/{bookID}/pdf/classification", server.requireAuthentication(http.HandlerFunc(server.savePDFClassification)))
	mux.Handle("PATCH /api/v1/books/{bookID}/pdf/ocr-mode", server.requireAuthentication(server.requireAdmin(http.HandlerFunc(server.updatePDFOCRMode))))
	mux.Handle("PUT /api/v1/books/{bookID}/pdf/pages/{pageNumber}/text", server.requireAuthentication(http.HandlerFunc(server.savePDFPageText)))
	mux.Handle("PUT /api/v1/books/{bookID}/pdf/cover", server.requireAuthentication(http.HandlerFunc(server.savePDFCover)))
	mux.Handle("GET /api/v1/books/{bookID}/pdf/search", server.requireAuthentication(http.HandlerFunc(server.searchPDF)))
	mux.Handle("POST /api/v1/books/{bookID}/pdf/ocr", server.requireAuthentication(http.HandlerFunc(server.recognizePDFPage)))
	mux.Handle("GET /api/v1/system/ocr", server.requireAuthentication(http.HandlerFunc(server.ocrStatus)))
	mux.Handle("GET /api/v1/books/{bookID}/annotations", server.requireAuthentication(http.HandlerFunc(server.listAnnotations)))
	mux.Handle("POST /api/v1/books/{bookID}/annotations", server.requireAuthentication(http.HandlerFunc(server.createAnnotation)))
	mux.Handle("GET /api/v1/books/{bookID}/annotations/export", server.requireAuthentication(http.HandlerFunc(server.exportAnnotations)))
	mux.Handle("PATCH /api/v1/annotations/{annotationID}", server.requireAuthentication(http.HandlerFunc(server.updateAnnotation)))
	mux.Handle("DELETE /api/v1/annotations/{annotationID}", server.requireAuthentication(http.HandlerFunc(server.deleteAnnotation)))
	mux.Handle("PATCH /api/v1/books/progress/completion", server.requireAuthentication(http.HandlerFunc(server.setBooksCompletion)))
	mux.Handle("PUT /api/v1/books/{bookID}/progress", server.requireAuthentication(http.HandlerFunc(server.saveBookProgress)))
	mux.Handle("DELETE /api/v1/books/{bookID}/progress", server.requireAuthentication(http.HandlerFunc(server.resetBookProgress)))
	mux.Handle("GET /api/v1/settings", server.requireAuthentication(http.HandlerFunc(server.instanceSettings)))
	mux.Handle("PATCH /api/v1/settings", server.requireAuthentication(server.requireAdmin(http.HandlerFunc(server.updateInstanceSettings))))
	mux.Handle("GET /api/v1/users", server.requireAuthentication(server.requireAdmin(http.HandlerFunc(server.listUsers))))
	mux.Handle("POST /api/v1/users", server.requireAuthentication(server.requireAdmin(http.HandlerFunc(server.createUser))))
	mux.Handle("PATCH /api/v1/users/{userID}", server.requireAuthentication(server.requireAdmin(http.HandlerFunc(server.updateUser))))
	mux.Handle("PUT /api/v1/users/{userID}/password", server.requireAuthentication(server.requireAdmin(http.HandlerFunc(server.resetUserPassword))))
	mux.Handle("DELETE /api/v1/users/{userID}", server.requireAuthentication(server.requireAdmin(http.HandlerFunc(server.deleteUser))))
	mux.Handle("GET /api/v1/system/metrics", server.requireAuthentication(server.requireAdmin(http.HandlerFunc(server.systemMetrics))))
	mux.Handle("DELETE /api/v1/system/cache/images", server.requireAuthentication(server.requireAdmin(http.HandlerFunc(server.purgeImageCache))))
	mux.Handle("GET /api/v1/system/info", server.requireAuthentication(server.requireAdmin(http.HandlerFunc(server.systemInfo))))
	mux.Handle("GET /api/v1/system/logs", server.requireAuthentication(server.requireAdmin(http.HandlerFunc(server.recentLogs))))
	mux.Handle("GET /api/v1/system/audit", server.requireAuthentication(server.requireAdmin(http.HandlerFunc(server.auditEvents))))
	mux.Handle("GET /api/v1/system/backup", server.requireAuthentication(server.requireAdmin(http.HandlerFunc(server.createBackup))))
	mux.Handle("GET /api/v1/system/support-bundle", server.requireAuthentication(server.requireAdmin(http.HandlerFunc(server.supportBundle))))
	mux.Handle("GET /", webui.Handler())

	var handler http.Handler = mux
	handler = securityHeaders(handler)
	handler = requestLogger(logger, handler)
	handler = recoverPanics(logger, handler)

	server.http = &http.Server{
		Addr:              config.Address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		// Uploads can be large and may take several minutes on a local network.
		// ReadHeaderTimeout still protects the header phase; body size is limited per route.
		ReadTimeout:    0,
		WriteTimeout:   0,
		IdleTimeout:    2 * time.Minute,
		MaxHeaderBytes: 1 << 20,
	}
	return server
}

func (s *Server) ListenAndServe() error {
	return s.http.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

func (s *Server) Handler() http.Handler {
	return s.http.Handler
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	databaseStatus := "ok"
	status := "ok"
	httpStatus := http.StatusOK
	if err := s.db.PingContext(ctx); err != nil {
		databaseStatus = "unavailable"
		status = "degraded"
		httpStatus = http.StatusServiceUnavailable
		s.logger.Error("database health check failed", "error", err)
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, httpStatus, healthResponse{
		Status:   status,
		Service:  "samrai",
		Version:  version.Version,
		Commit:   version.Commit,
		Database: databaseStatus,
		Time:     time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (s *Server) apiRoot(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"service": "samrai-api",
		"version": version.Version,
	})
}
