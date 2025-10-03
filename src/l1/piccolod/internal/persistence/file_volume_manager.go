package persistence

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"piccolod/internal/state/paths"
)

type fileVolumeManager struct {
	root    string
	volumes map[string]VolumeHandle
	mu      sync.RWMutex
}

func newFileVolumeManager(root string) *fileVolumeManager {
	if root == "" {
		root = paths.Root()
	}
	return &fileVolumeManager{root: root, volumes: make(map[string]VolumeHandle)}
}

func (f *fileVolumeManager) EnsureVolume(ctx context.Context, req VolumeRequest) (VolumeHandle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if handle, ok := f.volumes[req.ID]; ok {
		return handle, nil
	}

	volDir := filepath.Join(f.root, "volumes", req.ID)
	if err := os.MkdirAll(volDir, 0o700); err != nil {
		return VolumeHandle{}, fmt.Errorf("ensure volume %s: %w", req.ID, err)
	}

	handle := VolumeHandle{ID: req.ID, MountDir: volDir}
	f.volumes[req.ID] = handle
	return handle, nil
}

func (f *fileVolumeManager) Attach(ctx context.Context, handle VolumeHandle, opts AttachOptions) error {
	// For the in-process file volume manager, EnsureVolume already creates the
	// directory. Attach can remain a no-op until real mounting is wired.
	return nil
}

func (f *fileVolumeManager) Detach(ctx context.Context, handle VolumeHandle) error {
	// No-op for the stub implementation; real implementation will unmount.
	return nil
}

func (f *fileVolumeManager) RoleStream(volumeID string) (<-chan VolumeRole, error) {
	ch := make(chan VolumeRole)
	close(ch)
	return ch, nil
}

var _ VolumeManager = (*fileVolumeManager)(nil)
