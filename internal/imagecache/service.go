package imagecache

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	stddraw "image/draw"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gen2brain/webp"
	xdraw "golang.org/x/image/draw"

	"samrai/internal/catalog"
)

const cacheVersion = 2

const (
	KindPage  = "pages"
	KindCover = "covers"
)

type Options struct {
	DataDir         string
	MaxBytes        int64
	Workers         int
	Quality         int
	MaxWidth        int
	CleanupInterval time.Duration
}

type Service struct {
	db      *sql.DB
	catalog *catalog.Service
	logger  *slog.Logger
	options Options

	pageDir  string
	coverDir string
	workers  chan struct{}
	active   atomic.Int64
	bytes    atomic.Int64

	hits             atomic.Int64
	misses           atomic.Int64
	generated        atomic.Int64
	generatedBytes   atomic.Int64
	generationMillis atomic.Int64

	flightMu sync.Mutex
	flights  map[string]*flight
	flushMu  sync.Mutex
	flushed  persistedStats
	wait     sync.WaitGroup
}

type flight struct {
	done chan struct{}
	err  error
}

type persistedStats struct {
	hits             int64
	misses           int64
	generated        int64
	generatedBytes   int64
	generationMillis int64
}

type Asset struct {
	Stream     io.ReadCloser
	MediaType  string
	Size       int64
	ETag       string
	Width      int
	Height     int
	PageNumber int
	Cache      string
}

type Metrics struct {
	Hits                    int64   `json:"hits"`
	Misses                  int64   `json:"misses"`
	Generated               int64   `json:"generated"`
	GeneratedBytes          int64   `json:"generated_bytes"`
	GenerationMilliseconds  int64   `json:"generation_milliseconds"`
	AverageGenerationMillis float64 `json:"average_generation_milliseconds"`
	HitRate                 float64 `json:"hit_rate"`
	CacheBytes              int64   `json:"cache_bytes"`
	CacheLimitBytes         int64   `json:"cache_limit_bytes"`
	ActiveWorkers           int64   `json:"active_workers"`
	WorkerLimit             int     `json:"worker_limit"`
	Quality                 int     `json:"quality"`
	MaxWidth                int     `json:"max_width"`
}

func NewService(db *sql.DB, catalogService *catalog.Service, options Options, logger *slog.Logger) (*Service, error) {
	if db == nil || catalogService == nil || logger == nil {
		return nil, errors.New("database, catalog and logger are required")
	}
	if options.DataDir == "" || options.MaxBytes < 1 || options.Workers < 1 || options.Quality < 1 || options.Quality > 100 || options.MaxWidth < 320 {
		return nil, errors.New("invalid image cache options")
	}
	if options.CleanupInterval <= 0 {
		options.CleanupInterval = time.Hour
	}
	service := &Service{
		db:       db,
		catalog:  catalogService,
		logger:   logger,
		options:  options,
		pageDir:  filepath.Join(options.DataDir, "cache", "pages"),
		coverDir: filepath.Join(options.DataDir, "cache", "covers"),
		workers:  make(chan struct{}, options.Workers),
		flights:  make(map[string]*flight),
	}
	if err := os.MkdirAll(service.pageDir, 0o750); err != nil {
		return nil, fmt.Errorf("create page cache directory: %w", err)
	}
	if err := os.MkdirAll(service.coverDir, 0o750); err != nil {
		return nil, fmt.Errorf("create cover cache directory: %w", err)
	}
	if err := service.loadStats(context.Background()); err != nil {
		return nil, err
	}
	return service, nil
}

func (s *Service) Start(ctx context.Context) error {
	if err := s.recalculateSize(); err != nil {
		return fmt.Errorf("scan image cache: %w", err)
	}
	if err := s.Cleanup(ctx); err != nil {
		s.logger.Warn("initial image cache cleanup failed", "error", err)
	}

	s.wait.Add(1)
	go func() {
		defer s.wait.Done()
		cleanupTicker := time.NewTicker(s.options.CleanupInterval)
		flushTicker := time.NewTicker(time.Minute)
		defer cleanupTicker.Stop()
		defer flushTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				flushCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				_ = s.flushStats(flushCtx)
				cancel()
				return
			case <-cleanupTicker.C:
				if err := s.Cleanup(ctx); err != nil && ctx.Err() == nil {
					s.logger.Warn("image cache cleanup failed", "error", err)
				}
			case <-flushTicker.C:
				if err := s.flushStats(ctx); err != nil && ctx.Err() == nil {
					s.logger.Warn("persist image cache statistics", "error", err)
				}
			}
		}
	}()
	return nil
}

