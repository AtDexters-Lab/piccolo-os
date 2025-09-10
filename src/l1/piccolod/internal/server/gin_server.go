package server

import (
    "fmt"
    "log"
    "net/http"
    "os"
    "path/filepath"
    "io"
    "strings"
    stdfs "io/fs"

    "piccolod/internal/app"
    "piccolod/internal/services"
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

    webassets "piccolod"
)

// GinServer holds all the core components for our application using Gin framework.
type GinServer struct {
    containerManager  *container.Manager
    appManager        *app.FSManager
    serviceManager    *services.ServiceManager
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
    svcMgr := services.NewServiceManager()
    appMgr, err := app.NewFSManagerWithServices(podmanCLI, "", svcMgr)
    if err != nil {
        return nil, fmt.Errorf("failed to init app manager: %w", err)
    }

	// Set Gin to release mode for production (can be overridden by GIN_MODE env var)
	gin.SetMode(gin.ReleaseMode)

    s := &GinServer{
        containerManager:  cm,
        appManager:        appMgr,
        serviceManager:    svcMgr,
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
    port := os.Getenv("PORT")
    if port == "" {
        port = "80"
    }

    // Start mDNS advertising - this must succeed
    if err := s.mdnsManager.Start(); err != nil {
        return fmt.Errorf("FATAL: mDNS server failed to start: %w", err)
    }

    // Start background service watcher and proxies
    s.serviceManager.StartBackground()

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
    s.serviceManager.Stop()
    return nil
}

// setupGinRoutes defines all API endpoints using Gin router.
func (s *GinServer) setupGinRoutes() {
	r := gin.New()

	// Avoid implicit redirects that can cause loops during SPA routing
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false
	r.RemoveExtraSlash = false

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

		// Service discovery endpoints (v1)
		v1.GET("/services", s.handleGinServicesAll)
		v1.GET("/apps/:name/services", s.handleGinServicesByApp)

		// Demo mode: serve JSON fixtures under /api/v1/demo/* from ./testdata/api
		if os.Getenv("PICCOLO_DEMO") != "" {
			v1.Any("/demo/*path", s.handleDemoJSON)
		}
	}

	// Admin routes
	r.GET("/version", s.handleGinVersion)

	// Static file serving for web UI
	s.setupStaticRoutes(r)

	s.router = r
}

// handleDemoJSON serves JSON fixtures from ./testdata/api when PICCOLO_DEMO is set.
// Example: GET /api/v1/demo/services -> ./testdata/api/services.json
// Example: GET /api/v1/demo/apps/vaultwarden -> ./testdata/api/app_vaultwarden.json
func (s *GinServer) handleDemoJSON(c *gin.Context) {
    raw := c.Param("path") // begins with '/'
    clean := filepath.Clean(raw)
    if clean == "/" || clean == "." {
        c.JSON(http.StatusBadRequest, gin.H{"error": "missing fixture path"})
        return
    }

    // Map to fixture path under testdata/api with special handling for nested resources
    rel := filepath.ToSlash(clean[1:])
    parts := strings.Split(rel, "/")
    var fixturePath string
    if len(parts) >= 2 && parts[0] == "apps" {
        app := parts[1]
        if len(parts) == 2 {
            fixturePath = filepath.Join("testdata", "api", "app_"+app+".json")
        } else {
            sub := strings.Join(parts[2:], "/")
            fixturePath = filepath.Join("testdata", "api", "app_"+app, sub+".json")
        }
    } else if len(parts) >= 3 && parts[0] == "backup" && parts[1] == "app" {
        fixturePath = filepath.Join("testdata", "api", "backup_app_"+parts[2]+".json")
    } else if len(parts) >= 3 && parts[0] == "restore" && parts[1] == "app" {
        fixturePath = filepath.Join("testdata", "api", "restore_app_"+parts[2]+".json")
    } else {
        dir := filepath.Dir(rel)
        base := filepath.Base(rel)
        var fname string
        if dir == "." || dir == "" {
            fname = base + ".json"
        } else {
            fname = filepath.Base(dir) + "_" + base + ".json"
        }
        fixturePath = filepath.Join("testdata", "api", fname)
    }
    f, err := os.Open(fixturePath)
    if err != nil {
        // For demo actions without explicit fixtures, return a generic success to avoid UI breakage
        c.JSON(http.StatusOK, gin.H{"message": "demo"})
        return
    }
    defer f.Close()

    c.Header("Content-Type", "application/json; charset=utf-8")
    if _, err := io.Copy(c.Writer, f); err != nil {
        c.Status(http.StatusInternalServerError)
        return
    }
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

	// Otherwise serve the web UI (dev override or embedded) without triggering file-server redirects
	if uiDir := os.Getenv("PICCOLO_UI_DIR"); uiDir != "" {
		if b, err := os.ReadFile(filepath.Join(uiDir, "index.html")); err == nil {
			c.Data(http.StatusOK, "text/html; charset=utf-8", b)
			return
		}
	}
	uiFS := webassets.FS()
	if b, err := stdfs.ReadFile(uiFS, "index.html"); err == nil {
		c.Data(http.StatusOK, "text/html; charset=utf-8", b)
		return
	}
	c.String(http.StatusInternalServerError, "index.html not found")
}

