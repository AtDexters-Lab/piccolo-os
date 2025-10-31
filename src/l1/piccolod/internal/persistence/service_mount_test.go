package persistence

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"piccolod/internal/crypt"
)

func TestNewServiceFailsWhenBootstrapAttachFails(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("PICCOLO_ALLOW_UNMOUNTED_TESTS", "1")

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

func TestNewServiceAllowsBootstrapPendingSetup(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("PICCOLO_ALLOW_UNMOUNTED_TESTS", "1")

	cryptoMgr, err := crypt.NewManager(tempDir)
	if err != nil {
		t.Fatalf("crypto manager init: %v", err)
	}

	handles := make(map[string]VolumeHandle)
	attachCalls := 0
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
			attachCalls++
			return crypt.ErrNotInitialized
		}
		return nil
	}

	if _, err := NewService(Options{
		Crypto:   cryptoMgr,
		StateDir: tempDir,
		Volumes:  volumes,
	}); err != nil {
		t.Fatalf("service init should tolerate bootstrap not initialized, got %v", err)
	}
	if attachCalls != 1 {
		t.Fatalf("expected one bootstrap attach attempt, got %d", attachCalls)
	}
}

func TestModuleAttachesBootstrapOnUnlock(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("PICCOLO_ALLOW_UNMOUNTED_TESTS", "1")

	cryptoMgr, err := crypt.NewManager(tempDir)
	if err != nil {
		t.Fatalf("crypto manager init: %v", err)
	}

	handles := make(map[string]VolumeHandle)
	attachCalls := 0
	volumes := &stubVolumeManager{}
	volumes.onEnsure = func(_ context.Context, req VolumeRequest) (VolumeHandle, error) {
		if handle, ok := handles[req.ID]; ok {
			return handle, nil
		}
		handle := VolumeHandle{ID: req.ID, MountDir: filepath.Join(tempDir, "mounts", req.ID)}
		handles[req.ID] = handle
		if err := os.MkdirAll(handle.MountDir, 0o700); err != nil {
			t.Fatalf("mkdir mount dir: %v", err)
		}
		return handle, nil
	}
	volumes.onAttach = func(_ context.Context, handle VolumeHandle, _ AttachOptions) error {
		if handle.ID == "bootstrap" {
			attachCalls++
			if attachCalls == 1 {
				return crypt.ErrNotInitialized
			}
		}
		return nil
	}

	mod, err := NewService(Options{
		Crypto:   cryptoMgr,
		StateDir: tempDir,
		Volumes:  volumes,
	})
	if err != nil {
		t.Fatalf("service init: %v", err)
	}
	if attachCalls != 1 {
		t.Fatalf("expected initial bootstrap attach attempt, got %d", attachCalls)
	}

	if err := cryptoMgr.Setup("test-pass"); err != nil {
		t.Fatalf("crypto setup: %v", err)
	}
	if err := cryptoMgr.Unlock("test-pass"); err != nil {
		t.Fatalf("crypto unlock: %v", err)
	}
	prepareControlCipherDir(t, tempDir)

	if err := mod.setLockState(context.Background(), false); err != nil {
		t.Fatalf("unlock setLockState: %v", err)
	}
	if attachCalls != 2 {
		t.Fatalf("expected bootstrap attach retry after unlock, got %d", attachCalls)
	}
}

func TestModuleAttachVolumesIgnoresRequestCancellation(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("PICCOLO_ALLOW_UNMOUNTED_TESTS", "1")

	cryptoMgr, err := crypt.NewManager(tempDir)
	if err != nil {
		t.Fatalf("crypto manager init: %v", err)
	}
	if err := cryptoMgr.Setup("passphrase"); err != nil {
		t.Fatalf("crypto setup: %v", err)
	}
	if err := cryptoMgr.Unlock("passphrase"); err != nil {
		t.Fatalf("crypto unlock: %v", err)
	}

	handles := make(map[string]VolumeHandle)
	volumes := &stubVolumeManager{}
	volumes.onEnsure = func(_ context.Context, req VolumeRequest) (VolumeHandle, error) {
		if handle, ok := handles[req.ID]; ok {
			return handle, nil
		}
		handle := VolumeHandle{ID: req.ID, MountDir: filepath.Join(tempDir, "mounts", req.ID)}
		handles[req.ID] = handle
		if err := os.MkdirAll(handle.MountDir, 0o700); err != nil {
			t.Fatalf("mkdir mount dir: %v", err)
		}
		return handle, nil
	}
	volumes.onAttach = func(ctx context.Context, handle VolumeHandle, _ AttachOptions) error {
		if ctx.Done() != nil {
			t.Fatalf("expected attach context without cancellation, got Done channel")
		}
		return nil
	}

	mod, err := NewService(Options{
		Crypto:   cryptoMgr,
		StateDir: tempDir,
		Volumes:  volumes,
	})
	if err != nil {
		t.Fatalf("service init: %v", err)
	}

	reqCtx, cancel := context.WithCancel(context.Background())
	cancel()
	prepareControlCipherDir(t, tempDir)
	if err := mod.setLockState(reqCtx, false); err != nil {
		t.Fatalf("unlock setLockState: %v", err)
	}
}
