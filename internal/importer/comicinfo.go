package importer

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"

	"archive/zip"
)

const maxComicInfoBytes = 2 * 1024 * 1024

type comicInfoXML struct {
	Title       string `xml:"Title"`
	Series      string `xml:"Series"`
	Summary     string `xml:"Summary"`
	Writer      string `xml:"Writer"`
	Publisher   string `xml:"Publisher"`
	Year        string `xml:"Year"`
	Volume      string `xml:"Volume"`
	Number      string `xml:"Number"`
	LanguageISO string `xml:"LanguageISO"`
	Manga       string `xml:"Manga"`
}

type bookMetadata struct {
	Title            string
	Series           string
	Summary          string
	Writer           string
	Publisher        string
	PublicationYear  *int
	Volume           string
	Number           string
	Language         string
	ReadingDirection *string
}

func findComicInfo(files []*zip.File) *zip.File {
	var selected *zip.File
	for _, entry := range files {
		if entry.FileInfo().IsDir() || !strings.EqualFold(path.Base(entry.Name), "ComicInfo.xml") {
			continue
		}
		if selected == nil || strings.Count(entry.Name, "/") < strings.Count(selected.Name, "/") {
			selected = entry
		}
	}
	return selected
}

func readComicInfo(entry *zip.File) (bookMetadata, error) {
	if entry == nil {
		return bookMetadata{}, nil
	}
	if entry.UncompressedSize64 > maxComicInfoBytes {
		return bookMetadata{}, errors.New("ComicInfo.xml exceeds 2 MB")
	}
	stream, err := entry.Open()
	if err != nil {
		return bookMetadata{}, fmt.Errorf("open ComicInfo.xml: %w", err)
	}
	defer stream.Close()
	return readComicInfoReader(stream)
}

func readComicInfoFile(filename string) (bookMetadata, error) {
	info, err := os.Stat(filename)
	if err != nil {
		return bookMetadata{}, fmt.Errorf("open ComicInfo.xml: %w", err)
	}
	if info.Size() > maxComicInfoBytes {
		return bookMetadata{}, errors.New("ComicInfo.xml exceeds 2 MB")
	}
	stream, err := os.Open(filename)
	if err != nil {
		return bookMetadata{}, fmt.Errorf("open ComicInfo.xml: %w", err)
	}
	defer stream.Close()
	return readComicInfoReader(stream)
}

func readComicInfoReader(stream io.Reader) (bookMetadata, error) {
	decoder := xml.NewDecoder(io.LimitReader(stream, maxComicInfoBytes+1))
	decoder.Strict = true
	var document comicInfoXML
	if err := decoder.Decode(&document); err != nil {
		return bookMetadata{}, fmt.Errorf("invalid ComicInfo.xml: %w", err)
	}

	metadata := bookMetadata{
		Title:     cleanMetadata(document.Title, 240),
		Series:    cleanMetadata(document.Series, 240),
		Summary:   cleanMetadata(document.Summary, 20_000),
		Writer:    cleanMetadata(document.Writer, 1000),
		Publisher: cleanMetadata(document.Publisher, 500),
		Volume:    cleanMetadata(document.Volume, 100),
		Number:    cleanMetadata(document.Number, 100),
		Language:  cleanMetadata(document.LanguageISO, 32),
	}
	if year, err := strconv.Atoi(strings.TrimSpace(document.Year)); err == nil && year > 0 && year <= 9999 {
		metadata.PublicationYear = &year
	}
	manga := strings.ToLower(strings.TrimSpace(document.Manga))
	if strings.Contains(manga, "righttoleft") || strings.Contains(manga, "right-to-left") || manga == "rtl" {
		direction := "rtl"
		metadata.ReadingDirection = &direction
	}
	return metadata, nil
}

func cleanMetadata(value string, maximum int) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\x00", ""))
	runes := []rune(value)
	if len(runes) > maximum {
		value = string(runes[:maximum])
	}
	return value
}
