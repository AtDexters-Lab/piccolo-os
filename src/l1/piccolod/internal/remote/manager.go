package remote

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
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

type Manager struct {
	dir  string
	path string
	cfg  *Config
}

func NewManager(stateDir string) (*Manager, error) {
	if stateDir == "" {
		stateDir = "/var/lib/piccolod"
	}
	dir := filepath.Join(stateDir, "remote")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	m := &Manager{dir: dir, path: filepath.Join(dir, "config.json")}
	_ = m.load()
	return m, nil
}

func (m *Manager) load() error {
	data, err := os.ReadFile(m.path)
	if err != nil {
		return err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	m.cfg = &cfg
	return nil
}

func (m *Manager) save(cfg *Config) error {
	if cfg == nil {
		return errors.New("config cannot be nil")
	}
	// ensure deterministic ordering for credentials map
	if cfg.DNSCredentials == nil {
		cfg.DNSCredentials = map[string]string{}
	}
	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(m.path, payload, 0o644); err != nil {
		return err
	}
	m.cfg = cfg
	return nil
}

func (m *Manager) currentConfig() *Config {
	if m.cfg == nil {
		return &Config{}
	}
	return m.cfg
}

// Status returns the synthesized remote status for the API.
func (m *Manager) Status() Status {
	cfg := m.currentConfig()
	if !cfg.Enabled {
		state := "disabled"
		if cfg.Endpoint != "" && cfg.DeviceSecret != "" && cfg.TLD != "" {
			state = "provisioning"
		}
		return Status{
			Enabled:         cfg.Enabled,
			State:           state,
			Solver:          cfg.Solver,
			Endpoint:        cfg.Endpoint,
			TLD:             cfg.TLD,
			PortalHostname:  cfg.PortalHostname,
			GuideVerifiedAt: cfg.GuideVerifiedAt,
			Listeners:       buildListeners(cfg),
			Aliases:         cloneAliases(cfg.Aliases),
			Certificates:    cloneCertificates(cfg.Certificates),
			Warnings:        []string{},
		}
	}

	warnings := computeWarnings(cfg)
	state := "active"
	if cfg.LastPreflight == nil {
		state = "preflight_required"
	} else if len(warnings) > 0 {
		state = "warning"
	}
	if cfg.ExpiresAt.Before(time.Now()) {
		state = "error"
	}

	latency := intPtr(42) // placeholder latency until real metrics land
	issuer := cfg.Issuer
	exp := cfg.ExpiresAt
	next := cfg.NextRenewal

	return Status{
		Enabled:         true,
		State:           state,
		Solver:          cfg.Solver,
		Endpoint:        cfg.Endpoint,
		TLD:             cfg.TLD,
		PortalHostname:  cfg.PortalHostname,
		LatencyMS:       latency,
		LastHandshake:   timePtr(cfg.LastHandshake),
		NextRenewal:     &next,
		Issuer:          &issuer,
		ExpiresAt:       &exp,
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

	now := time.Now().UTC()
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
	cfg.LastPreflight = nil

	cfg.Certificates = defaultCertificates(cfg)
	cfg.Events = append(cfg.Events, Event{
		Timestamp: now,
		Level:     "info",
		Source:    "remote",
		Message:   "Remote configuration saved",
		NextStep:  "Run preflight",
	})

	if err := m.save(cfg); err != nil {
		return err
	}
	return nil
}

// Disable switches remote access off but retains configuration.
func (m *Manager) Disable() error {
	cfg := m.currentConfig()
	cfg.Enabled = false
	now := time.Now().UTC()
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
		Timestamp: time.Now().UTC(),
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
	id := fmt.Sprintf("alias-%d", time.Now().UnixNano()+rand.Int63n(1000))
	alias := Alias{
		ID:       id,
		Hostname: hostname,
		Listener: listener,
		Status:   "pending",
		Message:  "Awaiting DNS verification",
	}
	cfg.Aliases = append(cfg.Aliases, alias)
	cfg.Events = append(cfg.Events, Event{
		Timestamp: time.Now().UTC(),
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
		Timestamp: time.Now().UTC(),
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

// RenewCertificate simulates a manual renewal.
func (m *Manager) RenewCertificate(id string) error {
	cfg := m.currentConfig()
	for i := range cfg.Certificates {
		if cfg.Certificates[i].ID == id {
			now := time.Now().UTC()
			exp := now.Add(90 * 24 * time.Hour)
			next := now.Add(60 * 24 * time.Hour)
			cfg.Certificates[i].IssuedAt = &now
			cfg.Certificates[i].ExpiresAt = &exp
			cfg.Certificates[i].NextRenewal = &next
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

// RunPreflight performs a stubbed validation run.
func (m *Manager) RunPreflight() (PreflightResult, error) {
	cfg := m.currentConfig()
	if cfg.Endpoint == "" || cfg.TLD == "" {
		return PreflightResult{}, errors.New("remote not configured")
	}
	now := time.Now().UTC()
	checks := []PreflightCheck{
		{Name: "Nexus endpoint reachable", Status: "pass"},
		{Name: "DNS records", Status: "pass", Detail: fmt.Sprintf("CNAME *.%s → %s", cfg.TLD, extractHost(cfg.Endpoint))},
		{Name: "ACME solver", Status: "pass", Detail: fmt.Sprintf("Using %s", strings.ToUpper(cfg.Solver))},
	}
	if len(cfg.Aliases) > 0 {
		status := "pass"
		detail := "All aliases pending verification"
		for _, alias := range cfg.Aliases {
			if alias.Status != "active" {
				status = "warn"
				detail = "One or more aliases still pending"
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
	// order newest first for readability
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	return events
}

// MarkGuideVerified stores the helper verification timestamp and optional seed data.
type GuideVerification struct {
	Endpoint       string `json:"endpoint"`
	TLD            string `json:"tld"`
	PortalHostname string `json:"portal_hostname"`
	JWTSecret      string `json:"jwt_secret"`
}

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
	now := time.Now().UTC()
	cfg.GuideVerifiedAt = &now
	cfg.Events = append(cfg.Events, Event{
		Timestamp: now,
		Level:     "info",
		Source:    "remote",
		Message:   "Nexus helper verified",
	})
	return m.save(cfg)
}

// GuideInfo returns the static helper information along with verification metadata.
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

func buildListeners(cfg *Config) []ListenerSummary {
	if cfg.PortalHostname == "" {
		return []ListenerSummary{}
	}
	return []ListenerSummary{
		{Name: "portal", RemoteHost: cfg.PortalHostname},
	}
}

func computeWarnings(cfg *Config) []string {
	warnings := []string{}
	if cfg.NextRenewal.Before(time.Now().Add(7 * 24 * time.Hour)) {
		warnings = append(warnings, "Certificate renewal due soon")
	}
	for _, alias := range cfg.Aliases {
		if alias.Status != "active" {
			warnings = append(warnings, fmt.Sprintf("Alias %s is %s", alias.Hostname, alias.Status))
		}
	}
	return warnings
}

func defaultCertificates(cfg *Config) []Certificate {
	now := time.Now().UTC()
	exp := now.Add(90 * 24 * time.Hour)
	next := now.Add(60 * 24 * time.Hour)
	wildcard := ""
	if cfg.TLD != "" {
		wildcard = fmt.Sprintf("*.%s", cfg.TLD)
	}
	certificates := []Certificate{}
	if cfg.PortalHostname != "" {
		certificates = append(certificates, Certificate{
			ID:          "portal",
			Domains:     []string{cfg.PortalHostname},
			Solver:      cfg.Solver,
			IssuedAt:    &now,
			ExpiresAt:   &exp,
			NextRenewal: &next,
			Status:      "ok",
		})
	}
	if wildcard != "" {
		certificates = append(certificates, Certificate{
			ID:          "wildcard",
			Domains:     []string{wildcard},
			Solver:      cfg.Solver,
			IssuedAt:    &now,
			ExpiresAt:   &exp,
			NextRenewal: &next,
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
	for i := range in {
		out[i] = in[i]
	}
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

func extractHost(endpoint string) string {
	if endpoint == "" {
		return ""
	}
	if u, err := url.Parse(endpoint); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	stripped := strings.TrimPrefix(endpoint, "https://")
	stripped = strings.TrimPrefix(stripped, "http://")
	stripped = strings.TrimPrefix(stripped, "wss://")
	stripped = strings.TrimPrefix(stripped, "ws://")
	stripped = strings.SplitN(stripped, "/", 2)[0]
	stripped = strings.SplitN(stripped, ":", 2)[0]
	return stripped
}

func normalizePortalHost(tld, portal string) string {
	tld = strings.TrimSpace(tld)
	portal = strings.TrimSpace(portal)
	if portal == "" {
		if tld == "" {
			return ""
		}
		return fmt.Sprintf("portal.%s", tld)
	}
	if tld == "" {
		return portal
	}
	if portal == tld || strings.HasSuffix(portal, "."+tld) {
		return portal
	}
	// allow prefix only
	if !strings.Contains(portal, ".") {
		return fmt.Sprintf("%s.%s", portal, tld)
	}
	return portal
}

func intPtr(v int) *int { return &v }

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	tt := t
	return &tt
}
