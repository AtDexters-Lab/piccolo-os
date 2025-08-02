package update

import "log"

type Manager struct{}

func NewManager() *Manager {
	log.Println("INFO: Update Manager initialized (placeholder)")
	return &Manager{}
}

func (m *Manager) CheckForOSUpdate() (string, error) { return "v0.0.0", nil }
func (m *Manager) ApplyOSUpdate() error { return nil }
