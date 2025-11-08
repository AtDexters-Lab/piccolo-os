package webassets

import (
	"embed"
)

// Embedded UI assets. The directory name includes all files recursively.
//
//go:embed web web/_app
var FS embed.FS