func (s *Service) Wait() {
	s.wait.Wait()
}

func (s *Service) Open(ctx context.Context, bookID int64, pageNumber, requestedWidth int, kind, format string) (Asset, error) {
	descriptor, err := s.catalog.DescribePage(ctx, bookID, pageNumber)
	if err != nil {
		return Asset{}, err
	}
	if strings.ToLower(format) != "webp" || requestedWidth <= 0 {
		return s.openOriginal(descriptor)
	}

	targetWidth := s.normalizeWidth(requestedWidth, descriptor.Width)
	if targetWidth >= descriptor.Width && descriptor.MediaType == "image/webp" {
		return s.openOriginal(descriptor)
	}
	if kind != KindCover {
		kind = KindPage
	}
	cachePath := s.cachePath(descriptor, targetWidth, kind)
	if asset, ok := s.openCached(cachePath, descriptor, targetWidth); ok {
		s.hits.Add(1)
		return asset, nil
	}
	s.misses.Add(1)

	if err := s.generateOnce(ctx, cachePath, descriptor, targetWidth); err != nil {
		if ctx.Err() != nil {
			return Asset{}, ctx.Err()
		}
		s.logger.Warn("generate optimized image failed; serving original",
			"book_id", bookID, "page", pageNumber, "width", targetWidth, "error", err)
		return s.openOriginal(descriptor)
	}
	asset, ok := s.openCached(cachePath, descriptor, targetWidth)
	if !ok {
		return Asset{}, errors.New("optimized image disappeared after generation")
	}
	return asset, nil
}

func (s *Service) normalizeWidth(requested, original int) int {
	if requested < 320 {
		requested = 320
	}
	if requested > s.options.MaxWidth {
		requested = s.options.MaxWidth
	}
	buckets := []int{480, 720, 960, 1280, 1600, 1920, 2560, 3200, 3840, 5120}
	for _, bucket := range buckets {
		if bucket >= requested {
			requested = bucket
			break
		}
	}
	if requested > s.options.MaxWidth {
		requested = s.options.MaxWidth
	}
	if requested > original {
		return original
	}
	return requested
}

func (s *Service) cachePath(descriptor catalog.PageDescriptor, width int, kind string) string {
	name := fmt.Sprintf("p%05d-w%d-q%d-v%d.webp", descriptor.PageNumber, width, s.options.Quality, cacheVersion)
	if kind == KindCover {
		return filepath.Join(s.coverDir, descriptor.FileHash+"-"+name)
	}
	prefix := descriptor.FileHash
	if len(prefix) > 2 {
		prefix = prefix[:2]
	}
	return filepath.Join(s.pageDir, prefix, descriptor.FileHash, name)
}

func (s *Service) openOriginal(descriptor catalog.PageDescriptor) (Asset, error) {
	asset, err := s.catalog.OpenPageDescriptor(descriptor)
	if err != nil {
		return Asset{}, err
	}
	return Asset{
		Stream: asset.Stream, MediaType: asset.MediaType, Size: asset.Size, ETag: asset.ETag,
		Width: asset.Width, Height: asset.Height, PageNumber: asset.PageNumber, Cache: "original",
	}, nil
}

func (s *Service) openCached(path string, descriptor catalog.PageDescriptor, width int) (Asset, bool) {
	file, err := os.Open(path)
	if err != nil {
		return Asset{}, false
	}
	info, err := file.Stat()
	if err != nil || info.Size() == 0 {
		file.Close()
		return Asset{}, false
	}
	if time.Since(info.ModTime()) > 10*time.Minute {
		now := time.Now()
		_ = os.Chtimes(path, now, now)
	}
	height := descriptor.Height
	if descriptor.Width > 0 && width < descriptor.Width {
		height = max(1, int(float64(descriptor.Height)*float64(width)/float64(descriptor.Width)+0.5))
	}
	etag := fmt.Sprintf(`"%s-p%d-w%d-q%d-v%d"`, descriptor.FileHash, descriptor.PageNumber, width, s.options.Quality, cacheVersion)
	return Asset{
		Stream: file, MediaType: "image/webp", Size: info.Size(), ETag: etag,
		Width: width, Height: height, PageNumber: descriptor.PageNumber, Cache: "hit",
	}, true
}

