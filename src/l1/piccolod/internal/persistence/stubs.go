package persistence

import (
	"context"
	"errors"
)

var (
	ErrNotImplemented          = errors.New("persistence: not implemented")
	ErrInvalidCommand          = errors.New("persistence: invalid command payload")
	ErrLocked                  = errors.New("persistence: locked")
	ErrNotLeader               = errors.New("persistence: not leader")
	ErrCryptoUnavailable       = errors.New("persistence: crypto unavailable")
	ErrNotFound                = errors.New("persistence: not found")
	ErrVolumeMetadataCorrupted = errors.New("persistence: volume metadata corrupted")
)

// Bootstrap -----------------------------------------------------------------

type noopBootstrapStore struct{}

func newNoopBootstrapStore() *noopBootstrapStore { return &noopBootstrapStore{} }

func (n *noopBootstrapStore) Mount(ctx context.Context) error   { return nil }
func (n *noopBootstrapStore) Rebuild(ctx context.Context) error { return nil }
func (n *noopBootstrapStore) IsMounted() bool                   { return false }

// Control -------------------------------------------------------------------

type noopControlStore struct {
	auth    AuthRepo
	remote  RemoteRepo
	appRepo AppStateRepo
}

func newNoopControlStore() *noopControlStore {
	return &noopControlStore{
		auth:    &noopAuthRepo{},
		remote:  &noopRemoteRepo{},
		appRepo: &noopAppStateRepo{},
	}
}

func (n *noopControlStore) Auth() AuthRepo                  { return n.auth }
func (n *noopControlStore) Remote() RemoteRepo              { return n.remote }
func (n *noopControlStore) AppState() AppStateRepo          { return n.appRepo }
func (n *noopControlStore) Close(ctx context.Context) error { return nil }
func (n *noopControlStore) Revision(ctx context.Context) (uint64, string, error) {
	return 0, "", ErrNotImplemented
}

type noopAuthRepo struct{}

func (n *noopAuthRepo) IsInitialized(ctx context.Context) (bool, error) {
	return false, ErrNotImplemented
}
func (n *noopAuthRepo) SetInitialized(ctx context.Context) error { return ErrNotImplemented }
func (n *noopAuthRepo) PasswordHash(ctx context.Context) (string, error) {
	return "", ErrNotImplemented
}
func (n *noopAuthRepo) SavePasswordHash(ctx context.Context, hash string) error {
	return ErrNotImplemented
}
func (n *noopAuthRepo) Staleness(ctx context.Context) (AuthStaleness, error) {
	return AuthStaleness{}, ErrNotImplemented
}
func (n *noopAuthRepo) UpdateStaleness(ctx context.Context, update AuthStalenessUpdate) error {
	return ErrNotImplemented
}

type noopRemoteRepo struct{}

func (n *noopRemoteRepo) CurrentConfig(ctx context.Context) (RemoteConfig, error) {
	return RemoteConfig{}, ErrNotImplemented
}

func (n *noopRemoteRepo) SaveConfig(ctx context.Context, cfg RemoteConfig) error {
	return ErrNotImplemented
}

type noopAppStateRepo struct{}

func (n *noopAppStateRepo) ListApps(ctx context.Context) ([]AppRecord, error) {
	return nil, ErrNotImplemented
}

func (n *noopAppStateRepo) UpsertApp(ctx context.Context, record AppRecord) error {
	return ErrNotImplemented
}

// Volume --------------------------------------------------------------------

type noopVolumeManager struct{}

func newNoopVolumeManager() *noopVolumeManager { return &noopVolumeManager{} }

func (n *noopVolumeManager) EnsureVolume(ctx context.Context, req VolumeRequest) (VolumeHandle, error) {
	return VolumeHandle{ID: req.ID, MountDir: ""}, ErrNotImplemented
}

func (n *noopVolumeManager) Attach(ctx context.Context, handle VolumeHandle, opts AttachOptions) error {
	return ErrNotImplemented
}

func (n *noopVolumeManager) Detach(ctx context.Context, handle VolumeHandle) error {
	return ErrNotImplemented
}

func (n *noopVolumeManager) RoleStream(volumeID string) (<-chan VolumeRole, error) {
	ch := make(chan VolumeRole)
	close(ch)
	return ch, nil
}

// Device --------------------------------------------------------------------

type noopDeviceManager struct{}

func newNoopDeviceManager() *noopDeviceManager { return &noopDeviceManager{} }

func (n *noopDeviceManager) List(ctx context.Context) ([]PhysicalDevice, error) {
	return nil, ErrNotImplemented
}

func (n *noopDeviceManager) Observe() (<-chan DeviceEvent, error) {
	ch := make(chan DeviceEvent)
	close(ch)
	return ch, nil
}

// Exports -------------------------------------------------------------------

type noopExportManager struct{}

func newNoopExportManager() *noopExportManager { return &noopExportManager{} }

func (n *noopExportManager) RunControlPlane(ctx context.Context) (ExportArtifact, error) {
	return ExportArtifact{Kind: ExportKindControlOnly}, ErrNotImplemented
}

func (n *noopExportManager) RunFullData(ctx context.Context) (ExportArtifact, error) {
	return ExportArtifact{Kind: ExportKindFullData}, ErrNotImplemented
}

func (n *noopExportManager) ImportControlPlane(ctx context.Context, artifact ExportArtifact, opts ImportOptions) error {
	return ErrNotImplemented
}

func (n *noopExportManager) ImportFullData(ctx context.Context, artifact ExportArtifact, opts ImportOptions) error {
	return ErrNotImplemented
}

// Storage adapter -----------------------------------------------------------

type noopStorageAdapter struct{}

func newNoopStorageAdapter() *noopStorageAdapter { return &noopStorageAdapter{} }

func (n *noopStorageAdapter) CreateVolume(ctx context.Context, req VolumeRequest) (VolumeHandle, error) {
	return VolumeHandle{ID: req.ID}, ErrNotImplemented
}

func (n *noopStorageAdapter) RemoveVolume(ctx context.Context, id string) error {
	return ErrNotImplemented
}

// Consensus -----------------------------------------------------------------

type noopConsensusManager struct{}

func newNoopConsensusManager() *noopConsensusManager { return &noopConsensusManager{} }

func (n *noopConsensusManager) Start(ctx context.Context) error { return nil }

func (n *noopConsensusManager) Stop(ctx context.Context) error { return nil }
