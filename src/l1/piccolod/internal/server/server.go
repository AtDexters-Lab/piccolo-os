package server

import (
	"fmt"
	"log"
	"net/http"
	"piccolod/internal/backup"
	"piccolod/internal/container"
	"piccolod/internal/federation"
	"piccolod/internal/installer"
	"piccolod/internal/mdns"
	"piccolod/internal/network"
	"piccolod/internal/storage"
	"piccolod/internal/trust"
	"piccolod/internal/update"
	"github.com/coreos/go-systemd/v22/daemon"
)

// Server holds all the core components for our application.
type Server struct {
	containerManager  *container.Manager
	storageManager    *storage.Manager
	trustAgent        *trust.Agent
	installer         *installer.Installer
	updateManager     *update.Manager
	networkManager    *network.Manager
	backupManager     *backup.Manager
	federationManager *federation.Manager
	mdnsManager       *mdns.Manager
	router            http.Handler
	version           string
}

// ServerOption is a function that configures a Server.
type ServerOption func(*Server)

// WithVersion sets the version for the server.
func WithVersion(version string) ServerOption {
	return func(s *Server) {
		s.version = version
	}
}

// New creates the main server application and initializes all its components.
func New(opts ...ServerOption) (*Server, error) {
	cm, err := container.NewManager()
	if err != nil {
		return nil, fmt.Errorf("failed to init container manager: %w", err)
	}

	s := &Server{
		containerManager:  cm,
		storageManager:    storage.NewManager(),
		trustAgent:        trust.NewAgent(),
		installer:         installer.NewInstaller(),
		updateManager:     update.NewManager(),
		networkManager:    network.NewManager(),
		backupManager:     backup.NewManager(),
		federationManager: federation.NewManager(),
		mdnsManager:       mdns.NewManager(),
	}

	for _, opt := range opts {
		opt(s)
	}

	s.setupRoutes()
	return s, nil
}

// Start runs the HTTP server and starts mDNS advertising.
func (s *Server) Start() error {
	const port = "80"
	
	// Start mDNS advertising - this must succeed
	if err := s.mdnsManager.Start(); err != nil {
		return fmt.Errorf("FATAL: mDNS server failed to start: %w", err)
	}
	
	log.Printf("INFO: Starting piccolod server on http://localhost:%s", port)
	
	// Notify systemd that we're ready (for Type=notify services)
	// This enables proper health checking and rollback functionality in MicroOS
	if sent, err := daemon.SdNotify(false, daemon.SdNotifyReady); err != nil {
		log.Printf("WARN: Failed to notify systemd of readiness: %v", err)
	} else if sent {
		log.Printf("INFO: Notified systemd that service is ready")
	}
	
	return http.ListenAndServe(":"+port, s.router)
}

// Stop gracefully shuts down the server and all its components.
func (s *Server) Stop() error {
	if err := s.mdnsManager.Stop(); err != nil {
		log.Printf("WARN: Failed to stop mDNS server: %v", err)
	}
	return nil
}

// setupRoutes defines all API endpoints and assigns them to handlers.
func (s *Server) setupRoutes() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot())

	// API routes for container management
	mux.HandleFunc("/api/v1/containers", s.handleContainers())
	mux.HandleFunc("/api/v1/containers/", s.handleSingleContainer())

	// Admin routes
	mux.HandleFunc("/version", s.handleVersion())
	
	// Health and ecosystem testing
	mux.HandleFunc("/api/v1/health", s.handleEcosystemTest())           // Full ecosystem details
	mux.HandleFunc("/api/v1/health/ready", s.handleReadinessCheck())    // Simple boolean health
	mux.HandleFunc("/api/v1/ecosystem", s.handleEcosystemTest())        // Full ecosystem details

	s.router = mux
}
