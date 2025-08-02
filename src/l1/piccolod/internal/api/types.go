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
