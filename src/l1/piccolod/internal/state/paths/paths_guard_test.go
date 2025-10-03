package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNoHardcodedStateDir(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("unable to determine caller path")
	}
	// Walk all Go files under src/l1/piccolod
	root := filepath.Join(filepath.Dir(file), "..", "..", "..")
	allowed := map[string]struct{}{
		filepath.Clean(filepath.Join(root, "internal", "state", "paths", "paths.go")): {},
	}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if _, ok := allowed[filepath.Clean(path)]; ok {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), defaultRoot) {
			t.Fatalf("hard-coded state dir %q found in %s", defaultRoot, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk failed: %v", err)
	}
}

// TODO(persistence): Extend this guard (or add a companion test) to ensure
// storage-bearing modules do not call os.WriteFile/os.OpenFile directly once
// remote manager and others migrate to the persistence volume APIs.
