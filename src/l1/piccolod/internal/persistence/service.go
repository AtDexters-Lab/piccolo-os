package persistence

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"piccolod/internal/cluster"
	"piccolod/internal/crypt"
	"piccolod/internal/events"
	"piccolod/internal/runtime/commands"
	"piccolod/internal/state/paths"
)

// Options captures construction parameters for the persistence service.
type Options struct {
	Bootstrap      BootstrapStore
	Control        ControlStore
	Volumes        VolumeManager
	Devices        DeviceManager
	Exports        ExportManager
	StorageAdapter StorageAdapter
	Consensus      ConsensusManager
	Events         *events.Bus
	Leadership     *cluster.Registry
	Dispatcher     *commands.Dispatcher
	Crypto         *crypt.Manager
	StateDir       string
}

// Module implements the Service interface using pluggable sub-components.
type Module struct {
	bootstrap          BootstrapStore
	control            ControlStore
	volumes            VolumeManager
	devices            DeviceManager
	exports            ExportManager
	events             *events.Bus
	leadership         *cluster.Registry
	storage            StorageAdapter
	consensus          ConsensusManager
	crypto             *crypt.Manager
	stateDir           string
	bootstrapHandle    VolumeHandle
	controlHandle      VolumeHandle
	commitMu           sync.Mutex
	lastCommitRevision uint64
	pollCancel         context.CancelFunc
	lockStateMu        sync.RWMutex
	lockState          bool
}

// Ensure Module satisfies the Service interface.
var _ Service = (*Module)(nil)

// NewService builds a persistence module with no-op implementations. Concrete
// components can be supplied by replacing the defaults on the returned module.
func NewService(opts Options) (*Module, error) {
	stateDir := opts.StateDir
	if stateDir == "" {
		stateDir = paths.Root()
	}
	if err := ensureBootstrapRoot(stateDir); err != nil {
		return nil, err
	}
	mod := &Module{
		bootstrap:  opts.Bootstrap,
		control:    opts.Control,
		volumes:    opts.Volumes,
		devices:    opts.Devices,
		exports:    opts.Exports,
		storage:    opts.StorageAdapter,
		consensus:  opts.Consensus,
		events:     opts.Events,
		leadership: opts.Leadership,
		crypto:     opts.Crypto,
		stateDir:   stateDir,
		lockState:  true,
	}

	if mod.events == nil {
		mod.events = events.NewBus()
	}
	if mod.leadership == nil {
		mod.leadership = cluster.NewRegistry()
	}
	if mod.bootstrap == nil {
		mod.bootstrap = newNoopBootstrapStore()
	}
	if mod.control == nil {
		if mod.crypto == nil {
			return nil, ErrCryptoUnavailable
		}
		store, err := newEncryptedControlStore(mod.stateDir, mod.crypto)
		if err != nil {
			return nil, err
		}
		mod.control = newGuardedControlStore(store, func() bool {
			if mod.leadership == nil {
				return true
			}
			return mod.leadership.Current(cluster.ResourceKernel) != cluster.RoleFollower
		}, mod.onControlCommit)
	}
	if mod.volumes == nil {
		if mod.crypto == nil {
			return nil, ErrCryptoUnavailable
		}
		mod.volumes = newFileVolumeManager(mod.stateDir, mod.crypto, mod.events)
	}
	if fm, ok := mod.volumes.(*fileVolumeManager); ok {
		if fm.bus == nil {
			fm.bus = mod.events
		}
		fm.setRoleChecker(func(volumeID string, role VolumeRole) bool {
			if role != VolumeRoleLeader {
				return true
			}
			if volumeID == "control" {
				if mod.leadership == nil {
					return false
				}
				return mod.leadership.Current(cluster.ResourceKernel) == cluster.RoleLeader
			}
			return true
		})
	}
	if mod.devices == nil {
		mod.devices = newNoopDeviceManager()
	}
	if mod.exports == nil {
		mod.exports = newFileExportManager(mod.stateDir)
	}
	if mod.storage == nil {
		mod.storage = newNoopStorageAdapter()
	}
	if mod.consensus == nil {
		mod.consensus = newNoopConsensusManager()
	}

	if opts.Dispatcher != nil {
		mod.registerHandlers(opts.Dispatcher)
	}

	mod.observeLeadership()
	if err := mod.setLockState(context.Background(), true); err != nil {
		return nil, err
	}
	mod.publishLockState(true)

	if err := mod.ensureCoreVolumes(context.Background()); err != nil {
		return nil, err
	}
	mod.startRevisionPoller()

	return mod, nil
}

