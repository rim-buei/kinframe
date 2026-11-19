// Package migrations embeds goose SQL files for startup migration.
package migrations

import "embed"

// FS holds migration SQL files at the package root.
//
//go:embed *.sql
var FS embed.FS
