package remote

import (
	"context"
	"errors"

	"piccolod/internal/runtime/commands"
)

const (
	CommandConfigure    = "remote.configure"
	CommandDisable      = "remote.disable"
	CommandRotateSecret = "remote.rotate_secret"
	CommandRunPreflight = "remote.run_preflight"
	CommandAddAlias     = "remote.add_alias"
	CommandRemoveAlias  = "remote.remove_alias"
	CommandRenewCert    = "remote.renew_certificate"
	CommandGuideVerify  = "remote.guide_verify"
)

var ErrInvalidCommand = errors.New("remote: invalid command")

type ConfigureCommand struct {
	Req ConfigureRequest
}

func (ConfigureCommand) Name() string { return CommandConfigure }

type ConfigureResponse struct {
	Status Status
}

type DisableCommand struct{}

func (DisableCommand) Name() string { return CommandDisable }

type DisableResponse struct {
	Status Status
}

type RotateSecretCommand struct{}

func (RotateSecretCommand) Name() string { return CommandRotateSecret }

type RotateSecretResponse struct {
	Secret string
}

type RunPreflightCommand struct{}

func (RunPreflightCommand) Name() string { return CommandRunPreflight }

type RunPreflightResponse struct {
	Result PreflightResult
}

type AddAliasCommand struct {
	Listener string
	Hostname string
}

func (AddAliasCommand) Name() string { return CommandAddAlias }

type AddAliasResponse struct {
	Alias Alias
}

type RemoveAliasCommand struct {
	ID string
}

func (RemoveAliasCommand) Name() string { return CommandRemoveAlias }

type RenewCertCommand struct {
	ID string
}

func (RenewCertCommand) Name() string { return CommandRenewCert }

type GuideVerifyCommand struct {
	Verification GuideVerification
}

func (GuideVerifyCommand) Name() string { return CommandGuideVerify }

func RegisterHandlers(dispatcher *commands.Dispatcher, manager *Manager) {
	if dispatcher == nil || manager == nil {
		return
	}
	dispatcher.Register(CommandConfigure, commands.HandlerFunc(manager.handleConfigureCommand))
	dispatcher.Register(CommandDisable, commands.HandlerFunc(manager.handleDisableCommand))
	dispatcher.Register(CommandRotateSecret, commands.HandlerFunc(manager.handleRotateSecretCommand))
	dispatcher.Register(CommandRunPreflight, commands.HandlerFunc(manager.handleRunPreflightCommand))
	dispatcher.Register(CommandAddAlias, commands.HandlerFunc(manager.handleAddAliasCommand))
	dispatcher.Register(CommandRemoveAlias, commands.HandlerFunc(manager.handleRemoveAliasCommand))
	dispatcher.Register(CommandRenewCert, commands.HandlerFunc(manager.handleRenewCertCommand))
	dispatcher.Register(CommandGuideVerify, commands.HandlerFunc(manager.handleGuideVerifyCommand))
}

func (m *Manager) handleConfigureCommand(ctx context.Context, cmd commands.Command) (commands.Response, error) {
	request, ok := cmd.(ConfigureCommand)
	if !ok {
		return nil, ErrInvalidCommand
	}
	if err := m.Configure(request.Req); err != nil {
		return nil, err
	}
	return ConfigureResponse{Status: m.Status()}, nil
}

func (m *Manager) handleDisableCommand(ctx context.Context, cmd commands.Command) (commands.Response, error) {
	if _, ok := cmd.(DisableCommand); !ok {
		return nil, ErrInvalidCommand
	}
	if err := m.Disable(); err != nil {
		return nil, err
	}
	return DisableResponse{Status: m.Status()}, nil
}

func (m *Manager) handleRotateSecretCommand(ctx context.Context, cmd commands.Command) (commands.Response, error) {
	if _, ok := cmd.(RotateSecretCommand); !ok {
		return nil, ErrInvalidCommand
	}
	secret, err := m.Rotate()
	if err != nil {
		return nil, err
	}
	return RotateSecretResponse{Secret: secret}, nil
}

func (m *Manager) handleRunPreflightCommand(ctx context.Context, cmd commands.Command) (commands.Response, error) {
	if _, ok := cmd.(RunPreflightCommand); !ok {
		return nil, ErrInvalidCommand
	}
	result, err := m.RunPreflight()
	if err != nil {
		return nil, err
	}
	return RunPreflightResponse{Result: result}, nil
}

func (m *Manager) handleAddAliasCommand(ctx context.Context, cmd commands.Command) (commands.Response, error) {
	request, ok := cmd.(AddAliasCommand)
	if !ok {
		return nil, ErrInvalidCommand
	}
	alias, err := m.AddAlias(request.Listener, request.Hostname)
	if err != nil {
		return nil, err
	}
	return AddAliasResponse{Alias: alias}, nil
}

func (m *Manager) handleRemoveAliasCommand(ctx context.Context, cmd commands.Command) (commands.Response, error) {
	request, ok := cmd.(RemoveAliasCommand)
	if !ok {
		return nil, ErrInvalidCommand
	}
	if err := m.RemoveAlias(request.ID); err != nil {
		return nil, err
	}
	return nil, nil
}

func (m *Manager) handleRenewCertCommand(ctx context.Context, cmd commands.Command) (commands.Response, error) {
	request, ok := cmd.(RenewCertCommand)
	if !ok {
		return nil, ErrInvalidCommand
	}
	if err := m.RenewCertificate(request.ID); err != nil {
		return nil, err
	}
	return nil, nil
}

func (m *Manager) handleGuideVerifyCommand(ctx context.Context, cmd commands.Command) (commands.Response, error) {
	request, ok := cmd.(GuideVerifyCommand)
	if !ok {
		return nil, ErrInvalidCommand
	}
	if err := m.MarkGuideVerified(request.Verification); err != nil {
		return nil, err
	}
	return nil, nil
}
