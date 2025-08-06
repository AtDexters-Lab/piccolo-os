# Flatcar Health Check Integration

## Overview

This document details how `piccolod` integrates with Flatcar Linux's built-in health check and automatic rollback system to ensure reliable OS updates with automatic failure recovery.

## Flatcar's Health Check Mechanism

### GPT Partition Success Marking

Flatcar uses **GPT partition attributes** to track update success:

```bash
# Each root partition has three critical attributes:
sudo cgpt show "$(rootdev -s /usr)"
# priority: Boot order preference (higher = preferred)
# tries: Remaining boot attempts (decrements each boot)
# successful: Success flag (0 or 1)
```

### Automatic Rollback Flow

1. **After Update**: New partition boots with `successful=0`
2. **Boot Process**: GRUB decrements `tries` counter
3. **System Startup**: Services start, including `update_engine`
4. **Success Detection**: After ~1-2 minutes of stable operation
5. **Success Marking**: `update_engine` sets `successful=1` 
6. **Failure Case**: If `tries` reaches 0 without success → GRUB boots previous partition

## Integration Architecture

### SystemD Dependency Approach (Recommended)

**Core Principle**: Make `update-engine` service depend on `piccolod` success.

```ini
# /etc/systemd/system/update-engine.service.d/10-piccolod-dependency.conf
[Unit]
After=piccolod.service
Requires=piccolod.service
```

**How It Works:**
1. `piccolod.service` starts and runs comprehensive health checks
2. If `piccolod` fails to start → `update-engine` never marks partition successful
3. Flatcar automatically rolls back on next reboot (when `tries` reaches 0)
4. If `piccolod` starts successfully → `update-engine` proceeds with success marking

### piccolod Health Assessment

```go
// Internal health check performed during service startup
func (m *Manager) assessSystemHealth() bool {
    checks := []HealthCheck{
        m.checkDockerRunning,
        m.checkTPMFunctional, 
        m.checkStorageHealthy,
        m.checkNetworkConnectivity,
        m.checkContainersHealthy,
        m.checkAPIResponsive,
    }
    
    for _, check := range checks {
        if !check() {
            log.Printf("Health check failed: %v", check)
            return false
        }
    }
    return true
}
```

### Integration Implementation

#### 1. Build-Time Integration

**L0 Build System** (`build_piccolo.sh`):
```bash
# Create systemd override directory
mkdir -p "${ebuild_dir}/files/systemd/system/update-engine.service.d"

# Generate dependency override
cat > "${ebuild_dir}/files/systemd/system/update-engine.service.d/10-piccolod-dependency.conf" << EOF
[Unit]
After=piccolod.service
Requires=piccolod.service

[Service]
# Extended timeout for piccolod health checks
TimeoutStartSec=300
EOF
```

#### 2. Runtime Health Checks

**Health Check Components:**

