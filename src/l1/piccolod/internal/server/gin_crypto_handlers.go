package server

import (
    "net/http"
    "github.com/gin-gonic/gin"
)

// handleCryptoStatus: GET /api/v1/crypto/status
func (s *GinServer) handleCryptoStatus(c *gin.Context) {
    init := s.cryptoManager != nil && s.cryptoManager.IsInitialized()
    locked := false
    if init { locked = s.cryptoManager.IsLocked() }
    c.JSON(http.StatusOK, gin.H{"initialized": init, "locked": locked})
}

// handleCryptoSetup: POST /api/v1/crypto/setup { password }
func (s *GinServer) handleCryptoSetup(c *gin.Context) {
    var body struct{ Password string `json:"password"` }
    if err := c.ShouldBindJSON(&body); err != nil || body.Password == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
        return
    }
    if err := s.cryptoManager.Setup(body.Password); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// handleCryptoUnlock: POST /api/v1/crypto/unlock { password }
func (s *GinServer) handleCryptoUnlock(c *gin.Context) {
    var body struct{ Password string `json:"password"` }
    if err := c.ShouldBindJSON(&body); err != nil || body.Password == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
        return
    }
    if !s.cryptoManager.IsInitialized() {
        c.JSON(http.StatusBadRequest, gin.H{"error": "not initialized"})
        return
    }
    if err := s.cryptoManager.Unlock(body.Password); err != nil {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }
    c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// handleCryptoLock: POST /api/v1/crypto/lock
func (s *GinServer) handleCryptoLock(c *gin.Context) {
    if !s.cryptoManager.IsInitialized() {
        c.JSON(http.StatusBadRequest, gin.H{"error": "not initialized"})
        return
    }
    s.cryptoManager.Lock()
    c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

