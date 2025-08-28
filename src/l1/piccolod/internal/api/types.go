package api

// Container represents the data structure for a container in our public API.
type Container struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Image string `json:"image"`
	State string `json:"state"`
}

// CreateContainerRequest defines the JSON payload for creating a new container.
type CreateContainerRequest struct {
	Name      string    `json:"name"`
	Image     string    `json:"image"`
	Resources Resources `json:"resources,omitempty"`
}

// Resources defines CPU, RAM, and other resource quotas for a container.
type Resources struct {
	CPU    float64 `json:"cpu_cores,omitempty"` // e.g., 0.5 for half a core
	Memory int64   `json:"memory_mb,omitempty"` // Memory in Megabytes
}

// DiskInfo provides detailed, human-readable information about a physical disk.
type DiskInfo struct {
	Path      string `json:"path"`      // e.g., /dev/sda
	Model     string `json:"model"`     // e.g., "Samsung SSD 970 EVO"
	SizeBytes int64  `json:"size_bytes"`
	IsSSD     bool   `json:"is_ssd"`
}

// StoragePoolInfo represents the status of the main storage pool.
type StoragePoolInfo struct {
	TotalBytes     int64    `json:"total_bytes"`
	UsedBytes      int64    `json:"used_bytes"`
	FreeBytes      int64    `json:"free_bytes"`
	ComponentDisks []string `json:"component_disks"`
}

// BackupTarget defines a destination for a backup.
type BackupTarget struct {
	Type string `json:"type"`         // e.g., "local_drive", "google_drive", "piccolo_central"
	Path string `json:"path,omitempty"` // For local_drive, e.g., "/media/my-usb"
}

