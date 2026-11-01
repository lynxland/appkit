// Package migrations embeds the outbox table migration (goose format).
package migrations

import (
	"embed"
	"io/fs"
)

// FS is the migration filesystem (root = migration files themselves).
//
//go:embed *.sql
var files embed.FS

// FS returns the migration FS with root pointing at the files.
func FS() fs.FS { return files }