func (s *GinServer) handleGinContainers(c *gin.Context) {
	// TODO: Implement container management (existing functionality)
	c.JSON(http.StatusOK, gin.H{"message": "Container management (placeholder)"})
}

// handleGinServicesAll returns all service endpoints across apps
func (s *GinServer) handleGinServicesAll(c *gin.Context) {
    eps := s.serviceManager.GetAll()
    c.JSON(http.StatusOK, gin.H{"services": eps})
}

// handleGinServicesByApp returns services for a single app
func (s *GinServer) handleGinServicesByApp(c *gin.Context) {
    name := c.Param("name")
    eps, err := s.serviceManager.GetByApp(name)
    if err != nil {
        writeGinError(c, http.StatusNotFound, err.Error())
        return
    }
    c.JSON(http.StatusOK, gin.H{"app": name, "services": eps})
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
	// Development override: serve from disk when PICCOLO_UI_DIR is set
    if uiDir := os.Getenv("PICCOLO_UI_DIR"); uiDir != "" {
        assetsDir := filepath.Join(uiDir, "assets")
        r.Static("/assets", assetsDir)
        r.GET("/assets", func(c *gin.Context) { c.Status(http.StatusNoContent) })
        // Serve branding and other public files
        if _, err := os.Stat(filepath.Join(uiDir, "branding")); err == nil {
            r.Static("/branding", filepath.Join(uiDir, "branding"))
        }
        // Favicon and robots from root if present; otherwise 204
        r.GET("/favicon.ico", func(c *gin.Context) {
            fp := filepath.Join(uiDir, "favicon.ico")
            if _, err := os.Stat(fp); err == nil {
                c.File(fp)
                return
            }
            c.Status(http.StatusNoContent)
        })
        r.GET("/robots.txt", func(c *gin.Context) {
            fp := filepath.Join(uiDir, "robots.txt")
            if _, err := os.Stat(fp); err == nil {
                c.File(fp)
                return
            }
            c.Status(http.StatusNoContent)
        })
		// Root is handled by handleGinRoot; don't register here
		// Fallback for client-side routes
		r.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
				return
			}
			c.File(filepath.Join(uiDir, "index.html"))
		})
		return
	}

	// Default: serve embedded UI from go:embed FS
    uiFS := webassets.FS()
    if assetsFS, err := stdfs.Sub(uiFS, "assets"); err == nil {
        r.StaticFS("/assets", http.FS(assetsFS))
    }
    if brandingFS, err := stdfs.Sub(uiFS, "branding"); err == nil {
        r.StaticFS("/branding", http.FS(brandingFS))
    }
    r.GET("/assets", func(c *gin.Context) { c.Status(http.StatusNoContent) })
    r.GET("/favicon.ico", func(c *gin.Context) {
        if _, err := stdfs.Stat(uiFS, "favicon.ico"); err == nil {
            c.FileFromFS("favicon.ico", http.FS(uiFS))
            return
        }
        c.Status(http.StatusNoContent)
    })
    r.GET("/robots.txt", func(c *gin.Context) {
        if _, err := stdfs.Stat(uiFS, "robots.txt"); err == nil {
            c.FileFromFS("robots.txt", http.FS(uiFS))
            return
        }
        c.Status(http.StatusNoContent)
    })
	// Root is handled by handleGinRoot; don't register here
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.FileFromFS("index.html", http.FS(uiFS))
	})
}

// handleWebUI serves the web UI
func (s *GinServer) handleWebUI(c *gin.Context) {
	// Legacy route: serve SPA index from embedded FS
	uiFS := webassets.FS()
	c.FileFromFS("index.html", http.FS(uiFS))
}
