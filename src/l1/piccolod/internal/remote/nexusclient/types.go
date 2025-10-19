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
	Resolve(hostname string, remotePort int, isTLS bool) (int, bool)
}

// PortController is an optional extension. Implementers may choose to take
// explicit action when a local public port is no longer available (e.g.,
// proactively refuse or unregister routes).
type PortController interface {
	UnregisterPublicPort(port int)
}

// PortPublisher is an optional extension to re-enable a public port for
// inbound streams once a service comes back or a port is recycled.
type PortPublisher interface {
	RegisterPublicPort(port int)
}
