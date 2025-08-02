package installer

import (
	"log"
	"piccolod/internal/api" // Fictional import path
)

type Installer struct{}

func NewInstaller() *Installer {
	log.Println("INFO: Installer service initialized (placeholder)")
	return &Installer{}
}

func (i *Installer) GetAvailableDisks() ([]api.DiskInfo, error) { return nil, nil }
func (i *Installer) StartInstallation(diskPath string) error { return nil }
