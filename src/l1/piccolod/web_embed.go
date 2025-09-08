package webassets

import (
    "embed"
    "io/fs"
)

// Embedded UI assets. The directory name includes all files recursively.
//go:embed web
var content embed.FS

// FS returns an fs.FS rooted at the embedded web directory.
func FS() fs.FS {
    sub, err := fs.Sub(content, "web")
    if err != nil {
        // Should never happen as long as the path exists at build time.
        return content
    }
    return sub
}

