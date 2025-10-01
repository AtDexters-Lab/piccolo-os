package persistence

import (
	"context"
	"log"

	"piccolod/internal/cluster"
	"piccolod/internal/events"
	"piccolod/internal/runtime/commands"
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
}

// Module implements the Service interface using pluggable sub-components.
type Module struct {
	bootstrap  BootstrapStore
	control    ControlStore
	volumes    VolumeManager
	devices    DeviceManager
	exports    ExportManager
	events     *events.Bus
	leadership *cluster.Registry
	storage    StorageAdapter
	consensus  ConsensusManager
}

// Ensure Module satisfies the Service interface.
var _ Service = (*Module)(nil)

// NewService builds a persistence module with no-op implementations. Concrete
// components can be supplied by replacing the defaults on the returned module.
func NewService(opts Options) (*Module, error) {
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
		mod.control = newNoopControlStore()
	}
	if mod.volumes == nil {
		mod.volumes = newNoopVolumeManager()
	}
	if mod.devices == nil {
		mod.devices = newNoopDeviceManager()
	}
	if mod.exports == nil {
		mod.exports = newNoopExportManager()
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
	mod.publishLockState(true)

	return mod, nil
}

// registerHandlers wires persistence commands into the dispatcher.
func (m *Module) registerHandlers(dispatcher *commands.Dispatcher) {
	dispatcher.Register(CommandEnsureVolume, commands.HandlerFunc(m.handleEnsureVolume))
	dispatcher.Register(CommandAttachVolume, commands.HandlerFunc(m.handleAttachVolume))
	dispatcher.Register(CommandRunControlExport, commands.HandlerFunc(m.handleRunControlExport))
	dispatcher.Register(CommandRunFullExport, commands.HandlerFunc(m.handleRunFullExport))
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
			if payload.Resource != cluster.ResourceControlPlane {
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
	if m.control != nil {
		_ = m.control.Close(ctx)
	}
	// Other sub-components expose explicit Stop methods when implemented.
	return nil
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
