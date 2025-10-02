package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"piccolod/internal/persistence"
)

func (s *GinServer) notifyPersistenceLockState(ctx context.Context, locked bool) error {
	if s == nil || s.dispatcher == nil {
		return errors.New("persistence dispatcher unavailable")
	}
	_, err := s.dispatcher.Dispatch(ctx, persistence.RecordLockStateCommand{Locked: locked})
	return err
}

// handleCryptoStatus: GET /api/v1/crypto/status
func (s *GinServer) handleCryptoStatus(c *gin.Context) {
	init := s.cryptoManager != nil && s.cryptoManager.IsInitialized()
	locked := false
	if init {
		locked = s.cryptoManager.IsLocked()
	}
	c.JSON(http.StatusOK, gin.H{"initialized": init, "locked": locked})
}

// handleCryptoSetup: POST /api/v1/crypto/setup { password }
func (s *GinServer) handleCryptoSetup(c *gin.Context) {
	var body struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := s.cryptoManager.Setup(body.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.notifyPersistenceLockState(c.Request.Context(), true); err != nil {
		log.Printf("WARN: failed to propagate lock state: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update persistence state"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// handleCryptoUnlock: POST /api/v1/crypto/unlock { password }
func (s *GinServer) handleCryptoUnlock(c *gin.Context) {
	var body struct {
		Password    string `json:"password"`
		RecoveryKey string `json:"recovery_key"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if !s.cryptoManager.IsInitialized() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not initialized"})
		return
	}
	if strings.TrimSpace(body.Password) != "" {
		if err := s.cryptoManager.Unlock(body.Password); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		if err := s.notifyPersistenceLockState(c.Request.Context(), false); err != nil {
			log.Printf("WARN: failed to propagate unlock state: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update persistence state"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
		return
	}
	if strings.TrimSpace(body.RecoveryKey) != "" {
		words := strings.Fields(body.RecoveryKey)
		if err := s.cryptoManager.UnlockWithRecoveryKey(words); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		if err := s.notifyPersistenceLockState(c.Request.Context(), false); err != nil {
			log.Printf("WARN: failed to propagate unlock state: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update persistence state"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": "password or recovery_key required"})
}

// handleCryptoLock: POST /api/v1/crypto/lock
func (s *GinServer) handleCryptoLock(c *gin.Context) {
	if !s.cryptoManager.IsInitialized() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not initialized"})
		return
	}
	s.cryptoManager.Lock()
	if err := s.notifyPersistenceLockState(c.Request.Context(), true); err != nil {
		log.Printf("WARN: failed to propagate lock state: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update persistence state"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// handleCryptoRecoveryStatus: GET /api/v1/crypto/recovery-key
func (s *GinServer) handleCryptoRecoveryStatus(c *gin.Context) {
	present := false
	if s.cryptoManager != nil && s.cryptoManager.IsInitialized() {
		present = s.cryptoManager.HasRecoveryKey()
	}
	c.JSON(http.StatusOK, gin.H{"present": present})
}

// handleCryptoRecoveryGenerate: POST /api/v1/crypto/recovery-key/generate
func (s *GinServer) handleCryptoRecoveryGenerate(c *gin.Context) {
	if s.cryptoManager == nil || !s.cryptoManager.IsInitialized() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not initialized"})
		return
	}
	// Optional body: { password }
	var body struct {
		Password string `json:"password"`
	}
	_ = c.ShouldBindJSON(&body)
	var words []string
	var err error
	// Prefer unlocked path; else use direct with password
	if !s.cryptoManager.IsLocked() {
		words, err = s.cryptoManager.GenerateRecoveryKey()
	} else if strings.TrimSpace(body.Password) != "" {
		words, err = s.cryptoManager.GenerateRecoveryKeyWithPassword(body.Password)
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unlock required"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"words": words})
}
