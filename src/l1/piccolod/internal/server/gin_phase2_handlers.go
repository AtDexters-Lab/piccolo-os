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
	// Placeholder: storage manager not yet implemented; return empty list.
	c.JSON(http.StatusOK, gin.H{"disks": []gin.H{}})
}
