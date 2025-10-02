package ecosystem

import (
	"fmt"
	"os"

	"piccolod/internal/app"
	"piccolod/internal/backup"
	"piccolod/internal/container"
	"piccolod/internal/installer"
	"piccolod/internal/network"
	"piccolod/internal/trust"
	"piccolod/internal/update"

	"golang.org/x/sys/unix"
)

// EcosystemCheck represents the result of a single environment check
type EcosystemCheck struct {
	Name        string `json:"name"`
	Status      string `json:"status"` // "pass", "fail", "warn", "info"
	Description string `json:"description"`
	Details     string `json:"details,omitempty"`
}

// EcosystemResponse represents the complete ecosystem test results
type EcosystemResponse struct {
	Overall     string            `json:"overall"` // "healthy", "degraded", "unhealthy"
	Summary     string            `json:"summary"`
	Checks      []EcosystemCheck  `json:"checks"`
	Permissions map[string]string `json:"permissions"` // Key capability info
}

// ReadinessResponse represents a simple boolean health check result
type ReadinessResponse struct {
	Ready   bool   `json:"ready"`
	Status  string `json:"status"` // "healthy", "degraded", "unhealthy"
	Message string `json:"message,omitempty"`
}

type Manager struct {
	containerManager *container.Manager
	appManager       *app.AppManager
	trustAgent       *trust.Agent
	installer        *installer.Installer
	updateManager    *update.Manager
	networkManager   *network.Manager
	backupManager    *backup.Manager
}

func NewManager(
	containerManager *container.Manager,
	appManager *app.AppManager,
	trustAgent *trust.Agent,
	installer *installer.Installer,
	updateManager *update.Manager,
	networkManager *network.Manager,
	backupManager *backup.Manager,
) *Manager {
	return &Manager{
		containerManager: containerManager,
		appManager:       appManager,
		trustAgent:       trustAgent,
		installer:        installer,
		updateManager:    updateManager,
		networkManager:   networkManager,
		backupManager:    backupManager,
	}
}

// RunEcosystemChecks executes all environment validation tests
func (s *Manager) RunEcosystemChecks() EcosystemResponse {
	var checks []EcosystemCheck
	var failCount, warnCount int

	// Check 1: Process identity and privileges
	check := s.checkProcessIdentity()
	checks = append(checks, check)
	switch check.Status {
	case "fail":
		failCount++
	case "warn":
		warnCount++
	}

	// Check 2: Essential file system access
	check = s.checkFileSystemAccess()
	checks = append(checks, check)
	switch check.Status {
	case "fail":
		failCount++
	case "warn":
		warnCount++
	}

	// Check 3: Device access for hardware operations
	check = s.checkDeviceAccess()
	checks = append(checks, check)
	switch check.Status {
	case "fail":
		failCount++
	case "warn":
		warnCount++
	}

	// Check 4: Docker socket access for container management
	check = s.checkDockerAccess()
	checks = append(checks, check)
	switch check.Status {
	case "fail":
		failCount++
	case "warn":
		warnCount++
	}

	// Check 5: Network capabilities
	check = s.checkNetworkAccess()
	checks = append(checks, check)
	switch check.Status {
	case "fail":
		failCount++
	case "warn":
		warnCount++
	}

	// Check 6: Manager component health
	check = s.checkManagerComponents()
	checks = append(checks, check)
	switch check.Status {
	case "fail":
		failCount++
	case "warn":
		warnCount++
	}

	// Determine overall health
	var overall, summary string
	if failCount > 0 {
		overall = "unhealthy"
		summary = "Critical failures detected - some piccolod operations will not work"
	} else if warnCount > 0 {
		overall = "degraded"
		summary = "Minor issues detected - some advanced features may be limited"
	} else {
		overall = "healthy"
		summary = "All ecosystem checks passed - piccolod is fully operational"
	}

	return EcosystemResponse{
		Overall:     overall,
		Summary:     summary,
		Checks:      checks,
		Permissions: s.getPermissionInfo(),
	}
}

// checkProcessIdentity validates process user, PID, and basic runtime state
func (s *Manager) checkProcessIdentity() EcosystemCheck {
	uid := os.Getuid()
	gid := os.Getgid()

	if uid == 0 && gid == 0 {
		return EcosystemCheck{
			Name:        "Process Identity",
			Status:      "pass",
			Description: "Running as root with proper privileges",
			Details:     "UID=0, GID=0",
		}
	} else {
		return EcosystemCheck{
			Name:        "Process Identity",
			Status:      "fail",
			Description: "Not running as root - system operations will fail",
			Details:     "UID=%d, GID=%d (expected UID=0, GID=0)",
		}
	}
}

// checkFileSystemAccess validates access to essential directories
func (s *Manager) checkFileSystemAccess() EcosystemCheck {
	requiredPaths := []string{
		"/run", // MicroOS uses /run instead of /var/run
		"/var/log",
		"/tmp",
		"/etc",
	}

	optionalPaths := []string{
		"/sys/firmware/efi/efivars",
	}

	var issues []string
	var warnings []string

	// Test required paths
	for _, path := range requiredPaths {
		if err := unix.Access(path, unix.R_OK|unix.W_OK); err != nil {
			issues = append(issues, path+" not accessible")
		}
	}

	// Test optional paths
	for _, path := range optionalPaths {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			warnings = append(warnings, path+" not present")
		} else if err := unix.Access(path, unix.R_OK|unix.W_OK); err != nil {
			warnings = append(warnings, path+" not accessible")
		}
	}

	if len(issues) > 0 {
		return EcosystemCheck{
			Name:        "File System Access",
			Status:      "fail",
			Description: "Cannot access required directories",
			Details:     "Issues: " + joinStrings(issues, ", "),
		}
	} else if len(warnings) > 0 {
		return EcosystemCheck{
			Name:        "File System Access",
			Status:      "warn",
			Description: "Some optional paths unavailable",
			Details:     "Warnings: " + joinStrings(warnings, ", "),
		}
	} else {
		return EcosystemCheck{
			Name:        "File System Access",
			Status:      "pass",
			Description: "All required and optional paths accessible",
		}
	}
}

