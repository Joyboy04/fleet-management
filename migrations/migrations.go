// Package migrations embeds the SQL schema so the API can apply it on startup.
package migrations

import "embed"

// FS holds every *.sql file in this directory, applied in lexical order.
//
//go:embed *.sql
var FS embed.FS
