package persistence

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileVolumeManagerEnsureVolume(t *testing.T) {
	root := t.TempDir()
	mgr := newFileVolumeManager(root)

	handle, err := mgr.EnsureVolume(context.Background(), VolumeRequest{ID: "control", Class: VolumeClassControl})
	if err != nil {
		t.Fatalf("EnsureVolume: %v", err)
	}
	if handle.MountDir == "" {
		t.Fatalf("expected mount dir")
	}
	if _, err := os.Stat(handle.MountDir); err != nil {
		t.Fatalf("expected directory to exist: %v", err)
	}

	// second call should reuse
	handle2, err := mgr.EnsureVolume(context.Background(), VolumeRequest{ID: "control", Class: VolumeClassControl})
	if err != nil {
		t.Fatalf("EnsureVolume second: %v", err)
	}
	if handle2.MountDir != handle.MountDir {
		t.Fatalf("expected same mount dir, got %s vs %s", handle2.MountDir, handle.MountDir)
	}
}

func TestFileVolumeManagerAttach(t *testing.T) {
	root := t.TempDir()
	mgr := newFileVolumeManager(root)
	handle, err := mgr.EnsureVolume(context.Background(), VolumeRequest{ID: "bootstrap", Class: VolumeClassBootstrap})
	if err != nil {
		t.Fatalf("EnsureVolume: %v", err)
	}
	if err := mgr.Attach(context.Background(), handle, AttachOptions{Role: VolumeRoleLeader}); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "volumes", "bootstrap")); err != nil {
		t.Fatalf("expected bootstrap directory: %v", err)
	}
}
