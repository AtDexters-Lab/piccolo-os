package server

import (
	"fmt"
	"log"
	"net/http"

	"piccolod/internal/app"
	"piccolod/internal/backup"
	"piccolod/internal/container"
	"piccolod/internal/ecosystem"
	"piccolod/internal/federation"
	"piccolod/internal/installer"
	"piccolod/internal/mdns"
	"piccolod/internal/network"
	"piccolod/internal/storage"
	"piccolod/internal/trust"
	"piccolod/internal/update"

	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/gin-gonic/gin"
)

// GinServer holds all the core components for our application using Gin framework.
type GinServer struct {
	containerManager  *container.Manager
	appManager        *app.FSManager
	storageManager    *storage.Manager
	trustAgent        *trust.Agent
	installer         *installer.Installer
	updateManager     *update.Manager
	networkManager    *network.Manager
	backupManager     *backup.Manager
	federationManager *federation.Manager
	mdnsManager       *mdns.Manager
	ecosystemManager  *ecosystem.Manager
	router            *gin.Engine
	version           string
}

// GinServerOption is a function that configures a GinServer.
type GinServerOption func(*GinServer)

// WithVersion sets the version for the server.
func WithGinVersion(version string) GinServerOption {
	return func(s *GinServer) {
		s.version = version
	}
}

// NewGinServer creates the main server application using Gin and initializes all its components.
func NewGinServer(opts ...GinServerOption) (*GinServer, error) {
	cm, err := container.NewManager()
	if err != nil {
		return nil, fmt.Errorf("failed to init container manager: %w", err)
	}

	// Create Podman CLI for app management
	podmanCLI := &container.PodmanCLI{}

	// Initialize app manager with filesystem state management
	appMgr, err := app.NewFSManager(podmanCLI, "")
	if err != nil {
		return nil, fmt.Errorf("failed to init app manager: %w", err)
	}

	// Set Gin to release mode for production (can be overridden by GIN_MODE env var)
	gin.SetMode(gin.ReleaseMode)

	s := &GinServer{
		containerManager:  cm,
		appManager:        appMgr,
		storageManager:    storage.NewManager(),
		trustAgent:        trust.NewAgent(),
		installer:         installer.NewInstaller(),
		updateManager:     update.NewManager(),
		networkManager:    network.NewManager(),
		backupManager:     backup.NewManager(),
		federationManager: federation.NewManager(),
		mdnsManager:       mdns.NewManager(),
	}

	s.ecosystemManager = ecosystem.NewManager(
		s.containerManager,
		s.appManager,
		s.storageManager,
		s.trustAgent,
		s.installer,
		s.updateManager,
		s.networkManager,
		s.backupManager,
		s.federationManager,
	)

	for _, opt := range opts {
		opt(s)
	}

	s.setupGinRoutes()
	return s, nil
}

// Start runs the Gin HTTP server and starts mDNS advertising.
func (s *GinServer) Start() error {
	const port = "80"

	// Start mDNS advertising - this must succeed
	if err := s.mdnsManager.Start(); err != nil {
		return fmt.Errorf("FATAL: mDNS server failed to start: %w", err)
	}

	log.Printf("INFO: Starting piccolod server with Gin on http://localhost:%s", port)

	// Notify systemd that we're ready (for Type=notify services)
	// This enables proper health checking and rollback functionality in MicroOS
	if sent, err := daemon.SdNotify(false, daemon.SdNotifyReady); err != nil {
		log.Printf("WARN: Failed to notify systemd of readiness: %v", err)
	} else if sent {
		log.Printf("INFO: Notified systemd that service is ready")
	}

	return s.router.Run(":" + port)
}

// Stop gracefully shuts down the server and all its components.
func (s *GinServer) Stop() error {
	if err := s.mdnsManager.Stop(); err != nil {
		log.Printf("WARN: Failed to stop mDNS server: %v", err)
	}
	return nil
}

