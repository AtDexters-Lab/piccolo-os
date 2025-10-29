package persistence

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"piccolod/internal/crypt"
)

func TestNewServiceFailsWhenBootstrapAttachFails(t *testing.T) {
	tempDir := t.TempDir()

	cryptoMgr, err := crypt.NewManager(tempDir)
	if err != nil {
		t.Fatalf("crypto manager init: %v", err)
	}
	if !cryptoMgr.IsInitialized() {
		if err := cryptoMgr.Setup("test-pass"); err != nil {
			t.Fatalf("crypto setup: %v", err)
		}
	}
	if err := cryptoMgr.Unlock("test-pass"); err != nil {
		t.Fatalf("crypto unlock: %v", err)
	}

	handles := make(map[string]VolumeHandle)
	attachErr := errors.New("mount failure")
	volumes := &stubVolumeManager{}
	volumes.onEnsure = func(_ context.Context, req VolumeRequest) (VolumeHandle, error) {
		if handle, ok := handles[req.ID]; ok {
			return handle, nil
		}
		handle := VolumeHandle{ID: req.ID, MountDir: filepath.Join(tempDir, "mounts", req.ID)}
		handles[req.ID] = handle
		return handle, nil
	}
	volumes.onAttach = func(_ context.Context, handle VolumeHandle, _ AttachOptions) error {
		if handle.ID == "bootstrap" {
			return attachErr
		}
		return nil
	}

	_, err = NewService(Options{
		Crypto:   cryptoMgr,
		StateDir: tempDir,
		Volumes:  volumes,
	})
	if err == nil {
		t.Fatalf("expected failure when bootstrap attach fails")
	}
	if !errors.Is(err, attachErr) {
		t.Fatalf("expected attach error to propagate, got %v", err)
	}
}
