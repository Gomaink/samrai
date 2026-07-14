package migrations

import "embed"

// FS contains every versioned SQL migration bundled into the server binary.
//
//go:embed *.sql
var FS embed.FS
