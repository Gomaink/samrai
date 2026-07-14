package backup

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const FormatVersion = 1

type Manifest struct {
	FormatVersion int       `json:"format_version"`
	AppVersion    string    `json:"app_version"`
	CreatedAt     time.Time `json:"created_at"`
	IncludesBooks bool      `json:"includes_books"`
}
type Service struct {
	db                  *sql.DB
	dataDir, appVersion string
	mu                  sync.Mutex
}

func NewService(db *sql.DB, dataDir, appVersion string) *Service {
	return &Service{db: db, dataDir: dataDir, appVersion: appVersion}
}

func (s *Service) Create(ctx context.Context) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Join(s.dataDir, "backups")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", "", err
	}
	stamp := time.Now().UTC().Format("20060102-150405.000000000")
	name := "samrai-backup-" + stamp + ".zip"
	final := filepath.Join(dir, name)
	partial := final + ".part"
	snapshot := filepath.Join(dir, ".snapshot-"+stamp+".db")
	_ = os.Remove(partial)
	_ = os.Remove(snapshot)
	defer os.Remove(snapshot)
	quoted := strings.ReplaceAll(filepath.ToSlash(snapshot), "'", "''")
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO '"+quoted+"'"); err != nil {
		return "", "", fmt.Errorf("create SQLite snapshot: %w", err)
	}
	out, err := os.OpenFile(partial, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", "", err
	}
	zw := zip.NewWriter(out)
	fail := func(e error) (string, string, error) {
		_ = zw.Close()
		_ = out.Close()
		_ = os.Remove(partial)
		return "", "", e
	}
	entry, err := zw.Create("manifest.json")
	if err != nil {
		return fail(err)
	}
	payload, _ := json.MarshalIndent(Manifest{FormatVersion: FormatVersion, AppVersion: s.appVersion, CreatedAt: time.Now().UTC(), IncludesBooks: true}, "", "  ")
	if _, err = entry.Write(payload); err != nil {
		return fail(err)
	}
	if err = addFile(zw, snapshot, "config/samrai.db"); err != nil {
		return fail(err)
	}
	for _, directory := range []string{"library", "prepared"} {
		root := filepath.Join(s.dataDir, directory)
		if _, statErr := os.Stat(root); errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		err = filepath.WalkDir(root, func(current string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if d.IsDir() {
				return nil
			}
			info, e := d.Info()
			if e != nil {
				return e
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			rel, e := filepath.Rel(root, current)
			if e != nil {
				return e
			}
			return addFile(zw, current, filepath.ToSlash(filepath.Join(directory, rel)))
		})
		if err != nil {
			return fail(err)
		}
	}
	if err = zw.Close(); err != nil {
		_ = out.Close()
		return "", "", err
	}
	if err = out.Close(); err != nil {
		return "", "", err
	}
	if err = os.Rename(partial, final); err != nil {
		return "", "", err
	}
	return final, name, nil
}
func addFile(zw *zip.Writer, path, name string) error {
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	h, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	h.Name = name
	h.Method = zip.Store
	h.SetMode(0o600)
	entry, err := zw.CreateHeader(h)
	if err != nil {
		return err
	}
	_, err = io.Copy(entry, in)
	return err
}

func Restore(archivePath, dataDir string, force bool) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open backup: %w", err)
	}
	defer zr.Close()
	target, err := filepath.Abs(dataDir)
	if err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(target), ".samrai-restore-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	var manifest Manifest
	hasManifest, hasDB := false, false
	for _, f := range zr.File {
		clean, err := safePath(f.Name)
		if err != nil {
			return err
		}
		if clean == "manifest.json" {
			r, e := f.Open()
			if e != nil {
				return e
			}
			e = json.NewDecoder(io.LimitReader(r, 1<<20)).Decode(&manifest)
			r.Close()
			if e != nil {
				return e
			}
			hasManifest = true
			continue
		}
		isDatabase := clean == "config/samrai.db" || clean == "config/pageturner.db"
		if !isDatabase && !strings.HasPrefix(clean, "library/") && !strings.HasPrefix(clean, "prepared/") {
			continue
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("backup contains symbolic link %q", f.Name)
		}
		stageName := clean
		if clean == "config/pageturner.db" {
			stageName = "config/samrai.db"
		}
		dst := filepath.Join(stage, filepath.FromSlash(stageName))
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(dst, 0o750); err != nil {
				return err
			}
			continue
		}
		if isDatabase {
			hasDB = true
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			return err
		}
		r, e := f.Open()
		if e != nil {
			return e
		}
		o, e := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if e != nil {
			r.Close()
			return e
		}
		_, ce := io.Copy(o, r)
		oe := o.Close()
		r.Close()
		if ce != nil {
			return ce
		}
		if oe != nil {
			return oe
		}
	}
	if !hasManifest || manifest.FormatVersion != FormatVersion {
		return errors.New("unsupported or missing backup manifest")
	}
	if !hasDB {
		return errors.New("backup does not contain config/samrai.db")
	}
	for _, databaseName := range []string{"samrai.db", "pageturner.db"} {
		if _, err := os.Stat(filepath.Join(target, "config", databaseName)); err == nil && !force {
			return errors.New("target already contains a database; use --force only while the server is stopped")
		}
	}
	if err := os.MkdirAll(target, 0o750); err != nil {
		return err
	}
	if force {
		rollback := filepath.Join(target, "pre-restore-"+time.Now().UTC().Format("20060102-150405"))
		if err := os.MkdirAll(rollback, 0o750); err != nil {
			return err
		}
		for _, name := range []string{"config", "library", "prepared"} {
			current := filepath.Join(target, name)
			if _, err := os.Stat(current); err == nil {
				if err := os.Rename(current, filepath.Join(rollback, name)); err != nil {
					return err
				}
			}
		}
	}
	for _, name := range []string{"config", "library", "prepared"} {
		src := filepath.Join(stage, name)
		if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
			if name == "library" || name == "prepared" {
				if err := os.MkdirAll(filepath.Join(target, name), 0o750); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("restored %s is missing", name)
		}
		if err := os.Rename(src, filepath.Join(target, name)); err != nil {
			return err
		}
	}
	for _, name := range []string{"uploads", "staging", "prepared", "cache/pages", "cache/covers", "cache/thumbnails", "logs", "backups"} {
		if err := os.MkdirAll(filepath.Join(target, filepath.FromSlash(name)), 0o750); err != nil {
			return err
		}
	}
	return nil
}
func safePath(name string) (string, error) {
	normalized := strings.ReplaceAll(name, "\\", "/")
	clean := path.Clean(normalized)
	clean = strings.TrimPrefix(clean, "./")
	if clean == "." || clean == "" || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || strings.Contains(clean, ":") || strings.ContainsRune(clean, '\x00') {
		return "", fmt.Errorf("unsafe backup path %q", name)
	}
	return clean, nil
}
