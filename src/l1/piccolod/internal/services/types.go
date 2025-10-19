package services

import "piccolod/internal/api"

// PortRange defines an inclusive range of ports
type PortRange struct {
	Start int
	End   int
}

// ServiceEndpoint represents a fully allocated listener
type ServiceEndpoint struct {
	App         string
	Name        string
	GuestPort   int
	HostBind    int // 127.0.0.1:HostBind → container:GuestPort
	PublicPort  int // 0.0.0.0:PublicPort → HostBind
	Flow        api.ListenerFlow
	Protocol    api.ListenerProtocol
	Middleware  []api.AppProtocolMiddleware
	RemotePorts []int
}
