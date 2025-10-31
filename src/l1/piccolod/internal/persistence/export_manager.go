package persistence

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

func (m *fileExportManager) RunControlPlane(ctx context.Context) (ExportArtifact, error) {
	return m.streamExport(ctx, ExportKindControlOnly, []string{"control"}, filepath.Join(m.root, "exports", "control", "control-plane.pcv"))
}

func (m *fileExportManager) RunFullData(ctx context.Context) (ExportArtifact, error) {
	return m.streamExport(ctx, ExportKindFullData, []string{"control", "bootstrap"}, filepath.Join(m.root, "exports", "full", "full-data.pcv"))
}

func (m *fileExportManager) ImportControlPlane(ctx context.Context, artifact ExportArtifact, opts ImportOptions) error {
	return ErrNotImplemented
}

func (m *fileExportManager) ImportFullData(ctx context.Context, artifact ExportArtifact, opts ImportOptions) error {
	return ErrNotImplemented
}

func (m *fileExportManager) streamExport(ctx context.Context, kind ExportKind, volumes []string, dest string) (ExportArtifact, error) {
	if err := ctx.Err(); err != nil {
		return ExportArtifact{}, err
	}
	if len(volumes) == 0 {
		return ExportArtifact{}, fmt.Errorf("persistence: no volumes requested")
	}

	modTime := time.Now().UTC().Round(time.Second)

	tarFile, err := os.CreateTemp(m.root, "piccolo-export-*.tar")
	if err != nil {
		return ExportArtifact{}, err
	}
	defer func() {
		tarFile.Close()
		os.Remove(tarFile.Name())
	}()

	hasher := sha256.New()
	tw := tar.NewWriter(io.MultiWriter(tarFile, hasher))

	var entries int
	for _, volumeID := range volumes {
		if err := ctx.Err(); err != nil {
			tw.Close()
			return ExportArtifact{}, err
		}
		n, err := m.writeVolumeToTar(ctx, tw, volumeID, modTime)
		if err != nil {
			tw.Close()
			return ExportArtifact{}, err
		}
		entries += n
	}

	if entries == 0 {
		tw.Close()
		return ExportArtifact{}, fmt.Errorf("persistence: export contains no files: %w", fs.ErrNotExist)
	}

	if err := tw.Close(); err != nil {
		return ExportArtifact{}, err
	}
	if err := tarFile.Sync(); err != nil {
		return ExportArtifact{}, err
	}

	sum := hasher.Sum(nil)

	if _, err := tarFile.Seek(0, io.SeekStart); err != nil {
		return ExportArtifact{}, err
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return ExportArtifact{}, err
	}

	tmpPath := dest + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return ExportArtifact{}, err
	}

	success := false
	defer func() {
		if !success {
			out.Close()
			os.Remove(tmpPath)
		}
	}()

	if _, err := fmt.Fprintf(out, "{\n  \"kind\": \"%s\",\n  \"generated_at\": \"%s\",\n  \"sha256\": \"%x\",\n  \"blob_b64\": \"",
		kind, modTime.Format(time.RFC3339), sum); err != nil {
		return ExportArtifact{}, err
	}

	encoder := base64.NewEncoder(base64.StdEncoding, out)
	if _, err := io.Copy(encoder, tarFile); err != nil {
		encoder.Close()
		return ExportArtifact{}, err
	}
	if err := encoder.Close(); err != nil {
		return ExportArtifact{}, err
	}

	if _, err := io.WriteString(out, "\"\n}\n"); err != nil {
		return ExportArtifact{}, err
	}

	if err := out.Sync(); err != nil {
		return ExportArtifact{}, err
	}
	if err := out.Close(); err != nil {
		return ExportArtifact{}, err
	}

	if err := os.Rename(tmpPath, dest); err != nil {
		return ExportArtifact{}, err
	}

	success = true
	return ExportArtifact{Path: dest, Kind: kind}, nil
}

var _ ExportManager = (*fileExportManager)(nil)

func (m *fileExportManager) writeVolumeToTar(ctx context.Context, tw *tar.Writer, volumeID string, modTime time.Time) (int, error) {
	if volumeID == "" {
		return 0, fmt.Errorf("persistence: volume id required")
	}
	base := filepath.Join(m.root, "ciphertext", volumeID)
	info, err := os.Stat(base)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, fmt.Errorf("persistence: volume %s ciphertext missing: %w", volumeID, err)
		}
		return 0, err
	}
	if !info.IsDir() {
		return 0, fmt.Errorf("persistence: volume %s ciphertext invalid", volumeID)
	}

	var entries int

	err = filepath.WalkDir(base, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(filepath.Join(volumeID, rel))

		if d.IsDir() {
			dirName := name
			if rel == "." {
				dirName = volumeID
			}
			hdr := &tar.Header{
				Name:     ensureTarDirName(dirName),
				Typeflag: tar.TypeDir,
				Mode:     0o700,
				ModTime:  modTime,
			}
			return tw.WriteHeader(hdr)
		}

		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("persistence: refusing to export symlink %s", path)
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = name
		hdr.ModTime = modTime
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		if _, err := io.Copy(tw, file); err != nil {
			file.Close()
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}

		entries++
		return nil
	})
	if err != nil {
		return 0, err
	}

	return entries, nil
}

func ensureTarDirName(name string) string {
	if name == "" {
		return "./"
	}
	if !strings.HasSuffix(name, "/") {
		return name + "/"
	}
	return name
}