// setupGinRoutes defines all API endpoints using Gin router.
func (s *GinServer) setupGinRoutes() {
	r := gin.New()

	// Add basic middleware
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.Use(s.corsMiddleware())
	r.Use(s.securityHeadersMiddleware())

	// Root endpoint
	r.GET("/", s.handleGinRoot)

	// API v1 group
	v1 := r.Group("/api/v1")
	{
		// Container management endpoints (existing)
		v1.GET("/containers", s.handleGinContainers)
		v1.POST("/containers", s.handleGinContainers)

		// App management endpoints
		apps := v1.Group("/apps")
		{
			apps.POST("", s.handleGinAppInstall)           // POST /api/v1/apps
			apps.GET("", s.handleGinAppList)               // GET /api/v1/apps
			apps.GET("/:name", s.handleGinAppGet)          // GET /api/v1/apps/:name
			apps.DELETE("/:name", s.handleGinAppUninstall) // DELETE /api/v1/apps/:name

			// App actions
			apps.POST("/:name/start", s.handleGinAppStart)     // POST /api/v1/apps/:name/start
			apps.POST("/:name/stop", s.handleGinAppStop)       // POST /api/v1/apps/:name/stop
			apps.POST("/:name/enable", s.handleGinAppEnable)   // POST /api/v1/apps/:name/enable
			apps.POST("/:name/disable", s.handleGinAppDisable) // POST /api/v1/apps/:name/disable
		}

		// Health endpoints
		v1.GET("/health", s.handleGinEcosystemTest)        // Full ecosystem details
		v1.GET("/health/ready", s.handleGinReadinessCheck) // Simple boolean health
		v1.GET("/ecosystem", s.handleGinEcosystemTest)     // Full ecosystem details
	}

	// Admin routes
	r.GET("/version", s.handleGinVersion)

	// Static file serving for web UI
	s.setupStaticRoutes(r)

	s.router = r
}

// Root handler - serve web UI or API info based on Accept header
func (s *GinServer) handleGinRoot(c *gin.Context) {
	// Check if this is an API request (Accept: application/json)
	if c.GetHeader("Accept") == "application/json" {
		c.JSON(http.StatusOK, gin.H{
			"message": "Piccolo OS Container Platform API",
			"version": s.version,
			"status":  "running",
		})
		return
	}
	
	// Otherwise serve the web UI
	c.File("./web/index.html")
}

func (s *GinServer) handleGinContainers(c *gin.Context) {
	// TODO: Implement container management (existing functionality)
	c.JSON(http.StatusOK, gin.H{"message": "Container management (placeholder)"})
}

func (s *GinServer) handleGinVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version": s.version,
		"service": "piccolod",
	})
}

func (s *GinServer) handleGinEcosystemTest(c *gin.Context) {
	// Run the real ecosystem checks
	response := s.ecosystemManager.RunEcosystemChecks()

	// Determine HTTP status code based on overall health
	statusCode := http.StatusOK
	switch response.Overall {
	case "unhealthy":
		statusCode = http.StatusServiceUnavailable
	case "degraded":
		statusCode = http.StatusOK // Still operational, but with issues
	}

	c.JSON(statusCode, response)
}

func (s *GinServer) handleGinReadinessCheck(c *gin.Context) {
	// Run ecosystem checks and convert to simple readiness response
	ecosystemResponse := s.ecosystemManager.RunEcosystemChecks()

	var ready bool
	var statusCode int

	// Convert ecosystem status to simple boolean
	switch ecosystemResponse.Overall {
	case "healthy":
		ready = true
		statusCode = http.StatusOK
	case "degraded":
		ready = true // Still ready to serve traffic
		statusCode = http.StatusOK
	case "unhealthy":
		ready = false
		statusCode = http.StatusServiceUnavailable
	default:
		ready = false
		statusCode = http.StatusInternalServerError
	}

	response := ecosystem.ReadinessResponse{
		Ready:   ready,
		Status:  ecosystemResponse.Overall,
		Message: ecosystemResponse.Summary,
	}

	c.JSON(statusCode, response)
}

// setupStaticRoutes configures static file serving for web UI
func (s *GinServer) setupStaticRoutes(r *gin.Engine) {
	// TODO: Make these paths configurable
	const webDir = "./web"
	const staticDir = webDir + "/static"

	// Serve static assets (CSS, JS, images, etc.)
	r.Static("/static", staticDir)

	// Serve common static files
	r.StaticFile("/favicon.ico", staticDir+"/favicon.ico")
	r.StaticFile("/robots.txt", staticDir+"/robots.txt")

	// Web UI routes (when implemented)
	// For now, these will return 404, but routes are ready
	r.GET("/admin", s.handleWebUI)
	r.GET("/admin/*path", s.handleWebUI)
	r.GET("/apps", s.handleWebUI)
	r.GET("/apps/*path", s.handleWebUI)
}

// handleWebUI serves the web UI
func (s *GinServer) handleWebUI(c *gin.Context) {
	// Serve the main HTML file for all web UI routes
	// This enables client-side routing for the SPA
	c.File("./web/index.html")
}
