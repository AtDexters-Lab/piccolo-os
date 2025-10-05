package remote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"piccolod/internal/remote/nexusclient"
	"piccolod/internal/state/paths"
)

// Config holds the persisted remote (Nexus) configuration and runtime state.
type Config struct {
	Endpoint        string            `json:"endpoint"`
	DeviceSecret    string            `json:"device_secret"`
	Solver          string            `json:"solver"`
	TLD             string            `json:"tld"`
	PortalHostname  string            `json:"portal_hostname"`
	DNSProvider     string            `json:"dns_provider,omitempty"`
	DNSCredentials  map[string]string `json:"dns_credentials,omitempty"`
	Enabled         bool              `json:"enabled"`
	Issuer          string            `json:"issuer,omitempty"`
	ExpiresAt       time.Time         `json:"expires_at,omitempty"`
	NextRenewal     time.Time         `json:"next_renewal,omitempty"`
	LastHandshake   time.Time         `json:"last_handshake,omitempty"`
	LatencyMS       int               `json:"latency_ms,omitempty"`
	GuideVerifiedAt *time.Time        `json:"guide_verified_at,omitempty"`
	LastPreflight   *time.Time        `json:"last_preflight,omitempty"`
	Aliases         []Alias           `json:"aliases,omitempty"`
	Certificates    []Certificate     `json:"certificates,omitempty"`
	Events          []Event           `json:"events,omitempty"`
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

// Alias represents a remote alias domain attached to a listener.
type Alias struct {
	ID          string     `json:"id"`
	Hostname    string     `json:"hostname"`
	Listener    string     `json:"listener"`
	Status      string     `json:"status"`
	LastChecked *time.Time `json:"last_checked,omitempty"`
	Message     string     `json:"message,omitempty"`
}

// Certificate captures basic certificate metadata for the inventory table.
type Certificate struct {
	ID            string     `json:"id"`
	Domains       []string   `json:"domains"`
	Solver        string     `json:"solver,omitempty"`
	IssuedAt      *time.Time `json:"issued_at,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	NextRenewal   *time.Time `json:"next_renewal,omitempty"`
	Status        string     `json:"status,omitempty"`
	FailureReason string     `json:"failure_reason,omitempty"`
}

// Event is surfaced in the activity log for remote actions.
type Event struct {
	Timestamp time.Time `json:"ts"`
	Level     string    `json:"level"`
	Source    string    `json:"source"`
	Message   string    `json:"message"`
	NextStep  string    `json:"next_step,omitempty"`
}

// ListenerSummary mirrors the UI expectations for listener metadata.
type ListenerSummary struct {
	Name       string `json:"name"`
	RemoteHost string `json:"remote_host"`
}

// Status matches the shape consumed by the frontend remote page.
type Status struct {
	Enabled         bool              `json:"enabled"`
	State           string            `json:"state"`
	Solver          string            `json:"solver,omitempty"`
	Endpoint        string            `json:"endpoint,omitempty"`
	TLD             string            `json:"tld,omitempty"`
	PortalHostname  string            `json:"portal_hostname,omitempty"`
	LatencyMS       *int              `json:"latency_ms,omitempty"`
	LastHandshake   *time.Time        `json:"last_handshake,omitempty"`
	NextRenewal     *time.Time        `json:"next_renewal,omitempty"`
	Issuer          *string           `json:"issuer,omitempty"`
	ExpiresAt       *time.Time        `json:"expires_at,omitempty"`
	Warnings        []string          `json:"warnings,omitempty"`
	GuideVerifiedAt *time.Time        `json:"guide_verified_at,omitempty"`
	Listeners       []ListenerSummary `json:"listeners,omitempty"`
	Aliases         []Alias           `json:"aliases,omitempty"`
	Certificates    []Certificate     `json:"certificates,omitempty"`
}

// PreflightCheck represents a single validation step.
type PreflightCheck struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Detail   string `json:"detail,omitempty"`
	NextStep string `json:"next_step,omitempty"`
}

// PreflightResult aggregates the outcome of a preflight run.
type PreflightResult struct {
	Checks []PreflightCheck `json:"checks"`
	RanAt  time.Time        `json:"ran_at"`
}

type dialer interface {
	DialTimeout(network, address string, timeout time.Duration) (net.Conn, error)
}

type resolver interface {
	LookupHost(ctx context.Context, host string) ([]string, error)
	LookupCNAME(ctx context.Context, host string) (string, error)
}

var ErrLocked = errors.New("remote: storage locked")

type Storage interface {
	Load(ctx context.Context) (Config, error)
	Save(ctx context.Context, cfg Config) error
}

type Manager struct {
	storage       Storage
	cfg           *Config
	dialer        dialer
	resolver      resolver
	now           func() time.Time
	adapter       nexusclient.Adapter
	adapterMu     sync.Mutex
	adapterCancel context.CancelFunc
}

func NewManager(stateDir string) (*Manager, error) {
	storage, err := newFileStorage(stateDir)
	if err != nil {
		return nil, err
	}
	return newManagerWithDeps(storage, netDialer{}, netResolver{}, func() time.Time { return time.Now().UTC() })
}

func NewManagerWithStorage(storage Storage) (*Manager, error) {
	return newManagerWithDeps(storage, netDialer{}, netResolver{}, func() time.Time { return time.Now().UTC() })
}

func newManagerWithDeps(storage Storage, d dialer, r resolver, now func() time.Time) (*Manager, error) {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	m := &Manager{
		storage:  storage,
		dialer:   d,
		resolver: r,
		now:      now,
	}
	if storage != nil {
		cfg, err := storage.Load(context.Background())
		if err != nil {
			if !errors.Is(err, ErrLocked) {
				return nil, err
			}
		} else {
			m.cfg = &cfg
		}
	}
	if m.cfg == nil {
		m.cfg = &Config{}
	}
	return m, nil
}

// SetNexusAdapter injects the adapter responsible for proxy connectivity.
func (m *Manager) SetNexusAdapter(adapter nexusclient.Adapter) {
	m.adapterMu.Lock()
	defer m.adapterMu.Unlock()
	m.adapter = adapter
}

type netDialer struct{}

type persistentConn struct{ net.Conn }

func (netDialer) DialTimeout(network, address string, timeout time.Duration) (net.Conn, error) {
	var d net.Dialer
	d.Timeout = timeout
	return d.Dial(network, address)
}

type netResolver struct{}

func (netResolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	var r net.Resolver
	return r.LookupHost(ctx, host)
}

func (netResolver) LookupCNAME(ctx context.Context, host string) (string, error) {
	var r net.Resolver
	return r.LookupCNAME(ctx, host)
}

type fileStorage struct {
	path string
}

func newFileStorage(stateDir string) (*fileStorage, error) {
	if stateDir == "" {
		stateDir = paths.Root()
	}
	dir := filepath.Join(stateDir, "remote")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &fileStorage{path: filepath.Join(dir, "config.json")}, nil
}

func (s *fileStorage) Load(ctx context.Context) (Config, error) {
	_ = ctx
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (s *fileStorage) Save(ctx context.Context, cfg Config) error {
	_ = ctx
	if cfg.DNSCredentials == nil {
		cfg.DNSCredentials = map[string]string{}
	}
	payload, err := json.MarshalIndent(&cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, payload, 0o644)
}

func (m *Manager) save(cfg *Config) error {
	if cfg == nil {
		return errors.New("config cannot be nil")
	}
	if cfg.DNSCredentials == nil {
		cfg.DNSCredentials = map[string]string{}
	}
	if m.storage != nil {
		if err := m.storage.Save(context.Background(), *cfg); err != nil {
			return err
		}
	}
	m.cfg = cfg
	m.applyAdapterState()
	return nil
}

func (m *Manager) currentConfig() *Config {
	if m.cfg == nil {
		m.cfg = &Config{}
	}
	return m.cfg
}

func (m *Manager) Status() Status {
	cfg := m.currentConfig()
	warnings := computeWarnings(cfg)

	var latency *int
	if cfg.LatencyMS > 0 {
		latency = intPtr(cfg.LatencyMS)
	}

	state := "disabled"
	if cfg.Enabled {
		state = "active"
		if cfg.LastPreflight == nil {
			state = "preflight_required"
		} else if len(warnings) > 0 {
			state = "warning"
		}
		if !cfg.ExpiresAt.IsZero() && cfg.ExpiresAt.Before(m.now()) {
			state = "error"
		}
	} else if cfg.Endpoint != "" || cfg.DeviceSecret != "" || cfg.TLD != "" {
		state = "provisioning"
	}

	return Status{
		Enabled:         cfg.Enabled,
		State:           state,
		Solver:          cfg.Solver,
		Endpoint:        cfg.Endpoint,
		TLD:             cfg.TLD,
		PortalHostname:  cfg.PortalHostname,
		LatencyMS:       latency,
		LastHandshake:   timePtr(cfg.LastHandshake),
		NextRenewal:     timePtr(cfg.NextRenewal),
		Issuer:          stringPtr(cfg.Issuer),
		ExpiresAt:       timePtr(cfg.ExpiresAt),
		Warnings:        warnings,
		GuideVerifiedAt: cfg.GuideVerifiedAt,
		Listeners:       buildListeners(cfg),
		Aliases:         cloneAliases(cfg.Aliases),
		Certificates:    cloneCertificates(cfg.Certificates),
	}
}

// ConfigureRequest holds the payload accepted by Configure.
type ConfigureRequest struct {
	Endpoint       string            `json:"endpoint"`
	DeviceSecret   string            `json:"device_secret"`
	Solver         string            `json:"solver"`
	TLD            string            `json:"tld"`
	PortalHostname string            `json:"portal_hostname"`
	DNSProvider    string            `json:"dns_provider"`
	DNSCredentials map[string]string `json:"dns_credentials"`
}

// Configure persists a new remote configuration.
func (m *Manager) Configure(req ConfigureRequest) error {
	endpoint := strings.TrimSpace(req.Endpoint)
	if endpoint == "" {
		return errors.New("endpoint required")
	}
	if _, err := url.ParseRequestURI(endpoint); err != nil {
		return fmt.Errorf("invalid endpoint: %w", err)
	}

	solver := strings.ToLower(strings.TrimSpace(req.Solver))
	if solver == "" {
		solver = "http-01"
	}
	if solver != "http-01" && solver != "dns-01" {
		return fmt.Errorf("unsupported solver %q", solver)
	}

	tld := strings.TrimSpace(req.TLD)
	if tld == "" || !strings.Contains(tld, ".") {
		return errors.New("tld required")
	}

	portalHost := normalizePortalHost(tld, strings.TrimSpace(req.PortalHostname))
	if portalHost == "" {
		return errors.New("portal hostname invalid")
	}

	if solver == "dns-01" && strings.TrimSpace(req.DNSProvider) == "" {
		return errors.New("dns_provider required for dns-01")
	}

	now := m.now()
	expires := now.Add(90 * 24 * time.Hour)
	nextRenewal := now.Add(60 * 24 * time.Hour)

	cfg := m.currentConfig()
	cfg.Endpoint = endpoint
	cfg.DeviceSecret = strings.TrimSpace(req.DeviceSecret)
	cfg.Solver = solver
	cfg.TLD = tld
	cfg.PortalHostname = portalHost
	cfg.DNSProvider = strings.TrimSpace(req.DNSProvider)
	cfg.DNSCredentials = cloneCredentials(req.DNSCredentials)
	cfg.Enabled = true
	cfg.Issuer = "Let's Encrypt"
	cfg.ExpiresAt = expires
	cfg.NextRenewal = nextRenewal
	cfg.LastHandshake = now
	cfg.LatencyMS = 0
	cfg.LastPreflight = nil
	cfg.Certificates = defaultCertificates(cfg, now)
	cfg.Events = append(cfg.Events, Event{
		Timestamp: now,
		Level:     "info",
		Source:    "remote",
		Message:   "Remote configuration saved",
		NextStep:  "Run preflight",
	})

	return m.save(cfg)
}

// Disable switches remote access off but retains configuration.
func (m *Manager) Disable() error {
	cfg := m.currentConfig()
	cfg.Enabled = false
	now := m.now()
	cfg.Events = append(cfg.Events, Event{
		Timestamp: now,
		Level:     "info",
		Source:    "remote",
		Message:   "Remote access disabled",
	})
	return m.save(cfg)
}

// Rotate generates a placeholder device secret for testing.
func (m *Manager) Rotate() (string, error) {
	cfg := m.currentConfig()
	if cfg.Endpoint == "" {
		return "", errors.New("remote not configured")
	}
	newSecret := fmt.Sprintf("secret-%d", time.Now().UnixNano())
	cfg.DeviceSecret = newSecret
	cfg.Events = append(cfg.Events, Event{
		Timestamp: m.now(),
		Level:     "info",
		Source:    "remote",
		Message:   "Remote device secret rotated",
	})
	if err := m.save(cfg); err != nil {
		return "", err
	}
	return newSecret, nil
}

// ListAliases returns the current alias inventory.
func (m *Manager) ListAliases() []Alias {
	return cloneAliases(m.currentConfig().Aliases)
}

// AddAlias appends a new alias entry.
func (m *Manager) AddAlias(listener, hostname string) (Alias, error) {
	hostname = strings.TrimSpace(hostname)
	if hostname == "" || !strings.Contains(hostname, ".") {
		return Alias{}, errors.New("hostname required")
	}
	if listener == "" {
		listener = "portal"
	}
	cfg := m.currentConfig()
	alias := Alias{
		ID:       fmt.Sprintf("alias-%d", time.Now().UnixNano()+rand.Int63n(1000)),
		Hostname: hostname,
		Listener: listener,
		Status:   "pending",
		Message:  "Awaiting DNS verification",
	}
	cfg.Aliases = append(cfg.Aliases, alias)
	cfg.Events = append(cfg.Events, Event{
		Timestamp: m.now(),
		Level:     "info",
		Source:    "remote",
		Message:   fmt.Sprintf("Alias %s queued for listener %s", hostname, listener),
	})
	if err := m.save(cfg); err != nil {
		return Alias{}, err
	}
	return alias, nil
}

// RemoveAlias deletes an alias by ID.
func (m *Manager) RemoveAlias(id string) error {
	cfg := m.currentConfig()
	idx := -1
	for i, a := range cfg.Aliases {
		if a.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return errors.New("alias not found")
	}
	removed := cfg.Aliases[idx]
	cfg.Aliases = append(cfg.Aliases[:idx], cfg.Aliases[idx+1:]...)
	cfg.Events = append(cfg.Events, Event{
		Timestamp: m.now(),
		Level:     "info",
		Source:    "remote",
		Message:   fmt.Sprintf("Alias %s removed", removed.Hostname),
	})
	return m.save(cfg)
}

// ListCertificates returns the synthetic certificate inventory.
func (m *Manager) ListCertificates() []Certificate {
	return cloneCertificates(m.currentConfig().Certificates)
}

func (m *Manager) applyAdapterState() {
	m.adapterMu.Lock()
	adapter := m.adapter
	cfg := m.currentConfig()
	cancel := m.adapterCancel
	m.adapterMu.Unlock()

	if adapter == nil {
		return
	}

	adapterCfg := nexusclient.Config{
		Endpoint:       cfg.Endpoint,
		DeviceSecret:   cfg.DeviceSecret,
		PortalHostname: cfg.PortalHostname,
		TLD:            cfg.TLD,
	}
	if err := adapter.Configure(adapterCfg); err != nil {
		log.Printf("WARN: remote: configure nexus adapter failed: %v", err)
	}

	if !cfg.Enabled || cfg.Endpoint == "" || cfg.DeviceSecret == "" || cfg.PortalHostname == "" {
		m.stopAdapter()
		return
	}

	if cancel != nil {
		m.stopAdapter()
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.adapterMu.Lock()
	m.adapterCancel = cancel
	adapterRun := m.adapter
	m.adapterMu.Unlock()

	go func() {
		if err := adapterRun.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("WARN: remote: nexus adapter exited: %v", err)
		}
		m.adapterMu.Lock()
		m.adapterCancel = nil
		m.adapterMu.Unlock()
	}()
}

func (m *Manager) stopAdapter() {
	m.adapterMu.Lock()
	cancel := m.adapterCancel
	adapter := m.adapter
	m.adapterCancel = nil
	m.adapterMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if adapter != nil {
		if err := adapter.Stop(context.Background()); err != nil {
			log.Printf("WARN: remote: stopping nexus adapter: %v", err)
		}
	}
}

// RenewCertificate simulates a manual renewal.
func (m *Manager) RenewCertificate(id string) error {
	cfg := m.currentConfig()
	for i := range cfg.Certificates {
		if cfg.Certificates[i].ID == id {
			now := m.now()
			exp := now.Add(90 * 24 * time.Hour)
			next := now.Add(60 * 24 * time.Hour)
			cfg.Certificates[i].IssuedAt = timePtr(now)
			cfg.Certificates[i].ExpiresAt = timePtr(exp)
			cfg.Certificates[i].NextRenewal = timePtr(next)
			cfg.Certificates[i].Status = "ok"
			cfg.Certificates[i].FailureReason = ""
			cfg.Events = append(cfg.Events, Event{
				Timestamp: now,
				Level:     "info",
				Source:    "remote",
				Message:   fmt.Sprintf("Certificate %s renewed", id),
			})
			return m.save(cfg)
		}
	}
	return errors.New("certificate not found")
}

// RunPreflight performs validation checks for the remote configuration.
func (m *Manager) RunPreflight() (PreflightResult, error) {
	cfg := m.currentConfig()
	if cfg.Endpoint == "" || cfg.TLD == "" || cfg.PortalHostname == "" {
		return PreflightResult{}, errors.New("remote not configured")
	}

	now := m.now()
	var checks []PreflightCheck

	endpointCheck := m.checkEndpoint(cfg)
	checks = append(checks, endpointCheck)

	dnsStatus, dnsDetail := m.checkDNS(cfg)
	checks = append(checks, PreflightCheck{Name: "DNS records", Status: dnsStatus, Detail: dnsDetail})

	checks = append(checks, PreflightCheck{Name: "ACME solver", Status: "pass", Detail: fmt.Sprintf("Using %s", strings.ToUpper(cfg.Solver))})

	if len(cfg.Aliases) > 0 {
		status := "pass"
		detail := "All aliases verified"
		for _, alias := range cfg.Aliases {
			if alias.Status != "active" {
				status = "warn"
				detail = "One or more aliases pending verification"
				break
			}
		}
		checks = append(checks, PreflightCheck{Name: "Alias coverage", Status: status, Detail: detail})
	}

	cfg.LastPreflight = &now
	cfg.Events = append(cfg.Events, Event{
		Timestamp: now,
		Level:     "info",
		Source:    "remote",
		Message:   "Preflight completed",
	})
	if err := m.save(cfg); err != nil {
		return PreflightResult{}, err
	}
	return PreflightResult{Checks: checks, RanAt: now}, nil
}

// ListEvents returns the persisted remote-related events.
func (m *Manager) ListEvents() []Event {
	events := append([]Event(nil), m.currentConfig().Events...)
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	return events
}

// GuideVerification carries helper verification metadata.
type GuideVerification struct {
	Endpoint       string `json:"endpoint"`
	TLD            string `json:"tld"`
	PortalHostname string `json:"portal_hostname"`
	JWTSecret      string `json:"jwt_secret"`
}

// MarkGuideVerified stores the helper verification timestamp and optional seed data.
func (m *Manager) MarkGuideVerified(info GuideVerification) error {
	cfg := m.currentConfig()
	if info.Endpoint != "" {
		cfg.Endpoint = strings.TrimSpace(info.Endpoint)
	}
	if info.JWTSecret != "" {
		cfg.DeviceSecret = strings.TrimSpace(info.JWTSecret)
	}
	if info.TLD != "" {
		cfg.TLD = strings.TrimSpace(info.TLD)
	}
	if info.PortalHostname != "" {
		cfg.PortalHostname = normalizePortalHost(cfg.TLD, info.PortalHostname)
	}
	now := m.now()
	cfg.GuideVerifiedAt = &now
	cfg.Events = append(cfg.Events, Event{
		Timestamp: now,
		Level:     "info",
		Source:    "remote",
		Message:   "Nexus helper verified",
	})
	return m.save(cfg)
}

// GuideInfo returns static helper information along with verification timestamp.
type GuideInfo struct {
	Command      string     `json:"command"`
	Requirements []string   `json:"requirements"`
	Notes        []string   `json:"notes"`
	DocsURL      string     `json:"docs_url"`
	VerifiedAt   *time.Time `json:"verified_at,omitempty"`
}

func (m *Manager) GuideInfo() GuideInfo {
	cfg := m.currentConfig()
	return GuideInfo{
		Command: "sudo bash -c 'curl -fsSL https://raw.githubusercontent.com/AtDexters-Lab/nexus-proxy-server/main/scripts/install.sh | bash'",
		Requirements: []string{
			"Systemd-based Linux VM with sudo access",
			"Public ports 80 and 443 open",
			"DNS A/AAAA record ready for the Nexus host",
		},
		Notes: []string{
			"Installer prints the backend JWT secret on success.",
			"Keep the terminal open until the script finishes.",
		},
		DocsURL:    "https://github.com/AtDexters-Lab/nexus-proxy-server/blob/main/readme.md#install",
		VerifiedAt: cfg.GuideVerifiedAt,
	}
}

func (m *Manager) checkEndpoint(cfg *Config) PreflightCheck {
	host, port := endpointHostPort(cfg.Endpoint)
	if host == "" {
		return PreflightCheck{Name: "Nexus endpoint reachable", Status: "fail", Detail: "invalid endpoint"}
	}
	if port == "" {
		port = "443"
	}
	address := net.JoinHostPort(host, port)
	start := time.Now()
	conn, err := m.dialer.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		return PreflightCheck{Name: "Nexus endpoint reachable", Status: "fail", Detail: err.Error(), NextStep: "Verify firewall and DNS"}
	}
	latency := int(time.Since(start).Milliseconds())
	_ = conn.Close()
	cfg.LastHandshake = m.now()
	cfg.LatencyMS = latency
	return PreflightCheck{Name: "Nexus endpoint reachable", Status: "pass", Detail: fmt.Sprintf("Latency %d ms", latency)}
}

func (m *Manager) checkDNS(cfg *Config) (string, string) {
	host := cfg.PortalHostname
	if host == "" {
		return "fail", "portal hostname not configured"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cname, cnameErr := m.resolver.LookupCNAME(ctx, host)
	addresses, addrErr := m.resolver.LookupHost(ctx, host)

	detail := fmt.Sprintf("%s resolves to %v", host, addresses)
	if cnameErr == nil && cname != "" {
		detail = fmt.Sprintf("%s CNAME %s", host, strings.TrimSuffix(cname, "."))
	}

	status := "pass"
	if addrErr != nil {
		status = "warn"
		detail = fmt.Sprintf("portal host lookup failed: %v", addrErr)
	}

	if cfg.TLD != "" && cfg.PortalHostname != cfg.TLD {
		sample := fmt.Sprintf("app.%s", cfg.TLD)
		if _, err := m.resolver.LookupHost(ctx, sample); err != nil {
			status = "warn"
			detail = detail + "; wildcard host unresolved"
		} else {
			detail = detail + "; wildcard host resolves"
		}
	}
	return status, detail
}

func buildListeners(cfg *Config) []ListenerSummary {
	if cfg.PortalHostname == "" {
		return []ListenerSummary{}
	}
	return []ListenerSummary{{Name: "portal", RemoteHost: cfg.PortalHostname}}
}

func computeWarnings(cfg *Config) []string {
	var warnings []string
	if !cfg.NextRenewal.IsZero() && cfg.NextRenewal.Before(time.Now().Add(7*24*time.Hour)) {
		warnings = append(warnings, "Certificate renewal due soon")
	}
	if cfg.PortalHostname == "" {
		warnings = append(warnings, "Portal hostname missing")
	}
	for _, alias := range cfg.Aliases {
		if alias.Status != "active" {
			warnings = append(warnings, fmt.Sprintf("Alias %s is %s", alias.Hostname, alias.Status))
		}
	}
	return warnings
}

func defaultCertificates(cfg *Config, now time.Time) []Certificate {
	exp := now.Add(90 * 24 * time.Hour)
	next := now.Add(60 * 24 * time.Hour)
	certificates := []Certificate{}
	if cfg.PortalHostname != "" {
		certificates = append(certificates, Certificate{
			ID:          "portal",
			Domains:     []string{cfg.PortalHostname},
			Solver:      cfg.Solver,
			IssuedAt:    timePtr(now),
			ExpiresAt:   timePtr(exp),
			NextRenewal: timePtr(next),
			Status:      "ok",
		})
	}
	if cfg.TLD != "" {
		certificates = append(certificates, Certificate{
			ID:          "wildcard",
			Domains:     []string{fmt.Sprintf("*.%s", cfg.TLD)},
			Solver:      cfg.Solver,
			IssuedAt:    timePtr(now),
			ExpiresAt:   timePtr(exp),
			NextRenewal: timePtr(next),
			Status:      "ok",
		})
	}
	return certificates
}

func cloneAliases(in []Alias) []Alias {
	if len(in) == 0 {
		return []Alias{}
	}
	out := make([]Alias, len(in))
	copy(out, in)
	return out
}

func cloneCertificates(in []Certificate) []Certificate {
	if len(in) == 0 {
		return []Certificate{}
	}
	out := make([]Certificate, len(in))
	copy(out, in)
	return out
}

func cloneCredentials(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func endpointHostPort(endpoint string) (string, string) {
	if endpoint == "" {
		return "", ""
	}
	if u, err := url.Parse(endpoint); err == nil {
		host := u.Hostname()
		port := u.Port()
		if port == "" {
			if u.Scheme == "http" || u.Scheme == "ws" {
				port = "80"
			} else {
				port = "443"
			}
		}
		return host, port
	}
	stripped := strings.TrimPrefix(endpoint, "wss://")
	stripped = strings.TrimPrefix(stripped, "https://")
	stripped = strings.TrimPrefix(stripped, "ws://")
	stripped = strings.TrimPrefix(stripped, "http://")
	parts := strings.SplitN(stripped, "/", 2)
	hostPort := parts[0]
	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		return hostPort, ""
	}
	return host, port
}

func normalizePortalHost(tld, portal string) string {
	tld = strings.TrimSpace(tld)
	portal = strings.TrimSpace(portal)
	if tld == "" {
		return portal
	}
	if portal == "" {
		return fmt.Sprintf("portal.%s", tld)
	}
	if portal == tld || strings.HasSuffix(portal, "."+tld) {
		return portal
	}
	if !strings.Contains(portal, ".") {
		return fmt.Sprintf("%s.%s", portal, tld)
	}
	return portal
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func intPtr(v int) *int { return &v }

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	tt := t
	return &tt
}
