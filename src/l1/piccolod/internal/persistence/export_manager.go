package persistence

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"piccolod/internal/state/paths"
)

type fileExportManager struct {
	root string
}

func newFileExportManager(root string) *fileExportManager {
	if root == "" {
		root = paths.Root()
	}
	return &fileExportManager{root: root}
}

type exportPayload struct {
	Kind        ExportKind `json:"kind"`
	GeneratedAt time.Time  `json:"generated_at"`
	Sha256      string     `json:"sha256"`
	Blob        string     `json:"blob_b64"`
}

func (m *fileExportManager) RunControlPlane(ctx context.Context) (ExportArtifact, error) {
	return m.writeExport(ctx, ExportKindControlOnly, filepath.Join(m.root, "ciphertext", "control", "control.enc"), filepath.Join(m.root, "exports", "control", "control-plane.pcv"))
}

func (m *fileExportManager) RunFullData(ctx context.Context) (ExportArtifact, error) {
	// Until application volumes are implemented, reuse the control snapshot as a placeholder payload.
	return m.writeExport(ctx, ExportKindFullData, filepath.Join(m.root, "ciphertext", "control", "control.enc"), filepath.Join(m.root, "exports", "full", "full-data.pcv"))
}

func (m *fileExportManager) ImportControlPlane(ctx context.Context, artifact ExportArtifact, opts ImportOptions) error {
	return ErrNotImplemented
}

func (m *fileExportManager) ImportFullData(ctx context.Context, artifact ExportArtifact, opts ImportOptions) error {
	return ErrNotImplemented
}

// TODO(streaming): replace the temp-file approach with a streaming writer so large
// exports can be piped directly to the caller or remote sink without relying on
// local free space under the bootstrap mount.
func (m *fileExportManager) writeExport(ctx context.Context, kind ExportKind, source, dest string) (ExportArtifact, error) {
	if err := ctx.Err(); err != nil {
		return ExportArtifact{}, err
	}
	data, err := os.ReadFile(source)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ExportArtifact{}, fmt.Errorf("persistence: source %s missing: %w", source, err)
		}
		return ExportArtifact{}, err
	}
	payload := exportPayload{
		Kind:        kind,
		GeneratedAt: time.Now().UTC().Round(time.Second),
		Sha256:      fmt.Sprintf("%x", sha256.Sum256(data)),
		Blob:        base64.StdEncoding.EncodeToString(data),
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return ExportArtifact{}, err
	}
	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return ExportArtifact{}, err
	}
	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(&payload); err != nil {
		f.Close()
		os.Remove(tmp)
		return ExportArtifact{}, err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return ExportArtifact{}, err
	}
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return ExportArtifact{}, err
	}
	return ExportArtifact{Path: dest, Kind: kind}, nil
}

var _ ExportManager = (*fileExportManager)(nil)
