package systeminfo

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Limits struct {
	MaxUploadBytes  int64 `json:"max_upload_bytes"`
	ImageCacheBytes int64 `json:"image_cache_bytes"`
	ImageWorkers    int   `json:"image_workers"`
	ImageQuality    int   `json:"image_quality"`
	ImageMaxWidth   int   `json:"image_max_width"`
}
type Storage struct {
	DatabaseBytes int64 `json:"database_bytes"`
	LibraryBytes  int64 `json:"library_bytes"`
	PreparedBytes int64 `json:"prepared_bytes"`
	CacheBytes    int64 `json:"cache_bytes"`
	BackupBytes   int64 `json:"backup_bytes"`
}
type Database struct {
	Users  int `json:"users"`
	Books  int `json:"books"`
	Series int `json:"series"`
	Pages  int `json:"pages"`
	Jobs   int `json:"jobs"`
}
type Build struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
	Go      string `json:"go"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}
type Info struct {
	Build         Build    `json:"build"`
	Storage       Storage  `json:"storage"`
	Database      Database `json:"database"`
	Limits        Limits   `json:"limits"`
	UptimeSeconds int64    `json:"uptime_seconds"`
	DataDirectory string   `json:"data_directory"`
}
type Options struct {
	DataDir, DatabasePath, LogPath, Version, Commit, BuildDate string
	StartedAt                                                  time.Time
	Limits                                                     Limits
}
type Service struct {
	db      *sql.DB
	options Options
}

func NewService(db *sql.DB, o Options) *Service { return &Service{db: db, options: o} }
func (s *Service) Info(ctx context.Context) (Info, error) {
	i := Info{Build: Build{s.options.Version, s.options.Commit, s.options.BuildDate, runtime.Version(), runtime.GOOS, runtime.GOARCH}, Limits: s.options.Limits, UptimeSeconds: int64(time.Since(s.options.StartedAt).Seconds()), DataDirectory: s.options.DataDir}
	if st, e := os.Stat(s.options.DatabasePath); e == nil {
		i.Storage.DatabaseBytes = st.Size()
	} else if !errors.Is(e, os.ErrNotExist) {
		return Info{}, e
	}
	var e error
	if i.Storage.LibraryBytes, e = dirSize(filepath.Join(s.options.DataDir, "library")); e != nil {
		return Info{}, e
	}
	if i.Storage.PreparedBytes, e = dirSize(filepath.Join(s.options.DataDir, "prepared")); e != nil {
		return Info{}, e
	}
	if i.Storage.CacheBytes, e = dirSize(filepath.Join(s.options.DataDir, "cache")); e != nil {
		return Info{}, e
	}
	if i.Storage.BackupBytes, e = dirSize(filepath.Join(s.options.DataDir, "backups")); e != nil {
		return Info{}, e
	}
	queries := []struct {
		q string
		p *int
	}{{"SELECT COUNT(*) FROM users", &i.Database.Users}, {"SELECT COUNT(*) FROM books", &i.Database.Books}, {"SELECT COUNT(*) FROM series", &i.Database.Series}, {"SELECT COALESCE(SUM(page_count), 0) FROM books", &i.Database.Pages}, {"SELECT COUNT(*) FROM jobs", &i.Database.Jobs}}
	for _, x := range queries {
		if e := s.db.QueryRowContext(ctx, x.q).Scan(x.p); e != nil {
			return Info{}, e
		}
	}
	return i, nil
}
func (s *Service) RecentLogs(max int) ([]string, error) {
	if max < 1 {
		max = 1
	}
	if max > 500 {
		max = 500
	}
	f, e := os.Open(s.options.LogPath)
	if errors.Is(e, os.ErrNotExist) {
		return []string{}, nil
	}
	if e != nil {
		return nil, e
	}
	defer f.Close()
	lines := make([]string, 0, max)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if len(lines) == max {
			copy(lines, lines[1:])
			lines = lines[:max-1]
		}
		lines = append(lines, line)
	}
	return lines, scanner.Err()
}
func dirSize(root string) (int64, error) {
	var total int64
	e := filepath.WalkDir(root, func(_ string, d os.DirEntry, e error) error {
		if errors.Is(e, os.ErrNotExist) {
			return nil
		}
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		i, e := d.Info()
		if e != nil {
			return e
		}
		if i.Mode().IsRegular() {
			total += i.Size()
		}
		return nil
	})
	if errors.Is(e, os.ErrNotExist) {
		return 0, nil
	}
	if e != nil {
		return 0, fmt.Errorf("measure directory: %w", e)
	}
	return total, nil
}
