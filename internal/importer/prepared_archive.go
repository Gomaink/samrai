package importer

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

const preparedPathPrefix = "prepared:"

type externalArchiveTools struct {
	SevenZip string
	LSAR     string
	UNAR     string
}

type listedArchiveEntry struct {
	Name           string
	Size           int64
	CompressedSize int64
	Directory      bool
	Symlink        bool
	Encrypted      bool
}

func discoverArchiveTools(sevenZip, lsar, unar string) externalArchiveTools {
	return externalArchiveTools{
		SevenZip: discoverExecutable(sevenZip, sevenZipCandidates()),
		LSAR:     discoverExecutable(lsar, []string{"lsar"}),
		UNAR:     discoverExecutable(unar, []string{"unar"}),
	}
}

func discoverExecutable(configured string, candidates []string) string {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		if info, err := os.Stat(configured); err == nil && info.Mode().IsRegular() {
			return configured
		}
		if resolved, err := exec.LookPath(configured); err == nil {
			return resolved
		}
		return configured
	}
	for _, candidate := range candidates {
		if strings.ContainsAny(candidate, `/\\`) {
			if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
				return candidate
			}
			continue
		}
		if resolved, err := exec.LookPath(candidate); err == nil {
			return resolved
		}
	}
	return ""
}

func sevenZipCandidates() []string {
	candidates := []string{"7zz", "7z", "7za"}
	if runtime.GOOS == "windows" {
		candidates = append([]string{
			`C:\Program Files\7-Zip\7z.exe`,
			`C:\Program Files (x86)\7-Zip\7z.exe`,
		}, candidates...)
	}
	return candidates
}

func inspectPreparedComic(
	ctx context.Context,
	filename, archivePath, format, fileHash, preparedRoot string,
	limits Limits,
	tools externalArchiveTools,
) (book inspectedBook, err error) {
	if strings.TrimSpace(fileHash) == "" {
		return inspectedBook{}, fmt.Errorf("%w: hash ausente", ErrInvalidArchive)
	}
	if err := validateComicContainerSignature(archivePath, format); err != nil {
		return inspectedBook{}, err
	}
	if err := os.MkdirAll(preparedRoot, 0o750); err != nil {
		return inspectedBook{}, fmt.Errorf("create prepared root: %w", err)
	}
	random, err := randomHex(8)
	if err != nil {
		return inspectedBook{}, err
	}
	temporary := filepath.Join(preparedRoot, "."+fileHash+".tmp-"+random)
	sourceDir := filepath.Join(temporary, "source")
	if err := os.MkdirAll(sourceDir, 0o750); err != nil {
		return inspectedBook{}, fmt.Errorf("create archive staging: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(temporary)
		}
	}()

	switch format {
	case "cbt":
		err = extractTarSecure(ctx, archivePath, sourceDir, limits)
	case "cbr", "cb7":
		err = extractExternalArchive(ctx, archivePath, sourceDir, limits, tools)
	default:
		err = fmt.Errorf("%w: unsupported comic format %q", ErrInvalidArchive, format)
	}
	if err != nil {
		return inspectedBook{}, err
	}

	book, err = inspectPreparedDirectory(ctx, filename, format, sourceDir, temporary, limits)
	if err != nil {
		return inspectedBook{}, err
	}
	finalDirectory := filepath.Join(preparedRoot, fileHash)
	if err := os.RemoveAll(finalDirectory); err != nil {
		return inspectedBook{}, fmt.Errorf("replace prepared archive: %w", err)
	}
	if err := os.Rename(temporary, finalDirectory); err != nil {
		return inspectedBook{}, fmt.Errorf("publish prepared archive: %w", err)
	}
	book.PreparedDir = finalDirectory
	return book, nil
}

func validateComicContainerSignature(archivePath, format string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("%w: open archive: %v", ErrInvalidArchive, err)
	}
	defer file.Close()
	header := make([]byte, 8)
	n, _ := io.ReadFull(file, header)
	header = header[:n]
	switch format {
	case "cbr":
		if !bytes.HasPrefix(header, []byte("Rar!\x1a\x07\x00")) && !bytes.HasPrefix(header, []byte("Rar!\x1a\x07\x01\x00")) {
			return fmt.Errorf("%w: invalid RAR signature", ErrInvalidArchive)
		}
	case "cb7":
		if !bytes.HasPrefix(header, []byte{0x37, 0x7a, 0xbc, 0xaf, 0x27, 0x1c}) {
			return fmt.Errorf("%w: invalid 7z signature", ErrInvalidArchive)
		}
	case "cbt":
		// TAR has no mandatory magic at byte zero. archive/tar performs the
		// structural validation while entries are streamed below.
	default:
		return fmt.Errorf("%w: unknown format", ErrInvalidArchive)
	}
	return nil
}

