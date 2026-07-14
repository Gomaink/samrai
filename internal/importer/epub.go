package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

const maxEPUBTextResourceBytes = 8 << 20

var (
	markupScriptPattern = regexp.MustCompile(`(?is)<(?:script|style|noscript)\b[^>]*>.*?</(?:script|style|noscript)\s*>`)
	markupTagPattern    = regexp.MustCompile(`(?s)<[^>]+>`)
	whitespacePattern   = regexp.MustCompile(`\s+`)
	yearPattern         = regexp.MustCompile(`\b(\d{4})\b`)
	viewportPattern     = regexp.MustCompile(`(?i)<meta\b[^>]*name\s*=\s*["']viewport["']`)
	imagePattern        = regexp.MustCompile(`(?i)<(img|image)\b`)
	encryptionPattern   = regexp.MustCompile(`(?i)Algorithm\s*=\s*["']([^"']+)["']`)
)

type epubContainer struct {
	Rootfiles []struct {
		FullPath string `xml:"full-path,attr"`
		Media    string `xml:"media-type,attr"`
	} `xml:"rootfiles>rootfile"`
}

type epubPackage struct {
	Version  string `xml:"version,attr"`
	Metadata struct {
		Titles       []string   `xml:"title"`
		Creators     []string   `xml:"creator"`
		Publishers   []string   `xml:"publisher"`
		Languages    []string   `xml:"language"`
		Descriptions []string   `xml:"description"`
		Dates        []string   `xml:"date"`
		Metas        []epubMeta `xml:"meta"`
	} `xml:"metadata"`
	Manifest struct {
		Items []epubManifestItem `xml:"item"`
	} `xml:"manifest"`
	Spine struct {
		TOC                      string             `xml:"toc,attr"`
		PageProgressionDirection string             `xml:"page-progression-direction,attr"`
		Items                    []epubSpineItemref `xml:"itemref"`
	} `xml:"spine"`
}

type epubMeta struct {
	Name     string `xml:"name,attr"`
	Content  string `xml:"content,attr"`
	Property string `xml:"property,attr"`
	Refines  string `xml:"refines,attr"`
	ID       string `xml:"id,attr"`
	Value    string `xml:",chardata"`
}

type epubManifestItem struct {
	ID         string `xml:"id,attr"`
	Href       string `xml:"href,attr"`
	MediaType  string `xml:"media-type,attr"`
	Properties string `xml:"properties,attr"`
}

type epubSpineItemref struct {
	IDRef      string `xml:"idref,attr"`
	Linear     string `xml:"linear,attr"`
	Properties string `xml:"properties,attr"`
}

type epubNCX struct {
	NavMap struct {
		Points []epubNCXPoint `xml:"navPoint"`
	} `xml:"navMap"`
}

type epubNCXPoint struct {
	NavLabel struct {
		Text string `xml:"text"`
	} `xml:"navLabel"`
	Content struct {
		Src string `xml:"src,attr"`
	} `xml:"content"`
	Points []epubNCXPoint `xml:"navPoint"`
}

type rawTOCLink struct {
	Label string
	Href  string
	Depth int
}

