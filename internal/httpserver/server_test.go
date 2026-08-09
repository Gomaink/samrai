package httpserver

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"samrai/internal/auth"
	"samrai/internal/backup"
	"samrai/internal/catalog"
	"samrai/internal/database"
	"samrai/internal/importer"
	instancesettings "samrai/internal/settings"
	"samrai/internal/systeminfo"
	"samrai/internal/version"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	dataDir := t.TempDir()
	for _, directory := range []string{"uploads", "library"} {
		if err := os.MkdirAll(filepath.Join(dataDir, directory), 0o750); err != nil {
			t.Fatalf("create test directory: %v", err)
		}
	}

	db, err := database.Open(ctx, filepath.Join(dataDir, "test.db"), 1)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}

	authService, err := auth.NewService(db, auth.Options{
		SessionDuration: time.Hour,
		PasswordParams: auth.PasswordParams{
			MemoryKiB:   64,
			Iterations:  1,
			Parallelism: 1,
			SaltLength:  8,
			KeyLength:   16,
		},
	})
	if err != nil {
		t.Fatalf("create auth service: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	importService, err := importer.NewService(db, dataDir, importer.Limits{
		MaxUploadBytes:    10 * 1024 * 1024,
		MaxArchiveBytes:   50 * 1024 * 1024,
		MaxPageBytes:      5 * 1024 * 1024,
		MaxArchiveEntries: 100,
		MaxPages:          50,
	}, logger)
	if err != nil {
		t.Fatalf("create importer: %v", err)
	}
	if err := importService.Start(ctx); err != nil {
		t.Fatalf("start importer: %v", err)
	}

	t.Cleanup(func() {
		cancel()
		importService.Wait()
		_ = db.Close()
	})
	settingsService := instancesettings.NewService(db)
	backupService := backup.NewService(db, dataDir, version.Version)
	systemService := systeminfo.NewService(db, systeminfo.Options{DataDir: dataDir, DatabasePath: filepath.Join(dataDir, "test.db"), LogPath: filepath.Join(dataDir, "test.log"), Version: version.Version, Commit: version.Commit, BuildDate: version.Date, StartedAt: time.Now()})
	return New(Config{Address: ":0"}, db, authService, importService, catalog.NewService(db), nil, nil, settingsService, backupService, systemService, logger)
}

func TestHealth(t *testing.T) {
	server := newTestServer(t)
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}

	var payload healthResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if payload.Status != "ok" || payload.Database != "ok" {
		t.Fatalf("unexpected health payload: %+v", payload)
	}
	if response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("security headers were not set")
	}
}

func TestEPUBDocumentHeadersAllowSameOriginFraming(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		setEPUBDocumentHeaders(w)
		w.WriteHeader(http.StatusOK)
	}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/books/1/epub/content/OEBPS/chapter.xhtml", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if got := response.Header().Get("X-Frame-Options"); got != "SAMEORIGIN" {
		t.Fatalf("X-Frame-Options = %q, want SAMEORIGIN", got)
	}
	csp := response.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "frame-ancestors 'self'") {
		t.Fatalf("Content-Security-Policy does not allow same-origin framing: %q", csp)
	}
	if !strings.Contains(csp, "script-src 'none'") {
		t.Fatalf("Content-Security-Policy does not block EPUB scripts: %q", csp)
	}
	if got := response.Header().Get("Cache-Control"); got != "private, no-cache" {
		t.Fatalf("Cache-Control = %q, want private, no-cache", got)
	}
}

func TestEPUBNotModifiedKeepsSameOriginFramePolicy(t *testing.T) {
	asset := catalog.EPUBResourceAsset{
		MediaType:    "application/xhtml+xml",
		ETag:         `"book-epub-chapter"`,
		ResourcePath: "OEBPS/chapter.xhtml",
	}
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setEPUBResourceHeaders(w, asset)
		if r.Header.Get("If-None-Match") == asset.ETag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/books/1/epub/content/OEBPS/chapter.xhtml", nil)
	request.Header.Set("If-None-Match", asset.ETag)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotModified)
	}
	if got := response.Header().Get("X-Frame-Options"); got != "SAMEORIGIN" {
		t.Fatalf("X-Frame-Options on 304 = %q, want SAMEORIGIN", got)
	}
	if got := response.Header().Get("Content-Security-Policy"); !strings.Contains(got, "frame-ancestors 'self'") {
		t.Fatalf("304 Content-Security-Policy blocks the reader iframe: %q", got)
	}
}