func extractTarSecure(ctx context.Context, archivePath, destination string, limits Limits) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("%w: open TAR: %v", ErrInvalidArchive, err)
	}
	defer file.Close()
	reader := tar.NewReader(file)
	entries := 0
	var total int64
	seen := map[string]struct{}{}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, readErr := reader.Next()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return fmt.Errorf("%w: invalid TAR: %v", ErrInvalidArchive, readErr)
		}
		entries++
		if entries > limits.MaxArchiveEntries {
			return fmt.Errorf("%w: more than %d entries", ErrInvalidArchive, limits.MaxArchiveEntries)
		}
		if err := validateArchivePath(header.Name); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidArchive, err)
		}
		key := strings.ToLower(filepath.ToSlash(filepath.Clean(header.Name)))
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%w: duplicate entry %q", ErrInvalidArchive, header.Name)
		}
		seen[key] = struct{}{}
		switch header.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg, tar.TypeRegA:
		default:
			return fmt.Errorf("%w: links and special file types are not allowed in %q", ErrInvalidArchive, header.Name)
		}
		if header.Size < 0 || header.Size > limits.MaxArchiveBytes-total {
			return fmt.Errorf("%w: uncompressed content exceeds the limit", ErrInvalidArchive)
		}
		total += header.Size
		if isSupportedImage(header.Name) && header.Size > limits.MaxPageBytes {
			return fmt.Errorf("%w: page %q exceeds the limit", ErrInvalidArchive, header.Name)
		}
		if !isSupportedImage(header.Name) && !strings.EqualFold(filepath.Base(header.Name), "ComicInfo.xml") {
			continue
		}
		target, err := safeExtractionTarget(destination, header.Name)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidArchive, err)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
		if err != nil {
			return fmt.Errorf("extract %q: %w", header.Name, err)
		}
		written, copyErr := io.Copy(out, io.LimitReader(reader, header.Size+1))
		closeErr := out.Close()
		if copyErr != nil || closeErr != nil || written != header.Size {
			_ = os.Remove(target)
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			return fmt.Errorf("%w: truncated entry %q", ErrInvalidArchive, header.Name)
		}
	}
	if entries == 0 {
		return fmt.Errorf("%w: empty TAR", ErrInvalidArchive)
	}
	return nil
}

func extractExternalArchive(ctx context.Context, archivePath, destination string, limits Limits, tools externalArchiveTools) error {
	var errorsFound []string
	if tools.SevenZip != "" {
		entries, err := listWithSevenZip(ctx, tools.SevenZip, archivePath)
		if err == nil {
			selected, validateErr := validateListedEntries(entries, limits)
			if validateErr != nil {
				return validateErr
			}
			if expansionErr := validateArchiveExpansion(archivePath, entries); expansionErr != nil {
				return expansionErr
			}
			if extractErr := extractWithSevenZip(ctx, tools.SevenZip, archivePath, destination, selected); extractErr == nil {
				return verifyExtractedSelection(destination, selected, limits)
			} else {
				errorsFound = append(errorsFound, "7-Zip: "+extractErr.Error())
			}
		} else {
			errorsFound = append(errorsFound, "7-Zip: "+err.Error())
		}
	}
	if tools.LSAR != "" && tools.UNAR != "" {
		entries, err := listWithLSAR(ctx, tools.LSAR, archivePath)
		if err == nil {
			selected, validateErr := validateListedEntries(entries, limits)
			if validateErr != nil {
				return validateErr
			}
			if expansionErr := validateArchiveExpansion(archivePath, entries); expansionErr != nil {
				return expansionErr
			}
			if extractErr := extractWithUNAR(ctx, tools.UNAR, archivePath, destination); extractErr == nil {
				return verifyExtractedSelection(destination, selected, limits)
			} else {
				errorsFound = append(errorsFound, "unar: "+extractErr.Error())
			}
		} else {
			errorsFound = append(errorsFound, "lsar: "+err.Error())
		}
	}
	if len(errorsFound) == 0 {
		return ErrArchiveToolUnavailable
	}
	return fmt.Errorf("%w: %s", ErrInvalidArchive, strings.Join(errorsFound, "; "))
}

func listWithSevenZip(ctx context.Context, executable, archivePath string) ([]listedArchiveEntry, error) {
	command := exec.CommandContext(ctx, executable, "l", "-slt", "-ba", "-sccUTF-8", archivePath)
	output, err := command.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return nil, fmt.Errorf("could not list the archive: %s", cleanToolError(exit.Stderr))
		}
		return nil, err
	}
	return parseSevenZipListing(string(output))
}

