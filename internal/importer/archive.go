package importer

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	maxImageDimension = 100_000
	maxImagePixels    = 500_000_000
)

func inspectArchive(ctx context.Context, filename, archivePath string, limits Limits) (inspectedBook, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return inspectedBook{}, fmt.Errorf("%w: %v", ErrInvalidArchive, err)
	}
	defer reader.Close()

	if len(reader.File) == 0 {
		return inspectedBook{}, fmt.Errorf("%w: empty file", ErrInvalidArchive)
	}
	if len(reader.File) > limits.MaxArchiveEntries {
		return inspectedBook{}, fmt.Errorf("%w: more than %d entries", ErrInvalidArchive, limits.MaxArchiveEntries)
	}

	var totalUncompressed uint64
	imageFiles := make(map[string]*zip.File)
	imagePaths := make([]string, 0, len(reader.File))
	for _, entry := range reader.File {
		if err := ctx.Err(); err != nil {
			return inspectedBook{}, err
		}
		if err := validateArchivePath(entry.Name); err != nil {
			return inspectedBook{}, fmt.Errorf("%w: %v", ErrInvalidArchive, err)
		}
		if entry.FileInfo().IsDir() {
			continue
		}
		if entry.FileInfo().Mode()&os.ModeSymlink != 0 {
			return inspectedBook{}, fmt.Errorf("%w: symbolic link is not allowed in %q", ErrInvalidArchive, entry.Name)
		}

		maximumArchiveBytes := uint64(limits.MaxArchiveBytes)
		if entry.UncompressedSize64 > maximumArchiveBytes-totalUncompressed {
			return inspectedBook{}, fmt.Errorf("%w: uncompressed content exceeds the limit", ErrInvalidArchive)
		}
		totalUncompressed += entry.UncompressedSize64
		if totalUncompressed > maximumArchiveBytes {
			return inspectedBook{}, fmt.Errorf("%w: uncompressed content exceeds the limit", ErrInvalidArchive)
		}
		if entry.UncompressedSize64 > uint64(limits.MaxPageBytes) && isSupportedImage(entry.Name) {
			return inspectedBook{}, fmt.Errorf("%w: page %q exceeds the limit", ErrInvalidArchive, entry.Name)
		}
		if suspiciousCompressionRatio(entry) {
			return inspectedBook{}, fmt.Errorf("%w: suspicious compression ratio in %q", ErrInvalidArchive, entry.Name)
		}
		if !isSupportedImage(entry.Name) || isIgnoredArchiveEntry(entry.Name) {
			continue
		}
		if _, exists := imageFiles[entry.Name]; exists {
			return inspectedBook{}, fmt.Errorf("%w: duplicate page %q", ErrInvalidArchive, entry.Name)
		}

		imageFiles[entry.Name] = entry
		imagePaths = append(imagePaths, entry.Name)
	}

	if len(imagePaths) == 0 {
		return inspectedBook{}, fmt.Errorf("%w: no JPEG, PNG, or WebP images found", ErrInvalidArchive)
	}
	if len(imagePaths) > limits.MaxPages {
		return inspectedBook{}, fmt.Errorf("%w: more than %d pages", ErrInvalidArchive, limits.MaxPages)
	}

	naturalSort(imagePaths)
	pages := make([]inspectedPage, 0, len(imagePaths))
	for _, imagePath := range imagePaths {
		if err := ctx.Err(); err != nil {
			return inspectedBook{}, err
		}
		entry := imageFiles[imagePath]
		width, height, mediaType, err := readImageConfig(entry, limits.MaxPageBytes)
		if err != nil {
			return inspectedBook{}, fmt.Errorf("%w: page %q: %v", ErrInvalidArchive, imagePath, err)
		}
		pages = append(pages, inspectedPage{
			ArchivePath: imagePath,
			MediaType:   mediaType,
			Width:       width,
			Height:      height,
			FileSize:    int64(entry.UncompressedSize64),
		})
	}

	title := strings.TrimSpace(strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename)))
	if title == "" {
		title = "Untitled book"
	}
	metadata, metadataErr := readComicInfo(findComicInfo(reader.File))
	if metadataErr != nil {
		return inspectedBook{}, fmt.Errorf("%w: %v", ErrInvalidArchive, metadataErr)
	}
	if metadata.Title != "" {
		title = metadata.Title
	}
	return inspectedBook{
		Format:    "cbz",
		PageCount: len(pages),
		Title:     title, Series: metadata.Series, Summary: metadata.Summary,
		Writer: metadata.Writer, Publisher: metadata.Publisher,
		PublicationYear: metadata.PublicationYear, Volume: metadata.Volume,
		Number: metadata.Number, Language: metadata.Language,
		ReadingDirection: metadata.ReadingDirection, Pages: pages,
	}, nil
}

func validateArchivePath(name string) error {
	if name == "" {
		return errors.New("entry has no name")
	}
	if strings.ContainsRune(name, '\x00') || strings.Contains(name, "\\") {
		return fmt.Errorf("invalid path %q", name)
	}
	if strings.HasPrefix(name, "/") || filepath.IsAbs(name) || filepath.VolumeName(name) != "" || hasDrivePrefix(name) {
		return fmt.Errorf("absolute path %q", name)
	}
	cleaned := path.Clean(name)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("path escapes the archive: %q", name)
	}
	return nil
}

