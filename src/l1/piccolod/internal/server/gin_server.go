package server

import (
	"context"
	"errors"
	"fmt"
	stdfs "io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"piccolod/internal/app"
	authpkg "piccolod/internal/auth"
	"piccolod/internal/cluster"
	"piccolod/internal/consensus"
	"piccolod/internal/container"
	crypt "piccolod/internal/crypt"
	"piccolod/internal/events"
	"piccolod/internal/mdns"
	"piccolod/internal/persistence"
	"piccolod/internal/remote"
	"piccolod/internal/runtime/commands"
	"piccolod/internal/runtime/supervisor"
	"piccolod/internal/services"
	"piccolod/internal/storage"

	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/gin-gonic/gin"

	webassets "piccolod"
)

// GinServer holds all the core components for our application using Gin framework.
type GinServer struct {
	appManager     *app.FSManager
	serviceManager *services.ServiceManager
	storageManager *storage.Manager
	persistence    persistence.Service
	mdnsManager    *mdns.Manager
	remoteManager  *remote.Manager
	router         *gin.Engine
	version        string
	events         *events.Bus
	leadership     *cluster.Registry
	supervisor     *supervisor.Supervisor
	dispatcher     *commands.Dispatcher

	// Optional OpenAPI request validation (Phase 0)
	apiValidator *openAPIValidator

	// Auth & sessions (Phase 1)
	authManager *authpkg.Manager
	sessions    *authpkg.SessionStore
	// simple rate-limit counters for login failures
	loginFailures int

	// Crypto manager for lock/unlock of app data volumes
	cryptoManager *crypt.Manager
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
	// Create Podman CLI for app management
	podmanCLI := &container.PodmanCLI{}

	// Initialize shared infrastructure
	eventsBus := events.NewBus()
	leadershipReg := cluster.NewRegistry()
	sup := supervisor.New()
	dispatch := commands.NewDispatcher()
	consensusMgr := consensus.NewStub(leadershipReg, eventsBus)

	// Initialize app manager with filesystem state management
	svcMgr := services.NewServiceManager()
	appMgr, err := app.NewFSManagerWithServices(podmanCLI, "", svcMgr)
	if err != nil {
		return nil, fmt.Errorf("failed to init app manager: %w", err)
	}

	// Initialize persistence module (skeleton; concrete components wired later)
	persist, err := persistence.NewService(persistence.Options{
		Events:     eventsBus,
		Leadership: leadershipReg,
		Consensus:  consensusMgr,
		Dispatcher: dispatch,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to init persistence module: %w", err)
	}

	// Set Gin to release mode for production (can be overridden by GIN_MODE env var)
	gin.SetMode(gin.ReleaseMode)

	s := &GinServer{
		appManager:     appMgr,
		serviceManager: svcMgr,
		storageManager: storage.NewManager(),
		persistence:    persist,
		mdnsManager:    mdns.NewManager(),
		events:         eventsBus,
		leadership:     leadershipReg,
		supervisor:     sup,
		dispatcher:     dispatch,
	}

	s.supervisor.Register(supervisor.NewComponent("mdns", func(ctx context.Context) error {
		return s.mdnsManager.Start()
	}, func(ctx context.Context) error {
		return s.mdnsManager.Stop()
	}))

	s.supervisor.Register(supervisor.NewComponent("service-manager", func(ctx context.Context) error {
		s.serviceManager.StartBackground()
		return nil
	}, func(ctx context.Context) error {
		s.serviceManager.Stop()
		return nil
	}))

	s.supervisor.Register(supervisor.NewComponent("consensus", consensusMgr.Start, consensusMgr.Stop))
	s.supervisor.Register(newLeadershipObserver(eventsBus))

	for _, opt := range opts {
		opt(s)
	}

	// Initialize auth & sessions
	stateDir := os.Getenv("PICCOLO_STATE_DIR")
	am, err := authpkg.NewManager(stateDir)
	if err != nil {
		return nil, fmt.Errorf("auth manager init: %w", err)
	}
	s.authManager = am
	s.sessions = authpkg.NewSessionStore()

	// Initialize crypto manager
	cmgr, err := crypt.NewManager(stateDir)
	if err != nil {
		return nil, fmt.Errorf("crypto manager init: %w", err)
	}
	s.cryptoManager = cmgr

	// Remote manager
	rm, err := remote.NewManager(stateDir)
	if err != nil {
		return nil, fmt.Errorf("remote manager init: %w", err)
	}
	s.remoteManager = rm

	// Rehydrate proxies for containers that survived restarts
	appMgr.RestoreServices(context.Background())

	s.setupGinRoutes()
	return s, nil
}

// Start runs the Gin HTTP server and starts mDNS advertising.
func (s *GinServer) Start() error {
	port := os.Getenv("PORT")
	if port == "" {
		port = "80"
	}

	if err := s.supervisor.Start(context.Background()); err != nil {
		return fmt.Errorf("failed to start runtime components: %w", err)
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
	if err := s.supervisor.Stop(context.Background()); err != nil {
		log.Printf("WARN: Failed to stop components cleanly: %v", err)
		return err
	}
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

	// Optional: OpenAPI request validation (enabled when validator is initialized)
	if s.apiValidator == nil {
		// Try lazy init based on env var
		if os.Getenv("PICCOLO_API_VALIDATE") == "1" {
			if v, err := newOpenAPIValidator(); err == nil {
				s.apiValidator = v
			} else {
				log.Printf("OpenAPI validation disabled: %v", err)
			}
		}
	}
	if s.apiValidator != nil {
		r.Use(s.apiValidator.Middleware())
	}

	// Root endpoint
	r.GET("/", s.handleGinRoot)

	// API v1 group
	v1 := r.Group("/api/v1")
	{
		// Serve embedded OpenAPI document for tooling/debug (no auth)
		v1.GET("/openapi.yaml", func(c *gin.Context) {
			if b, err := loadOpenAPISpec(); err == nil {
				c.Data(http.StatusOK, "application/yaml; charset=utf-8", b)
			} else {
				c.JSON(http.StatusNotFound, gin.H{"error": "spec not found"})
			}
		})

		// Auth & sessions (no auth required)
		v1.GET("/auth/session", s.handleAuthSession)
		v1.POST("/auth/login", s.handleAuthLogin)
		v1.POST("/auth/logout", s.handleAuthLogout)
		v1.POST("/auth/password", s.handleAuthPassword)
		v1.GET("/auth/csrf", s.handleAuthCSRF)
		v1.GET("/auth/initialized", s.handleAuthInitialized)
		v1.POST("/auth/setup", s.handleAuthSetup)

		// Selected read-only status endpoints remain public
		v1.GET("/updates/os", s.handleOSUpdateStatus)
		v1.GET("/remote/status", s.handleRemoteStatus)
		v1.GET("/storage/disks", s.handleStorageDisks)
		v1.GET("/health/ready", s.handleGinReadinessCheck)

		// All other API endpoints require session + CSRF
		authed := v1.Group("/")
		authed.Use(s.requireSession())
		authed.Use(s.csrfMiddleware())

		// Crypto endpoints
		authed.GET("/crypto/status", s.handleCryptoStatus)
		authed.POST("/crypto/setup", s.handleCryptoSetup)
		authed.POST("/crypto/unlock", s.handleCryptoUnlock)
		authed.POST("/crypto/lock", s.handleCryptoLock)
		authed.GET("/crypto/recovery-key", s.handleCryptoRecoveryStatus)
		authed.POST("/crypto/recovery-key/generate", s.handleCryptoRecoveryGenerate)

		// App management endpoints
		apps := authed.Group("/apps")
		{
			apps.POST("", s.requireUnlocked(), s.handleGinAppInstall)           // POST /api/v1/apps
			apps.POST("/validate", s.handleGinAppValidate)                      // POST /api/v1/apps/validate
			apps.GET("", s.handleGinAppList)                                    // GET /api/v1/apps
			apps.GET("/:name", s.handleGinAppGet)                               // GET /api/v1/apps/:name
			apps.DELETE("/:name", s.requireUnlocked(), s.handleGinAppUninstall) // DELETE /api/v1/apps/:name

			// App actions
			apps.POST("/:name/start", s.requireUnlocked(), s.handleGinAppStart) // POST /api/v1/apps/:name/start
			apps.POST("/:name/stop", s.requireUnlocked(), s.handleGinAppStop)   // POST /api/v1/apps/:name/stop
		}

		// Remote config endpoints require auth
		authed.POST("/remote/configure", s.handleRemoteConfigure)
		authed.POST("/remote/disable", s.handleRemoteDisable)
		authed.POST("/remote/rotate", s.handleRemoteRotate)
		authed.POST("/remote/preflight", s.handleRemotePreflight)
		authed.GET("/remote/aliases", s.handleRemoteAliasesList)
		authed.POST("/remote/aliases", s.handleRemoteAliasesCreate)
		authed.DELETE("/remote/aliases/:id", s.handleRemoteAliasesDelete)
		authed.GET("/remote/certificates", s.handleRemoteCertificatesList)
		authed.POST("/remote/certificates/:id/renew", s.handleRemoteCertificateRenew)
		authed.GET("/remote/events", s.handleRemoteEvents)
		authed.GET("/remote/dns/providers", s.handleRemoteDNSProviders)
		authed.GET("/remote/nexus-guide", s.handleRemoteGuideInfo)
		authed.POST("/remote/nexus-guide/verify", s.handleRemoteGuideVerify)

		// Persistence exports (prototype)
		authed.POST("/exports/control", s.requireUnlocked(), s.handlePersistenceControlExport)
		authed.POST("/exports/full", s.requireUnlocked(), s.handlePersistenceFullExport)

		// Catalog (read-only) and services require auth
		authed.GET("/catalog", s.handleGinCatalog)
		authed.GET("/catalog/:name/template", s.handleGinCatalogTemplate)
		authed.GET("/services", s.handleGinServicesAll)
		authed.GET("/apps/:name/services", s.handleGinServicesByApp)
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

// handleGinServicesAll returns all service endpoints across apps
func (s *GinServer) handleGinServicesAll(c *gin.Context) {
	eps := s.serviceManager.GetAll()
	out := make([]gin.H, 0, len(eps))
	for _, ep := range eps {
		out = append(out, gin.H{
			"app":          ep.App,
			"name":         ep.Name,
			"guest_port":   ep.GuestPort,
			"host_port":    ep.HostBind,
			"public_port":  ep.PublicPort,
			"remote_ports": ep.RemotePorts,
			"flow":         ep.Flow,
			"protocol":     ep.Protocol,
			"middleware":   ep.Middleware,
			"scheme":       determineScheme(ep.Flow, ep.Protocol),
		})
	}
	c.JSON(http.StatusOK, gin.H{"services": out})
}

// handleGinServicesByApp returns services for a single app
func (s *GinServer) handleGinServicesByApp(c *gin.Context) {
	name := c.Param("name")
	eps, err := s.serviceManager.GetByApp(name)
	if err != nil {
		writeGinError(c, http.StatusNotFound, err.Error())
		return
	}
	out := make([]gin.H, 0, len(eps))
	for _, ep := range eps {
		out = append(out, gin.H{
			"app":          ep.App,
			"name":         ep.Name,
			"guest_port":   ep.GuestPort,
			"host_port":    ep.HostBind,
			"public_port":  ep.PublicPort,
			"remote_ports": ep.RemotePorts,
			"flow":         ep.Flow,
			"protocol":     ep.Protocol,
			"middleware":   ep.Middleware,
			"scheme":       determineScheme(ep.Flow, ep.Protocol),
		})
	}
	c.JSON(http.StatusOK, gin.H{"services": out})
}

func (s *GinServer) handlePersistenceControlExport(c *gin.Context) {
	if s.dispatcher == nil {
		writeGinError(c, http.StatusInternalServerError, "command dispatcher not available")
		return
	}
	resp, err := s.dispatcher.Dispatch(c.Request.Context(), persistence.RunControlExportCommand{})
	if err != nil {
		if errors.Is(err, persistence.ErrNotImplemented) {
			writeGinError(c, http.StatusNotImplemented, "control-plane export not implemented yet")
		} else {
			writeGinError(c, http.StatusInternalServerError, "failed to start control export: "+err.Error())
		}
		return
	}
	artifact, ok := resp.(persistence.ExportArtifact)
	if !ok {
		writeGinError(c, http.StatusInternalServerError, "unexpected response from persistence")
		return
	}
	writeGinSuccess(c, gin.H{"artifact": artifact}, "control-plane export started")
}

func (s *GinServer) handlePersistenceFullExport(c *gin.Context) {
	if s.dispatcher == nil {
		writeGinError(c, http.StatusInternalServerError, "command dispatcher not available")
		return
	}
	resp, err := s.dispatcher.Dispatch(c.Request.Context(), persistence.RunFullExportCommand{})
	if err != nil {
		if errors.Is(err, persistence.ErrNotImplemented) {
			writeGinError(c, http.StatusNotImplemented, "full export not implemented yet")
		} else {
			writeGinError(c, http.StatusInternalServerError, "failed to start full export: "+err.Error())
		}
		return
	}
	artifact, ok := resp.(persistence.ExportArtifact)
	if !ok {
		writeGinError(c, http.StatusInternalServerError, "unexpected response from persistence")
		return
	}
	writeGinSuccess(c, gin.H{"artifact": artifact}, "full export started")
}

func (s *GinServer) handleGinVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version": s.version,
		"service": "piccolod",
	})
}

func (s *GinServer) handleGinReadinessCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"ready":  true,
		"status": "healthy",
	})
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