func parseSevenZipListing(output string) ([]listedArchiveEntry, error) {
	blocks := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n\n")
	entries := make([]listedArchiveEntry, 0, len(blocks))
	for _, block := range blocks {
		values := map[string]string{}
		for _, line := range strings.Split(block, "\n") {
			key, value, ok := strings.Cut(line, " = ")
			if ok {
				values[strings.TrimSpace(key)] = strings.TrimSpace(value)
			}
		}
		name := strings.ReplaceAll(values["Path"], "\\", "/")
		if name == "" || values["Size"] == "" {
			continue
		}
		size, err := strconv.ParseInt(values["Size"], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid size for %q", name)
		}
		packed, _ := strconv.ParseInt(values["Packed Size"], 10, 64)
		attributes := strings.TrimSpace(values["Attributes"])
		lowerAttributes := strings.ToLower(attributes)
		entry := listedArchiveEntry{
			Name: name, Size: size, CompressedSize: packed,
			Directory: values["Folder"] == "+" || strings.HasPrefix(attributes, "D") || strings.HasPrefix(lowerAttributes, "d"),
			Symlink:   values["Symbolic Link"] != "" || values["Hard Link"] != "" || strings.HasPrefix(lowerAttributes, "l"),
			Encrypted: values["Encrypted"] == "+",
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return nil, errors.New("empty listing or unsupported format")
	}
	return entries, nil
}

func listWithLSAR(ctx context.Context, executable, archivePath string) ([]listedArchiveEntry, error) {
	command := exec.CommandContext(ctx, executable, "-ja", "-nr", archivePath)
	output, err := command.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return nil, fmt.Errorf("could not list the archive: %s", cleanToolError(exit.Stderr))
		}
		return nil, err
	}
	var document struct {
		Contents []map[string]any `json:"lsarContents"`
	}
	if err := json.Unmarshal(output, &document); err != nil {
		return nil, fmt.Errorf("invalid JSON listing: %w", err)
	}
	entries := make([]listedArchiveEntry, 0, len(document.Contents))
	for _, value := range document.Contents {
		name, _ := value["XADFileName"].(string)
		name = strings.ReplaceAll(name, "\\", "/")
		if name == "" {
			continue
		}
		entry := listedArchiveEntry{
			Name:           name,
			Size:           jsonNumber(value["XADFileSize"]),
			CompressedSize: jsonNumber(value["XADCompressedSize"]),
			Directory:      jsonBool(value["XADIsDirectory"]),
			Symlink:        jsonBool(value["XADIsLink"]) || value["XADLinkDestination"] != nil,
			Encrypted:      jsonBool(value["XADIsEncrypted"]),
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return nil, errors.New("empty listing or unsupported format")
	}
	return entries, nil
}

func jsonNumber(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case json.Number:
		result, _ := typed.Int64()
		return result
	case string:
		result, _ := strconv.ParseInt(typed, 10, 64)
		return result
	default:
		return 0
	}
}

func jsonBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return typed == "1" || strings.EqualFold(typed, "true") || typed == "+"
	case float64:
		return typed != 0
	default:
		return false
	}
}

func validateListedEntries(entries []listedArchiveEntry, limits Limits) ([]listedArchiveEntry, error) {
	if len(entries) > limits.MaxArchiveEntries {
		return nil, fmt.Errorf("%w: more than %d entries", ErrInvalidArchive, limits.MaxArchiveEntries)
	}
	selected := make([]listedArchiveEntry, 0)
	seen := map[string]struct{}{}
	var total int64
	pages := 0
	for _, entry := range entries {
		if err := validateArchivePath(entry.Name); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidArchive, err)
		}
		key := strings.ToLower(filepath.ToSlash(filepath.Clean(entry.Name)))
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("%w: duplicate entry %q", ErrInvalidArchive, entry.Name)
		}
		seen[key] = struct{}{}
		if entry.Symlink {
			return nil, fmt.Errorf("%w: links are not allowed em %q", ErrInvalidArchive, entry.Name)
		}
		if entry.Encrypted {
			return nil, fmt.Errorf("%w: encrypted archives are not supported", ErrInvalidArchive)
		}
		if entry.Directory {
			continue
		}
		if entry.Size < 0 || entry.Size > limits.MaxArchiveBytes-total {
			return nil, fmt.Errorf("%w: uncompressed content exceeds the limit", ErrInvalidArchive)
		}
		total += entry.Size
		if isSupportedImage(entry.Name) {
			pages++
			if pages > limits.MaxPages {
				return nil, fmt.Errorf("%w: more than %d pages", ErrInvalidArchive, limits.MaxPages)
			}
			if entry.Size > limits.MaxPageBytes {
				return nil, fmt.Errorf("%w: page %q exceeds the limit", ErrInvalidArchive, entry.Name)
			}
		}
		if isSupportedImage(entry.Name) || strings.EqualFold(filepath.Base(entry.Name), "ComicInfo.xml") {
			selected = append(selected, entry)
		}
	}
	if pages == 0 {
		return nil, fmt.Errorf("%w: no JPEG, PNG, or WebP images found", ErrInvalidArchive)
	}
	return selected, nil
}