// AppDefinition represents an app.yaml definition file
type AppDefinition struct {
	Name        string            `yaml:"name" json:"name"`
	Image       string            `yaml:"image,omitempty" json:"image,omitempty"`
	Build       *AppBuild         `yaml:"build,omitempty" json:"build,omitempty"`
	Subdomain   string            `yaml:"subdomain,omitempty" json:"subdomain,omitempty"`
	Type        string            `yaml:"type,omitempty" json:"type,omitempty"` // "system" or "user"
	Ports       map[string]AppPort `yaml:"ports,omitempty" json:"ports,omitempty"`
	Storage     *AppStorage       `yaml:"storage,omitempty" json:"storage,omitempty"`
	Filesystem  *AppFilesystem    `yaml:"filesystem,omitempty" json:"filesystem,omitempty"`
	Permissions *AppPermissions   `yaml:"permissions,omitempty" json:"permissions,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty" json:"environment,omitempty"`
	Resources   *AppResources     `yaml:"resources,omitempty" json:"resources,omitempty"`
	HealthCheck *AppHealthCheck   `yaml:"healthcheck,omitempty" json:"healthcheck,omitempty"`
	DependsOn   []string          `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
	AppConfig   interface{}       `yaml:"app_config,omitempty" json:"app_config,omitempty"`
	Extensions  map[string]interface{} `yaml:"x-piccolo,omitempty" json:"x-piccolo,omitempty"`
}

// AppBuild defines container build configuration
type AppBuild struct {
	Containerfile string            `yaml:"containerfile,omitempty" json:"containerfile,omitempty"` // Path or inline content
	Context       string            `yaml:"context,omitempty" json:"context,omitempty"`
	BuildArgs     map[string]string `yaml:"build_args,omitempty" json:"build_args,omitempty"`
	Target        string            `yaml:"target,omitempty" json:"target,omitempty"`
	Git           string            `yaml:"git,omitempty" json:"git,omitempty"`
	Branch        string            `yaml:"branch,omitempty" json:"branch,omitempty"`
}

// AppStorage defines storage configuration
type AppStorage struct {
	Persistent map[string]AppVolume `yaml:"persistent,omitempty" json:"persistent,omitempty"`
	Temporary  map[string]AppVolume `yaml:"temporary,omitempty" json:"temporary,omitempty"`
}

// AppFilesystem defines filesystem persistence
type AppFilesystem struct {
	Persistent bool `yaml:"persistent,omitempty" json:"persistent,omitempty"`
	// Note: Storage is always local (no federated option for filesystem persistence)
}

// AppPermissions defines security permissions
type AppPermissions struct {
	Network    *AppNetworkPermissions    `yaml:"network,omitempty" json:"network,omitempty"`
	Resources  *AppResourcePermissions   `yaml:"resources,omitempty" json:"resources,omitempty"`
	Filesystem *AppFilesystemPermissions `yaml:"filesystem,omitempty" json:"filesystem,omitempty"`
	Preset     string                    `yaml:"preset,omitempty" json:"preset,omitempty"`
}

type AppNetworkPermissions struct {
	Internet       string   `yaml:"internet,omitempty" json:"internet,omitempty"` // "allow" or "deny"
	LocalNetwork   string   `yaml:"local_network,omitempty" json:"local_network,omitempty"`
	DNS            string   `yaml:"dns,omitempty" json:"dns,omitempty"`
	AllowedDomains []string `yaml:"allowed_domains,omitempty" json:"allowed_domains,omitempty"`
	AllowedIPs     []string `yaml:"allowed_ips,omitempty" json:"allowed_ips,omitempty"`
}

type AppResourcePermissions struct {
	MaxProcesses  int  `yaml:"max_processes,omitempty" json:"max_processes,omitempty"`
	MaxOpenFiles  int  `yaml:"max_open_files,omitempty" json:"max_open_files,omitempty"`
	Privileged    bool `yaml:"privileged,omitempty" json:"privileged,omitempty"`
}

type AppFilesystemPermissions struct {
	ReadOnlyRoot bool   `yaml:"read_only_root,omitempty" json:"read_only_root,omitempty"`
	DeviceAccess string `yaml:"device_access,omitempty" json:"device_access,omitempty"` // "allow" or "deny"
}

// AppResources defines resource limits
type AppResources struct {
	Limits *AppResourceLimits `yaml:"limits,omitempty" json:"limits,omitempty"`
}

type AppResourceLimits struct {
	Memory  string  `yaml:"memory,omitempty" json:"memory,omitempty"`
	CPU     float64 `yaml:"cpu,omitempty" json:"cpu,omitempty"`
	Storage string  `yaml:"storage,omitempty" json:"storage,omitempty"`
}

// AppHealthCheck defines health monitoring
type AppHealthCheck struct {
	HTTP *AppHTTPHealthCheck `yaml:"http,omitempty" json:"http,omitempty"`
}

type AppHTTPHealthCheck struct {
	Path    string `yaml:"path" json:"path"`
	Port    string `yaml:"port" json:"port"`
	Timeout string `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Retries int    `yaml:"retries,omitempty" json:"retries,omitempty"`
}

// AppPort defines port mapping for an application
type AppPort struct {
	Container int `yaml:"container" json:"container"`
	Host      int `yaml:"host" json:"host"`
}

// AppVolume defines volume mapping for an application
type AppVolume struct {
	Container string `yaml:"container" json:"container"`
	Host      string `yaml:"host,omitempty" json:"host,omitempty"` // Auto-generated if not specified
	SizeLimit string `yaml:"size_limit,omitempty" json:"size_limit,omitempty"`
}

// App represents an installed application
type App struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Image       string            `json:"image"`
	Subdomain   string            `json:"subdomain"`
	Type        string            `json:"type"`
	Status      string            `json:"status"` // "running", "stopped", "error"
	ContainerID string            `json:"container_id,omitempty"`
	Ports       []AppPort         `json:"ports,omitempty"`
	Volumes     []AppVolume       `json:"volumes,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
}

// InstallAppRequest defines the request to install an app
type InstallAppRequest struct {
	AppDefinition string `json:"app_definition"` // YAML content as string
}

// AppListResponse defines the response for listing apps
type AppListResponse struct {
	Apps []App `json:"apps"`
}