func TestFrontendHEADUsesGETFallback(t *testing.T) {
	server := newTestServer(t)
	request := httptest.NewRequest(http.MethodHead, "/", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if response.Body.Len() != 0 {
		t.Fatalf("HEAD response body length = %d, want 0", response.Body.Len())
	}
}

func TestAuthenticationFlow(t *testing.T) {
	server := newTestServer(t)

	status := performRequest(t, server, http.MethodGet, "/api/v1/auth/status", nil, nil)
	if status.Code != http.StatusOK || !bytes.Contains(status.Body.Bytes(), []byte(`"setup_required":true`)) {
		t.Fatalf("unexpected initial status: %d %s", status.Code, status.Body.String())
	}

	setup := performRequest(t, server, http.MethodPost, "/api/v1/auth/setup", map[string]string{
		"username": "samuel",
		"password": "a-secure-password",
	}, nil)
	if setup.Code != http.StatusCreated {
		t.Fatalf("setup status = %d: %s", setup.Code, setup.Body.String())
	}
	cookie := responseCookie(t, setup, sessionCookieName)
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("insecure session cookie: %+v", cookie)
	}

	me := performRequest(t, server, http.MethodGet, "/api/v1/auth/me", nil, cookie)
	if me.Code != http.StatusOK || !bytes.Contains(me.Body.Bytes(), []byte(`"role":"admin"`)) {
		t.Fatalf("me status = %d: %s", me.Code, me.Body.String())
	}

	secondSetup := performRequest(t, server, http.MethodPost, "/api/v1/auth/setup", map[string]string{
		"username": "other",
		"password": "another-secure-password",
	}, nil)
	if secondSetup.Code != http.StatusConflict {
		t.Fatalf("second setup status = %d: %s", secondSetup.Code, secondSetup.Body.String())
	}

	logout := performRequest(t, server, http.MethodPost, "/api/v1/auth/logout", nil, cookie)
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d: %s", logout.Code, logout.Body.String())
	}

	afterLogout := performRequest(t, server, http.MethodGet, "/api/v1/auth/me", nil, cookie)
	if afterLogout.Code != http.StatusUnauthorized {
		t.Fatalf("me after logout status = %d: %s", afterLogout.Code, afterLogout.Body.String())
	}

	wrongLogin := performRequest(t, server, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": "samuel",
		"password": "wrong-password",
	}, nil)
	if wrongLogin.Code != http.StatusUnauthorized {
		t.Fatalf("wrong login status = %d: %s", wrongLogin.Code, wrongLogin.Body.String())
	}

	login := performRequest(t, server, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": "SAMUEL",
		"password": "a-secure-password",
	}, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d: %s", login.Code, login.Body.String())
	}
}

func TestLegacySessionCookieIsMigrated(t *testing.T) {
	server := newTestServer(t)
	setup := performRequest(t, server, http.MethodPost, "/api/v1/auth/setup", map[string]string{
		"username": "samuel",
		"password": "a-secure-password",
	}, nil)
	if setup.Code != http.StatusCreated {
		t.Fatalf("setup status = %d: %s", setup.Code, setup.Body.String())
	}
	current := responseCookie(t, setup, sessionCookieName)
	legacy := *current
	legacy.Name = legacySessionCookieName

	status := performRequest(t, server, http.MethodGet, "/api/v1/auth/status", nil, &legacy)
	if status.Code != http.StatusOK || !bytes.Contains(status.Body.Bytes(), []byte(`"authenticated":true`)) {
		t.Fatalf("legacy cookie status = %d: %s", status.Code, status.Body.String())
	}
	migrated := responseCookie(t, status, sessionCookieName)
	if migrated.Value != current.Value {
		t.Fatalf("migrated cookie token changed")
	}
	clearedLegacy := responseCookie(t, status, legacySessionCookieName)
	if clearedLegacy.MaxAge != -1 {
		t.Fatalf("legacy cookie MaxAge = %d, want -1", clearedLegacy.MaxAge)
	}
}