func (s *Service) generateOnce(ctx context.Context, path string, descriptor catalog.PageDescriptor, width int) error {
	s.flightMu.Lock()
	if existing, ok := s.flights[path]; ok {
		s.flightMu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-existing.done:
			return existing.err
		}
	}
	current := &flight{done: make(chan struct{})}
	s.flights[path] = current
	s.flightMu.Unlock()

	current.err = s.generate(ctx, path, descriptor, width)
	close(current.done)
	s.flightMu.Lock()
	delete(s.flights, path)
	s.flightMu.Unlock()
	return current.err
}

func (s *Service) generate(ctx context.Context, path string, descriptor catalog.PageDescriptor, width int) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.workers <- struct{}{}:
	}
	s.active.Add(1)
	defer func() {
		s.active.Add(-1)
		<-s.workers
	}()

	started := time.Now()
	original, err := s.catalog.OpenPageDescriptor(descriptor)
	if err != nil {
		return err
	}
	defer original.Stream.Close()

	var decoded image.Image
	if descriptor.MediaType == "image/webp" {
		decoded, err = webp.Decode(original.Stream)
	} else {
		decoded, _, err = image.Decode(original.Stream)
	}
	if err != nil {
		return fmt.Errorf("decode source image: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	bounds := decoded.Bounds()
	output := decoded
	if width < bounds.Dx() {
		height := max(1, int(float64(bounds.Dy())*float64(width)/float64(bounds.Dx())+0.5))
		resized := image.NewNRGBA(image.Rect(0, 0, width, height))
		xdraw.CatmullRom.Scale(resized, resized.Bounds(), decoded, bounds, stddraw.Src, nil)
		output = resized
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create cache directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".samrai-*.tmp")
	if err != nil {
		return fmt.Errorf("create cache file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)

	encodeErr := webp.Encode(temporary, output, webp.Options{Quality: s.options.Quality, Method: 4})
	closeErr := temporary.Close()
	if encodeErr != nil {
		return fmt.Errorf("encode webp: %w", encodeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close cache file: %w", closeErr)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		if _, statErr := os.Stat(path); statErr == nil {
			return nil
		}
		return fmt.Errorf("publish cache file: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat generated image: %w", err)
	}
	elapsed := time.Since(started).Milliseconds()
	s.generated.Add(1)
	s.generatedBytes.Add(info.Size())
	s.generationMillis.Add(elapsed)
	s.bytes.Add(info.Size())
	return nil
}

func (s *Service) Metrics() Metrics {
	hits := s.hits.Load()
	misses := s.misses.Load()
	generated := s.generated.Load()
	generationMillis := s.generationMillis.Load()
	requests := hits + misses
	var hitRate float64
	if requests > 0 {
		hitRate = float64(hits) / float64(requests)
	}
	var average float64
	if generated > 0 {
		average = float64(generationMillis) / float64(generated)
	}
	return Metrics{
		Hits: hits, Misses: misses, Generated: generated,
		GeneratedBytes: s.generatedBytes.Load(), GenerationMilliseconds: generationMillis,
		AverageGenerationMillis: average, HitRate: hitRate,
		CacheBytes: s.bytes.Load(), CacheLimitBytes: s.options.MaxBytes,
		ActiveWorkers: s.active.Load(), WorkerLimit: s.options.Workers,
		Quality: s.options.Quality, MaxWidth: s.options.MaxWidth,
	}
}

func (s *Service) Cleanup(ctx context.Context) error {
	type entry struct {
		path    string
		size    int64
		modTime time.Time
	}
	entries := make([]entry, 0)
	var total int64
	for _, root := range []string{s.pageDir, s.coverDir} {
		err := filepath.WalkDir(root, func(path string, dir os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if dir.IsDir() || strings.HasSuffix(dir.Name(), ".tmp") {
				return nil
			}
			info, err := dir.Info()
			if err != nil {
				return err
			}
			total += info.Size()
			entries = append(entries, entry{path: path, size: info.Size(), modTime: info.ModTime()})
			return nil
		})
		if err != nil {
			return err
		}
	}
	s.bytes.Store(total)
	if total <= s.options.MaxBytes {
		return nil
	}
	target := s.options.MaxBytes * 9 / 10
	sort.Slice(entries, func(i, j int) bool { return entries[i].modTime.Before(entries[j].modTime) })
	for _, item := range entries {
		if total <= target {
			break
		}
		if err := os.Remove(item.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.logger.Warn("remove cached image", "path", item.path, "error", err)
			continue
		}
		total -= item.size
	}
	s.bytes.Store(max(int64(0), total))
	return nil
}

func (s *Service) PurgeAll() error {
	for _, root := range []string{s.pageDir, s.coverDir} {
		if err := os.RemoveAll(root); err != nil {
			return fmt.Errorf("remove image cache: %w", err)
		}
		if err := os.MkdirAll(root, 0o750); err != nil {
			return fmt.Errorf("recreate image cache: %w", err)
		}
	}
	s.bytes.Store(0)
	return nil
}

func (s *Service) PurgeBook(fileHash string) error {
	var removed int64
	pagePath := filepath.Join(s.pageDir, firstTwo(fileHash), fileHash)
	removed += directorySize(pagePath)
	if err := os.RemoveAll(pagePath); err != nil {
		return fmt.Errorf("remove page cache: %w", err)
	}
	matches, err := filepath.Glob(filepath.Join(s.coverDir, fileHash+"-*"))
	if err != nil {
		return fmt.Errorf("find cover cache: %w", err)
	}
	for _, match := range matches {
		if info, statErr := os.Stat(match); statErr == nil {
			removed += info.Size()
		}
		if removeErr := os.Remove(match); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return fmt.Errorf("remove cover cache: %w", removeErr)
		}
	}
	s.bytes.Add(-removed)
	if s.bytes.Load() < 0 {
		s.bytes.Store(0)
	}
	return nil
}

func (s *Service) recalculateSize() error {
	var total int64
	for _, root := range []string{s.pageDir, s.coverDir} {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			total += info.Size()
			return nil
		})
		if err != nil {
			return err
		}
	}
	s.bytes.Store(total)
	return nil
}

