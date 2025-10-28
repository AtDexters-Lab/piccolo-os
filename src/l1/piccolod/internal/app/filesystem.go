package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"piccolod/internal/api"
	"piccolod/internal/state/paths"
)

const (
	AppsDir    = "apps"
	EnabledDir = "enabled"
	CacheDir   = "cache"
)

// FilesystemStateManager manages app state using filesystem as source of truth
type FilesystemStateManager struct {
	stateDir   string
	appsDir    string
	enabledDir string
	cacheDir   string

	// In-memory cache for performance
	cache   map[string]*AppInstance
	cacheMu sync.RWMutex

	// File system mutex for atomic operations
	fsMu sync.Mutex
}

// AppMetadata represents runtime metadata stored separately from app.yaml
type AppMetadata struct {
	Name        string    `json:"name"`
	Status      string    `json:"status"` // "created", "running", "stopped", "error"
	ContainerID string    `json:"container_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Enabled     bool      `json:"enabled"`
}

// NewFilesystemStateManager creates a new filesystem state manager
func NewFilesystemStateManager(stateDir string) (*FilesystemStateManager, error) {
	if stateDir == "" {
		stateDir = paths.Root()
	}

	info, err := os.Stat(stateDir)
	if err != nil {
		return nil, fmt.Errorf("state directory unavailable: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("state directory %s is not a directory", stateDir)
	}

	fsm := &FilesystemStateManager{
		stateDir:   stateDir,
		appsDir:    filepath.Join(stateDir, AppsDir),
		enabledDir: filepath.Join(stateDir, EnabledDir),
		cacheDir:   filepath.Join(stateDir, CacheDir),
		cache:      make(map[string]*AppInstance),
	}

	// Create directory structure
	if err := fsm.initDirectories(); err != nil {
		return nil, fmt.Errorf("failed to initialize directories: %w", err)
	}

	// Load apps from filesystem into cache
	if err := fsm.loadCache(); err != nil {
		return nil, fmt.Errorf("failed to load cache: %w", err)
	}

	return fsm, nil
}

// initDirectories creates the required directory structure
func (fsm *FilesystemStateManager) initDirectories() error {
	dirs := []string{fsm.appsDir, fsm.enabledDir, fsm.cacheDir}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// loadCache loads all apps from filesystem into memory cache
func (fsm *FilesystemStateManager) loadCache() error {
	entries, err := os.ReadDir(fsm.appsDir)
	if err != nil {
		return fmt.Errorf("failed to read apps directory: %w", err)
	}

	fsm.cacheMu.Lock()
	defer fsm.cacheMu.Unlock()

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		appName := entry.Name()
		app, err := fsm.loadAppFromDisk(appName)
		if err != nil {
			// Log error but continue loading other apps
			fmt.Printf("Warning: failed to load app %s: %v\n", appName, err)
			continue
		}

		fsm.cache[appName] = app
	}

	return nil
}

// loadAppFromDisk loads a single app from filesystem
func (fsm *FilesystemStateManager) loadAppFromDisk(appName string) (*AppInstance, error) {
	appDir := filepath.Join(fsm.appsDir, appName)

	// Load app.yaml
	appDefPath := filepath.Join(appDir, "app.yaml")
	appDefData, err := os.ReadFile(appDefPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read app.yaml: %w", err)
	}

	appDef, err := ParseAppDefinition(appDefData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse app.yaml: %w", err)
	}

	// Load metadata.json
	metadataPath := filepath.Join(appDir, "metadata.json")
	metadataData, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata.json: %w", err)
	}

	var metadata AppMetadata
	if err := json.Unmarshal(metadataData, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse metadata.json: %w", err)
	}

	// Check if app is enabled (symlink exists)
	enabledPath := filepath.Join(fsm.enabledDir, appName)
	_, err = os.Lstat(enabledPath)
	// enabled := err == nil  // Not currently used in AppInstance

	// Create AppInstance
	app := &AppInstance{
		Name:        appDef.Name,
		Image:       appDef.Image,
		Type:        appDef.Type,
		Status:      metadata.Status,
		ContainerID: metadata.ContainerID,
		// Ports removed in listeners model
		Environment: appDef.Environment,
		CreatedAt:   metadata.CreatedAt,
		UpdatedAt:   metadata.UpdatedAt,
	}

	return app, nil
}

// BackupCurrentAppDefinition writes current app.yaml to app.prev.yaml for rollback
func (fsm *FilesystemStateManager) BackupCurrentAppDefinition(name string) error {
	fsm.fsMu.Lock()
	defer fsm.fsMu.Unlock()
	appDir := filepath.Join(fsm.appsDir, name)
	cur := filepath.Join(appDir, "app.yaml")
	prev := filepath.Join(appDir, "app.prev.yaml")
	data, err := os.ReadFile(cur)
	if err != nil {
		return fmt.Errorf("read current app.yaml: %w", err)
	}
	if err := os.WriteFile(prev, data, 0644); err != nil {
		return fmt.Errorf("write app.prev.yaml: %w", err)
	}
	return nil
}

// GetPreviousAppDefinition reads app.prev.yaml if present
func (fsm *FilesystemStateManager) GetPreviousAppDefinition(name string) (*api.AppDefinition, error) {
	appDir := filepath.Join(fsm.appsDir, name)
	prev := filepath.Join(appDir, "app.prev.yaml")
	data, err := os.ReadFile(prev)
	if err != nil {
		return nil, fmt.Errorf("previous definition not found: %w", err)
	}
	def, err := ParseAppDefinition(data)
	if err != nil {
		return nil, fmt.Errorf("parse previous app.yaml: %w", err)
	}
	return def, nil
}

// GetAppDefinition reads and parses app.yaml for a given app name
func (fsm *FilesystemStateManager) GetAppDefinition(name string) (*api.AppDefinition, error) {
	appDir := filepath.Join(fsm.appsDir, name)
	appDefPath := filepath.Join(appDir, "app.yaml")
	data, err := os.ReadFile(appDefPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read app.yaml: %w", err)
	}
	appDef, err := ParseAppDefinition(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse app.yaml: %w", err)
	}
	return appDef, nil
}

// StoreApp saves app definition and metadata to filesystem
func (fsm *FilesystemStateManager) StoreApp(app *AppInstance, appDef *api.AppDefinition) error {
	fsm.fsMu.Lock()
	defer fsm.fsMu.Unlock()

	appDir := filepath.Join(fsm.appsDir, app.Name)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return fmt.Errorf("failed to create app directory: %w", err)
	}

	// Store app.yaml
	appDefPath := filepath.Join(appDir, "app.yaml")
	appDefData, err := SerializeAppDefinition(appDef)
	if err != nil {
		return fmt.Errorf("failed to serialize app definition: %w", err)
	}

	if err := os.WriteFile(appDefPath, appDefData, 0644); err != nil {
		return fmt.Errorf("failed to write app.yaml: %w", err)
	}

	// Store metadata.json
	metadata := AppMetadata{
		Name:        app.Name,
		Status:      app.Status,
		ContainerID: app.ContainerID,
		CreatedAt:   app.CreatedAt,
		UpdatedAt:   app.UpdatedAt,
	}

	metadataData, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize metadata: %w", err)
	}

	metadataPath := filepath.Join(appDir, "metadata.json")
	if err := os.WriteFile(metadataPath, metadataData, 0644); err != nil {
		return fmt.Errorf("failed to write metadata.json: %w", err)
	}

	// Update cache
	fsm.cacheMu.Lock()
	fsm.cache[app.Name] = app
	fsm.cacheMu.Unlock()

	return nil
}

// UpdateAppStatus updates just the app status and updated timestamp
func (fsm *FilesystemStateManager) UpdateAppStatus(name, status string) error {
	fsm.fsMu.Lock()
	defer fsm.fsMu.Unlock()

	// Update cache first
	fsm.cacheMu.Lock()
	app, exists := fsm.cache[name]
	if !exists {
		fsm.cacheMu.Unlock()
		return fmt.Errorf("app not found: %s", name)
	}

	app.Status = status
	app.UpdatedAt = time.Now()
	fsm.cacheMu.Unlock()

	// Update filesystem
	appDir := filepath.Join(fsm.appsDir, name)
	metadataPath := filepath.Join(appDir, "metadata.json")

	metadata := AppMetadata{
		Name:        app.Name,
		Status:      status,
		ContainerID: app.ContainerID,
		CreatedAt:   app.CreatedAt,
		UpdatedAt:   app.UpdatedAt,
	}

	metadataData, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize metadata: %w", err)
	}

	if err := os.WriteFile(metadataPath, metadataData, 0644); err != nil {
		return fmt.Errorf("failed to write metadata.json: %w", err)
	}

	return nil
}

// GetApp retrieves an app from cache (fast access)
func (fsm *FilesystemStateManager) GetApp(name string) (*AppInstance, bool) {
	fsm.cacheMu.RLock()
	defer fsm.cacheMu.RUnlock()

	app, exists := fsm.cache[name]
	return app, exists
}

// ListApps returns all apps from cache
func (fsm *FilesystemStateManager) ListApps() []*AppInstance {
	fsm.cacheMu.RLock()
	defer fsm.cacheMu.RUnlock()

	apps := make([]*AppInstance, 0, len(fsm.cache))
	for _, app := range fsm.cache {
		apps = append(apps, app)
	}

	return apps
}

// RemoveApp removes an app from both filesystem and cache
func (fsm *FilesystemStateManager) RemoveApp(name string) error {
	fsm.fsMu.Lock()
	defer fsm.fsMu.Unlock()

	// Remove from filesystem
	appDir := filepath.Join(fsm.appsDir, name)
	if err := os.RemoveAll(appDir); err != nil {
		return fmt.Errorf("failed to remove app directory: %w", err)
	}

	// Remove enabled symlink if it exists
	enabledPath := filepath.Join(fsm.enabledDir, name)
	_ = os.Remove(enabledPath) // Ignore error if symlink doesn't exist

	// Remove from cache
	fsm.cacheMu.Lock()
	delete(fsm.cache, name)
	fsm.cacheMu.Unlock()

	return nil
}

// EnableApp creates a symlink to enable app (systemctl-style)
func (fsm *FilesystemStateManager) EnableApp(name string) error {
	fsm.fsMu.Lock()
	defer fsm.fsMu.Unlock()

	appDir := filepath.Join(fsm.appsDir, name)
	enabledPath := filepath.Join(fsm.enabledDir, name)

	// Check if app exists
	if _, err := os.Stat(appDir); err != nil {
		return fmt.Errorf("app not found: %s", name)
	}

	// Create symlink (relative path for portability)
	relativePath := filepath.Join("..", AppsDir, name)
	if err := os.Symlink(relativePath, enabledPath); err != nil {
		if os.IsExist(err) {
			return nil // Already enabled
		}
		return fmt.Errorf("failed to create symlink: %w", err)
	}

	return nil
}

// DisableApp removes the symlink to disable app
func (fsm *FilesystemStateManager) DisableApp(name string) error {
	fsm.fsMu.Lock()
	defer fsm.fsMu.Unlock()

	enabledPath := filepath.Join(fsm.enabledDir, name)
	if err := os.Remove(enabledPath); err != nil {
		if os.IsNotExist(err) {
			return nil // Already disabled
		}
		return fmt.Errorf("failed to remove symlink: %w", err)
	}

	return nil
}

// IsAppEnabled checks if app is enabled (symlink exists)
func (fsm *FilesystemStateManager) IsAppEnabled(name string) bool {
	enabledPath := filepath.Join(fsm.enabledDir, name)
	_, err := os.Lstat(enabledPath)
	return err == nil
}

// ListEnabledApps returns names of all enabled apps
func (fsm *FilesystemStateManager) ListEnabledApps() ([]string, error) {
	entries, err := os.ReadDir(fsm.enabledDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read enabled directory: %w", err)
	}

	var enabled []string
	for _, entry := range entries {
		// Only count symlinks
		if entry.Type()&os.ModeSymlink != 0 {
			enabled = append(enabled, entry.Name())
		}
	}

	return enabled, nil
}
