package persistence

import (
	"context"
	"errors"
	"log"

	"piccolod/internal/runtime/commands"
)

const (
	CommandEnsureVolume     = "persistence.ensure_volume"
	CommandAttachVolume     = "persistence.attach_volume"
	CommandRecordLockState  = "persistence.record_lock_state"
	CommandRunControlExport = "persistence.run_control_export"
	CommandRunFullExport    = "persistence.run_full_export"
)

// EnsureVolumeCommand requests creation (or retrieval) of a volume matching
// the provided request parameters.
type EnsureVolumeCommand struct {
	Req VolumeRequest
}

func (c EnsureVolumeCommand) Name() string { return CommandEnsureVolume }

type EnsureVolumeResponse struct {
	Handle VolumeHandle
}

// AttachVolumeCommand requests the module to attach a volume using the
// specified options (e.g., leader/follower mode).
type AttachVolumeCommand struct {
	Handle VolumeHandle
	Opts   AttachOptions
}

func (c AttachVolumeCommand) Name() string { return CommandAttachVolume }

// RecordLockStateCommand notifies the persistence module about the current
// control-store lock state so it can broadcast to other components.
type RecordLockStateCommand struct {
	Locked bool
}

func (c RecordLockStateCommand) Name() string { return CommandRecordLockState }

// RunControlExportCommand triggers a control-plane-only PCV export.
type RunControlExportCommand struct{}

func (c RunControlExportCommand) Name() string { return CommandRunControlExport }

// RunFullExportCommand triggers a full-data export.
type RunFullExportCommand struct{}

func (c RunFullExportCommand) Name() string { return CommandRunFullExport }

func (m *Module) handleEnsureVolume(ctx context.Context, cmd commands.Command) (commands.Response, error) {
	request, ok := cmd.(EnsureVolumeCommand)
	if !ok {
		return nil, ErrInvalidCommand
	}
	handle, err := m.volumes.EnsureVolume(ctx, request.Req)
	if err != nil {
		return nil, err
	}
	return EnsureVolumeResponse{Handle: handle}, nil
}

func (m *Module) handleAttachVolume(ctx context.Context, cmd commands.Command) (commands.Response, error) {
	request, ok := cmd.(AttachVolumeCommand)
	if !ok {
		return nil, ErrInvalidCommand
	}
	if err := m.volumes.Attach(ctx, request.Handle, request.Opts); err != nil {
		return nil, err
	}
	return nil, nil
}

func (m *Module) handleRecordLockState(ctx context.Context, cmd commands.Command) (commands.Response, error) {
	request, ok := cmd.(RecordLockStateCommand)
	if !ok {
		return nil, ErrInvalidCommand
	}
	if err := m.setLockState(ctx, request.Locked); err != nil {
		return nil, err
	}
	m.publishLockState(request.Locked)
	return nil, nil
}

func (m *Module) handleRunControlExport(ctx context.Context, cmd commands.Command) (commands.Response, error) {
	if _, ok := cmd.(RunControlExportCommand); !ok {
		return nil, ErrInvalidCommand
	}
	artifact, err := m.runExportWithLock(ctx, false, m.exports.RunControlPlane)
	if err != nil {
		return nil, err
	}
	if artifact.Kind == "" {
		artifact.Kind = ExportKindControlOnly
	}
	if artifact.Path == "" {
		log.Printf("WARN: export artifact missing path; ExportManager should supply absolute path")
		return nil, errors.New("persistence: export artifact missing path")
	}
	return artifact, nil
}

func (m *Module) handleRunFullExport(ctx context.Context, cmd commands.Command) (commands.Response, error) {
	if _, ok := cmd.(RunFullExportCommand); !ok {
		return nil, ErrInvalidCommand
	}
	artifact, err := m.runExportWithLock(ctx, true, m.exports.RunFullData)
	if err != nil {
		return nil, err
	}
	if artifact.Kind == "" {
		artifact.Kind = ExportKindFullData
	}
	if artifact.Path == "" {
		log.Printf("WARN: full export artifact missing path; ExportManager should supply absolute path")
		return nil, errors.New("persistence: full export artifact missing path")
	}
	return artifact, nil
}
