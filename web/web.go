// Package web embeds the single-page dashboard served by the API at "/".
package web

import _ "embed"

// Index is the dashboard page.
//
//go:embed index.html
var Index []byte