func inspectEPUB(ctx context.Context, filename, archivePath string, limits Limits) (inspectedBook, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return inspectedBook{}, fmt.Errorf("%w: %v", ErrInvalidEPUB, err)
	}
	defer reader.Close()

	if len(reader.File) == 0 {
		return inspectedBook{}, fmt.Errorf("%w: empty file", ErrInvalidEPUB)
	}
	if len(reader.File) > limits.MaxArchiveEntries {
		return inspectedBook{}, fmt.Errorf("%w: more than %d entries", ErrInvalidEPUB, limits.MaxArchiveEntries)
	}

	entries := make(map[string]*zip.File, len(reader.File))
	var totalUncompressed uint64
	for _, entry := range reader.File {
		if err := ctx.Err(); err != nil {
			return inspectedBook{}, err
		}
		if err := validateArchivePath(entry.Name); err != nil {
			return inspectedBook{}, fmt.Errorf("%w: %v", ErrInvalidEPUB, err)
		}
		if entry.FileInfo().IsDir() {
			continue
		}
		if entry.FileInfo().Mode()&os.ModeSymlink != 0 {
			return inspectedBook{}, fmt.Errorf("%w: symbolic links are not allowed", ErrInvalidEPUB)
		}
		if entry.UncompressedSize64 > uint64(limits.MaxArchiveBytes)-totalUncompressed {
			return inspectedBook{}, fmt.Errorf("%w: uncompressed content exceeds the limit", ErrInvalidEPUB)
		}
		totalUncompressed += entry.UncompressedSize64
		if suspiciousCompressionRatio(entry) {
			return inspectedBook{}, fmt.Errorf("%w: suspicious compression ratio in %q", ErrInvalidEPUB, entry.Name)
		}
		cleaned := path.Clean(entry.Name)
		if _, exists := entries[cleaned]; exists {
			return inspectedBook{}, fmt.Errorf("%w: duplicate entry %q", ErrInvalidEPUB, cleaned)
		}
		entries[cleaned] = entry
	}

	if mimetype, ok := entries["mimetype"]; ok {
		data, readErr := readZIPEntry(mimetype, 256)
		if readErr != nil || strings.TrimSpace(string(data)) != "application/epub+zip" {
			return inspectedBook{}, fmt.Errorf("%w: invalid mimetype", ErrInvalidEPUB)
		}
	}

	containerEntry := entries["META-INF/container.xml"]
	if containerEntry == nil {
		return inspectedBook{}, fmt.Errorf("%w: META-INF/container.xml ausente", ErrInvalidEPUB)
	}
	containerData, err := readZIPEntry(containerEntry, 1<<20)
	if err != nil {
		return inspectedBook{}, fmt.Errorf("%w: could not read container.xml", ErrInvalidEPUB)
	}
	var container epubContainer
	if err := xml.Unmarshal(containerData, &container); err != nil || len(container.Rootfiles) == 0 {
		return inspectedBook{}, fmt.Errorf("%w: invalid container.xml", ErrInvalidEPUB)
	}

	if encryption := entries["META-INF/encryption.xml"]; encryption != nil {
		data, readErr := readZIPEntry(encryption, 2<<20)
		if readErr != nil {
			return inspectedBook{}, fmt.Errorf("%w: invalid encryption.xml", ErrInvalidEPUB)
		}
		if unsupportedEPUBEncryption(data) {
			return inspectedBook{}, ErrEPUBDRM
		}
	}

	packagePath, err := normalizeEPUBArchivePath("", container.Rootfiles[0].FullPath)
	if err != nil {
		return inspectedBook{}, fmt.Errorf("%w: invalid package path", ErrInvalidEPUB)
	}
	packageEntry := entries[packagePath]
	if packageEntry == nil {
		return inspectedBook{}, fmt.Errorf("%w: OPF file not found", ErrInvalidEPUB)
	}
	packageData, err := readZIPEntry(packageEntry, 4<<20)
	if err != nil {
		return inspectedBook{}, fmt.Errorf("%w: could not read the OPF package", ErrInvalidEPUB)
	}
	var pkg epubPackage
	if err := xml.Unmarshal(packageData, &pkg); err != nil {
		return inspectedBook{}, fmt.Errorf("%w: invalid OPF package: %v", ErrInvalidEPUB, err)
	}
	if len(pkg.Manifest.Items) == 0 || len(pkg.Spine.Items) == 0 {
		return inspectedBook{}, fmt.Errorf("%w: manifest or reading order is missing", ErrInvalidEPUB)
	}
	if len(pkg.Spine.Items) > limits.MaxPages {
		return inspectedBook{}, fmt.Errorf("%w: more than %d items in the reading order", ErrInvalidEPUB, limits.MaxPages)
	}

	packageDir := path.Dir(packagePath)
	if packageDir == "." {
		packageDir = ""
	}
	resourceByID := make(map[string]inspectedEPUBResource, len(pkg.Manifest.Items))
	resources := make([]inspectedEPUBResource, 0, len(pkg.Manifest.Items))
	for _, item := range pkg.Manifest.Items {
		if err := ctx.Err(); err != nil {
			return inspectedBook{}, err
		}
		item.ID = strings.TrimSpace(item.ID)
		item.MediaType = strings.ToLower(strings.TrimSpace(item.MediaType))
		if item.ID == "" || strings.TrimSpace(item.Href) == "" || item.MediaType == "" {
			return inspectedBook{}, fmt.Errorf("%w: incomplete manifest item", ErrInvalidEPUB)
		}
		resolvedPath, err := normalizeEPUBArchivePath(packageDir, item.Href)
		if err != nil {
			return inspectedBook{}, fmt.Errorf("%w: resource %q has an invalid path", ErrInvalidEPUB, item.Href)
		}
		entry := entries[resolvedPath]
		if entry == nil {
			return inspectedBook{}, fmt.Errorf("%w: recurso ausente %q", ErrInvalidEPUB, resolvedPath)
		}
		resource := inspectedEPUBResource{
			Path:       resolvedPath,
			ItemID:     item.ID,
			MediaType:  item.MediaType,
			Properties: normalizeSpace(item.Properties),
			FileSize:   int64(entry.UncompressedSize64),
		}
		resourceByID[item.ID] = resource
		resources = append(resources, resource)
	}

	coverPath := resolveEPUBCoverPath(pkg, resourceByID)
	layout := detectEPUBLayout(pkg)
	spine := make([]inspectedEPUBSpine, 0, len(pkg.Spine.Items))
	spinePathIndex := make(map[string]int, len(pkg.Spine.Items))
	fixedHints := 0
	for _, itemref := range pkg.Spine.Items {
		if err := ctx.Err(); err != nil {
			return inspectedBook{}, err
		}
		resource, ok := resourceByID[strings.TrimSpace(itemref.IDRef)]
		if !ok {
			return inspectedBook{}, fmt.Errorf("%w: spine item %q does not exist in the manifest", ErrInvalidEPUB, itemref.IDRef)
		}
		if !isEPUBContentDocument(resource.MediaType) {
			return inspectedBook{}, fmt.Errorf("%w: spine item %q is not a content document", ErrInvalidEPUB, resource.Path)
		}
		entry := entries[resource.Path]
		data, readErr := readZIPEntry(entry, maxEPUBTextResourceBytes)
		if readErr != nil {
			return inspectedBook{}, fmt.Errorf("%w: chapter %q exceeds the limit", ErrInvalidEPUB, resource.Path)
		}
		text := extractEPUBText(data)
		properties := normalizeSpace(strings.TrimSpace(resource.Properties + " " + itemref.Properties))
		if hasProperty(properties, "rendition:layout-pre-paginated") || looksLikeFixedLayout(data, text) {
			fixedHints++
		}
		index := len(spine)
		spinePathIndex[resource.Path] = index
		spine = append(spine, inspectedEPUBSpine{
			Index:         index,
			ItemID:        itemref.IDRef,
			Path:          resource.Path,
			MediaType:     resource.MediaType,
			Properties:    properties,
			Linear:        !strings.EqualFold(strings.TrimSpace(itemref.Linear), "no"),
			TextContent:   text,
			SearchContent: normalizeEPUBSearchText(text),
		})
	}
	if layout == "auto" {
		if fixedHints > 0 && fixedHints*3 >= len(spine)*2 {
			layout = "fixed"
		} else {
			layout = "reflowable"
		}
	}

	tocLinks, err := extractEPUBTOC(entries, pkg, resourceByID)
	if err != nil {
		return inspectedBook{}, fmt.Errorf("%w: invalid table of contents: %v", ErrInvalidEPUB, err)
	}
	toc := make([]inspectedEPUBTOC, 0, len(tocLinks))
	spineTitles := make(map[string]string)
	for _, link := range tocLinks {
		resolvedPath, fragment, resolveErr := resolveRootedEPUBHref(link.Href)
		if resolveErr != nil {
			continue
		}
		spineIndex, found := spinePathIndex[resolvedPath]
		var spineIndexPointer *int
		if found {
			value := spineIndex
			spineIndexPointer = &value
			if spineTitles[resolvedPath] == "" {
				spineTitles[resolvedPath] = strings.TrimSpace(link.Label)
			}
		}
		toc = append(toc, inspectedEPUBTOC{
			Position: len(toc), Label: strings.TrimSpace(link.Label), Path: resolvedPath,
			Fragment: fragment, SpineIndex: spineIndexPointer, Depth: max(0, link.Depth),
		})
	}
	for index := range spine {
		spine[index].Title = spineTitles[spine[index].Path]
		if spine[index].Title == "" {
			spine[index].Title = fmt.Sprintf("Section %d", index+1)
		}
	}

	title := firstNonEmpty(pkg.Metadata.Titles...)
	if title == "" {
		title = strings.TrimSpace(strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename)))
	}
	if title == "" {
		title = "EPUB book"
	}
	writer := strings.Join(nonEmptyStrings(pkg.Metadata.Creators), ", ")
	publisher := firstNonEmpty(pkg.Metadata.Publishers...)
	language := firstNonEmpty(pkg.Metadata.Languages...)
	summary := firstNonEmpty(pkg.Metadata.Descriptions...)
	series, volume := epubSeriesMetadata(pkg.Metadata.Metas)
	publicationYear := epubPublicationYear(pkg.Metadata.Dates)
	readingDirection := (*string)(nil)
	switch strings.ToLower(strings.TrimSpace(pkg.Spine.PageProgressionDirection)) {
	case "rtl":
		value := "rtl"
		readingDirection = &value
	case "ltr":
		value := "ltr"
		readingDirection = &value
	}

	return inspectedBook{
		Format: "epub", PageCount: len(spine), Title: title, Series: series,
		Summary: summary, Writer: writer, Publisher: publisher, PublicationYear: publicationYear,
		Volume: volume, Language: language, ReadingDirection: readingDirection,
		EPUBLayout: layout, EPUBVersion: strings.TrimSpace(pkg.Version), EPUBPackagePath: packagePath,
		EPUBCoverPath: coverPath, EPUBResources: resources, EPUBSpine: spine, EPUBTOC: toc,
	}, nil
}