func isIgnoredArchiveEntry(name string) bool {
	for _, segment := range strings.Split(path.Clean(name), "/") {
		lower := strings.ToLower(segment)
		if lower == "__macosx" || strings.HasPrefix(segment, "._") {
			return true
		}
	}
	return false
}

func isSupportedImage(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".webp":
		return true
	default:
		return false
	}
}

func suspiciousCompressionRatio(entry *zip.File) bool {
	if entry.UncompressedSize64 < 10*1024*1024 {
		return false
	}
	if entry.CompressedSize64 == 0 {
		return entry.UncompressedSize64 > 0
	}
	return entry.UncompressedSize64/entry.CompressedSize64 > 1000
}

func readImageConfig(entry *zip.File, maxBytes int64) (int, int, string, error) {
	stream, err := entry.Open()
	if err != nil {
		return 0, 0, "", err
	}
	defer stream.Close()
	return readImageConfigReader(stream, maxBytes)
}

func readImageConfigReader(stream io.Reader, maxBytes int64) (int, int, string, error) {
	reader := bufio.NewReader(io.LimitReader(stream, maxBytes+1))
	header, err := reader.Peek(12)
	if err != nil {
		return 0, 0, "", errors.New("truncated image")
	}

	if isWebPHeader(header) {
		width, height, err := decodeWebPConfig(reader)
		if err != nil {
			return 0, 0, "", err
		}
		return width, height, "image/webp", nil
	}

	config, format, err := image.DecodeConfig(reader)
	if err != nil {
		return 0, 0, "", fmt.Errorf("invalid image: %w", err)
	}
	if err := validateImageDimensions(config.Width, config.Height); err != nil {
		return 0, 0, "", err
	}
	switch format {
	case "jpeg":
		return config.Width, config.Height, "image/jpeg", nil
	case "png":
		return config.Width, config.Height, "image/png", nil
	default:
		return 0, 0, "", fmt.Errorf("unsupported format %q", format)
	}
}

func isWebPHeader(header []byte) bool {
	return len(header) >= 12 && string(header[:4]) == "RIFF" && string(header[8:12]) == "WEBP"
}

func decodeWebPConfig(reader *bufio.Reader) (int, int, error) {
	header := make([]byte, 30)
	if _, err := io.ReadFull(reader, header[:20]); err != nil {
		return 0, 0, errors.New("WebP truncado")
	}
	chunk := string(header[12:16])

	switch chunk {
	case "VP8X":
		if _, err := io.ReadFull(reader, header[20:30]); err != nil {
			return 0, 0, errors.New("WebP VP8X truncado")
		}
		width := 1 + int(header[24]) + int(header[25])<<8 + int(header[26])<<16
		height := 1 + int(header[27]) + int(header[28])<<8 + int(header[29])<<16
		return validDimensions(width, height)
	case "VP8L":
		if _, err := io.ReadFull(reader, header[20:25]); err != nil {
			return 0, 0, errors.New("WebP VP8L truncado")
		}
		if header[20] != 0x2f {
			return 0, 0, errors.New("invalid WebP VP8L signature")
		}
		bits := binary.LittleEndian.Uint32(header[21:25])
		width := int(bits&0x3fff) + 1
		height := int((bits>>14)&0x3fff) + 1
		return validDimensions(width, height)
	case "VP8 ":
		buffer := make([]byte, 64)
		copy(buffer, header[:20])
		n, _ := io.ReadFull(reader, buffer[20:])
		buffer = buffer[:20+n]
		for index := 20; index+9 < len(buffer); index++ {
			if buffer[index] == 0x9d && buffer[index+1] == 0x01 && buffer[index+2] == 0x2a {
				width := int(binary.LittleEndian.Uint16(buffer[index+3:index+5]) & 0x3fff)
				height := int(binary.LittleEndian.Uint16(buffer[index+5:index+7]) & 0x3fff)
				return validDimensions(width, height)
			}
		}
		return 0, 0, errors.New("invalid WebP VP8 header")
	default:
		return 0, 0, fmt.Errorf("unsupported WebP type %q", chunk)
	}
}

func validDimensions(width, height int) (int, int, error) {
	if err := validateImageDimensions(width, height); err != nil {
		return 0, 0, err
	}
	return width, height, nil
}

func validateImageDimensions(width, height int) error {
	if width < 1 || height < 1 {
		return errors.New("invalid dimensions")
	}
	if width > maxImageDimension || height > maxImageDimension {
		return fmt.Errorf("dimensions exceed %d pixels", maxImageDimension)
	}
	if int64(width)*int64(height) > maxImagePixels {
		return errors.New("pixel count exceeds the limit")
	}
	return nil
}

func hasDrivePrefix(name string) bool {
	if len(name) < 2 || name[1] != ':' {
		return false
	}
	first := name[0]
	return (first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z')
}
