package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"piccolod/internal/events"
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
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if !s.cryptoManager.IsInitialized() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not initialized"})
		return
	}
	password := strings.TrimSpace(body.Password)
	if password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password required"})
		return
	}
	if err := s.cryptoManager.Unlock(password); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	if err := s.notifyPersistenceLockState(c.Request.Context(), false); err != nil {
		log.Printf("WARN: failed to propagate unlock state: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update persistence state"})
		return
	}
	// Best-effort: verify admin credentials and create a session automatically.
	ctx := c.Request.Context()
	init := false
	if s.authManager != nil {
		if initialized, err := s.authManager.IsInitialized(ctx); err == nil {
			init = initialized
			if !initialized {
				if err := s.authManager.Setup(ctx, password); err != nil {
					log.Printf("WARN: auth setup during unlock failed: %v", err)
				} else {
					init = true
				}
			}
		}
		if init {
			if ok, err := s.authManager.Verify(ctx, "admin", password); err == nil && ok {
				sess := s.sessions.Create("admin", 3600)
				s.setSessionCookie(c, sess.ID, time.Hour)
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// handleCryptoResetPassword: POST /api/v1/crypto/reset-password
func (s *GinServer) handleCryptoResetPassword(c *gin.Context) {
	var body struct {
		RecoveryKey string `json:"recovery_key"`
		NewPassword string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	recoveryKey := strings.TrimSpace(body.RecoveryKey)
	newPassword := body.NewPassword
	if recoveryKey == "" || newPassword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "recovery_key and new_password required"})
		return
	}
	if s.cryptoManager == nil || !s.cryptoManager.IsInitialized() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not initialized"})
		return
	}
	if s.authManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "auth unavailable"})
		return
	}

	ctx := c.Request.Context()
	words := strings.Fields(recoveryKey)
	wasLocked := s.cryptoManager.IsLocked()
	needRelock := wasLocked

	if err := s.cryptoManager.UnlockWithRecoveryKey(words); err != nil {
		if s.recordResetFailure() {
			c.Header("Retry-After", "5")
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too Many Requests"})
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		}
		return
	}
	s.resetResetFailures()

	if wasLocked {
		if err := s.notifyPersistenceLockState(ctx, false); err != nil {
			log.Printf("WARN: reset-password unlock notify failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to unlock persistence"})
			return
		}
		defer func() {
			if needRelock {
				s.cryptoManager.Lock()
				if err := s.notifyPersistenceLockState(ctx, true); err != nil {
					log.Printf("WARN: reset-password relock notify failed: %v", err)
				}
			}
		}()
	}

	if err := s.authManager.ChangePasswordWithRecovery(ctx, newPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.cryptoManager.RewrapUnlocked(newPassword); err != nil {
		log.Printf("ERROR: reset-password rewrap failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to rewrap keys"})
		return
	}

	now := time.Now().UTC()
	update := persistence.AuthStalenessUpdate{
		PasswordStale:   boolPtr(true),
		PasswordStaleAt: timePtr(now),
		PasswordAckAt:   timePtr(time.Time{}),
		RecoveryStale:   boolPtr(true),
		RecoveryStaleAt: timePtr(now),
		RecoveryAckAt:   timePtr(time.Time{}),
	}
	if err := s.applyStalenessUpdate(ctx, update); err != nil {
		log.Printf("WARN: failed to mark staleness: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mark staleness"})
		return
	}

	if wasLocked {
		s.cryptoManager.Lock()
		if err := s.notifyPersistenceLockState(ctx, true); err != nil {
			log.Printf("WARN: reset-password relock notify failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to relock persistence"})
			return
		}
		needRelock = false
	}

	if s.events != nil {
		s.events.Publish(events.Event{
			Topic: events.TopicAudit,
			Payload: events.AuditEvent{
				Kind:   "auth.reset_with_recovery",
				Time:   now,
				Source: c.ClientIP(),
				Metadata: map[string]any{
					"was_locked": wasLocked,
				},
			},
		})
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
	stale := false
	if st, err := s.readAuthStaleness(c.Request.Context()); err == nil {
		stale = st.RecoveryStale
	}
	c.JSON(http.StatusOK, gin.H{"present": present, "stale": stale})
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
	rotating := s.cryptoManager.HasRecoveryKey()
	// Prefer unlocked path; else use direct with password
	if !s.cryptoManager.IsLocked() {
		words, err = s.cryptoManager.GenerateRecoveryKey(rotating)
	} else if strings.TrimSpace(body.Password) != "" {
		words, err = s.cryptoManager.GenerateRecoveryKeyWithPassword(body.Password, rotating)
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unlock required"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.applyStalenessUpdate(c.Request.Context(), persistence.AuthStalenessUpdate{
		RecoveryStale:   boolPtr(false),
		RecoveryStaleAt: timePtr(time.Time{}),
		RecoveryAckAt:   timePtr(time.Time{}),
	}); err != nil {
		log.Printf("WARN: failed to clear recovery staleness: %v", err)
	}
	if s.events != nil {
		s.events.Publish(events.Event{
			Topic: events.TopicAudit,
			Payload: events.AuditEvent{
				Kind:   "auth.recovery_key_generate",
				Time:   time.Now().UTC(),
				Source: c.ClientIP(),
				Metadata: map[string]any{
					"rotated": rotating,
				},
			},
		})
	}
	c.JSON(http.StatusOK, gin.H{"words": words})
}
