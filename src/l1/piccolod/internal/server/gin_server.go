package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"piccolod/internal/api"
	"piccolod/internal/app"
	authpkg "piccolod/internal/auth"
	"piccolod/internal/cluster"
	"piccolod/internal/consensus"
	"piccolod/internal/container"
	crypt "piccolod/internal/crypt"
	"piccolod/internal/events"
	"piccolod/internal/health"
	"piccolod/internal/mdns"
	"piccolod/internal/persistence"
	"piccolod/internal/remote"
	"piccolod/internal/remote/nexusclient"
	"piccolod/internal/router"
	"piccolod/internal/runtime/commands"
	"piccolod/internal/runtime/supervisor"
	"piccolod/internal/services"
	"piccolod/internal/state/paths"

	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"

	webassets "piccolod"
)

const (
	acmeHTTPFallbackPort  = services.ACMEHTTPFallbackPort
	maxStaticAssetPathLen = 4 * 1024 // guard against path-based DoS
)

var errInvalidStaticPath = errors.New("invalid static asset path")

type unlockReloader interface {
	ReloadFromStorage() error
}

// GinServer holds all the core components for our application using Gin framework.
type GinServer struct {
	appManager     *app.AppManager
	serviceManager *services.ServiceManager
	persistence    persistence.Service
	authRepo       persistence.AuthRepo
	mdnsManager    *mdns.Manager
	remoteManager  *remote.Manager
	router         *gin.Engine
	version        string
	events         *events.Bus
	leadership     *cluster.Registry
	supervisor     *supervisor.Supervisor
	dispatcher     *commands.Dispatcher
	routeManager   *router.Manager
	tlsMux         *services.TlsMux
	remoteResolver *serviceRemoteResolver

	secureSrv      *http.Server
	secureListener net.Listener
	securePort     int

	// Optional OpenAPI request validation (Phase 0)
	apiValidator *openAPIValidator

	// Auth & sessions (Phase 1)
	authManager *authpkg.Manager
	sessions    *authpkg.SessionStore
	// simple rate-limit counters for login failures
	loginFailures int
	resetFailures int

	// Crypto manager for lock/unlock of app data volumes
	cryptoManager *crypt.Manager
	healthTracker *health.Tracker

	reloadersMu     sync.RWMutex
	unlockReloaders []unlockReloader
}

type secureContextKey struct{}

var secureContextKeyInstance = secureContextKey{}

// portUnpublisherFunc adapts a function into services.PortUnpublisher.
type portUnpublisherFunc func(int)

func (f portUnpublisherFunc) Unpublish(p int) { f(p) }

// portPublisherFunc adapts a function into services.PortPublisher.
type portPublisherFunc func(int)

func (f portPublisherFunc) Publish(p int) { f(p) }

type serviceRemoteResolver struct {
	services   *services.ServiceManager
	mu         sync.RWMutex
	domain     string
	portal     string
	port       int
	tlsMuxPort int
}

func newServiceRemoteResolver(svc *services.ServiceManager) *serviceRemoteResolver {
	port := 80
	if p := os.Getenv("PORT"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			port = v
		}
	}
	return &serviceRemoteResolver{services: svc, port: port}
}

func (r *serviceRemoteResolver) UpdateConfig(cfg nexusclient.Config) {
	r.mu.Lock()
	r.portal = strings.TrimSuffix(strings.ToLower(cfg.PortalHostname), ".")
	domain := strings.TrimSuffix(strings.ToLower(cfg.TLD), ".")
	if domain == "" && r.portal != "" {
		if idx := strings.Index(r.portal, "."); idx != -1 {
			domain = r.portal[idx+1:]
		}
	}
	r.domain = domain
	r.mu.Unlock()
}