func validateArchiveExpansion(archivePath string, entries []listedArchiveEntry) error {
	info, err := os.Stat(archivePath)
	if err != nil {
		return err
	}
	var total int64
	for _, entry := range entries {
		if entry.Size > 0 {
			total += entry.Size
		}
	}
	if total >= 10*1024*1024 && info.Size() > 0 && total/info.Size() > 1000 {
		return fmt.Errorf("%w: suspicious overall compression ratio", ErrInvalidArchive)
	}
	return nil
}

func extractWithSevenZip(ctx context.Context, executable, archivePath, destination string, entries []listedArchiveEntry) error {
	listFile := filepath.Join(filepath.Dir(destination), "selected-files.txt")
	var names strings.Builder
	for _, entry := range entries {
		if strings.ContainsAny(entry.Name, "\r\n") {
			return fmt.Errorf("invalid entry name %q", entry.Name)
		}
		names.WriteString(entry.Name)
		names.WriteByte('\n')
	}
	if err := os.WriteFile(listFile, []byte(names.String()), 0o600); err != nil {
		return err
	}
	defer os.Remove(listFile)
	command := exec.CommandContext(ctx, executable,
		"x", "-y", "-bd", "-bb0", "-aoa", "-spd", "-sccUTF-8", "-scsUTF-8",
		"-o"+destination, archivePath, "-i@"+listFile,
	)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("extraction failed: %s", cleanToolError(stderr.Bytes()))
	}
	return nil
}

func extractWithUNAR(ctx context.Context, executable, archivePath, destination string) error {
	command := exec.CommandContext(ctx, executable, "-q", "-f", "-D", "-o", destination, archivePath)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("extraction failed: %s", cleanToolError(stderr.Bytes()))
	}
	return nil
}

func verifyExtractedSelection(destination string, selected []listedArchiveEntry, limits Limits) error {
	allowed := make(map[string]listedArchiveEntry, len(selected))
	for _, entry := range selected {
		allowed[strings.ToLower(filepath.ToSlash(filepath.Clean(entry.Name)))] = entry
	}
	var total int64
	found := map[string]bool{}
	err := filepath.WalkDir(destination, func(current string, item os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == destination {
			return nil
		}
		info, err := item.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: extracted links are not allowed", ErrInvalidArchive)
		}
		if item.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: special file type extracted", ErrInvalidArchive)
		}
		relative, err := filepath.Rel(destination, current)
		if err != nil {
			return err
		}
		key := strings.ToLower(filepath.ToSlash(filepath.Clean(relative)))
		entry, ok := allowed[key]
		if !ok {
			// unar may extract unrelated entries; remove them immediately.
			return os.Remove(current)
		}
		if info.Size() > limits.MaxArchiveBytes-total {
			return fmt.Errorf("%w: extracted content exceeds the limit", ErrInvalidArchive)
		}
		total += info.Size()
		if entry.Size > 0 && info.Size() != entry.Size {
			return fmt.Errorf("%w: unexpected size for %q", ErrInvalidArchive, entry.Name)
		}
		found[key] = true
		return nil
	})
	if err != nil {
		return err
	}
	for key, entry := range allowed {
		if !found[key] {
			return fmt.Errorf("%w: entry %q was not extracted", ErrInvalidArchive, entry.Name)
		}
	}
	return nil
}

