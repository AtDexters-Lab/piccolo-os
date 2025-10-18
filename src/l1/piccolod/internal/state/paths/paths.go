package paths

import (
	"os"
	"path/filepath"
	"sync"
)

const defaultRoot = "/var/lib/piccolod"

var (
	root string
	once sync.Once
)

func resolveRoot() {
	candidate := os.Getenv("PICCOLO_STATE_DIR")
	if candidate == "" {
		candidate = defaultRoot
	}
	root = filepath.Clean(candidate)
}

// Root returns the base directory where the bootstrap volume is (or will be) mounted.
func Root() string {
	once.Do(resolveRoot)
	return root
}

// Join resolves a path relative to the state root.
func Join(elements ...string) string {
	all := append([]string{Root()}, elements...)
	return filepath.Join(all...)
}

func CryptoDir() string    { return Join("crypto") }
func ControlDir() string   { return Join("control") }
func ExportsDir() string   { return Join("exports") }
func BootstrapDir() string { return Join("bootstrap") }
func VolumesDir() string   { return Join("volumes") }

// SetRootForTest resets the cached root so tests can override PICCOLO_STATE_DIR.
func SetRootForTest(dir string) {
	if dir != "" {
		os.Setenv("PICCOLO_STATE_DIR", dir)
	}
	root = ""
	once = sync.Once{}
}
