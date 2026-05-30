package migrations

import "embed"

// FS contains the embedded SQLite migrations used by the application at startup.
//
//go:embed *.sql
var FS embed.FS