func (m *Module) ensureCoreVolumes(ctx context.Context) error {
	if m.volumes == nil {
		return nil
	}
	if fm, ok := m.volumes.(*fileVolumeManager); ok {
		if err := fm.reconcileAllVolumeStates(); err != nil {
			return err
		}
	}
	bootstrapReq := VolumeRequest{ID: "bootstrap", Class: VolumeClassBootstrap, ClusterMode: ClusterModeStateful}
	if handle, err := m.volumes.EnsureVolume(ctx, bootstrapReq); err != nil {
		return err
	} else {
		m.bootstrapHandle = handle
		if err := m.volumes.Attach(ctx, handle, AttachOptions{Role: VolumeRoleLeader}); err != nil {
			if errors.Is(err, ErrNotImplemented) {
				log.Printf("INFO: bootstrap volume attachment not supported: %v", err)
			} else if errors.Is(err, crypt.ErrLocked) {
				log.Printf("INFO: bootstrap volume locked; will retry after unlock")
			} else {
				return fmt.Errorf("attach bootstrap volume: %w", err)
			}
		}
	}
	controlReq := VolumeRequest{ID: "control", Class: VolumeClassControl, ClusterMode: ClusterModeStateful}
	if handle, err := m.volumes.EnsureVolume(ctx, controlReq); err != nil {
		return err
	} else {
		m.controlHandle = handle
	}
	return nil
}

func (m *Module) attachControlVolume(ctx context.Context) error {
	if m.volumes == nil {
		return nil
	}
	if m.controlHandle.ID == "" {
		return fmt.Errorf("control volume handle unavailable")
	}
	role := VolumeRoleLeader
	if m.leadership != nil {
		if current := m.leadership.Current(cluster.ResourceKernel); current == cluster.RoleFollower {
			role = VolumeRoleFollower
		}
	}
	if err := m.volumes.Attach(ctx, m.controlHandle, AttachOptions{Role: role}); err != nil {
		if errors.Is(err, ErrNotImplemented) {
			return nil
		}
		return err
	}
	return nil
}

// registerHandlers wires persistence commands into the dispatcher.
func (m *Module) registerHandlers(dispatcher *commands.Dispatcher) {
	dispatcher.Register(CommandEnsureVolume, commands.HandlerFunc(m.handleEnsureVolume))
	dispatcher.Register(CommandAttachVolume, commands.HandlerFunc(m.handleAttachVolume))
	dispatcher.Register(CommandRecordLockState, commands.HandlerFunc(m.handleRecordLockState))
	dispatcher.Register(CommandRunControlExport, commands.HandlerFunc(m.handleRunControlExport))
	dispatcher.Register(CommandRunFullExport, commands.HandlerFunc(m.handleRunFullExport))
}

type lockableControlStore interface {
	Lock()
	Unlock(context.Context) error
}

func (m *Module) observeLeadership() {
	if m.events == nil {
		return
	}
	ch := m.events.Subscribe(events.TopicLeadershipRoleChanged, 8)
	go func() {
		for evt := range ch {
			payload, ok := evt.Payload.(events.LeadershipChanged)
			if !ok {
				continue
			}
			if payload.Resource != cluster.ResourceKernel {
				continue
			}
			log.Printf("INFO: persistence observed control-plane role=%s", payload.Role)
		}
	}()
}

func (m *Module) Bootstrap() BootstrapStore {
	return m.bootstrap
}

func (m *Module) Control() ControlStore {
	return m.control
}

func (m *Module) ControlVolume() VolumeHandle {
	return m.controlHandle
}

func (m *Module) BootstrapVolume() VolumeHandle {
	return m.bootstrapHandle
}

func (m *Module) Volumes() VolumeManager {
	return m.volumes
}

func (m *Module) Devices() DeviceManager {
	return m.devices
}

func (m *Module) Exports() ExportManager {
	return m.exports
}

func (m *Module) StorageAdapter() StorageAdapter {
	return m.storage
}

func (m *Module) Consensus() ConsensusManager {
	return m.consensus
}

func (m *Module) setLockState(ctx context.Context, locked bool) error {
	store, ok := m.control.(lockableControlStore)
	if !ok {
		if locked {
			m.lockStateMu.Lock()
			m.lockState = true
			m.lockStateMu.Unlock()
			return nil
		}
		return ErrNotImplemented
	}
	if locked {
		store.Lock()
		if err := m.detachVolumeIfMounted(ctx, m.controlHandle); err != nil {
			return err
		}
		m.lockStateMu.Lock()
		m.lockState = true
		m.lockStateMu.Unlock()
		return nil
	}
	if err := store.Unlock(ctx); err != nil {
		return err
	}
	if err := m.attachControlVolume(ctx); err != nil {
		store.Lock()
		return fmt.Errorf("attach control volume: %w", err)
	}
	m.lockStateMu.Lock()
	m.lockState = false
	m.lockStateMu.Unlock()
	return nil
}

// SwapBootstrap allows wiring a real bootstrap store after construction.
func (m *Module) SwapBootstrap(store BootstrapStore) {
	if store != nil {
		m.bootstrap = store
	}
}

// SwapControl allows wiring a real control store after construction.
func (m *Module) SwapControl(store ControlStore) {
	if store != nil {
		m.control = store
	}
}

