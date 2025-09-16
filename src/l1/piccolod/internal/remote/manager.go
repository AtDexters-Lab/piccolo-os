package remote

import (
    "encoding/json"
    "fmt"
    "net/url"
    "os"
    "path/filepath"
    "strings"
    "time"
)

// Config holds the persisted remote (Nexus) configuration
type Config struct {
    Endpoint     string    `json:"endpoint"`
    DeviceID     string    `json:"device_id"`
    DeviceSecret string    `json:"device_secret"`
    Hostname     string    `json:"hostname"`
    Enabled      bool      `json:"enabled"`
    Issuer       string    `json:"issuer,omitempty"`
    ExpiresAt    time.Time `json:"expires_at,omitempty"`
    NextRenewal  time.Time `json:"next_renewal,omitempty"`
}

// Status shape mirrors docs/api/openapi.yaml RemoteStatus
type Status struct {
    Enabled     bool       `json:"enabled"`
    PublicURL   *string    `json:"public_url"`
    Issuer      *string    `json:"issuer"`
    ExpiresAt   *time.Time `json:"expires_at"`
    NextRenewal *time.Time `json:"next_renewal"`
    Warnings    []string   `json:"warnings"`
}

type Manager struct {
    dir   string
    cfg   *Config
    path  string
}

func NewManager(stateDir string) (*Manager, error) {
    if stateDir == "" { stateDir = "/var/lib/piccolod" }
    dir := filepath.Join(stateDir, "remote")
    if err := os.MkdirAll(dir, 0755); err != nil { return nil, err }
    m := &Manager{dir: dir, path: filepath.Join(dir, "config.json")}
    _ = m.load()
    return m, nil
}

func (m *Manager) load() error {
    b, err := os.ReadFile(m.path)
    if err != nil { return err }
    var cfg Config
    if err := json.Unmarshal(b, &cfg); err != nil { return err }
    m.cfg = &cfg
    return nil
}

func (m *Manager) save(cfg *Config) error {
    b, err := json.MarshalIndent(cfg, "", "  ")
    if err != nil { return err }
    if err := os.WriteFile(m.path, b, 0644); err != nil { return err }
    m.cfg = cfg
    return nil
}

func (m *Manager) Status() Status {
    if m.cfg == nil || !m.cfg.Enabled {
        return Status{ Enabled: false, Warnings: []string{} }
    }
    pub := fmt.Sprintf("%s/%s", strings.TrimRight(m.cfg.Endpoint, "/"), m.cfg.Hostname)
    issuer := m.cfg.Issuer
    exp := m.cfg.ExpiresAt
    nxt := m.cfg.NextRenewal
    return Status{
        Enabled: true,
        PublicURL: &pub,
        Issuer: &issuer,
        ExpiresAt: &exp,
        NextRenewal: &nxt,
        Warnings: []string{},
    }
}

type ConfigureRequest struct {
    Endpoint     string `json:"endpoint"`
    DeviceID     string `json:"device_id"`
    DeviceSecret string `json:"device_secret"`
    Hostname     string `json:"hostname"`
    ACMEDirectory string `json:"acme_directory"`
}

func (m *Manager) Configure(req ConfigureRequest) error {
    // Minimal preflight: URL and hostname sanity
    if req.Endpoint == "" { return fmt.Errorf("endpoint required") }
    if _, err := url.ParseRequestURI(req.Endpoint); err != nil { return fmt.Errorf("invalid endpoint: %w", err) }
    if req.Hostname == "" || !strings.Contains(req.Hostname, ".") { return fmt.Errorf("invalid hostname") }
    if req.DeviceID == "" { return fmt.Errorf("device_id required") }
    // For v1 we don't require device_secret

    cfg := &Config{
        Endpoint: req.Endpoint,
        DeviceID: req.DeviceID,
        DeviceSecret: req.DeviceSecret,
        Hostname: req.Hostname,
        Enabled: true,
        Issuer: "Let's Encrypt",
        ExpiresAt: time.Now().Add(90*24*time.Hour).UTC(),
        NextRenewal: time.Now().Add(60*24*time.Hour).UTC(),
    }
    return m.save(cfg)
}

func (m *Manager) Disable() error {
    if m.cfg == nil { return nil }
    m.cfg.Enabled = false
    return m.save(m.cfg)
}

func (m *Manager) Rotate() error {
    if m.cfg == nil { return fmt.Errorf("remote not configured") }
    // Generate a dummy secret for now
    m.cfg.DeviceSecret = fmt.Sprintf("rotated-%d", time.Now().Unix())
    return m.save(m.cfg)
}

