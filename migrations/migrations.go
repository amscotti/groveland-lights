// Package migrations embeds the SQL migration files so the server binary is
// self-contained. go:embed cannot reach parent directories, so the embed
// directive lives inside this directory; sqlc reads only the .sql files here
// and ignores this Go file.
package migrations

import "embed"

// Files holds every migration, keyed by filename. An embed.FS always lists
// entries in lexical (sorted) order, which is the application order: 0001
// before 0002 before 0003, forever. Files are append-only; a merged file must
// never be edited.
//
//go:embed *.sql
var Files embed.FS
