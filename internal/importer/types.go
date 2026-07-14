package importer

import (
	"errors"
	"time"
)

var (
	ErrInvalidExtension       = errors.New("only .cbz, .zip, .cbr, .rar, .cb7, .7z, .cbt, .tar, .pdf, or .epub files are accepted")
	ErrUploadTooLarge         = errors.New("the file exceeds the upload limit")
	ErrInvalidArchive         = errors.New("the archive is not a valid comic book")
	ErrInvalidPDF             = errors.New("the file is not a valid PDF")
	ErrInvalidEPUB            = errors.New("the file is not a valid EPUB")
	ErrEPUBDRM                = errors.New("DRM-protected EPUB files are not supported")
	ErrArchiveToolUnavailable = errors.New("CBR and CB7 files require 7-Zip or unar on the server")
	ErrDuplicateBook          = errors.New("this file already exists in the library")
)

type Limits struct {
	MaxUploadBytes    int64
	MaxArchiveBytes   int64
	MaxPageBytes      int64
	MaxArchiveEntries int
	MaxPages          int
	SevenZipPath      string
	LSARPath          string
	UNARPath          string
}

type Job struct {
	ID               int64      `json:"id"`
	Type             string     `json:"type"`
	Status           string     `json:"status"`
	Progress         int        `json:"progress"`
	OriginalFilename string     `json:"original_filename"`
	ErrorMessage     string     `json:"error_message,omitempty"`
	BookID           *int64     `json:"book_id,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
}

// UploadMetadata contains optional user-reviewed metadata supplied with an
// upload. Empty values keep the metadata discovered inside the source file.
type UploadMetadata struct {
	Title  string `json:"title,omitempty"`
	Series string `json:"series,omitempty"`
	Volume string `json:"volume,omitempty"`
}

type uploadPayload struct {
	UploadPath       string         `json:"upload_path"`
	OriginalFilename string         `json:"original_filename"`
	FileSize         int64          `json:"file_size"`
	FileHash         string         `json:"file_hash"`
	Metadata         UploadMetadata `json:"metadata,omitempty"`
	BookID           *int64         `json:"book_id,omitempty"`
}

type inspectedPage struct {
	ArchivePath string
	MediaType   string
	Width       int
	Height      int
	FileSize    int64
}

type inspectedEPUBResource struct {
	Path       string
	ItemID     string
	MediaType  string
	Properties string
	FileSize   int64
}

type inspectedEPUBSpine struct {
	Index         int
	ItemID        string
	Path          string
	MediaType     string
	Properties    string
	Linear        bool
	Title         string
	TextContent   string
	SearchContent string
}

type inspectedEPUBTOC struct {
	Position   int
	Label      string
	Path       string
	Fragment   string
	SpineIndex *int
	Depth      int
}

type inspectedBook struct {
	Format            string
	PDFAnalysisStatus string
	PageCount         int
	Title             string
	Series            string
	Summary           string
	Writer            string
	Publisher         string
	PublicationYear   *int
	Volume            string
	Number            string
	Language          string
	ReadingDirection  *string
	Pages             []inspectedPage
	EPUBLayout        string
	EPUBVersion       string
	EPUBPackagePath   string
	EPUBCoverPath     string
	EPUBResources     []inspectedEPUBResource
	EPUBSpine         []inspectedEPUBSpine
	EPUBTOC           []inspectedEPUBTOC
	PreparedDir       string
}
