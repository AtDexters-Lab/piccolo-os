package persistence

import (
	"context"
	"time"

	"piccolod/internal/cluster"
)

// Service defines the entry point for persistence-related capabilities.
type Service interface {
	Bootstrap() BootstrapStore
	Control() ControlStore
	Volumes() VolumeManager
	Devices() DeviceManager
	Exports() ExportManager
	StorageAdapter() StorageAdapter
	Consensus() ConsensusManager
	BootstrapVolume() VolumeHandle
	ControlVolume() VolumeHandle
}

// BootstrapStore manages the device-local bootstrap shard lifecycle.
type BootstrapStore interface {
	Mount(ctx context.Context) error
	Rebuild(ctx context.Context) error
	IsMounted() bool
}

// ControlStore exposes repositories backed by the control-plane dataset.
type ControlStore interface {
	Auth() AuthRepo
	Remote() RemoteRepo
	AppState() AppStateRepo
	Close(ctx context.Context) error
	Revision(ctx context.Context) (uint64, string, error)
	QuickCheck(ctx context.Context) (ControlHealthReport, error)
}

// VolumeManager orchestrates encrypted volumes via the storage backend.
type VolumeManager interface {
	EnsureVolume(ctx context.Context, req VolumeRequest) (VolumeHandle, error)
	Attach(ctx context.Context, handle VolumeHandle, opts AttachOptions) error
	Detach(ctx context.Context, handle VolumeHandle) error
	RoleStream(volumeID string) (<-chan VolumeRole, error)
}

// DeviceManager discovers and manages physical storage devices.
type DeviceManager interface {
	List(ctx context.Context) ([]PhysicalDevice, error)
	Observe() (<-chan DeviceEvent, error)
}

// ExportManager coordinates PCV and full-data export/import flows.
type ExportManager interface {
	RunControlPlane(ctx context.Context) (ExportArtifact, error)
	RunFullData(ctx context.Context) (ExportArtifact, error)
	ImportControlPlane(ctx context.Context, artifact ExportArtifact, opts ImportOptions) error
	ImportFullData(ctx context.Context, artifact ExportArtifact, opts ImportOptions) error
}

// StorageAdapter provides low-level access to the storage backend (e.g., AionFS).
type StorageAdapter interface {
	CreateVolume(ctx context.Context, req VolumeRequest) (VolumeHandle, error)
	RemoveVolume(ctx context.Context, id string) error
}

// ConsensusManager coordinates leader election and role dissemination.
type ConsensusManager interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// Repository interfaces ------------------------------------------------------

type AuthRepo interface {
	IsInitialized(ctx context.Context) (bool, error)
	SetInitialized(ctx context.Context) error
	PasswordHash(ctx context.Context) (string, error)
	SavePasswordHash(ctx context.Context, hash string) error
	Staleness(ctx context.Context) (AuthStaleness, error)
	UpdateStaleness(ctx context.Context, update AuthStalenessUpdate) error
}

type RemoteRepo interface {
	CurrentConfig(ctx context.Context) (RemoteConfig, error)
	SaveConfig(ctx context.Context, cfg RemoteConfig) error
}

type AppStateRepo interface {
	ListApps(ctx context.Context) ([]AppRecord, error)
	UpsertApp(ctx context.Context, record AppRecord) error
}

// Data structures -----------------------------------------------------------

type VolumeRequest struct {
	ID          string
	Class       VolumeClass
	ClusterMode ClusterMode
}

type VolumeHandle struct {
	ID       string
	MountDir string
}

type AttachOptions struct {
	Role VolumeRole
}

// VolumeClass captures high-level intent for replication/encryption policy.
type VolumeClass string

const (
	VolumeClassBootstrap   VolumeClass = "bootstrap"
	VolumeClassControl     VolumeClass = "control"
	VolumeClassApplication VolumeClass = "application"
)

type ClusterMode string

const (
	ClusterModeStateful          ClusterMode = "stateful"
	ClusterModeStatelessReadOnly ClusterMode = "stateless_read_only"
)

type VolumeRole = cluster.Role

const (
	VolumeRoleUnknown  VolumeRole = cluster.RoleUnknown
	VolumeRoleLeader   VolumeRole = cluster.RoleLeader
	VolumeRoleFollower VolumeRole = cluster.RoleFollower
)

type PhysicalDevice struct {
	ID        string
	Model     string
	SizeBytes uint64
}

type DeviceEvent struct {
	Device PhysicalDevice
	Type   DeviceEventType
}

type DeviceEventType string

const (
	DeviceEventAdded   DeviceEventType = "added"
	DeviceEventRemoved DeviceEventType = "removed"
	DeviceEventUpdated DeviceEventType = "updated"
)

type ExportArtifact struct {
	Path string
	Kind ExportKind
}

type ExportKind string

const (
	ExportKindControlOnly ExportKind = "control_only"
	ExportKindFullData    ExportKind = "full_data"
)

type ImportOptions struct {
	Force bool
}

type RemoteConfig struct {
	Payload []byte
}

type AppRecord struct {
	Name string
}

type ControlHealthStatus string

const (
	ControlHealthStatusOK       ControlHealthStatus = "ok"
	ControlHealthStatusDegraded ControlHealthStatus = "degraded"
	ControlHealthStatusError    ControlHealthStatus = "error"
	ControlHealthStatusUnknown  ControlHealthStatus = "unknown"
)

type ControlHealthReport struct {
	Status    ControlHealthStatus
	Message   string
	CheckedAt time.Time
}

// AuthStaleness captures credential health flags and audit timestamps.
type AuthStaleness struct {
	PasswordStale   bool
	PasswordStaleAt time.Time
	PasswordAckAt   time.Time
	RecoveryStale   bool
	RecoveryStaleAt time.Time
	RecoveryAckAt   time.Time
}

// AuthStalenessUpdate describes partial updates applied atomically.
type AuthStalenessUpdate struct {
	PasswordStale   *bool
	PasswordStaleAt *time.Time
	PasswordAckAt   *time.Time
	RecoveryStale   *bool
	RecoveryStaleAt *time.Time
	RecoveryAckAt   *time.Time
}
