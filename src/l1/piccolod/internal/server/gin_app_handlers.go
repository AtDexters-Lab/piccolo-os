package server

import (
    "fmt"
    "net/http"
    "strings"
    "os"
    "time"
    "strconv"

    "github.com/gin-gonic/gin"
    "piccolod/internal/app"
)

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
        var req struct{ AppDefinition string `json:"app_definition"` }
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
    case "vaultwarden":
        yaml = "name: vaultwarden\nimage: vaultwarden/server:1.30.5\nlisteners:\n  - name: http\n    guest_port: 80\n    flow: tcp\n    protocol: http\n"
    case "gitea":
        yaml = "name: gitea\nimage: gitea/gitea:1.21\nlisteners:\n  - name: http\n    guest_port: 3000\n    flow: tcp\n    protocol: http\n"
    case "wordpress":
        yaml = "name: wordpress\nimage: wordpress:6\nlisteners:\n  - name: http\n    guest_port: 80\n    flow: tcp\n    protocol: http\n"
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
	
    // Install or update (upsert) the app
    appInstance, err := s.appManager.Upsert(c.Request.Context(), appDef)
    if err != nil {
        writeGinError(c, http.StatusInternalServerError, "Failed to install app: "+err.Error())
        return
    }
	
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
        if strings.Contains(err.Error(), "not found") {
            writeGinError(c, http.StatusNotFound, err.Error())
        } else {
            writeGinError(c, http.StatusInternalServerError, "Failed to get app: "+err.Error())
        }
        return
    }

    // Include services inline
    services, _ := s.serviceManager.GetByApp(appName)
    writeGinSuccess(c, gin.H{"app": appInstance, "services": services}, "")
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
        if strings.Contains(err.Error(), "not found") {
            writeGinError(c, http.StatusNotFound, err.Error())
        } else {
            writeGinError(c, http.StatusInternalServerError, "Failed to stop app: "+err.Error())
        }
        return
    }
    
    writeGinSuccess(c, nil, "App '"+appName+"' stopped successfully")
}

// handleGinAppEnable handles POST /api/v1/apps/:name/enable - Enable app (start on boot)
func (s *GinServer) handleGinAppEnable(c *gin.Context) {
	appName := c.Param("name")
	
	err := s.appManager.Enable(c.Request.Context(), appName)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeGinError(c, http.StatusNotFound, err.Error())
		} else {
			writeGinError(c, http.StatusInternalServerError, "Failed to enable app: "+err.Error())
		}
		return
	}
	
	writeGinSuccess(c, nil, "App '"+appName+"' enabled successfully")
}

// handleGinAppDisable handles POST /api/v1/apps/:name/disable - Disable app (manual start only)
func (s *GinServer) handleGinAppDisable(c *gin.Context) {
	appName := c.Param("name")
	
	err := s.appManager.Disable(c.Request.Context(), appName)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeGinError(c, http.StatusNotFound, err.Error())
		} else {
			writeGinError(c, http.StatusInternalServerError, "Failed to disable app: "+err.Error())
		}
		return
	}
	
	writeGinSuccess(c, nil, "App '"+appName+"' disabled successfully")
}

// handleGinAppUpdate handles POST /api/v1/apps/:name/update - Update app to a newer tag (optional body {tag}).
func (s *GinServer) handleGinAppUpdate(c *gin.Context) {
    appName := c.Param("name")
    var req struct{ Tag *string `json:"tag"` }
    _ = c.ShouldBindJSON(&req)
    // Demo mode: always OK
    if os.Getenv("PICCOLO_DEMO") != "" {
        writeGinSuccess(c, nil, "App '"+appName+"' updated")
        return
    }
    if err := s.appManager.UpdateImage(c.Request.Context(), appName, req.Tag); err != nil {
        if strings.Contains(err.Error(), "not found") {
            writeGinError(c, http.StatusNotFound, err.Error())
        } else {
            writeGinError(c, http.StatusInternalServerError, "Failed to update app: "+err.Error())
        }
        return
    }
    writeGinSuccess(c, nil, "App '"+appName+"' updated")
}

// handleGinAppRevert handles POST /api/v1/apps/:name/revert - Revert to previous version (placeholder).
func (s *GinServer) handleGinAppRevert(c *gin.Context) {
    appName := c.Param("name")
    if os.Getenv("PICCOLO_DEMO") != "" {
        writeGinSuccess(c, nil, "App '"+appName+"' reverted")
        return
    }
    if err := s.appManager.Revert(c.Request.Context(), appName); err != nil {
        if strings.Contains(err.Error(), "not found") {
            writeGinError(c, http.StatusNotFound, err.Error())
        } else {
            writeGinError(c, http.StatusBadRequest, "Failed to revert app: "+err.Error())
        }
        return
    }
    writeGinSuccess(c, nil, "App '"+appName+"' reverted")
}

// handleGinAppLogs handles GET /api/v1/apps/:name/logs - Return recent logs for an app (placeholder).
func (s *GinServer) handleGinAppLogs(c *gin.Context) {
    appName := c.Param("name")
    // Verify app exists; in prod fetch real logs
    if os.Getenv("PICCOLO_DEMO") != "" {
        entries := []gin.H{
            {"ts": time.Now().Add(-2 * time.Minute).UTC().Format(time.RFC3339), "level": "info", "message": appName + " starting"},
            {"ts": time.Now().Add(-1 * time.Minute).UTC().Format(time.RFC3339), "level": "info", "message": appName + " running"},
        }
        c.JSON(http.StatusOK, gin.H{"app": appName, "entries": entries})
        return
    }
    lines := 200
    if q := c.Query("tail"); q != "" {
        if n, err := strconv.Atoi(q); err == nil && n > 0 { lines = n }
    }
    out, err := s.appManager.Logs(c.Request.Context(), appName, lines)
    if err != nil {
        if strings.Contains(err.Error(), "not found") {
            writeGinError(c, http.StatusNotFound, err.Error())
        } else {
            writeGinError(c, http.StatusInternalServerError, "Failed to get logs: "+err.Error())
        }
        return
    }
    // Return as entries (array of lines)
    c.JSON(http.StatusOK, gin.H{"app": appName, "entries": out})
}

// handleGinCatalog handles GET /api/v1/catalog - returns curated catalog.
func (s *GinServer) handleGinCatalog(c *gin.Context) {
    apps := []gin.H{
        {"name": "vaultwarden", "image": "vaultwarden/server:1.30.5", "description": "Password manager"},
        {"name": "gitea", "image": "gitea/gitea:1.21", "description": "Git hosting"},
        {"name": "wordpress", "image": "wordpress:6", "description": "Blog/CMS"},
    }
    c.JSON(http.StatusOK, gin.H{"apps": apps})
}