func readZIPEntry(entry *zip.File, maxBytes int64) ([]byte, error) {
	if entry == nil {
		return nil, errors.New("entry not found")
	}
	if entry.UncompressedSize64 > uint64(maxBytes) {
		return nil, errors.New("entry exceeds limit")
	}
	stream, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	limited := &io.LimitedReader{R: stream, N: maxBytes + 1}
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, errors.New("entry exceeds limit")
	}
	return data, nil
}

func unsupportedEPUBEncryption(data []byte) bool {
	matches := encryptionPattern.FindAllSubmatch(data, -1)
	for _, match := range matches {
		algorithm := strings.ToLower(strings.TrimSpace(string(match[1])))
		if strings.Contains(algorithm, "idpf.org/2008/embedding") || strings.Contains(algorithm, "adobe.com/pdf/enc#rc") {
			continue
		}
		return true
	}
	return false
}

func normalizeEPUBArchivePath(baseDir, href string) (string, error) {
	resolvedPath, _, err := resolveEPUBHref(baseDir, href)
	return resolvedPath, err
}

func resolveEPUBHref(baseDir, href string) (string, string, error) {
	href = strings.TrimSpace(href)
	if href == "" {
		return "", "", errors.New("empty href")
	}
	parsed, err := url.Parse(href)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" || strings.HasPrefix(href, "//") {
		return "", "", errors.New("external or invalid href")
	}
	decodedPath, err := url.PathUnescape(parsed.Path)
	if err != nil || decodedPath == "" {
		return "", "", errors.New("invalid escaped path")
	}
	decodedPath = strings.ReplaceAll(decodedPath, "\\", "/")
	joined := path.Clean(path.Join(baseDir, decodedPath))
	if err := validateArchivePath(joined); err != nil {
		return "", "", err
	}
	return joined, parsed.Fragment, nil
}

