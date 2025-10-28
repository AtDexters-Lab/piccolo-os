package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"piccolod/internal/api"
	"piccolod/internal/app"
	"piccolod/internal/persistence"
	"piccolod/internal/remote"
)

func determineScheme(flow api.ListenerFlow, protocol api.ListenerProtocol) string {
	switch protocol {
	case api.ListenerProtocolHTTP:
		if flow == api.FlowTLS {
			return "https"
		}
		return "http"
	case api.ListenerProtocolWebsocket:
		if flow == api.FlowTLS {
			return "wss"
		}
		return "ws"
	default:
		if flow == api.FlowTLS {
			return "https"
		}
		return "http"
	}
}

func (s *GinServer) queueAppRemoteCertificates(appName string) {
	if s == nil || s.remoteManager == nil || s.serviceManager == nil {
		return
	}
	status := s.remoteManager.Status()
	if !status.Enabled {
		return
	}
	if !strings.EqualFold(status.Solver, "http-01") {
		return
	}
	tld := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(status.TLD)), ".")
	if tld == "" {
		return
	}
	endpoints, err := s.serviceManager.GetByApp(appName)
	if err != nil {
		log.Printf("WARN: remote: queue certificates for app %s: %v", appName, err)
		return
	}
	hosts := map[string]struct{}{}
	for _, ep := range endpoints {
		if ep.Flow == api.FlowTLS {
			continue
		}
		switch ep.Protocol {
		case api.ListenerProtocolHTTP, api.ListenerProtocolWebsocket:
			// allowed
		default:
			continue
		}
		name := strings.ToLower(strings.TrimSpace(ep.Name))
		if name == "" {
			continue
		}
		if !isValidDNSLabel(name) {
			log.Printf("WARN: remote: skipping remote certificate queue for listener %q on app %q (not DNS-safe)", ep.Name, appName)
			continue
		}
		host := name + "." + tld
		hosts[host] = struct{}{}
	}
	for h := range hosts {
		s.remoteManager.QueueHostnameCertificate(h)
	}
}

func isValidDNSLabel(label string) bool {
	if label == "" || len(label) > 63 {
		return false
	}
	if label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for i := 0; i < len(label); i++ {
		ch := label[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' {
			continue
		}
		return false
	}
	return true
}

// APIError represents a structured API error response
type APIError struct {
	Error   string `json:"error"`
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
}

// GinAppResponse represents the standardized API response format
type GinAppResponse struct {
	Data    interface{} `json:"data,omitempty"`
	Message string      `json:"message,omitempty"`
	Error   *APIError   `json:"error,omitempty"`
}

// writeGinError writes a structured error response using Gin
func writeGinError(c *gin.Context, statusCode int, message string) {
	response := GinAppResponse{
		Error: &APIError{
			Error:   http.StatusText(statusCode),
			Code:    statusCode,
			Message: message,
		},
	}
	c.JSON(statusCode, response)
}

// writeGinSuccess writes a successful response using Gin
func writeGinSuccess(c *gin.Context, data interface{}, message string) {
	response := GinAppResponse{
		Data:    data,
		Message: message,
	}
	c.JSON(http.StatusOK, response)
}

