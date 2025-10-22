package persistence

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFileExportManager_RunControlPlane(t *testing.T) {
	root := t.TempDir()
	controlDir := filepath.Join(root, "ciphertext", "control")
	if err := os.MkdirAll(controlDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	sample := []byte("encrypted-control-data")
	if err := os.WriteFile(filepath.Join(controlDir, "control.enc"), sample, 0o600); err != nil {
		t.Fatalf("write control: %v", err)
	}

	mgr := newFileExportManager(root)
	art, err := mgr.RunControlPlane(context.Background())
	if err != nil {
		t.Fatalf("RunControlPlane: %v", err)
	}
	if art.Kind != ExportKindControlOnly {
		t.Fatalf("expected control export kind, got %s", art.Kind)
	}
	if _, err := os.Stat(art.Path); err != nil {
		t.Fatalf("export file missing: %v", err)
	}

	var payload exportPayload
	data, err := os.ReadFile(art.Path)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Kind != ExportKindControlOnly {
		t.Fatalf("payload kind mismatch: %s", payload.Kind)
	}
	decoded, err := base64.StdEncoding.DecodeString(payload.Blob)
	if err != nil {
		t.Fatalf("decode blob: %v", err)
	}
	if string(decoded) != string(sample) {
		t.Fatalf("payload mismatch: %q", decoded)
	}
}

func TestFileExportManager_RunFullData(t *testing.T) {
	root := t.TempDir()
	controlDir := filepath.Join(root, "ciphertext", "control")
	if err := os.MkdirAll(controlDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	sample := []byte("control-snapshot")
	if err := os.WriteFile(filepath.Join(controlDir, "control.enc"), sample, 0o600); err != nil {
		t.Fatalf("write control: %v", err)
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
}
