package site

import "embed"

// FS holds the dashboard template and stylesheet.
//
//go:embed templates/index.html static/style.css
var FS embed.FS
