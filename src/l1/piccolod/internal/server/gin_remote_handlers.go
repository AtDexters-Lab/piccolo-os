package server

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"piccolod/internal/api"
	"piccolod/internal/remote"
)

type remoteConfigureRequest struct {
	Endpoint       string            `json:"endpoint"`
	DeviceSecret   string            `json:"device_secret"`
	Solver         string            `json:"solver"`
	TLD            string            `json:"tld"`
	PortalHostname string            `json:"portal_hostname"`
	DNSProvider    string            `json:"dns_provider"`
	DNSCredentials map[string]string `json:"dns_credentials"`
}

// handleRemoteConfigure handles POST /api/v1/remote/configure
func (s *GinServer) handleRemoteConfigure(c *gin.Context) {
	var req remoteConfigureRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeGinError(c, http.StatusBadRequest, "invalid json body")
		return
	}
	configureReq := remote.ConfigureRequest{
		Endpoint:       req.Endpoint,
		DeviceSecret:   req.DeviceSecret,
		Solver:         req.Solver,
		TLD:            req.TLD,
		PortalHostname: req.PortalHostname,
		DNSProvider:    req.DNSProvider,
		DNSCredentials: req.DNSCredentials,
	}
	if s.dispatcher != nil {
		resp, err := s.dispatcher.Dispatch(c.Request.Context(), remote.ConfigureCommand{Req: configureReq})
		if err != nil {
			if errors.Is(err, remote.ErrLocked) {
				writeGinError(c, http.StatusLocked, "storage locked; unlock Piccolo to continue")
				return
			}
			writeGinError(c, http.StatusBadRequest, err.Error())
			return
		}
		if _, ok := resp.(remote.ConfigureResponse); !ok {
			writeGinError(c, http.StatusInternalServerError, "unexpected response from remote dispatcher")
			return
		}
	} else {
		if err := s.remoteManager.Configure(configureReq); err != nil {
			if errors.Is(err, remote.ErrLocked) {
				writeGinError(c, http.StatusLocked, "storage locked; unlock Piccolo to continue")
				return
			}
			writeGinError(c, http.StatusBadRequest, err.Error())
			return
		}
	}
	s.refreshRemoteRuntime()
	// For HTTP-01 solver (wildcard unsupported), proactively issue per-listener certs
	if strings.EqualFold(configureReq.Solver, "http-01") && configureReq.TLD != "" && s.remoteManager != nil {
		hosts := map[string]struct{}{}
		for _, ep := range s.serviceManager.GetAll() {
			if ep.Flow == api.FlowTLS {
				continue
			}
			switch ep.Protocol {
			case api.ListenerProtocolHTTP, api.ListenerProtocolWebsocket:
			default:
				continue
			}
			if ep.Name == "" {
				continue
			}
			host := strings.ToLower(ep.Name + "." + configureReq.TLD)
			hosts[host] = struct{}{}
		}
		for h := range hosts {
			s.remoteManager.QueueHostnameCertificate(h)
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "remote configured"})
}

func (s *GinServer) resolvePortalPort() int {
	if s != nil && s.securePort > 0 {
		return s.securePort
	}
	if p := os.Getenv("PORT"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			return v
		}
	}
	return 80
}