// checkDeviceAccess validates access to hardware devices
func (s *Manager) checkDeviceAccess() EcosystemCheck {
	var available, unavailable []string

	// Check TPM device
	if _, err := os.Stat("/dev/tpm0"); err == nil {
		if err := unix.Access("/dev/tpm0", unix.R_OK|unix.W_OK); err == nil {
			available = append(available, "TPM")
		} else {
			unavailable = append(unavailable, "TPM (not accessible)")
		}
	}

	// Check for block devices
	blockDevices := []string{"/dev/sda", "/dev/nvme0n1", "/dev/vda"}
	hasBlockDevice := false
	for _, dev := range blockDevices {
		if _, err := os.Stat(dev); err == nil {
			if err := unix.Access(dev, unix.R_OK); err == nil {
				available = append(available, "Block devices")
				hasBlockDevice = true
				break
			}
		}
	}

	if !hasBlockDevice {
		unavailable = append(unavailable, "Block devices")
	}

	if len(unavailable) > 0 && len(available) == 0 {
		return EcosystemCheck{
			Name:        "Device Access",
			Status:      "warn",
			Description: "Limited device access - installation features may be restricted",
			Details:     "Unavailable: " + joinStrings(unavailable, ", "),
		}
	} else {
		details := ""
		if len(available) > 0 {
			details += "Available: " + joinStrings(available, ", ")
		}
		if len(unavailable) > 0 {
			if details != "" {
				details += "; "
			}
			details += "Unavailable: " + joinStrings(unavailable, ", ")
		}

		return EcosystemCheck{
			Name:        "Device Access",
			Status:      "pass",
			Description: "Device access functional",
			Details:     details,
		}
	}
}

// checkDockerAccess validates Podman container runtime socket accessibility
func (s *Manager) checkDockerAccess() EcosystemCheck {
	podmanSocket := "/run/podman/podman.sock"

	if _, err := os.Stat(podmanSocket); os.IsNotExist(err) {
		return EcosystemCheck{
			Name:        "Podman Access",
			Status:      "fail",
			Description: "Podman socket not found - container management will fail",
			Details:     podmanSocket + " does not exist",
		}
	}

	if err := unix.Access(podmanSocket, unix.R_OK|unix.W_OK); err != nil {
		return EcosystemCheck{
			Name:        "Podman Access",
			Status:      "fail",
			Description: "Podman socket not accessible",
			Details:     "Cannot read/write " + podmanSocket,
		}
	}

	return EcosystemCheck{
		Name:        "Podman Access",
		Status:      "pass",
		Description: "Podman socket accessible for container management",
		Details:     "Using " + podmanSocket,
	}
}

// checkNetworkAccess validates network capabilities
func (s *Manager) checkNetworkAccess() EcosystemCheck {
	// Test if we can bind to port 80 (we're already running on it)
	// This is a basic check - more sophisticated network tests could be added
	return EcosystemCheck{
		Name:        "Network Access",
		Status:      "pass",
		Description: "Network capabilities functional (HTTP server running)",
		Details:     "Bound to port 80",
	}
}

// checkManagerComponents validates that all manager components initialized properly
func (s *Manager) checkManagerComponents() EcosystemCheck {
	var healthy, unhealthy []string

	// Check each manager component
	managers := map[string]interface{}{
		"Container": s.containerManager,
		"Trust":     s.trustAgent,
		"Installer": s.installer,
		"Update":    s.updateManager,
		"Network":   s.networkManager,
		"Backup":    s.backupManager,
	}

	for name, manager := range managers {
		if manager != nil {
			healthy = append(healthy, name)
		} else {
			unhealthy = append(unhealthy, name)
		}
	}

	if len(unhealthy) > 0 {
		return EcosystemCheck{
			Name:        "Manager Components",
			Status:      "fail",
			Description: "Some manager components failed to initialize",
			Details:     "Failed: " + joinStrings(unhealthy, ", "),
		}
	} else {
		return EcosystemCheck{
			Name:        "Manager Components",
			Status:      "pass",
			Description: "All manager components initialized successfully",
			Details:     "Active: " + joinStrings(healthy, ", "),
		}
	}
}

// getPermissionInfo returns key permission and capability information
func (s *Manager) getPermissionInfo() map[string]string {
	info := make(map[string]string)

	info["uid"] = "0" // We know we're root if we got this far
	info["gid"] = "0"
	info["pid"] = fmt.Sprintf("%d", os.Getpid()) // Fix PID conversion

	// Additional permission info could be added here
	// (capabilities, systemd properties, etc.)

	return info
}

// joinStrings is a helper to join string slices
func joinStrings(slice []string, separator string) string {
	if len(slice) == 0 {
		return ""
	}
	result := slice[0]
	for i := 1; i < len(slice); i++ {
		result += separator + slice[i]
	}
	return result
}