func resolveRootedEPUBHref(href string) (string, string, error) {
	href = strings.TrimSpace(href)
	if href == "" {
		return "", "", errors.New("empty href")
	}
	parsed, err := url.Parse(href)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" || strings.HasPrefix(href, "//") {
		return "", "", errors.New("external or invalid href")
	}
	decodedPath, err := url.PathUnescape(parsed.Path)
	if err != nil || decodedPath == "" {
		return "", "", errors.New("invalid escaped path")
	}
	decodedPath = strings.ReplaceAll(decodedPath, "\\", "/")
	cleaned := path.Clean(decodedPath)
	if err := validateArchivePath(cleaned); err != nil {
		return "", "", err
	}
	return cleaned, parsed.Fragment, nil
}

func resolveEPUBCoverPath(pkg epubPackage, resources map[string]inspectedEPUBResource) string {
	for _, resource := range resources {
		if hasProperty(resource.Properties, "cover-image") && strings.HasPrefix(resource.MediaType, "image/") {
			return resource.Path
		}
	}
	coverID := ""
	for _, meta := range pkg.Metadata.Metas {
		if strings.EqualFold(strings.TrimSpace(meta.Name), "cover") {
			coverID = strings.TrimSpace(meta.Content)
			break
		}
	}
	if resource, ok := resources[coverID]; ok && strings.HasPrefix(resource.MediaType, "image/") {
		return resource.Path
	}
	for id, resource := range resources {
		lower := strings.ToLower(id + " " + resource.Path)
		if strings.Contains(lower, "cover") && strings.HasPrefix(resource.MediaType, "image/") {
			return resource.Path
		}
	}
	return ""
}