func TestSetupRejectsShortPassword(t *testing.T) {
	server := newTestServer(t)
	response := performRequest(t, server, http.MethodPost, "/api/v1/auth/setup", map[string]string{
		"username": "samuel",
		"password": "curta",
	}, nil)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
}

func TestUploadImportsCBZAndListsBook(t *testing.T) {
	server := newTestServer(t)
	setup := performRequest(t, server, http.MethodPost, "/api/v1/auth/setup", map[string]string{
		"username": "samuel",
		"password": "a-secure-password",
	}, nil)
	if setup.Code != http.StatusCreated {
		t.Fatalf("setup status = %d: %s", setup.Code, setup.Body.String())
	}
	cookie := responseCookie(t, setup, sessionCookieName)

	archive := makeTestCBZ(t)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "volume-01.cbz")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write(archive); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("upload status = %d: %s", response.Code, response.Body.String())
	}

	deadline := time.Now().Add(4 * time.Second)
	for {
		books := performRequest(t, server, http.MethodGet, "/api/v1/books", nil, cookie)
		if books.Code == http.StatusOK && bytes.Contains(books.Body.Bytes(), []byte(`"title":"volume-01"`)) {
			if !bytes.Contains(books.Body.Bytes(), []byte(`"page_count":2`)) {
				t.Fatalf("unexpected books payload: %s", books.Body.String())
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("book was not imported: %d %s", books.Code, books.Body.String())
		}
		time.Sleep(25 * time.Millisecond)
	}

	cover := performRequest(t, server, http.MethodGet, "/api/v1/books/1/cover", nil, cookie)
	if cover.Code != http.StatusOK || cover.Header().Get("Content-Type") != "image/png" || cover.Body.Len() == 0 {
		t.Fatalf("cover response = %d %q bytes=%d", cover.Code, cover.Header().Get("Content-Type"), cover.Body.Len())
	}

	book := performRequest(t, server, http.MethodGet, "/api/v1/books/1", nil, cookie)
	if book.Code != http.StatusOK || !bytes.Contains(book.Body.Bytes(), []byte(`"page_count":2`)) {
		t.Fatalf("book response = %d %s", book.Code, book.Body.String())
	}

	page := performRequest(t, server, http.MethodGet, "/api/v1/books/1/pages/0/image", nil, cookie)
	if page.Code != http.StatusOK || page.Header().Get("Content-Type") != "image/png" || page.Body.Len() == 0 {
		t.Fatalf("page response = %d %q bytes=%d", page.Code, page.Header().Get("Content-Type"), page.Body.Len())
	}
	if page.Header().Get("Cache-Control") != "private, max-age=31536000, immutable" {
		t.Fatalf("unexpected page cache header: %q", page.Header().Get("Cache-Control"))
	}

	headPage := performRequest(t, server, http.MethodHead, "/api/v1/books/1/pages/0/image", nil, cookie)
	if headPage.Code != http.StatusOK || headPage.Body.Len() != 0 {
		t.Fatalf("HEAD page response = %d bytes=%d", headPage.Code, headPage.Body.Len())
	}

	progress := performRequest(t, server, http.MethodPut, "/api/v1/books/1/progress", map[string]int{
		"current_page": 1,
	}, cookie)
	if progress.Code != http.StatusOK || !bytes.Contains(progress.Body.Bytes(), []byte(`"completed":true`)) {
		t.Fatalf("progress response = %d %s", progress.Code, progress.Body.String())
	}

	booksAfterProgress := performRequest(t, server, http.MethodGet, "/api/v1/books", nil, cookie)
	if booksAfterProgress.Code != http.StatusOK ||
		!bytes.Contains(booksAfterProgress.Body.Bytes(), []byte(`"current_page":1`)) ||
		!bytes.Contains(booksAfterProgress.Body.Bytes(), []byte(`"completed":true`)) {
		t.Fatalf("books after progress = %d %s", booksAfterProgress.Code, booksAfterProgress.Body.String())
	}

	markUnread := performRequest(t, server, http.MethodPatch, "/api/v1/books/progress/completion", map[string]any{
		"book_ids":  []int64{1},
		"completed": false,
	}, cookie)
	if markUnread.Code != http.StatusOK || !bytes.Contains(markUnread.Body.Bytes(), []byte(`"updated":1`)) || !bytes.Contains(markUnread.Body.Bytes(), []byte(`"completed":false`)) {
		t.Fatalf("mark unread response = %d %s", markUnread.Code, markUnread.Body.String())
	}
	bookAfterUnread := performRequest(t, server, http.MethodGet, "/api/v1/books/1", nil, cookie)
	if bookAfterUnread.Code != http.StatusOK ||
		!bytes.Contains(bookAfterUnread.Body.Bytes(), []byte(`"current_page":1`)) ||
		!bytes.Contains(bookAfterUnread.Body.Bytes(), []byte(`"completed":false`)) {
		t.Fatalf("book after unread = %d %s", bookAfterUnread.Code, bookAfterUnread.Body.String())
	}

	markRead := performRequest(t, server, http.MethodPatch, "/api/v1/books/progress/completion", map[string]any{
		"book_ids":  []int64{1},
		"completed": true,
	}, cookie)
	if markRead.Code != http.StatusOK || !bytes.Contains(markRead.Body.Bytes(), []byte(`"completed":true`)) {
		t.Fatalf("mark read response = %d %s", markRead.Code, markRead.Body.String())
	}

	missingPage := performRequest(t, server, http.MethodGet, "/api/v1/books/1/pages/2/image", nil, cookie)
	if missingPage.Code != http.StatusNotFound {
		t.Fatalf("missing page status = %d: %s", missingPage.Code, missingPage.Body.String())
	}
}