// handleGinAppValidate handles POST /api/v1/apps/validate - Validate app.yaml without installing
func (s *GinServer) handleGinAppValidate(c *gin.Context) {
	contentType := c.GetHeader("Content-Type")
	if !strings.Contains(contentType, "application/x-yaml") && !strings.Contains(contentType, "text/yaml") && !strings.Contains(contentType, "application/json") {
		writeGinError(c, http.StatusUnsupportedMediaType, "Content-Type must be application/x-yaml or text/yaml or application/json")
		return
	}
	var yamlData []byte
	if strings.Contains(contentType, "application/json") {
		// Accept { app_definition: "...yaml..." }
		var req struct {
			AppDefinition string `json:"app_definition"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.AppDefinition) == "" {
			writeGinError(c, http.StatusBadRequest, "Invalid JSON body; expected {app_definition}")
			return
		}
		yamlData = []byte(req.AppDefinition)
	} else {
		body, err := c.GetRawData()
		if err != nil || len(body) == 0 {
			writeGinError(c, http.StatusBadRequest, "Request body cannot be empty")
			return
		}
		yamlData = body
	}
	if _, err := app.ParseAppDefinition(yamlData); err != nil {
		writeGinError(c, http.StatusBadRequest, "Invalid app.yaml: "+err.Error())
		return
	}
	writeGinSuccess(c, gin.H{"valid": true}, "valid")
}

// handleGinCatalogTemplate handles GET /api/v1/catalog/:name/template - return YAML template for a catalog app
func (s *GinServer) handleGinCatalogTemplate(c *gin.Context) {
	name := strings.ToLower(strings.TrimSpace(c.Param("name")))
	var yaml string
	switch name {
	case "wordpress":
		yaml = "name: wordpress\nimage: docker.io/library/wordpress:6\nlisteners:\n  - name: http\n    guest_port: 80\n    flow: tcp\n    protocol: http\n"
	default:
		c.JSON(http.StatusNotFound, gin.H{"error": "template not found"})
		return
	}
	c.Data(http.StatusOK, "application/x-yaml; charset=utf-8", []byte(yaml))
}

// handleGinAppInstall handles POST /api/v1/apps - Install app from app.yaml upload
func (s *GinServer) handleGinAppInstall(c *gin.Context) {
	// Check Content-Type
	contentType := c.GetHeader("Content-Type")
	if !strings.Contains(contentType, "application/x-yaml") && !strings.Contains(contentType, "text/yaml") {
		writeGinError(c, http.StatusUnsupportedMediaType, "Content-Type must be application/x-yaml or text/yaml")
		return
	}

	// Read request body
	yamlData, err := c.GetRawData()
	if err != nil {
		writeGinError(c, http.StatusBadRequest, "Failed to read request body: "+err.Error())
		return
	}

	if len(yamlData) == 0 {
		writeGinError(c, http.StatusBadRequest, "Request body cannot be empty")
		return
	}

	// Parse app.yaml
	appDef, err := app.ParseAppDefinition(yamlData)
	if err != nil {
		writeGinError(c, http.StatusBadRequest, "Invalid app.yaml: "+err.Error())
		return
	}

	if err := s.ensureAppVolume(c.Request.Context(), appDef); err != nil {
		writeGinError(c, http.StatusInternalServerError, err.Error())
		return
	}

	// Install or update (upsert) the app
	appInstance, err := s.appManager.Upsert(c.Request.Context(), appDef)
	if err != nil {
		if handleAppManagerError(c, err, "install app") {
			return
		}
		writeGinError(c, http.StatusInternalServerError, "Failed to install app: "+err.Error())
		return
	}

	s.queueAppRemoteCertificates(appInstance.Name)

	response := GinAppResponse{
		Data:    appInstance,
		Message: "App '" + appInstance.Name + "' installed successfully",
	}
	c.JSON(http.StatusCreated, response)
}

// handleGinAppList handles GET /api/v1/apps - List all apps with status
func (s *GinServer) handleGinAppList(c *gin.Context) {
	apps, err := s.appManager.List(c.Request.Context())
	if err != nil {
		if handleAppManagerError(c, err, "list apps") {
			return
		}
		writeGinError(c, http.StatusInternalServerError, "Failed to list apps: "+err.Error())
		return
	}

	writeGinSuccess(c, apps, fmt.Sprintf("Found %d apps", len(apps)))
}

// handleGinAppGet handles GET /api/v1/apps/:name - Get specific app details
func (s *GinServer) handleGinAppGet(c *gin.Context) {
	appName := c.Param("name")

	appInstance, err := s.appManager.Get(c.Request.Context(), appName)
	if err != nil {
		if handleAppManagerError(c, err, "fetch app") {
			return
		}
		if strings.Contains(err.Error(), "not found") {
			writeGinError(c, http.StatusNotFound, err.Error())
		} else {
			writeGinError(c, http.StatusInternalServerError, "Failed to get app: "+err.Error())
		}
		return
	}

	// Include services inline
	services, _ := s.serviceManager.GetByApp(appName)
	serviceStatus := make([]gin.H, 0, len(services))
	var remoteStatus *remote.Status
	if s.remoteManager != nil {
		st := s.remoteManager.Status()
		remoteStatus = &st
	}
	for _, ep := range services {
		remoteHost := s.remoteServiceHostname(remoteStatus, ep)
		var remoteHostValue interface{}
		if remoteHost != "" {
			remoteHostValue = remoteHost
		}
		serviceStatus = append(serviceStatus, gin.H{
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
	writeGinSuccess(c, gin.H{"app": appInstance, "services": serviceStatus}, "")
}

// handleGinAppUninstall handles DELETE /api/v1/apps/:name - Uninstall app completely
func (s *GinServer) handleGinAppUninstall(c *gin.Context) {
	appName := c.Param("name")
	// Optional purge=true to delete app data
	purge := false
	switch c.Query("purge") {
	case "1", "true", "yes", "on":
		purge = true
	}

	err := s.appManager.UninstallWithOptions(c.Request.Context(), appName, purge)
	if err != nil {
		if handleAppManagerError(c, err, "uninstall app") {
			return
		}
		if strings.Contains(err.Error(), "not found") {
			writeGinError(c, http.StatusNotFound, err.Error())
		} else {
			writeGinError(c, http.StatusInternalServerError, "Failed to uninstall app: "+err.Error())
		}
		return
	}

	if purge {
		writeGinSuccess(c, nil, "App '"+appName+"' uninstalled and data purged successfully")
	} else {
		writeGinSuccess(c, nil, "App '"+appName+"' uninstalled successfully")
	}
}

// handleGinAppStart handles POST /api/v1/apps/:name/start - Start app container
func (s *GinServer) handleGinAppStart(c *gin.Context) {
	appName := c.Param("name")
	// Demo mode: simulate success without backend
	if os.Getenv("PICCOLO_DEMO") != "" {
		writeGinSuccess(c, nil, "App '"+appName+"' started successfully")
		return
	}

	err := s.appManager.Start(c.Request.Context(), appName)
	if err != nil {
		if handleAppManagerError(c, err, "start app") {
			return
		}
		if strings.Contains(err.Error(), "not found") {
			writeGinError(c, http.StatusNotFound, err.Error())
		} else {
			writeGinError(c, http.StatusInternalServerError, "Failed to start app: "+err.Error())
		}
		return
	}

	writeGinSuccess(c, nil, "App '"+appName+"' started successfully")
}

// handleGinAppStop handles POST /api/v1/apps/:name/stop - Stop app container
func (s *GinServer) handleGinAppStop(c *gin.Context) {
	appName := c.Param("name")
	// Demo mode: simulate success without backend
	if os.Getenv("PICCOLO_DEMO") != "" {
		writeGinSuccess(c, nil, "App '"+appName+"' stopped successfully")
		return
	}

	err := s.appManager.Stop(c.Request.Context(), appName)
	if err != nil {
		if handleAppManagerError(c, err, "stop app") {
			return
		}
		if strings.Contains(err.Error(), "not found") {
			writeGinError(c, http.StatusNotFound, err.Error())
		} else {
			writeGinError(c, http.StatusInternalServerError, "Failed to stop app: "+err.Error())
		}
		return
	}

	writeGinSuccess(c, nil, "App '"+appName+"' stopped successfully")
}

// handleGinCatalog handles GET /api/v1/catalog - returns curated catalog.
func (s *GinServer) handleGinCatalog(c *gin.Context) {
	apps := []gin.H{
		{
			"name":        "wordpress",
			"image":       "docker.io/library/wordpress:6",
			"description": "WordPress + SQLite",
			"template":    "name: wordpress\nimage: docker.io/library/wordpress:6\nlisteners:\n  - name: web\n    guest_port: 80\n    flow: tcp\n    protocol: http\n",
		},
	}
	c.JSON(http.StatusOK, gin.H{"apps": apps})
}

func handleAppManagerError(c *gin.Context, err error, action string) bool {
	if errors.Is(err, app.ErrLocked) {
		msg := fmt.Sprintf("Unable to %s while storage is locked. Unlock Piccolo to continue.", action)
		writeGinError(c, http.StatusLocked, msg)
		return true
	}
	return false
}

func (s *GinServer) ensureAppVolume(ctx context.Context, appDef *api.AppDefinition) error {
	if s.dispatcher == nil || appDef == nil {
		return nil
	}
	volID := fmt.Sprintf("app-%s", appDef.Name)
	req := persistence.VolumeRequest{ID: volID, Class: persistence.VolumeClassApplication, ClusterMode: persistence.ClusterModeStateful}
	resp, err := s.dispatcher.Dispatch(ctx, persistence.EnsureVolumeCommand{Req: req})
	if err != nil {
		return fmt.Errorf("failed to ensure app volume: %w", err)
	}
	if _, ok := resp.(persistence.EnsureVolumeResponse); !ok {
		return fmt.Errorf("unexpected response from persistence for volume %s", volID)
	}
	return nil
}
