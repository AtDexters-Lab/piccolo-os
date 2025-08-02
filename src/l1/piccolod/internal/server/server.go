package server

import (
	"fmt"
	"log"
	"net/http"
	// Fictional import paths for structure
	"piccolod/internal/backup"
	"piccolod/internal/container"
	"piccolod/internal/federation"
	"piccolod/internal/installer"
	"piccolod/internal/network"
	"piccolod/internal/storage"
	"piccolod/internal/trust"
	"piccolod/internal/update"
)

// Server holds all the core components (dependencies) for our application.
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
}

// New creates the main server application and initializes all its components.
func New() (*Server, error) {
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
	// ... other routes for start, stop, etc.

	// TODO: Add placeholder API routes for other components
	// mux.HandleFunc("/api/v1/storage/disks", s.handleListDisks())
	// mux.HandleFunc("/api/v1/system/attest", s.handleAttestation())
	// mux.HandleFunc("/api/v1/install", s.handleInstallation())
	// mux.HandleFunc("/api/v1/backups/system-state", s.handleSystemStateBackup())

	s.router = mux
}