```go
// Docker runtime health
func (m *Manager) checkDockerRunning() bool {
    client, err := docker.NewClientFromEnv()
    if err != nil {
        return false
    }
    _, err = client.Ping(context.Background())
    return err == nil
}

// TPM functionality
func (m *Manager) checkTPMFunctional() bool {
    rw, err := tpm2.OpenTPM("/dev/tpm0")
    if err != nil {
        return false
    }
    defer rw.Close()
    
    // Perform basic TPM operation (e.g., read PCRs)
    _, err = tpm2.ReadPCR(rw, 0, tpm2.AlgSHA256)
    return err == nil
}

// Storage health
func (m *Manager) checkStorageHealthy() bool {
    // Check disk space, I/O errors, filesystem health
    var stat syscall.Statfs_t
    err := syscall.Statfs("/", &stat)
    if err != nil {
        return false
    }
    
    // Ensure at least 10% free space
    freePercent := float64(stat.Bavail) / float64(stat.Blocks) * 100
    return freePercent > 10.0
}

// Network connectivity
func (m *Manager) checkNetworkConnectivity() bool {
    // Test connectivity to essential services
    targets := []string{
        "8.8.8.8:53",           // DNS
        "piccolo.space:443",    // Piccolo services
    }
    
    for _, target := range targets {
        conn, err := net.DialTimeout("tcp", target, 5*time.Second)
        if err != nil {
            return false
        }
        conn.Close()
    }
    return true
}

// Container health
func (m *Manager) checkContainersHealthy() bool {
    // Check critical containers are running
    client, err := docker.NewClientFromEnv()
    if err != nil {
        return false
    }
    
    containers, err := client.ContainerList(context.Background(), types.ContainerListOptions{})
    if err != nil {
        return false
    }
    
    // Verify no containers in unhealthy state
    for _, container := range containers {
        if container.State == "unhealthy" {
            return false
        }
    }
    return true
}

// API responsiveness
func (m *Manager) checkAPIResponsive() bool {
    // Test internal API endpoint
    resp, err := http.Get("http://localhost:8080/api/v1/system/health")
    if err != nil {
        return false
    }
    defer resp.Body.Close()
    return resp.StatusCode == 200
}
```

#### 3. Service Configuration

**piccolod.service** with health-aware startup:
```ini
[Unit]
Description=Piccolo OS Daemon
After=network.target docker.service
Wants=network.target docker.service

[Service]
Type=notify
ExecStart=/usr/bin/piccolod --health-check-mode
Restart=always
RestartSec=10
TimeoutStartSec=180

# Health check environment
Environment=PICCOLOD_HEALTH_CHECK_TIMEOUT=120
Environment=PICCOLOD_STARTUP_HEALTH_CHECKS=true

# Security
User=root
Group=root

[Install]
WantedBy=multi-user.target
```

**Health Check Startup Sequence:**
```go
func main() {
    // Initialize core components
    manager := NewManager()
    
    if os.Getenv("PICCOLOD_STARTUP_HEALTH_CHECKS") == "true" {
        log.Println("Performing startup health checks...")
        
        // Wait for system stabilization
        time.Sleep(30 * time.Second)
        
        // Run comprehensive health assessment
        if !manager.assessSystemHealth() {
            log.Fatal("Startup health checks failed - preventing update success marking")
        }
        
        log.Println("All health checks passed - allowing update success")
    }
    
    // Send systemd notification that service is ready
    daemon.SdNotify(false, daemon.SdNotifyReady)
    
    // Start main service loop
    manager.Run()
}
```

## Health Check API

### External Health Endpoint

For integration with monitoring systems and manual verification:

```http
GET /api/v1/system/health
{
  "status": "healthy|degraded|unhealthy",
  "timestamp": "2024-01-15T10:35:00Z",
  "version": "1.2.3",
  "boot_id": "abc123def456",
  "uptime_seconds": 3600,
  "components": {
    "piccolod": {
      "status": "healthy",
      "last_check": "2024-01-15T10:35:00Z"
    },
    "docker": {
      "status": "healthy", 
      "version": "24.0.6",
      "containers_running": 5
    },
    "tpm": {
      "status": "healthy",
      "version": "2.0",
      "manufacturer": "Infineon"
    },
    "storage": {
      "status": "healthy",
      "free_space_percent": 65.2,
      "io_errors": 0
    },
    "network": {
      "status": "healthy",
      "connectivity_test": "passed",
      "dns_resolution": "working"
    }
  },
  "update_status": "completed",
  "disk_encryption": "enabled",
  "startup_health_check": "passed"
}
```

### Health Check Thresholds

```go
type HealthThresholds struct {
    MinDiskSpacePercent  float64 // 10%
    MaxIOErrors         int     // 0
    NetworkTimeoutSec   int     // 5
    TPMOperationTimeout int     // 10
    ContainerStartupSec int     // 30
}
```

## Testing Integration

### VM Testing with Health Checks

