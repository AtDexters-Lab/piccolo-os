package server

import (
	"fmt"
	"net/http"
	"strings"

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