func TestUploadAppliesReviewedMetadataAndRejectsDuplicateHash(t *testing.T) {
	server := newTestServer(t)
	setup := performRequest(t, server, http.MethodPost, "/api/v1/auth/setup", map[string]string{
		"username": "samuel",
		"password": "a-secure-password",
	}, nil)
	if setup.Code != http.StatusCreated {
		t.Fatalf("setup status = %d: %s", setup.Code, setup.Body.String())
	}
	cookie := responseCookie(t, setup, sessionCookieName)
	archive := makeTestCBZ(t)

	first := uploadTestFile(t, server, cookie,
		"/api/v1/uploads?title=Berserk+01&series=Berserk&volume=1",
		"Berserk - Volume 01.cbz", archive)
	if first.Code != http.StatusAccepted {
		t.Fatalf("first upload status = %d: %s", first.Code, first.Body.String())
	}

	duplicate := uploadTestFile(t, server, cookie, "/api/v1/uploads", "copia.cbz", archive)
	if duplicate.Code != http.StatusConflict || !bytes.Contains(duplicate.Body.Bytes(), []byte(`"code":"duplicate_book"`)) {
		t.Fatalf("duplicate upload status = %d: %s", duplicate.Code, duplicate.Body.String())
	}

	deadline := time.Now().Add(4 * time.Second)
	for {
		books := performRequest(t, server, http.MethodGet, "/api/v1/books", nil, cookie)
		payload := books.Body.Bytes()
		if books.Code == http.StatusOK &&
			bytes.Contains(payload, []byte(`"title":"Berserk 01"`)) &&
			bytes.Contains(payload, []byte(`"series":"Berserk"`)) &&
			bytes.Contains(payload, []byte(`"volume":"1"`)) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("reviewed metadata was not applied: %d %s", books.Code, books.Body.String())
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func uploadTestFile(t *testing.T, server *Server, cookie *http.Cookie, path, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, path, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}

func makeTestCBZ(t *testing.T) []byte {
	t.Helper()
	var archive bytes.Buffer
	zipWriter := zip.NewWriter(&archive)
	for _, name := range []string{"pages/10.png", "pages/2.png"} {
		entry, err := zipWriter.Create(name)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		canvas := image.NewRGBA(image.Rect(0, 0, 16, 24))
		canvas.Set(0, 0, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		if err := png.Encode(entry, canvas); err != nil {
			t.Fatalf("encode test PNG: %v", err)
		}
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close test archive: %v", err)
	}
	return archive.Bytes()
}

func performRequest(t *testing.T, server *Server, method, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		reader = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, path, reader)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}

func responseCookie(t *testing.T, response *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("cookie %q was not set", name)
	return nil
}
