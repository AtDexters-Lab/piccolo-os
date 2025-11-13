package server

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"piccolod/internal/crypt"
	"piccolod/internal/events"
	"piccolod/internal/persistence"
)

// cookie name as per OpenAPI cookieAuth
const sessionCookieName = "piccolo_session"

func (s *GinServer) setSessionCookie(c *gin.Context, id string, ttl time.Duration) {
	secure := false
	if s != nil {
		secure = s.isSecureRequest(c.Request)
	}
	// Prefer SameSite=Lax for session cookie
	c.SetSameSite(http.SameSiteLaxMode)
	// Use explicit cookie to control SameSite and HttpOnly flags
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    id,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *GinServer) clearSessionCookie(c *gin.Context) {
	// Clear with SameSite=Lax
	c.SetSameSite(http.SameSiteLaxMode)
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *GinServer) recordLoginFailure() bool {
	s.loginFailures++
	return s.loginFailures >= 5
}

func (s *GinServer) resetLoginFailures() {
	s.loginFailures = 0
}

func (s *GinServer) recordResetFailure() bool {
	s.resetFailures++
	return s.resetFailures >= 5
}

func (s *GinServer) resetResetFailures() {
	s.resetFailures = 0
}

func (s *GinServer) getSession(c *gin.Context) (id string, ok bool) {
	v, err := c.Cookie(sessionCookieName)
	if err != nil || v == "" {
		return "", false
	}
	return v, true
}

// handleAuthSession: GET /api/v1/auth/session
func (s *GinServer) handleAuthSession(c *gin.Context) {
	id, ok := s.getSession(c)
	passwordStale := false
	recoveryStale := false
	if st, err := s.readAuthStaleness(c.Request.Context()); err == nil {
		passwordStale = st.PasswordStale
		recoveryStale = st.RecoveryStale
	}
	if ok {
		if sess, ok := s.sessions.Get(id); ok {
			locked := false
			if s.cryptoManager != nil && s.cryptoManager.IsInitialized() {
				locked = s.cryptoManager.IsLocked()
			}
			c.JSON(http.StatusOK, gin.H{
				"authenticated":  true,
				"user":           sess.User,
				"expires_at":     time.Unix(sess.ExpiresAt, 0).UTC().Format(time.RFC3339),
				"volumes_locked": locked,
				"password_stale": passwordStale,
				"recovery_stale": recoveryStale,
			})
			return
		}
	}
	locked := false
	if s.cryptoManager != nil && s.cryptoManager.IsInitialized() {
		locked = s.cryptoManager.IsLocked()
	}
	c.JSON(http.StatusOK, gin.H{
		"authenticated":  false,
		"user":           "",
		"expires_at":     time.Now().UTC().Format(time.RFC3339),
		"volumes_locked": locked,
		"password_stale": passwordStale,
		"recovery_stale": recoveryStale,
	})
}

