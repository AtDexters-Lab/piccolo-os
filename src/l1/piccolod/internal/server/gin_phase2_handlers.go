package server

import (
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
)

// handleOSUpdateStatus returns a read-only snapshot of OS update info.
func (s *GinServer) handleOSUpdateStatus(c *gin.Context) {
    // Placeholder: in future, query update manager/transactional-update
    c.JSON(http.StatusOK, gin.H{
        "current_version":   s.version,
        "available_version": s.version,
        "pending":           false,
        "requires_reboot":   false,
        "last_checked":      time.Now().UTC().Format(time.RFC3339),
    })
}

// handleRemoteStatus returns basic remote access status (device-terminated TLS).
func (s *GinServer) handleRemoteStatus(c *gin.Context) {
    st := s.remoteManager.Status()
    c.JSON(http.StatusOK, st)
}

// handleStorageDisks lists physical disks (read-only); returns an empty list if unknown.
func (s *GinServer) handleStorageDisks(c *gin.Context) {
    disks := []gin.H{}
    if s.storageManager != nil {
        if infos, err := s.storageManager.ListPhysicalDisks(); err == nil && infos != nil {
            for _, d := range infos {
                disks = append(disks, gin.H{
                    "id":          d.Path,
                    "model":       d.Model,
                    "size_bytes":  d.SizeBytes,
                    "health":      "unknown",
                    "status":      "unknown",
                    "mountpoint":  nil,
                    "encrypted":   false,
                    "label":       nil,
                })
            }
        }
    }
    c.JSON(http.StatusOK, gin.H{"disks": disks})
}
