package server

import (
    "net/http"
    "strings"

    "github.com/gin-gonic/gin"
    "piccolod/internal/remote"
)

// handleRemoteConfigure handles POST /api/v1/remote/configure
func (s *GinServer) handleRemoteConfigure(c *gin.Context) {
    // Accept either device_id/device_secret or legacy device_key
    var body map[string]string
    if err := c.ShouldBindJSON(&body); err != nil {
        writeGinError(c, http.StatusBadRequest, "invalid json body")
        return
    }
    endpoint := strings.TrimSpace(body["endpoint"])
    hostname := strings.TrimSpace(body["hostname"])
    deviceID := strings.TrimSpace(body["device_id"])
    if deviceID == "" { deviceID = strings.TrimSpace(body["device_key"]) }
    deviceSecret := strings.TrimSpace(body["device_secret"]) // optional
    req := struct{
        Endpoint string
        DeviceID string
        DeviceSecret string
        Hostname string
        ACMEDirectory string
    }{ endpoint, deviceID, deviceSecret, hostname, strings.TrimSpace(body["acme_directory"]) }

    if err := s.remoteManager.Configure(remoteConfigureToInternal(req)); err != nil {
        writeGinError(c, http.StatusBadRequest, err.Error())
        return
    }
    writeGinSuccess(c, nil, "remote configured")
}

// handleRemoteDisable handles POST /api/v1/remote/disable
func (s *GinServer) handleRemoteDisable(c *gin.Context) {
    if err := s.remoteManager.Disable(); err != nil {
        writeGinError(c, http.StatusInternalServerError, err.Error())
        return
    }
    writeGinSuccess(c, nil, "remote disabled")
}

// handleRemoteRotate handles POST /api/v1/remote/rotate
func (s *GinServer) handleRemoteRotate(c *gin.Context) {
    if err := s.remoteManager.Rotate(); err != nil {
        writeGinError(c, http.StatusBadRequest, err.Error())
        return
    }
    writeGinSuccess(c, nil, "credentials rotated")
}

// internal helper to map request to remote.ConfigureRequest without import cycle
type remoteConfigure struct{
    Endpoint string
    DeviceID string
    DeviceSecret string
    Hostname string
    ACMEDirectory string
}

func remoteConfigureToInternal(in remoteConfigure) remote.ConfigureRequest {
    return remote.ConfigureRequest{
        Endpoint: in.Endpoint,
        DeviceID: in.DeviceID,
        DeviceSecret: in.DeviceSecret,
        Hostname: in.Hostname,
        ACMEDirectory: in.ACMEDirectory,
    }
}
