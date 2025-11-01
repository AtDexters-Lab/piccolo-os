package persistence

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var errVolumeNotMounted = errors.New("test: volume not mounted")

func TestModuleSetLockStateIgnoresStaleCipherMarker(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mountDir := filepath.Join(root, "mounts", "control")
	if err := os.MkdirAll(mountDir, 0o700); err != nil {
		t.Fatalf("mkdir mount dir: %v", err)
	}
	// Simulate a stale marker that survives an unclean shutdown even though the
	// FUSE mount is already gone.
	if err := os.WriteFile(filepath.Join(mountDir, ".cipher"), []byte("/ciphertext/control"), 0o600); err != nil {
		t.Fatalf("write cipher marker: %v", err)
	}

	ctrl := &stubLockableControl{}
	vol := &stubVolumeManager{}
	vol.onDetach = func(context.Context, VolumeHandle) error {
		return errVolumeNotMounted
	}

	mod := &Module{
		control:       ctrl,
		volumes:       vol,
		controlHandle: VolumeHandle{ID: "control", MountDir: mountDir},
	}

	if err := mod.setLockState(context.Background(), true); err != nil {
		t.Fatalf("setLockState should tolerate stale marker: %v", err)
	}
}

func TestModuleRunExportWithLockRestoresState(t *testing.T) {
	mod := &Module{
		control:       &stubLockableControl{},
		volumes:       &stubVolumeManager{},
		controlHandle: VolumeHandle{ID: "control"},
	}
	mod.lockState = false

	artifact, err := mod.runExportWithLock(context.Background(), false, func(ctx context.Context) (ExportArtifact, error) {
		ctrl := mod.control.(*stubLockableControl)
		if !ctrl.locked {
			t.Fatalf("expected control store locked during export")
		}
		return ExportArtifact{Path: "/tmp/control.pcv", Kind: ExportKindControlOnly}, nil
	})
	if err != nil {
		t.Fatalf("runExportWithLock returned error: %v", err)
	}
	ctrl := mod.control.(*stubLockableControl)
	if ctrl.locked {
		t.Fatalf("expected control store unlocked after export")
	}
	if mod.ControlLocked() {
		t.Fatalf("module lock state not restored")
	}
	if artifact.Kind != ExportKindControlOnly {
		t.Fatalf("unexpected artifact kind %s", artifact.Kind)
	}
}

type stubLockableControl struct {
	locked bool
}

func (s *stubLockableControl) Lock() {
	s.locked = true
}

func (s *stubLockableControl) Unlock(context.Context) error {
	s.locked = false
	return nil
}

func (s *stubLockableControl) Auth() AuthRepo {
	return nil
}

func (s *stubLockableControl) Remote() RemoteRepo {
	return nil
}

func (s *stubLockableControl) AppState() AppStateRepo {
	return nil
}

func (s *stubLockableControl) Close(context.Context) error {
	return nil
}

func (s *stubLockableControl) Revision(context.Context) (uint64, string, error) {
	return 0, "", nil
}

func (s *stubLockableControl) QuickCheck(context.Context) (ControlHealthReport, error) {
	return ControlHealthReport{Status: ControlHealthStatusOK, Message: "ok", CheckedAt: time.Now().UTC()}, nil
}

type stubVolumeManager struct {
	onEnsure func(context.Context, VolumeRequest) (VolumeHandle, error)
	onAttach func(context.Context, VolumeHandle, AttachOptions) error
	onDetach func(context.Context, VolumeHandle) error
	onStream func(string) (<-chan VolumeRole, error)
}

func (s *stubVolumeManager) EnsureVolume(ctx context.Context, req VolumeRequest) (VolumeHandle, error) {
	if s.onEnsure != nil {
		return s.onEnsure(ctx, req)
	}
	return VolumeHandle{}, nil
}

func (s *stubVolumeManager) Attach(ctx context.Context, handle VolumeHandle, opts AttachOptions) error {
	if s.onAttach != nil {
		return s.onAttach(ctx, handle, opts)
	}
	return nil
}

func (s *stubVolumeManager) Detach(ctx context.Context, handle VolumeHandle) error {
	if s.onDetach != nil {
		return s.onDetach(ctx, handle)
	}
	return nil
}

func (s *stubVolumeManager) RoleStream(id string) (<-chan VolumeRole, error) {
	if s.onStream != nil {
		return s.onStream(id)
	}
	ch := make(chan VolumeRole)
	close(ch)
	return ch, nil
}
