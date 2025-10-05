package nexusclient

import "context"

// Config represents the minimum information needed to connect to the nexus proxy.
type Config struct {
	Endpoint       string
	DeviceSecret   string
	PortalHostname string
	TLD            string
}

// Adapter provides a lifecycle wrapper around the nexus backend client.
type Adapter interface {
	Configure(Config) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// RemoteResolver resolves incoming Nexus requests to local listener ports.
type RemoteResolver interface {
	Resolve(hostname string, remotePort int) (int, bool)
}