// SwapVolumes allows wiring a real volume manager after construction.
func (m *Module) SwapVolumes(manager VolumeManager) {
	if manager != nil {
		m.volumes = manager
	}
}

// SwapDevices allows wiring a real device manager after construction.
func (m *Module) SwapDevices(manager DeviceManager) {
	if manager != nil {
		m.devices = manager
	}
}

// SwapExports allows wiring a real export manager after construction.
func (m *Module) SwapExports(manager ExportManager) {
	if manager != nil {
		m.exports = manager
	}
}

// SwapStorageAdapter allows wiring a real storage adapter after construction.
func (m *Module) SwapStorageAdapter(adapter StorageAdapter) {
	if adapter != nil {
		m.storage = adapter
	}
}

// SwapConsensus allows wiring a real consensus manager after construction.
func (m *Module) SwapConsensus(manager ConsensusManager) {
	if manager != nil {
		m.consensus = manager
	}
}

// Shutdown terminates sub-components that require cleanup.
func (m *Module) Shutdown(ctx context.Context) error {
	m.commitMu.Lock()
	if m.pollCancel != nil {
		m.pollCancel()
		m.pollCancel = nil
	}
	m.commitMu.Unlock()
	if m.control != nil {
		_ = m.control.Close(ctx)
	}
	if err := m.detachVolumeIfMounted(ctx, m.controlHandle); err != nil {
		log.Printf("WARN: persistence failed to detach control volume: %v", err)
	}
	if err := m.detachVolumeIfMounted(ctx, m.bootstrapHandle); err != nil {
		log.Printf("WARN: persistence failed to detach bootstrap volume: %v", err)
	}
	// Other sub-components expose explicit Stop methods when implemented.
	return nil
}

func (m *Module) detachVolumeIfMounted(ctx context.Context, handle VolumeHandle) error {
	if m.volumes == nil || handle.ID == "" || handle.MountDir == "" {
		return nil
	}
	marker := filepath.Join(handle.MountDir, ".cipher")
	mounted, err := isMountPoint(handle.MountDir)
	if err != nil {
		return err
	}
	if !mounted {
		// The mount disappeared (e.g., after an unclean shutdown) but our
		// sentinel files remain. Best-effort cleanup so subsequent lock attempts
		// do not mis-detect a mounted volume.
		if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
			return err
		}
		modeMarker := filepath.Join(handle.MountDir, ".mode")
		if err := os.Remove(modeMarker); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if _, err := os.Stat(marker); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return m.volumes.Detach(ctx, handle)
}
func (m *Module) publishLockState(locked bool) {
	if m.events == nil {
		return
	}
	m.events.Publish(events.Event{
		Topic: events.TopicLockStateChanged,
		Payload: events.LockStateChanged{
			Locked: locked,
		},
	})
}

// ControlLocked reports whether the control store is currently locked.
func (m *Module) ControlLocked() bool {
	m.lockStateMu.RLock()
	defer m.lockStateMu.RUnlock()
	return m.lockState
}

type revisionReporter interface {
	Revision(context.Context) (uint64, string, error)
}

func (m *Module) revisionSource() revisionReporter {
	if rep, ok := m.control.(revisionReporter); ok {
		return rep
	}
	return nil
}

func (m *Module) onControlCommit(ctx context.Context) {
	rep := m.revisionSource()
	if rep == nil {
		return
	}
	rev, checksum, err := rep.Revision(ctx)
	if err != nil {
		log.Printf("WARN: persistence commit revision read failed: %v", err)
		return
	}
	m.publishControlCommit(cluster.RoleLeader, rev, checksum)
}

func (m *Module) publishControlCommit(role cluster.Role, rev uint64, checksum string) {
	m.commitMu.Lock()
	if rev <= m.lastCommitRevision {
		m.commitMu.Unlock()
		return
	}
	m.lastCommitRevision = rev
	m.commitMu.Unlock()
	if m.events == nil {
		return
	}
	m.events.Publish(events.Event{
		Topic:   events.TopicControlStoreCommit,
		Payload: events.ControlStoreCommit{Revision: rev, Checksum: checksum, Role: role},
	})
}

func (m *Module) startRevisionPoller() {
	rep := m.revisionSource()
	if rep == nil {
		return
	}
	m.commitMu.Lock()
	if m.pollCancel != nil {
		m.commitMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.pollCancel = cancel
	m.commitMu.Unlock()
	go m.revisionPollLoop(ctx, rep)
}

func (m *Module) revisionPollLoop(ctx context.Context, rep revisionReporter) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.pollRevision(ctx, rep)
		}
	}
}

func (m *Module) pollRevision(ctx context.Context, rep revisionReporter) {
	if m.leadership != nil && m.leadership.Current(cluster.ResourceKernel) == cluster.RoleLeader {
		return
	}
	rev, checksum, err := rep.Revision(ctx)
	if err != nil {
		return
	}
	m.publishControlCommit(cluster.RoleFollower, rev, checksum)
}
