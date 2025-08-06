package server

import (
	"fmt"
	"log"
	"net/http"
	"piccolod/internal/backup"
	"piccolod/internal/container"
	"piccolod/internal/federation"
	"piccolod/internal/installer"
	"piccolod/internal/network"
	"piccolod/internal/storage"
	"piccolod/internal/trust"
	"piccolod/internal/update"
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
	}

	for _, opt := range opts {
		opt(s)
	}

	s.setupRoutes()
	return s, nil
}

// Start runs the HTTP server.
func (s *Server) Start() error {
	const port = "8080"
	log.Printf("INFO: Starting piccolod server on http://localhost:%s", port)
	return http.ListenAndServe(":"+port, s.router)
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
	mux.HandleFunc("/api/v1/health", s.handleEcosystemTest())
	mux.HandleFunc("/api/v1/ecosystem", s.handleEcosystemTest())

	s.router = mux
}
