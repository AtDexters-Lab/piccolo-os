package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// corsMiddleware adds CORS headers for web UI access
func (s *GinServer) corsMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        // For same-origin UI, CORS is mostly unused, but keep minimal support for dev
        origin := c.GetHeader("Origin")
        if origin != "" {
            c.Header("Access-Control-Allow-Origin", origin)
        } else {
            c.Header("Access-Control-Allow-Origin", "*")
        }
        c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
        c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-Requested-With, X-CSRF-Token")
        c.Header("Access-Control-Allow-Credentials", "true")

		// Handle preflight requests
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}

		c.Next()
	}
}

// securityHeadersMiddleware adds security headers
func (s *GinServer) securityHeadersMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        // Security headers
        c.Header("X-Content-Type-Options", "nosniff")
        c.Header("X-Frame-Options", "DENY")
        c.Header("X-XSS-Protection", "1; mode=block")
        // Only set HSTS when request is HTTPS (or forwarded as HTTPS)
        host := c.Request.Host
        if (c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https") && host != "localhost" && host != "127.0.0.1" {
            c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
        }
        c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
        
        // API identification
        c.Header("X-Powered-By", "Piccolo OS")
        c.Header("X-Service-Version", s.version)
        if s != nil && s.apiValidator != nil {
            c.Header("X-API-Validation", "enabled")
        } else {
            c.Header("X-API-Validation", "disabled")
        }

		c.Next()
	}
}

// rateLimitMiddleware provides basic rate limiting (placeholder for future enhancement)
func (s *GinServer) rateLimitMiddleware() gin.HandlerFunc {
	// This is a placeholder - in production, use gin-contrib/limiter
	return func(c *gin.Context) {
		// TODO: Implement rate limiting with redis or memory store
		// For now, just pass through
		c.Next()
	}
}

// authMiddleware provides authentication (placeholder for future enhancement) 
func (s *GinServer) authMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        // TODO: Implement JWT or session-based authentication
        // For now, just pass through

        // Example of how it would work:
		// token := c.GetHeader("Authorization")
		// if token == "" {
		//     c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization required"})
		//     c.Abort()
		//     return
		// }
		// 
		// // Validate token...
		// c.Set("user_id", userID)
		c.Next()
	}
}

// requestLoggingMiddleware provides structured request logging
func (s *GinServer) requestLoggingMiddleware() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		return fmt.Sprintf("[PICCOLO] %s - [%s] \"%s %s %s %d %s \"%s\" %s\"\n",
			param.ClientIP,
			param.TimeStamp.Format(time.RFC3339),
			param.Method,
			param.Path,
			param.Request.Proto,
			param.StatusCode,
			param.Latency,
			param.Request.UserAgent(),
			param.ErrorMessage,
		)
    })
}

// requireSession ensures a valid session cookie is present and not expired
func (s *GinServer) requireSession() gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := s.getSession(c)
        if !ok {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
            c.Abort()
            return
        }
        if _, ok := s.sessions.Get(id); !ok {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
            c.Abort()
            return
        }
        c.Next()
    }
}

// csrfMiddleware enforces X-CSRF-Token on state-changing requests when session exists
func (s *GinServer) csrfMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        // Only enforce on non-GET/HEAD/OPTIONS
        switch c.Request.Method {
        case http.MethodGet, http.MethodHead, http.MethodOptions:
            c.Next()
            return
        }
        id, ok := s.getSession(c)
        if !ok {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
            c.Abort()
            return
        }
        sess, ok := s.sessions.Get(id)
        if !ok {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
            c.Abort()
            return
        }
        token := c.GetHeader("X-CSRF-Token")
        if token == "" || token != sess.CSRF {
            c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
            c.Abort()
            return
        }
        c.Next()
    }
}