// handleRemoteDisable handles POST /api/v1/remote/disable
func (s *GinServer) handleRemoteDisable(c *gin.Context) {
	if s.dispatcher != nil {
		if _, err := s.dispatcher.Dispatch(c.Request.Context(), remote.DisableCommand{}); err != nil {
			if errors.Is(err, remote.ErrLocked) {
				writeGinError(c, http.StatusLocked, "storage locked; unlock Piccolo to continue")
				return
			}
			writeGinError(c, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		if err := s.remoteManager.Disable(); err != nil {
			if errors.Is(err, remote.ErrLocked) {
				writeGinError(c, http.StatusLocked, "storage locked; unlock Piccolo to continue")
				return
			}
			writeGinError(c, http.StatusInternalServerError, err.Error())
			return
		}
	}
	s.refreshRemoteRuntime()
	c.JSON(http.StatusOK, gin.H{"message": "remote disabled"})
}

// handleRemoteRotate handles POST /api/v1/remote/rotate
func (s *GinServer) handleRemoteRotate(c *gin.Context) {
	var secret string
	if s.dispatcher != nil {
		resp, err := s.dispatcher.Dispatch(c.Request.Context(), remote.RotateSecretCommand{})
		if err != nil {
			if errors.Is(err, remote.ErrLocked) {
				writeGinError(c, http.StatusLocked, "storage locked; unlock Piccolo to continue")
				return
			}
			writeGinError(c, http.StatusBadRequest, err.Error())
			return
		}
		rotateResp, ok := resp.(remote.RotateSecretResponse)
		if !ok {
			writeGinError(c, http.StatusInternalServerError, "unexpected response from remote dispatcher")
			return
		}
		secret = rotateResp.Secret
	} else {
		resp, err := s.remoteManager.Rotate()
		if err != nil {
			if errors.Is(err, remote.ErrLocked) {
				writeGinError(c, http.StatusLocked, "storage locked; unlock Piccolo to continue")
				return
			}
			writeGinError(c, http.StatusBadRequest, err.Error())
			return
		}
		secret = resp
	}
	c.JSON(http.StatusOK, gin.H{"device_secret": secret})
}

// handleRemotePreflight runs a preflight validation.
func (s *GinServer) handleRemotePreflight(c *gin.Context) {
	var result remote.PreflightResult
	if s.dispatcher != nil {
		resp, err := s.dispatcher.Dispatch(c.Request.Context(), remote.RunPreflightCommand{})
		if err != nil {
			if errors.Is(err, remote.ErrLocked) {
				writeGinError(c, http.StatusLocked, "storage locked; unlock Piccolo to continue")
				return
			}
			writeGinError(c, http.StatusBadRequest, err.Error())
			return
		}
		preResp, ok := resp.(remote.RunPreflightResponse)
		if !ok {
			writeGinError(c, http.StatusInternalServerError, "unexpected response from remote dispatcher")
			return
		}
		result = preResp.Result
	} else {
		resp, err := s.remoteManager.RunPreflight()
		if err != nil {
			if errors.Is(err, remote.ErrLocked) {
				writeGinError(c, http.StatusLocked, "storage locked; unlock Piccolo to continue")
				return
			}
			writeGinError(c, http.StatusBadRequest, err.Error())
			return
		}
		result = resp
	}
	c.JSON(http.StatusOK, gin.H{
		"checks": result.Checks,
		"ran_at": result.RanAt.Format(time.RFC3339),
	})
}

// handleRemoteAliasesList returns the current alias inventory.
func (s *GinServer) handleRemoteAliasesList(c *gin.Context) {
	aliases := s.remoteManager.ListAliases()
	c.JSON(http.StatusOK, gin.H{"aliases": aliases})
}

type remoteAliasRequest struct {
	Listener string `json:"listener"`
	Hostname string `json:"hostname"`
}

// handleRemoteAliasesCreate appends a new alias.
func (s *GinServer) handleRemoteAliasesCreate(c *gin.Context) {
	var req remoteAliasRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeGinError(c, http.StatusBadRequest, "invalid json body")
		return
	}
	var alias remote.Alias
	if s.dispatcher != nil {
		resp, err := s.dispatcher.Dispatch(c.Request.Context(), remote.AddAliasCommand{Listener: req.Listener, Hostname: req.Hostname})
		if err != nil {
			if errors.Is(err, remote.ErrLocked) {
				writeGinError(c, http.StatusLocked, "storage locked; unlock Piccolo to continue")
				return
			}
			writeGinError(c, http.StatusBadRequest, err.Error())
			return
		}
		aliasResp, ok := resp.(remote.AddAliasResponse)
		if !ok {
			writeGinError(c, http.StatusInternalServerError, "unexpected response from remote dispatcher")
			return
		}
		alias = aliasResp.Alias
	} else {
		resp, err := s.remoteManager.AddAlias(req.Listener, req.Hostname)
		if err != nil {
			if errors.Is(err, remote.ErrLocked) {
				writeGinError(c, http.StatusLocked, "storage locked; unlock Piccolo to continue")
				return
			}
			writeGinError(c, http.StatusBadRequest, err.Error())
			return
		}
		alias = resp
	}
	c.JSON(http.StatusOK, alias)
}

// handleRemoteAliasesDelete removes an alias by ID.
func (s *GinServer) handleRemoteAliasesDelete(c *gin.Context) {
	id := c.Param("id")
	if s.dispatcher != nil {
		if _, err := s.dispatcher.Dispatch(c.Request.Context(), remote.RemoveAliasCommand{ID: id}); err != nil {
			if errors.Is(err, remote.ErrLocked) {
				writeGinError(c, http.StatusLocked, "storage locked; unlock Piccolo to continue")
				return
			}
			writeGinError(c, http.StatusNotFound, err.Error())
			return
		}
	} else {
		if err := s.remoteManager.RemoveAlias(id); err != nil {
			if errors.Is(err, remote.ErrLocked) {
				writeGinError(c, http.StatusLocked, "storage locked; unlock Piccolo to continue")
				return
			}
			writeGinError(c, http.StatusNotFound, err.Error())
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "alias removed"})
}

// handleRemoteCertificatesList returns certificate metadata.
func (s *GinServer) handleRemoteCertificatesList(c *gin.Context) {
	certs := s.remoteManager.ListCertificates()
	c.JSON(http.StatusOK, gin.H{"certificates": certs})
}

// handleRemoteCertificateRenew triggers a manual renewal.
func (s *GinServer) handleRemoteCertificateRenew(c *gin.Context) {
	id := c.Param("id")
	if s.dispatcher != nil {
		if _, err := s.dispatcher.Dispatch(c.Request.Context(), remote.RenewCertCommand{ID: id}); err != nil {
			if errors.Is(err, remote.ErrLocked) {
				writeGinError(c, http.StatusLocked, "storage locked; unlock Piccolo to continue")
				return
			}
			writeGinError(c, http.StatusNotFound, err.Error())
			return
		}
	} else {
		if err := s.remoteManager.RenewCertificate(id); err != nil {
			if errors.Is(err, remote.ErrLocked) {
				writeGinError(c, http.StatusLocked, "storage locked; unlock Piccolo to continue")
				return
			}
			writeGinError(c, http.StatusNotFound, err.Error())
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "renewal queued"})
}

// handleRemoteEvents returns the activity log.
func (s *GinServer) handleRemoteEvents(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"events": s.remoteManager.ListEvents()})
}