// handleAuthSetup: POST /api/v1/auth/setup
func (s *GinServer) handleAuthSetup(c *gin.Context) {
	var body struct {
		Password string `json:"password"`
	}
	if err := c.BindJSON(&body); err != nil || body.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	ctx := c.Request.Context()
	initialized, err := s.authManager.IsInitialized(ctx)
	if err != nil {
		if errors.Is(err, persistence.ErrLocked) {
			c.JSON(http.StatusLocked, gin.H{"error": "storage locked; unlock Piccolo to continue"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read auth state"})
		return
	}
	if initialized {
		c.JSON(http.StatusBadRequest, gin.H{"error": "already initialized"})
		return
	}
	if err := s.authManager.Setup(ctx, body.Password); err != nil {
		if errors.Is(err, persistence.ErrLocked) {
			c.JSON(http.StatusLocked, gin.H{"error": "storage locked; unlock Piccolo to continue"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// handleAuthLogin: POST /api/v1/auth/login
func (s *GinServer) handleAuthLogin(c *gin.Context) {
	var body struct{ Username, Password string }
	if err := c.BindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	username := strings.TrimSpace(body.Username)
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username required"})
		return
	}
	// Single local admin account; verify password only
	ctx := c.Request.Context()
	ok, err := s.authManager.Verify(ctx, username, body.Password)
	if err != nil {
		if errors.Is(err, persistence.ErrLocked) && s.cryptoManager != nil {
			if unlockErr := s.cryptoManager.Unlock(body.Password); unlockErr != nil {
				if errors.Is(unlockErr, crypt.ErrNotInitialized) {
					c.JSON(http.StatusBadRequest, gin.H{"error": "not initialized"})
					return
				}
				if s.recordLoginFailure() {
					c.Header("Retry-After", "5")
					c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too Many Requests"})
				} else {
					c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
				}
				return
			}
			if notifyErr := s.notifyPersistenceLockState(ctx, false); notifyErr != nil {
				log.Printf("WARN: auth login persistence unlock failed: %v", notifyErr)
			}
			ok, err = s.authManager.Verify(ctx, username, body.Password)
		}
		if err != nil {
			if errors.Is(err, persistence.ErrLocked) {
				c.JSON(http.StatusLocked, gin.H{"error": "storage locked; unlock Piccolo to continue"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify credentials"})
			return
		}
	}
	if !ok {
		if s.recordLoginFailure() {
			c.Header("Retry-After", "5")
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too Many Requests"})
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		}
		return
	}
	s.resetLoginFailures()
	sess := s.sessions.Create("admin", 3600) // 1h default
	s.setSessionCookie(c, sess.ID, time.Hour)
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// handleAuthLogout: POST /api/v1/auth/logout
func (s *GinServer) handleAuthLogout(c *gin.Context) {
	if id, ok := s.getSession(c); ok {
		s.sessions.Delete(id)
	}
	s.clearSessionCookie(c)
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// handleAuthPassword: POST /api/v1/auth/password
func (s *GinServer) handleAuthPassword(c *gin.Context) {
	id, ok := s.getSession(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	if _, ok := s.sessions.Get(id); !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	var body struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.BindJSON(&body); err != nil || body.OldPassword == "" || body.NewPassword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := s.authManager.ChangePassword(c.Request.Context(), body.OldPassword, body.NewPassword); err != nil {
		if errors.Is(err, persistence.ErrLocked) {
			c.JSON(http.StatusLocked, gin.H{"error": "storage locked; unlock Piccolo to continue"})
			return
		}
		switch err.Error() {
		case "invalid credentials":
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		case "not initialized":
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}
	// Rewrap crypto SDEK if initialized
	if s.cryptoManager != nil && s.cryptoManager.IsInitialized() {
		if err := s.cryptoManager.Rewrap(body.OldPassword, body.NewPassword); err != nil {
			// Surface as 400 but keep auth changed; user can recover via recovery key
			c.JSON(http.StatusBadRequest, gin.H{"error": "crypto rewrap failed: " + err.Error()})
			return
		}
	}
	update := persistence.AuthStalenessUpdate{
		PasswordStale:   boolPtr(false),
		PasswordStaleAt: timePtr(time.Time{}),
		PasswordAckAt:   timePtr(time.Time{}),
	}
	if err := s.applyStalenessUpdate(c.Request.Context(), update); err != nil {
		log.Printf("WARN: failed to clear password staleness: %v", err)
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// handleAuthStalenessAck: POST /api/v1/auth/staleness/ack
func (s *GinServer) handleAuthStalenessAck(c *gin.Context) {
	var body struct {
		Password bool `json:"password"`
		Recovery bool `json:"recovery"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if !body.Password && !body.Recovery {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no flags selected"})
		return
	}
	ctx := c.Request.Context()
	now := time.Now().UTC()
	falseVal := false
	update := persistence.AuthStalenessUpdate{}
	if body.Password {
		update.PasswordStale = boolPtr(falseVal)
		update.PasswordAckAt = timePtr(now)
	}
	if body.Recovery {
		update.RecoveryStale = boolPtr(falseVal)
		update.RecoveryAckAt = timePtr(now)
	}
	if err := s.applyStalenessUpdate(ctx, update); err != nil {
		log.Printf("WARN: staleness ack failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update staleness"})
		return
	}
	if s.events != nil {
		targets := []string{}
		if body.Password {
			targets = append(targets, "password")
		}
		if body.Recovery {
			targets = append(targets, "recovery")
		}
		s.events.Publish(events.Event{
			Topic: events.TopicAudit,
			Payload: events.AuditEvent{
				Kind:   "auth.staleness_ack",
				Time:   now,
				Source: c.ClientIP(),
				Metadata: map[string]any{
					"flags": targets,
				},
			},
		})
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// handleAuthCSRF: GET /api/v1/auth/csrf
func (s *GinServer) handleAuthCSRF(c *gin.Context) {
	id, ok := s.getSession(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	if sess, ok := s.sessions.Get(id); ok {
		c.JSON(http.StatusOK, gin.H{"token": sess.CSRF})
		return
	}
	c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
}

// handleAuthInitialized: GET /api/v1/auth/initialized
func (s *GinServer) handleAuthInitialized(c *gin.Context) {
	init, err := s.authManager.IsInitialized(c.Request.Context())
	if err != nil {
		if errors.Is(err, persistence.ErrLocked) {
			c.JSON(http.StatusLocked, gin.H{"error": "storage locked; unlock Piccolo to continue"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read auth state"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"initialized": init})
}