func detectEPUBLayout(pkg epubPackage) string {
	for _, meta := range pkg.Metadata.Metas {
		property := strings.ToLower(strings.TrimSpace(meta.Property))
		name := strings.ToLower(strings.TrimSpace(meta.Name))
		if property != "rendition:layout" && name != "rendition:layout" {
			continue
		}
		value := strings.ToLower(firstNonEmpty(meta.Value, meta.Content))
		switch value {
		case "pre-paginated":
			return "fixed"
		case "reflowable":
			return "reflowable"
		}
	}
	return "auto"
}

func looksLikeFixedLayout(data []byte, text string) bool {
	return viewportPattern.Match(data) && imagePattern.Match(data) && len([]rune(strings.TrimSpace(text))) < 160
}

func extractEPUBText(data []byte) string {
	cleaned := markupScriptPattern.ReplaceAll(data, []byte(" "))
	cleaned = markupTagPattern.ReplaceAll(cleaned, []byte(" "))
	value := html.UnescapeString(string(cleaned))
	return normalizeSpace(value)
}

func normalizeEPUBSearchText(value string) string {
	value = strings.ToLower(value)
	value = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsSpace(r) {
			return r
		}
		return ' '
	}, value)
	return normalizeSpace(value)
}

func normalizeSpace(value string) string {
	return strings.TrimSpace(whitespacePattern.ReplaceAllString(value, " "))
}

func hasProperty(properties, expected string) bool {
	expected = strings.ToLower(expected)
	for _, property := range strings.Fields(strings.ToLower(properties)) {
		if property == expected {
			return true
		}
	}
	return false
}

func isEPUBContentDocument(mediaType string) bool {
	switch strings.ToLower(mediaType) {
	case "application/xhtml+xml", "image/svg+xml", "text/html":
		return true
	default:
		return false
	}
}

func extractEPUBTOC(entries map[string]*zip.File, pkg epubPackage, resources map[string]inspectedEPUBResource) ([]rawTOCLink, error) {
	for _, resource := range resources {
		if !hasProperty(resource.Properties, "nav") {
			continue
		}
		data, err := readZIPEntry(entries[resource.Path], maxEPUBTextResourceBytes)
		if err != nil {
			return nil, err
		}
		links, err := parseEPUBNav(data)
		if err != nil {
			return nil, err
		}
		return rebaseTOCLinks(path.Dir(resource.Path), links), nil
	}

	ncxID := strings.TrimSpace(pkg.Spine.TOC)
	if ncxID == "" {
		for id, resource := range resources {
			if resource.MediaType == "application/x-dtbncx+xml" {
				ncxID = id
				break
			}
		}
	}
	resource, ok := resources[ncxID]
	if !ok {
		return nil, nil
	}
	data, err := readZIPEntry(entries[resource.Path], maxEPUBTextResourceBytes)
	if err != nil {
		return nil, err
	}
	var ncx epubNCX
	if err := xml.Unmarshal(data, &ncx); err != nil {
		return nil, err
	}
	links := make([]rawTOCLink, 0)
	var walk func([]epubNCXPoint, int)
	walk = func(points []epubNCXPoint, depth int) {
		for _, point := range points {
			links = append(links, rawTOCLink{Label: normalizeSpace(point.NavLabel.Text), Href: point.Content.Src, Depth: depth})
			walk(point.Points, depth+1)
		}
	}
	walk(ncx.NavMap.Points, 0)
	return rebaseTOCLinks(path.Dir(resource.Path), links), nil
}