type guideVerifyRequest struct {
	Endpoint       string `json:"endpoint"`
	TLD            string `json:"tld"`
	PortalHostname string `json:"portal_hostname"`
	JWTSecret      string `json:"jwt_secret"`
}

// handleRemoteGuideVerify records helper verification details.
func (s *GinServer) handleRemoteGuideVerify(c *gin.Context) {
	var req guideVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeGinError(c, http.StatusBadRequest, "invalid json body")
		return
	}
	verification := remote.GuideVerification{
		Endpoint:       req.Endpoint,
		TLD:            req.TLD,
		PortalHostname: req.PortalHostname,
		JWTSecret:      req.JWTSecret,
	}
	if s.dispatcher != nil {
		if _, err := s.dispatcher.Dispatch(c.Request.Context(), remote.GuideVerifyCommand{Verification: verification}); err != nil {
			if errors.Is(err, remote.ErrLocked) {
				writeGinError(c, http.StatusLocked, "storage locked; unlock Piccolo to continue")
				return
			}
			writeGinError(c, http.StatusBadRequest, err.Error())
			return
		}
	} else {
		if err := s.remoteManager.MarkGuideVerified(verification); err != nil {
			if errors.Is(err, remote.ErrLocked) {
				writeGinError(c, http.StatusLocked, "storage locked; unlock Piccolo to continue")
				return
			}
			writeGinError(c, http.StatusBadRequest, err.Error())
			return
		}
	}
	info := s.remoteManager.GuideInfo()
	c.JSON(http.StatusOK, gin.H{
		"verified_at": info.VerifiedAt,
		"message":     "nexus helper verified",
	})
}

// handleRemoteGuideInfo returns the static guide metadata.
func (s *GinServer) handleRemoteGuideInfo(c *gin.Context) {
	info := s.remoteManager.GuideInfo()
	c.JSON(http.StatusOK, info)
}

// handleRemoteDNSProviders returns the supported DNS provider metadata.
func (s *GinServer) handleRemoteDNSProviders(c *gin.Context) {
	providers := []gin.H{
		{
			"id":       "cloudflare",
			"name":     "Cloudflare",
			"docs_url": "https://go-acme.github.io/lego/dns/cloudflare/",
			"fields": []gin.H{
				{"id": "api_token", "label": "API Token", "secret": true},
			},
		},
		{
			"id":       "route53",
			"name":     "AWS Route53",
			"docs_url": "https://go-acme.github.io/lego/dns/route53/",
			"fields": []gin.H{
				{"id": "access_key", "label": "Access Key ID"},
				{"id": "secret_key", "label": "Secret Access Key", "secret": true},
			},
		},
	}
	c.JSON(http.StatusOK, gin.H{"providers": providers})
}
