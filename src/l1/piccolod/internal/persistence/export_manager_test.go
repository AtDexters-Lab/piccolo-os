package persistence

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type exportPayload struct {
	Kind        ExportKind `json:"kind"`
	GeneratedAt time.Time  `json:"generated_at"`
	Sha256      string     `json:"sha256"`
	Blob        string     `json:"blob_b64"`
}

func TestFileExportManager_RunControlPlane(t *testing.T) {
	root := t.TempDir()
	cipherDir := filepath.Join(root, "ciphertext", "control")
	if err := os.MkdirAll(cipherDir, 0o700); err != nil {
		t.Fatalf("mkdir control ciphertext: %v", err)
	}
	sample := []byte("sqlite-control-db")
	if err := os.WriteFile(filepath.Join(cipherDir, "control.db"), sample, 0o600); err != nil {
		t.Fatalf("write db: %v", err)
	}

	mgr := newFileExportManager(root)
	art, err := mgr.RunControlPlane(context.Background())
	if err != nil {
		t.Fatalf("RunControlPlane: %v", err)
	}
	if art.Kind != ExportKindControlOnly {
		t.Fatalf("expected control export kind, got %s", art.Kind)
	}
	payload := readPayload(t, art.Path)
	if payload.Kind != ExportKindControlOnly {
		t.Fatalf("payload kind mismatch: %s", payload.Kind)
	}
	files := untarPayload(t, payload.Blob)
	if data, ok := files["control/control.db"]; !ok || string(data) != string(sample) {
		t.Fatalf("tar missing control/control.db or mismatch")
	}
}

func TestFileExportManager_RunFullData(t *testing.T) {
	root := t.TempDir()
	controlCipher := filepath.Join(root, "ciphertext", "control")
	if err := os.MkdirAll(controlCipher, 0o700); err != nil {
		t.Fatalf("mkdir control ciphertext: %v", err)
	}
	if err := os.WriteFile(filepath.Join(controlCipher, "control.db"), []byte("db"), 0o600); err != nil {
		t.Fatalf("write db: %v", err)
	}
	bootstrapCipher := filepath.Join(root, "ciphertext", "bootstrap", "remote")
	if err := os.MkdirAll(bootstrapCipher, 0o700); err != nil {
		t.Fatalf("mkdir bootstrap ciphertext: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bootstrapCipher, "config.json"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("write bootstrap config: %v", err)
	}

	mgr := newFileExportManager(root)
	art, err := mgr.RunFullData(context.Background())
	if err != nil {
		t.Fatalf("RunFullData: %v", err)
	}
	if art.Kind != ExportKindFullData {
		t.Fatalf("expected full export kind, got %s", art.Kind)
	}
	if _, err := os.Stat(art.Path); err != nil {
		t.Fatalf("full export missing: %v", err)
	}
	payload := readPayload(t, art.Path)
	files := untarPayload(t, payload.Blob)
	if _, ok := files["control/control.db"]; !ok {
		t.Fatalf("full export missing control db")
	}
	if _, ok := files["bootstrap/remote/config.json"]; !ok {
		t.Fatalf("full export missing bootstrap config")
	}
}

func TestFileExportManagerLegacyFallback(t *testing.T) {
	root := t.TempDir()
	controlDir := filepath.Join(root, "ciphertext", "control")
	if err := os.MkdirAll(controlDir, 0o700); err != nil {
		t.Fatalf("mkdir control: %v", err)
	}
	sample := []byte("legacy-control")
	if err := os.WriteFile(filepath.Join(controlDir, "control.enc"), sample, 0o600); err != nil {
		t.Fatalf("write control.enc: %v", err)
	}

	mgr := newFileExportManager(root)
	art, err := mgr.RunControlPlane(context.Background())
	if err != nil {
		t.Fatalf("RunControlPlane: %v", err)
	}
	payload := readPayload(t, art.Path)
	files := untarPayload(t, payload.Blob)
	if data, ok := files["control/control.enc"]; !ok || string(data) != string(sample) {
		t.Fatalf("expected legacy control/control.enc in tar")
	}
}

func readPayload(t *testing.T, path string) exportPayload {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	var payload struct {
		Kind        ExportKind `json:"kind"`
		GeneratedAt time.Time  `json:"generated_at"`
		Sha256      string     `json:"sha256"`
		Blob        string     `json:"blob_b64"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return payload
}

func untarPayload(t *testing.T, blob string) map[string][]byte {
	t.Helper()
	decoded, err := base64.StdEncoding.DecodeString(blob)
	if err != nil {
		t.Fatalf("decode blob: %v", err)
	}
	r := tar.NewReader(bytes.NewReader(decoded))
	files := make(map[string][]byte)
	for {
		hdr, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("read tar: %v", err)
		}
		data, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("read tar entry: %v", err)
		}
		files[hdr.Name] = data
	}
	return files
}