func parseEPUBNav(data []byte) ([]rawTOCLink, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	links := make([]rawTOCLink, 0)
	fallback := make([]rawTOCLink, 0)
	inNav := false
	tocNav := false
	navDepth := 0
	listDepth := 0
	inAnchor := false
	anchorDepth := 0
	anchorHref := ""
	var anchorText strings.Builder

	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch value := token.(type) {
		case xml.StartElement:
			local := strings.ToLower(value.Name.Local)
			if local == "nav" && !inNav {
				inNav = true
				navDepth = 1
				tocNav = false
				for _, attr := range value.Attr {
					if strings.EqualFold(attr.Name.Local, "type") && strings.Contains(strings.ToLower(attr.Value), "toc") {
						tocNav = true
					}
				}
				continue
			}
			if inNav {
				navDepth++
				if local == "ol" || local == "ul" {
					listDepth++
				}
				if local == "a" && !inAnchor {
					inAnchor = true
					anchorDepth = listDepth
					anchorHref = ""
					anchorText.Reset()
					for _, attr := range value.Attr {
						if strings.EqualFold(attr.Name.Local, "href") {
							anchorHref = attr.Value
						}
					}
				}
			}
		case xml.CharData:
			if inAnchor {
				anchorText.Write(value)
				anchorText.WriteByte(' ')
			}
		case xml.EndElement:
			local := strings.ToLower(value.Name.Local)
			if inNav && local == "a" && inAnchor {
				link := rawTOCLink{Label: normalizeSpace(anchorText.String()), Href: anchorHref, Depth: max(0, anchorDepth-1)}
				if link.Label != "" && link.Href != "" {
					fallback = append(fallback, link)
					if tocNav {
						links = append(links, link)
					}
				}
				inAnchor = false
			}
			if inNav && (local == "ol" || local == "ul") && listDepth > 0 {
				listDepth--
			}
			if inNav {
				navDepth--
				if local == "nav" || navDepth <= 0 {
					inNav = false
					navDepth = 0
					listDepth = 0
				}
			}
		}
	}
	if len(links) > 0 {
		return links, nil
	}
	return fallback, nil
}

func rebaseTOCLinks(baseDir string, links []rawTOCLink) []rawTOCLink {
	for index := range links {
		parsed, err := url.Parse(strings.TrimSpace(links[index].Href))
		if err != nil || parsed.Scheme != "" || parsed.Host != "" || parsed.Path == "" {
			continue
		}
		decoded, err := url.PathUnescape(parsed.Path)
		if err != nil {
			continue
		}
		parsed.Path = path.Clean(path.Join(baseDir, decoded))
		links[index].Href = parsed.String()
	}
	return links
}

func epubSeriesMetadata(metas []epubMeta) (string, string) {
	series := ""
	volume := ""
	for _, meta := range metas {
		property := strings.ToLower(strings.TrimSpace(meta.Property))
		name := strings.ToLower(strings.TrimSpace(meta.Name))
		value := normalizeSpace(firstNonEmpty(meta.Value, meta.Content))
		switch {
		case name == "calibre:series":
			series = value
		case name == "calibre:series_index":
			volume = value
		case property == "belongs-to-collection" && value != "":
			if series == "" {
				series = value
			}
		case property == "group-position" && value != "":
			volume = value
		}
	}
	return series, volume
}

func epubPublicationYear(dates []string) *int {
	for _, date := range dates {
		match := yearPattern.FindStringSubmatch(date)
		if len(match) != 2 {
			continue
		}
		year, err := strconv.Atoi(match[1])
		if err == nil && year >= 0 && year <= 9999 {
			return &year
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := normalizeSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func nonEmptyStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := normalizeSpace(value); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