func (s *Service) loadStats(ctx context.Context) error {
	var values persistedStats
	err := s.db.QueryRowContext(ctx, `
		SELECT hits, misses, generated, generated_bytes, generation_milliseconds
		FROM cache_statistics WHERE cache_name = 'images'
	`).Scan(&values.hits, &values.misses, &values.generated, &values.generatedBytes, &values.generationMillis)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load image cache statistics: %w", err)
	}
	s.hits.Store(values.hits)
	s.misses.Store(values.misses)
	s.generated.Store(values.generated)
	s.generatedBytes.Store(values.generatedBytes)
	s.generationMillis.Store(values.generationMillis)
	s.flushed = values
	return nil
}

func (s *Service) flushStats(ctx context.Context) error {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()
	current := persistedStats{
		hits: s.hits.Load(), misses: s.misses.Load(), generated: s.generated.Load(),
		generatedBytes: s.generatedBytes.Load(), generationMillis: s.generationMillis.Load(),
	}
	delta := persistedStats{
		hits:             current.hits - s.flushed.hits,
		misses:           current.misses - s.flushed.misses,
		generated:        current.generated - s.flushed.generated,
		generatedBytes:   current.generatedBytes - s.flushed.generatedBytes,
		generationMillis: current.generationMillis - s.flushed.generationMillis,
	}
	if delta == (persistedStats{}) {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE cache_statistics SET
			hits = hits + ?, misses = misses + ?, generated = generated + ?,
			generated_bytes = generated_bytes + ?, generation_milliseconds = generation_milliseconds + ?,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE cache_name = 'images'
	`, delta.hits, delta.misses, delta.generated, delta.generatedBytes, delta.generationMillis)
	if err != nil {
		return err
	}
	s.flushed = current
	return nil
}

func firstTwo(value string) string {
	if len(value) <= 2 {
		return value
	}
	return value[:2]
}

func directorySize(root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if info, infoErr := entry.Info(); infoErr == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

func max[T ~int | ~int64](a, b T) T {
	if a > b {
		return a
	}
	return b
}

func ParseWidth(value string, fallback, maximum int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 1 {
		return fallback
	}
	if parsed > maximum {
		return maximum
	}
	return parsed
}