Extend existing `test_piccolo_os_image.sh`:

```bash
test_health_check_integration() {
    log "Testing health check integration..."
    
    # Start VM with test scenarios
    start_piccolo_vm_with_network
    
    # Wait for boot completion
    wait_for_ssh_connectivity
    
    # Test healthy scenario
    ssh_exec "systemctl status piccolod"
    ssh_exec "curl -f http://localhost:8080/api/v1/system/health"
    
    # Test unhealthy scenario (simulate failure)
    ssh_exec "systemctl stop docker"
    ssh_exec "systemctl restart piccolod"
    
    # Verify health check fails
    if ssh_exec "curl -f http://localhost:8080/api/v1/system/health"; then
        log "ERROR: Health check should have failed"
        return 1
    fi
    
    # Verify rollback protection
    ssh_exec "cgpt show /dev/vda | grep successful=0"
    
    log "Health check integration test passed"
}
```

### Simulated Update Failure Testing

```bash
test_update_failure_rollback() {
    log "Testing update failure and rollback..."
    
    # Create a "bad" update that breaks piccolod
    create_broken_update_image
    
    # Apply the update
    trigger_update_via_api
    
    # Wait for reboot
    wait_for_reboot
    
    # Verify system rolled back to previous version
    check_version_rollback
    
    # Verify system is functional again
    verify_piccolod_running
    
    log "Update failure rollback test passed"
}
```

## Monitoring and Observability

### Health Metrics

```go
// Prometheus metrics for health monitoring
var (
    healthCheckDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "piccolod_health_check_duration_seconds",
            Help: "Duration of health checks",
        },
        []string{"component"},
    )
    
    healthCheckStatus = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "piccolod_health_check_status",
            Help: "Health check status (1=healthy, 0=unhealthy)",
        },
        []string{"component"},
    )
)
```

### Logging Integration

```go
func (m *Manager) logHealthStatus(component string, healthy bool, details map[string]interface{}) {
    log.WithFields(log.Fields{
        "component": component,
        "healthy": healthy,
        "details": details,
        "timestamp": time.Now(),
        "boot_id": getBootID(),
    }).Info("Health check completed")
}
```

## Advanced Integration Scenarios

### Graceful Degradation

```go
// Allow updates to proceed with non-critical component failures
func (m *Manager) assessSystemHealthWithGracefulDegradation() (status string, critical bool) {
    criticalChecks := []HealthCheck{
        m.checkTPMFunctional,
        m.checkStorageHealthy,
        m.checkAPIResponsive,
    }
    
    nonCriticalChecks := []HealthCheck{
        m.checkDockerRunning,
        m.checkNetworkConnectivity,
    }
    
    // Critical checks must pass
    for _, check := range criticalChecks {
        if !check() {
            return "unhealthy", true
        }
    }
    
    // Non-critical can be degraded
    nonCriticalPassed := 0
    for _, check := range nonCriticalChecks {
        if check() {
            nonCriticalPassed++
        }
    }
    
    if nonCriticalPassed == len(nonCriticalChecks) {
        return "healthy", false
    } else if nonCriticalPassed > 0 {
        return "degraded", false // Still allow update success
    } else {
        return "unhealthy", true
    }
}
```

### Custom Health Policies

```yaml
# /etc/piccolod/health-policy.yaml
health_policy:
  startup:
    timeout_seconds: 120
    required_components: ["tpm", "storage", "api"]
    optional_components: ["docker", "network"]
  
  runtime:
    check_interval_seconds: 60
    failure_threshold: 3
    
  rollback:
    enable_automatic: true
    failure_conditions:
      - critical_component_failure
      - startup_timeout
      - api_unresponsive_300s
```

This health check integration provides robust, automatic failure detection and recovery while leveraging Flatcar's proven rollback mechanisms. The system ensures that updates only succeed when `piccolod` and all critical components are functioning correctly.