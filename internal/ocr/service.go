package ocr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

var (
	ErrUnavailable         = errors.New("ocr unavailable")
	ErrUnsupportedLanguage = errors.New("ocr language unavailable")
	ErrInvalidImage        = errors.New("invalid ocr image")
)

var (
	languagePattern       = regexp.MustCompile(`^[a-z]{3}(?:\+[a-z]{3})*$`)
	singleLanguagePattern = regexp.MustCompile(`^[a-z]{3}$`)
)

type Options struct {
	Executable string
	Workers    int
	Timeout    time.Duration
	TempDir    string
}

type Status struct {
	Available  bool     `json:"available"`
	Executable string   `json:"executable,omitempty"`
	Languages  []string `json:"languages"`
	Workers    int      `json:"workers"`
	Message    string   `json:"message,omitempty"`
}

type Result struct {
	Text       string  `json:"text"`
	Language   string  `json:"language"`
	DurationMS int64   `json:"duration_ms"`
	Confidence float64 `json:"confidence,omitempty"`
}

type Service struct {
	executable string
	languages  map[string]struct{}
	workers    chan struct{}
	timeout    time.Duration
	tempDir    string
	logger     *slog.Logger
	message    string
}

func NewService(options Options, logger *slog.Logger) *Service {
	if options.Workers < 1 {
		options.Workers = 1
	}
	if options.Workers > 4 {
		options.Workers = 4
	}
	if options.Timeout <= 0 {
		options.Timeout = 90 * time.Second
	}
	tempDir := strings.TrimSpace(options.TempDir)
	if tempDir == "" {
		tempDir = filepath.Join(os.TempDir(), "samrai-ocr")
	}
	service := &Service{
		workers: make(chan struct{}, options.Workers),
		timeout: options.Timeout,
		tempDir: tempDir,
		logger:  logger,
	}
	service.executable = findExecutable(options.Executable)
	if service.executable == "" {
		service.message = "Tesseract OCR was not found. Install it or set SAMRAI_TESSERACT_PATH."
		return service
	}
	service.languages = listLanguages(service.executable)
	if len(service.languages) == 0 {
		service.message = "Tesseract was found, but no OCR language data is available."
		service.executable = ""
		return service
	}
	return service
}

func (s *Service) Status() Status {
	languages := make([]string, 0, len(s.languages))
	for language := range s.languages {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	return Status{
		Available:  s.executable != "",
		Executable: s.executable,
		Languages:  languages,
		Workers:    cap(s.workers),
		Message:    s.message,
	}
}

func (s *Service) Recognize(ctx context.Context, image []byte, language string) (Result, error) {
	if s.executable == "" {
		return Result{}, ErrUnavailable
	}
	if len(image) < 32 || len(image) > 12<<20 {
		return Result{}, ErrInvalidImage
	}
	extension := imageExtension(image)
	if extension == "" {
		return Result{}, ErrInvalidImage
	}
	language = s.selectLanguage(language)
	if language == "" {
		return Result{}, ErrUnsupportedLanguage
	}

	select {
	case s.workers <- struct{}{}:
		defer func() { <-s.workers }()
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	if err := os.MkdirAll(s.tempDir, 0o750); err != nil {
		return Result{}, fmt.Errorf("create ocr temporary directory: %w", err)
	}
	file, err := os.CreateTemp(s.tempDir, "samrai-ocr-*"+extension)
	if err != nil {
		return Result{}, fmt.Errorf("create ocr image: %w", err)
	}
	path := file.Name()
	defer os.Remove(path)
	if _, err := file.Write(image); err != nil {
		file.Close()
		return Result{}, fmt.Errorf("write ocr image: %w", err)
	}
	if err := file.Close(); err != nil {
		return Result{}, fmt.Errorf("close ocr image: %w", err)
	}

	started := time.Now()
	command := exec.CommandContext(
		timeoutCtx, s.executable, path, "stdout",
		"-l", language, "--oem", "1", "--psm", "3",
		"-c", "preserve_interword_spaces=1",
		"-c", "user_defined_dpi=200",
	)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
			return Result{}, fmt.Errorf("ocr timeout: %w", timeoutCtx.Err())
		}
		detail := strings.TrimSpace(stderr.String())
		if len(detail) > 500 {
			detail = detail[:500]
		}
		if detail == "" {
			return Result{}, fmt.Errorf("run tesseract: %w", err)
		}
		return Result{}, fmt.Errorf("run tesseract: %w: %s", err, detail)
	}
	text := normalizeText(string(output))
	duration := time.Since(started).Milliseconds()
	if s.logger != nil {
		s.logger.Info("ocr page completed", "language", language, "duration_ms", duration, "characters", len([]rune(text)))
	}
	return Result{Text: text, Language: language, DurationMS: duration}, nil
}

func (s *Service) selectLanguage(requested string) string {
	requested = strings.ToLower(strings.TrimSpace(requested))
	if languagePattern.MatchString(requested) {
		parts := strings.Split(requested, "+")
		valid := true
		for _, part := range parts {
			if _, ok := s.languages[part]; !ok {
				valid = false
				break
			}
		}
		if valid {
			return requested
		}
	}
	for _, fallback := range []string{"por", "eng"} {
		if _, ok := s.languages[fallback]; ok {
			return fallback
		}
	}
	for language := range s.languages {
		return language
	}
	return ""
}

func findExecutable(configured string) string {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		if path, err := exec.LookPath(configured); err == nil {
			return path
		}
		if info, err := os.Stat(configured); err == nil && !info.IsDir() {
			return configured
		}
	}
	if path, err := exec.LookPath("tesseract"); err == nil {
		return path
	}
	if runtime.GOOS == "windows" {
		for _, candidate := range []string{
			filepath.Join(os.Getenv("ProgramFiles"), "Tesseract-OCR", "tesseract.exe"),
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "Tesseract-OCR", "tesseract.exe"),
		} {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
		}
	}
	return ""
}

func listLanguages(executable string) map[string]struct{} {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, executable, "--list-langs").CombinedOutput()
	if err != nil {
		return nil
	}
	languages := make(map[string]struct{})
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.ToLower(strings.TrimSpace(line))
		if singleLanguagePattern.MatchString(line) {
			languages[line] = struct{}{}
		}
	}
	return languages
}

func imageExtension(image []byte) string {
	if len(image) >= 8 && bytes.Equal(image[:8], []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}) {
		return ".png"
	}
	if len(image) >= 3 && image[0] == 0xff && image[1] == 0xd8 && image[2] == 0xff {
		return ".jpg"
	}
	if len(image) >= 12 && bytes.Equal(image[:4], []byte("RIFF")) && bytes.Equal(image[8:12], []byte("WEBP")) {
		return ".webp"
	}
	return ""
}

func normalizeText(value string) string {
	value = strings.ReplaceAll(value, "\x00", "")
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return strings.Join(cleaned, "\n")
}