func inspectPreparedDirectory(ctx context.Context, filename, format, sourceDir, temporaryRoot string, limits Limits) (inspectedBook, error) {
	var imagePaths []string
	var comicInfoPath string
	err := filepath.WalkDir(sourceDir, func(current string, item os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == sourceDir || item.IsDir() {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := item.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("%w: file type is not allowed", ErrInvalidArchive)
		}
		relative, err := filepath.Rel(sourceDir, current)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if isIgnoredArchiveEntry(relative) {
			return nil
		}
		if isSupportedImage(relative) {
			imagePaths = append(imagePaths, relative)
			return nil
		}
		if strings.EqualFold(filepath.Base(relative), "ComicInfo.xml") && (comicInfoPath == "" || strings.Count(relative, "/") < strings.Count(comicInfoPath, "/")) {
			comicInfoPath = relative
		}
		return nil
	})
	if err != nil {
		return inspectedBook{}, err
	}
	if len(imagePaths) == 0 {
		return inspectedBook{}, fmt.Errorf("%w: no JPEG, PNG, or WebP images found", ErrInvalidArchive)
	}
	if len(imagePaths) > limits.MaxPages {
		return inspectedBook{}, fmt.Errorf("%w: more than %d pages", ErrInvalidArchive, limits.MaxPages)
	}
	naturalSort(imagePaths)
	pagesDirectory := filepath.Join(temporaryRoot, "pages")
	if err := os.MkdirAll(pagesDirectory, 0o750); err != nil {
		return inspectedBook{}, err
	}
	pages := make([]inspectedPage, 0, len(imagePaths))
	for index, relative := range imagePaths {
		if err := ctx.Err(); err != nil {
			return inspectedBook{}, err
		}
		source, err := safeExtractionTarget(sourceDir, relative)
		if err != nil {
			return inspectedBook{}, err
		}
		width, height, mediaType, err := readImageConfigFile(source, limits.MaxPageBytes)
		if err != nil {
			return inspectedBook{}, fmt.Errorf("%w: page %q: %v", ErrInvalidArchive, relative, err)
		}
		info, err := os.Stat(source)
		if err != nil {
			return inspectedBook{}, err
		}
		extension := mediaTypeExtension(mediaType)
		generated := fmt.Sprintf("%06d%s", index, extension)
		target := filepath.Join(pagesDirectory, generated)
		if err := os.Rename(source, target); err != nil {
			if copyErr := copyRegularFile(source, target, info.Size()); copyErr != nil {
				return inspectedBook{}, copyErr
			}
			_ = os.Remove(source)
		}
		pages = append(pages, inspectedPage{
			ArchivePath: preparedPathPrefix + "pages/" + generated,
			MediaType:   mediaType, Width: width, Height: height, FileSize: info.Size(),
		})
	}
	metadata := bookMetadata{}
	if comicInfoPath != "" {
		metadataPath, err := safeExtractionTarget(sourceDir, comicInfoPath)
		if err != nil {
			return inspectedBook{}, err
		}
		metadata, err = readComicInfoFile(metadataPath)
		if err != nil {
			return inspectedBook{}, fmt.Errorf("%w: %v", ErrInvalidArchive, err)
		}
	}
	_ = os.RemoveAll(sourceDir)
	title := strings.TrimSpace(strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename)))
	if title == "" {
		title = "Untitled book"
	}
	if metadata.Title != "" {
		title = metadata.Title
	}
	return inspectedBook{
		Format: format, PageCount: len(pages), Title: title,
		Series: metadata.Series, Summary: metadata.Summary, Writer: metadata.Writer,
		Publisher: metadata.Publisher, PublicationYear: metadata.PublicationYear,
		Volume: metadata.Volume, Number: metadata.Number, Language: metadata.Language,
		ReadingDirection: metadata.ReadingDirection, Pages: pages,
	}, nil
}

func safeExtractionTarget(root, archiveName string) (string, error) {
	if err := validateArchivePath(filepath.ToSlash(archiveName)); err != nil {
		return "", err
	}
	rootAbsolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	target := filepath.Join(rootAbsolute, filepath.FromSlash(filepath.ToSlash(archiveName)))
	relative, err := filepath.Rel(rootAbsolute, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes the target directory: %q", archiveName)
	}
	return target, nil
}

func readImageConfigFile(filename string, maxBytes int64) (int, int, string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return 0, 0, "", err
	}
	defer file.Close()
	return readImageConfigReader(file, maxBytes)
}

func mediaTypeExtension(mediaType string) string {
	switch mediaType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ".bin"
	}
}

func copyRegularFile(source, destination string, expectedSize int64) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		_ = os.Remove(destination)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(destination)
		return closeErr
	}
	if expectedSize >= 0 && written != expectedSize {
		_ = os.Remove(destination)
		return errors.New("copied file size mismatch")
	}
	return nil
}

func cleanToolError(value []byte) string {
	message := strings.TrimSpace(string(value))
	if message == "" {
		return "unspecified error"
	}
	lines := strings.Fields(message)
	if len(lines) > 40 {
		lines = lines[:40]
	}
	return strings.Join(lines, " ")
}

func sortedEntryNames(entries []listedArchiveEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name)
	}
	sort.Strings(names)
	return names
}