func (r *serviceRemoteResolver) IsRemoteHostname(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	r.mu.RLock()
	portal := r.portal
	domain := r.domain
	r.mu.RUnlock()
	if host == "" {
		return false
	}
	if portal != "" && host == portal {
		return true
	}
	if domain != "" {
		if host == domain {
			return true
		}
		if strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func (r *serviceRemoteResolver) SetTlsMuxPort(p int) { r.mu.Lock(); r.tlsMuxPort = p; r.mu.Unlock() }

func (r *serviceRemoteResolver) RecordConnectionHint(localPort, sourcePort, remotePort int, isTLS bool) {
	if r.services == nil || sourcePort <= 0 {
		return
	}
	if localPort == r.port {
		return
	}
	r.services.RegisterProxyHint(localPort, sourcePort, remotePort, isTLS)
}

func (r *serviceRemoteResolver) Resolve(hostname string, remotePort int, isTLS bool) (int, bool) {
	h := strings.TrimSuffix(strings.ToLower(hostname), ".")
	r.mu.RLock()
	portal := r.portal
	domain := r.domain
	portalPort := r.port
	tlsMuxPort := r.tlsMuxPort
	r.mu.RUnlock()

	normPort := remotePort
	if normPort == acmeHTTPFallbackPort {
		normPort = 80
	}

	// Portal host: treat as flow=tcp (device-terminated TLS when not 80)
	if portal != "" && h == portal {
		if normPort == 80 {
			return portalPort, true
		}
		if isTLS && tlsMuxPort > 0 {
			return tlsMuxPort, true
		}
		// Fallback to portalPort if mux not running (unit tests)
		return portalPort, true
	}

	listener := ""
	if domain != "" {
		suffix := "." + domain
		if strings.HasSuffix(h, suffix) {
			label := h[:len(h)-len(suffix)]
			if idx := strings.Index(label, "."); idx != -1 {
				label = label[:idx]
			}
			listener = label
		}
	} else if idx := strings.Index(h, "."); idx != -1 {
		listener = h[:idx]
	}

	// Listener host
	if listener != "" {
		if ep, ok := r.services.ResolveListener(listener, normPort); ok {
			if ep.Flow == api.FlowTLS {
				return ep.PublicPort, true
			}
			if normPort == 80 {
				return ep.PublicPort, true
			}
			if isTLS && tlsMuxPort > 0 {
				return tlsMuxPort, true
			}
			return ep.PublicPort, true
		}
	}

	// Fallback by port only (rare): apply same flow policy when we find an ep
	if ep, ok := r.services.ResolveByRemotePort(normPort); ok {
		if ep.Flow == api.FlowTLS {
			return ep.PublicPort, true
		}
		if normPort == 80 {
			return ep.PublicPort, true
		}
		if isTLS && tlsMuxPort > 0 {
			return tlsMuxPort, true
		}
		return ep.PublicPort, true
	}

	return 0, false
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
	stateDir := paths.Root()
	cmgr, err := crypt.NewManager(stateDir)
	if err != nil {
		return nil, fmt.Errorf("crypto manager init: %w", err)
	}
	healthTracker := health.NewTracker()

	// Initialize app manager with filesystem state management
	svcMgr := services.NewServiceManager()
	routeMgr := router.NewManager()
	remoteResolver := newServiceRemoteResolver(svcMgr)
	svcMgr.ObserveRuntimeEvents(eventsBus)
	// TLS mux (loopback, remote-only) — created now, started when remote is configured
	tlsMux := services.NewTlsMux(svcMgr)
	// Wire ACME HTTP-01 handler into HTTP proxies (set after remote manager init)
	appMgr, err := app.NewAppManagerWithServices(podmanCLI, "", svcMgr, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to init app manager: %w", err)
	}
	appMgr.ObserveRuntimeEvents(eventsBus)
	appMgr.SetRouter(routeMgr)

	// Initialize persistence module (skeleton; concrete components wired later)
	persist, err := persistence.NewService(persistence.Options{
		Events:     eventsBus,
		Leadership: leadershipReg,
		Consensus:  consensusMgr,
		Dispatcher: dispatch,
		Crypto:     cmgr,
		StateDir:   stateDir,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to init persistence module: %w", err)
	}

	controlDir := persist.ControlVolume().MountDir
	if strings.TrimSpace(controlDir) == "" {
		return nil, fmt.Errorf("control volume mount unavailable")
	}
	// NOTE: Today we do not migrate existing app state into the control volume because we have
	// no pre-existing deployments. If that assumption changes we must add a migration path,
	// otherwise legacy installations would appear empty after upgrade.
	appMgr.SetStateBaseDir(controlDir)
	appMgr.SetLockReader(persist)
	svcMgr.SetLockReader(persist)

	// Set Gin to release mode for production (can be overridden by GIN_MODE env var)
	gin.SetMode(gin.ReleaseMode)

	// Dispatcher middleware slot available for metrics/auditing; lock/leader
	// enforcement is handled in persistence and managers to avoid duplication.

	mdnsDisabled := os.Getenv("PICCOLO_DISABLE_MDNS") == "1"
	var mdnsMgr *mdns.Manager
	if !mdnsDisabled {
		mdnsMgr = mdns.NewManager()
	}

	s := &GinServer{
		appManager:     appMgr,
		serviceManager: svcMgr,
		persistence:    persist,
		mdnsManager:    mdnsMgr,
		routeManager:   routeMgr,
		tlsMux:         tlsMux,
		remoteResolver: remoteResolver,
		events:         eventsBus,
		leadership:     leadershipReg,
		supervisor:     sup,
		dispatcher:     dispatch,
		cryptoManager:  cmgr,
		healthTracker:  healthTracker,
	}
	// Seed baseline health statuses
	healthTracker.Setf("http", health.LevelOK, "HTTP server initialized")
	healthTracker.Setf("app-manager", health.LevelWarn, "app manager gated by lock state")
	healthTracker.Setf("service-manager", health.LevelOK, "service manager running")
	if mdnsDisabled {
		healthTracker.Setf("mdns", health.LevelWarn, "mdns disabled via PICCOLO_DISABLE_MDNS")
	} else {
		healthTracker.Setf("mdns", health.LevelOK, "mdns supervisor registered")
	}
	healthTracker.Setf("remote", health.LevelWarn, "remote manager initializing")
	healthTracker.Setf("persistence", health.LevelWarn, "control store locked")

	if !mdnsDisabled {
		s.supervisor.Register(supervisor.NewComponent("mdns", func(ctx context.Context) error {
			return s.mdnsManager.Start()
		}, func(ctx context.Context) error {
			return s.mdnsManager.Stop()
		}))
	}

	s.supervisor.Register(supervisor.NewComponent("service-manager", func(ctx context.Context) error {
		s.serviceManager.StartBackground()
		return nil
	}, func(ctx context.Context) error {
		s.serviceManager.Stop()
		return nil
	}))

	s.supervisor.Register(supervisor.NewComponent("consensus", consensusMgr.Start, consensusMgr.Stop))
	s.supervisor.Register(newLeadershipObserver(eventsBus))
	s.observeLockState(eventsBus)
	s.observeLeadership(eventsBus)
	s.observeRemoteConfig(eventsBus)

	for _, opt := range opts {
		opt(s)
	}

	// Initialize auth & sessions
	authRepo := persist.Control().Auth()
	authStorage := newPersistenceAuthStorage(authRepo)
	var am *authpkg.Manager
	if authStorage != nil {
		am, err = authpkg.NewManagerWithStorage(authStorage)
	} else {
		am, err = authpkg.NewManager(stateDir)
	}
	if err != nil {
		return nil, fmt.Errorf("auth manager init: %w", err)
	}
	s.authManager = am
	s.sessions = authpkg.NewSessionStore()
	s.authRepo = authRepo

	// Remote manager
	bootstrapDir := persist.BootstrapVolume().MountDir
	if strings.TrimSpace(bootstrapDir) == "" {
		return nil, fmt.Errorf("bootstrap volume mount unavailable")
	}
	remoteStorage := newBootstrapRemoteStorage(persist.Control().Remote(), bootstrapDir)
	var rm *remote.Manager
	if remoteStorage != nil {
		rm, err = remote.NewManagerWithStorage(remoteStorage, bootstrapDir)
	} else {
		rm, err = remote.NewManager(bootstrapDir)
	}
	if err != nil {
		return nil, fmt.Errorf("remote manager init: %w", err)
	}
	s.remoteManager = rm
	s.registerUnlockReloader(rm)
	rm.SetEventsBus(eventsBus)
	// Now that remote manager exists, wire ACME challenge handler and cert provider
	if rm != nil && svcMgr != nil {
		svcMgr.ProxyManager().SetAcmeHandler(rm.HTTPChallengeHandler())
		certProv := remote.NewFileCertProvider(rm.CertDirectory())
		tlsMux.SetCertProvider(certProv)
	}
	var nexusAdapter nexusclient.Adapter
	if os.Getenv("PICCOLO_NEXUS_USE_STUB") == "1" {
		nexusAdapter = nexusclient.NewStub()
	} else {
		nexusAdapter = nexusclient.NewBackendAdapter(routeMgr, remoteResolver)
	}
	rm.SetNexusAdapter(nexusAdapter)
	remote.RegisterHandlers(dispatch, rm)
	s.healthTracker.Setf("remote", health.LevelOK, "remote manager ready")
	s.refreshRemoteRuntime()

	// (Simplified) No dynamic port publish/unpublish wiring; allow dial to fail gracefully.

	// Rehydrate proxies for containers that survived restarts
	appMgr.RestoreServices(context.Background())

	s.setupGinRoutes()
	if err := s.initSecureLoopback(); err != nil {
		return nil, fmt.Errorf("secure loopback init: %w", err)
	}
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

	s.startSecureLoopback()

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
	if s.appManager != nil {
		s.appManager.StopRuntimeEvents()
	}
	s.stopSecureLoopback()
	if err := s.supervisor.Stop(context.Background()); err != nil {
		log.Printf("WARN: Failed to stop components cleanly: %v", err)
		return err
	}
	return nil
}

// setupGinRoutes defines all API endpoints using Gin router.
func (s *GinServer) setupGinRoutes() {
	r := gin.New()

	// Add basic middleware
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.Use(gzip.Gzip(gzip.DefaultCompression))
	r.Use(s.corsMiddleware())
	r.Use(s.httpsRedirectMiddleware())
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

	// ACME HTTP-01 challenge for portal hostname
	r.GET("/.well-known/acme-challenge/:token", func(c *gin.Context) {
		if s.remoteManager == nil {
			c.Status(http.StatusNotFound)
			return
		}
		h := s.remoteManager.HTTPChallengeHandler()
		if h == nil {
			c.Status(http.StatusNotFound)
			return
		}
		// Delegate to handler (ensures correct content-type and body)
		h.ServeHTTP(c.Writer, c.Request)
	})

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

		// Auth & sessions (selected public endpoints)
		v1.GET("/auth/session", s.handleAuthSession)
		v1.GET("/auth/initialized", s.handleAuthInitialized)
		v1.POST("/auth/login", s.handleAuthLogin)
		v1.POST("/auth/setup", s.handleAuthSetup)

		// Selected read-only status endpoints remain public
		v1.GET("/updates/os", s.handleOSUpdateStatus)
		v1.GET("/remote/status", s.handleRemoteStatus)
		v1.GET("/storage/disks", s.handleStorageDisks)
		v1.GET("/health/live", s.handleHealthLive)
		v1.GET("/health/ready", s.handleGinReadinessCheck)
		v1.GET("/health/detail", s.handleHealthDetail)

		// Allow unlocking without a session to break the initial lock/setup cycle.
		// Crypto: expose status/setup/unlock publicly to break circular dependency with sessions.
		v1.GET("/crypto/status", s.handleCryptoStatus)
		v1.POST("/crypto/setup", s.handleCryptoSetup)
		v1.POST("/crypto/unlock", s.handleCryptoUnlock)
		v1.POST("/crypto/reset-password", s.handleCryptoResetPassword)
		v1.GET("/crypto/recovery-key", s.handleCryptoRecoveryStatus)

		// All other API endpoints require session + CSRF
		authed := v1.Group("/")
		authed.Use(s.requireSession())
		authed.Use(s.csrfMiddleware())

		// Crypto endpoints (session required for lock/recovery management)
		authed.POST("/crypto/lock", s.handleCryptoLock)
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

		// Auth-only endpoints
		authed.POST("/auth/logout", s.handleAuthLogout)
		authed.POST("/auth/password", s.handleAuthPassword)
		authed.POST("/auth/staleness/ack", s.handleAuthStalenessAck)
		authed.GET("/auth/csrf", s.handleAuthCSRF)

		// Catalog (read-only) and services require auth
		authed.GET("/catalog", s.handleGinCatalog)
		authed.GET("/catalog/:name/template", s.handleGinCatalogTemplate)
		authed.GET("/services", s.handleGinServicesAll)
		authed.GET("/apps/:name/services", s.handleGinServicesByApp)
	}

	// Admin routes
	r.GET("/version", s.handleGinVersion)

	// Static file serving for web UI and fallback
	r.NoRoute(func(c *gin.Context) {
		if c.Request.Method == http.MethodGet {
			requestedPath := c.Request.URL.Path
			if strings.HasSuffix(requestedPath, "/") {
				requestedPath += "entry.html"
			}

			fspath := "web" + requestedPath
			if _, err := fs.Stat(webassets.FS, fspath); err != nil {
				fspath = "web/entry.html"
			}
			c.FileFromFS(fspath, http.FS(webassets.FS))
		} else {
			c.Status(http.StatusNotFound)
		}
	})

	s.router = r
}

// handleGinServicesAll returns all service endpoints across apps
func (s *GinServer) handleGinServicesAll(c *gin.Context) {
	eps := s.serviceManager.GetAll()
	out := make([]gin.H, 0, len(eps))
	var remoteStatus *remote.Status
	if s.remoteManager != nil {
		st := s.remoteManager.Status()
		remoteStatus = &st
	}
	for _, ep := range eps {
		remoteHost := s.remoteServiceHostname(remoteStatus, ep)
		var remoteHostValue interface{}
		if remoteHost != "" {
			remoteHostValue = remoteHost
		}
		out = append(out, gin.H{
			"app":          ep.App,
			"name":         ep.Name,
			"guest_port":   ep.GuestPort,
			"host_port":    ep.HostBind,
			"public_port":  ep.PublicPort,
			"remote_ports": ep.RemotePorts,
			"remote_host":  remoteHostValue,
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
	var remoteStatus *remote.Status
	if s.remoteManager != nil {
		st := s.remoteManager.Status()
		remoteStatus = &st
	}
	for _, ep := range eps {
		remoteHost := s.remoteServiceHostname(remoteStatus, ep)
		var remoteHostValue interface{}
		if remoteHost != "" {
			remoteHostValue = remoteHost
		}
		out = append(out, gin.H{
			"app":          ep.App,
			"name":         ep.Name,
			"guest_port":   ep.GuestPort,
			"host_port":    ep.HostBind,
			"public_port":  ep.PublicPort,
			"remote_ports": ep.RemotePorts,
			"remote_host":  remoteHostValue,
			"flow":         ep.Flow,
			"protocol":     ep.Protocol,
			"middleware":   ep.Middleware,
			"scheme":       determineScheme(ep.Flow, ep.Protocol),
		})
	}
	c.JSON(http.StatusOK, gin.H{"services": out})
}

func (s *GinServer) remoteServiceHostname(status *remote.Status, ep services.ServiceEndpoint) string {
	if s == nil || status == nil || !status.Enabled {
		return ""
	}
	tld := strings.Trim(strings.TrimSuffix(strings.ToLower(status.TLD), "."), " ")
	if tld == "" {
		return ""
	}
	name := strings.TrimSpace(ep.Name)
	if name == "" {
		return ""
	}
	label := strings.ToLower(name)
	if !isValidDNSLabel(label) {
		return ""
	}
	return label + "." + tld
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

func (s *GinServer) registerUnlockReloader(r unlockReloader) {
	if s == nil || r == nil {
		return
	}
	s.reloadersMu.Lock()
	s.unlockReloaders = append(s.unlockReloaders, r)
	s.reloadersMu.Unlock()
}

func (s *GinServer) reloadComponentsAfterUnlock() {
	if s == nil {
		return
	}
	s.reloadersMu.RLock()
	reloaders := append([]unlockReloader(nil), s.unlockReloaders...)
	s.reloadersMu.RUnlock()
	for _, r := range reloaders {
		if r == nil {
			continue
		}
		if err := r.ReloadFromStorage(); err != nil {
			log.Printf("WARN: unlock reload failed: %v", err)
		}
	}
}

func (s *GinServer) refreshRemoteRuntime() {
	if s == nil || s.remoteManager == nil {
		return
	}
	status := s.remoteManager.Status()
	s.applyRemoteRuntimeFromStatus(status)
}

func (s *GinServer) applyRemoteRuntimeFromStatus(status remote.Status) {
	if s == nil || s.tlsMux == nil {
		return
	}
	if s.remoteResolver != nil {
		s.remoteResolver.UpdateConfig(nexusclient.Config{
			PortalHostname: status.PortalHostname,
			TLD:            status.TLD,
		})
	}
	s.tlsMux.UpdateConfig(status.PortalHostname, status.TLD, s.resolvePortalPort())
	if status.Enabled && strings.TrimSpace(status.PortalHostname) != "" {
		if port, err := s.tlsMux.Start(); err == nil {
			if s.remoteResolver != nil {
				s.remoteResolver.SetTlsMuxPort(port)
			}
		} else {
			log.Printf("WARN: TLS mux start failed: %v", err)
		}
	} else {
		s.tlsMux.Stop()
		if s.remoteResolver != nil {
			s.remoteResolver.SetTlsMuxPort(0)
		}
	}
}

func (s *GinServer) observeLockState(bus *events.Bus) {
	if bus == nil || s.healthTracker == nil {
		return
	}
	ch := bus.Subscribe(events.TopicLockStateChanged, 8)
	go func() {
		for evt := range ch {
			payload, ok := evt.Payload.(events.LockStateChanged)
			if !ok {
				continue
			}
			if payload.Locked {
				s.healthTracker.Setf("persistence", health.LevelWarn, "control store locked")
				s.healthTracker.Setf("app-manager", health.LevelWarn, "app manager gated by lock state")
			} else {
				s.healthTracker.Setf("persistence", health.LevelOK, "control store unlocked")
				s.healthTracker.Setf("app-manager", health.LevelOK, "app manager ready")
				s.reloadComponentsAfterUnlock()
			}
		}
	}()
}

func (s *GinServer) observeLeadership(bus *events.Bus) {
	if bus == nil || s.healthTracker == nil {
		return
	}
	ch := bus.Subscribe(events.TopicLeadershipRoleChanged, 8)
	go func() {
		for evt := range ch {
			payload, ok := evt.Payload.(events.LeadershipChanged)
			if !ok {
				continue
			}
			if payload.Resource != cluster.ResourceKernel {
				continue
			}
			// Standby (follower) is not a degraded state for the control plane in single-node context.
			// Reflect role in the message but keep LevelOK.
			if s.routeManager != nil {
				mode := router.ModeLocal
				if payload.Role == cluster.RoleFollower {
					mode = router.ModeTunnel
				}
				s.routeManager.RegisterKernelRoute(mode, "")
			}
			switch payload.Role {
			case cluster.RoleLeader:
				s.healthTracker.Setf("service-manager", health.LevelOK, "service manager role=leader")
			case cluster.RoleFollower:
				s.healthTracker.Setf("service-manager", health.LevelOK, "service manager role=follower (standby)")
			}
		}
	}()
}

func (s *GinServer) observeRemoteConfig(bus *events.Bus) {
	if bus == nil {
		return
	}
	ch := bus.Subscribe(events.TopicRemoteConfigChanged, 8)
	go func() {
		for evt := range ch {
			status, ok := evt.Payload.(remote.Status)
			if !ok {
				continue
			}
			s.applyRemoteRuntimeFromStatus(status)
		}
	}()
}

func (s *GinServer) handleGinReadinessCheck(c *gin.Context) {
	if s.healthTracker == nil {
		c.JSON(http.StatusOK, gin.H{"ready": true, "status": "unknown"})
		return
	}
	required := []string{"persistence", "app-manager", "service-manager"}
	ready, snapshot := s.healthTracker.Ready(required...)
	payload := gin.H{
		"ready":      ready,
		"status":     s.healthTracker.Overall().String(),
		"components": flattenHealth(snapshot),
	}
	// TODO(ballast): once the health tracker distinguishes fatal states (e.g. control
	// store cannot unlock due to corruption), emit 503 here so MicroOS can roll
	// back automatically. For now we always return 200 to stay compatible with
	// piccolod-health-check-prod.sh which only inspects the status code.
	c.JSON(http.StatusOK, payload)
}

func (s *GinServer) handleHealthLive(c *gin.Context) {
	overall := "unknown"
	if s.healthTracker != nil {
		overall = s.healthTracker.Overall().String()
	}
	c.JSON(http.StatusOK, gin.H{"status": overall})
}

func (s *GinServer) handleHealthDetail(c *gin.Context) {
	if s.healthTracker == nil {
		c.JSON(http.StatusOK, gin.H{"overall": "unknown", "components": []gin.H{}})
		return
	}
	snapshot := s.healthTracker.Snapshot()
	c.JSON(http.StatusOK, gin.H{
		"overall":    s.healthTracker.Overall().String(),
		"components": flattenHealth(snapshot),
	})
}

func flattenHealth(snapshot map[string]health.Status) []gin.H {
	components := make([]gin.H, 0, len(snapshot))
	for name, st := range snapshot {
		components = append(components, gin.H{
			"name":       name,
			"level":      st.Level.String(),
			"message":    st.Message,
			"details":    st.Details,
			"updated_at": st.UpdatedAt,
		})
	}
	return components
}

func (s *GinServer) initSecureLoopback() error {
	if s == nil {
		return nil
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	s.secureListener = ln
	if addr, ok := ln.Addr().(*net.TCPAddr); ok {
		s.securePort = addr.Port
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), secureContextKeyInstance, true)
		s.router.ServeHTTP(w, r.WithContext(ctx))
	})
	s.secureSrv = &http.Server{
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	return nil
}

func (s *GinServer) startSecureLoopback() {
	if s == nil || s.secureSrv == nil || s.secureListener == nil {
		return
	}
	go func() {
		if err := s.secureSrv.Serve(s.secureListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("WARN: secure loopback server stopped: %v", err)
		}
	}()
	log.Printf("INFO: Secure loopback portal listening on 127.0.0.1:%d", s.securePort)
}

func (s *GinServer) stopSecureLoopback() {
	if s == nil || s.secureSrv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.secureSrv.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("WARN: secure loopback shutdown failed: %v", err)
	}
	s.secureSrv = nil
	s.secureListener = nil
	s.securePort = 0
}

func (s *GinServer) httpsRedirectMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s == nil || s.remoteResolver == nil {
			c.Next()
			return
		}
		if strings.HasPrefix(c.Request.URL.Path, "/.well-known/acme-challenge/") {
			c.Next()
			return
		}
		host := canonicalHost(c.Request.Host)
		if host == "" {
			c.Next()
			return
		}
		// Local development and mDNS names (e.g., piccolo.local) should remain HTTP even
		// if the remote resolver is configured with a matching TLD.
		if strings.HasSuffix(host, ".local") || host == "localhost" || host == "127.0.0.1" {
			c.Next()
			return
		}
		log.Println("[DEBUG] doing redirect")
		if net.ParseIP(host) != nil {
			c.Next()
			return
		}
		if !s.remoteResolver.IsRemoteHostname(host) {
			c.Next()
			return
		}
		if s.isSecureRequest(c.Request) {
			c.Next()
			return
		}
		target := "https://" + host + c.Request.URL.RequestURI()
		c.Redirect(http.StatusMovedPermanently, target)
		c.Abort()
	}
}

func (s *GinServer) isSecureRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	if v := r.Context().Value(secureContextKeyInstance); v != nil {
		return true
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	return false
}

func canonicalHost(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if i := strings.Index(v, ","); i != -1 {
		v = v[:i]
	}
	if strings.HasPrefix(v, "[") {
		if idx := strings.Index(v, "]"); idx != -1 {
			v = v[1:idx]
		}
	} else {
		if h, _, err := net.SplitHostPort(v); err == nil {
			v = h
		} else if i := strings.Index(v, ":"); i != -1 {
			v = v[:i]
		}
	}
	v = strings.Trim(v, "[]")
	return strings.TrimSuffix(strings.ToLower(v), ".")
}
