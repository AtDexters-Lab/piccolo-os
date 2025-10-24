package container

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestValidateContainerName tests container name validation
func TestValidateContainerName(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{"valid simple name", "nginx", false},
		{"valid name with hyphen", "my-app", false},
		{"valid registry image", "docker.io/nginx:latest", false},
		{"valid with underscore", "my_app", false},
		{"empty name", "", true},
		{"too long", string(make([]byte, 256)), true},
		{"invalid chars", "app!@#", true},
		{"starts with hyphen", "-app", true},
		{"ends with hyphen", "app-", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateContainerName(tt.input)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for input %q but got none", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for input %q: %v", tt.input, err)
				}
			}
		})
	}
}

// TestValidatePort tests port validation
func TestValidatePort(t *testing.T) {
	tests := []struct {
		name        string
		port        int
		expectError bool
	}{
		{"valid port 80", 80, false},
		{"valid port 8080", 8080, false},
		{"valid port 65535", 65535, false},
		{"port too low", 0, true},
		{"port too high", 65536, true},
		{"negative port", -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePort(tt.port)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for port %d but got none", tt.port)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for port %d: %v", tt.port, err)
				}
			}
		})
	}
}

// TestValidatePath tests path validation
func TestValidatePath(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		expectError bool
	}{
		{"valid absolute path", "/data", false},
		{"valid nested path", "/var/lib/data", false},
		{"relative path", "data", true},
		{"empty path", "", true},
		{"path traversal", "/data/../etc/passwd", true},
		{"invalid chars", "/data$pecial", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePath(tt.path)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for path %q but got none", tt.path)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for path %q: %v", tt.path, err)
				}
			}
		})
	}
}

// TestValidateEnvKey tests environment key validation
func TestValidateEnvKey(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		expectError bool
	}{
		{"valid key", "NODE_ENV", false},
		{"valid underscore", "_DEBUG", false},
		{"starts with letter", "DEBUG", false},
		{"empty key", "", true},
		{"starts with number", "123KEY", true},
		{"invalid chars", "KEY-NAME", true},
		{"too long", string(make([]byte, 256)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEnvKey(tt.key)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for key %q but got none", tt.key)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for key %q: %v", tt.key, err)
				}
			}
		})
	}
}

// TestValidateEnvValue tests environment value validation
func TestValidateEnvValue(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		expectError bool
	}{
		{"valid value", "production", false},
		{"empty value", "", false},
		{"value with spaces", "hello world", false},
		{"control character", "hello\x00world", true},
		{"too long", string(make([]byte, 4097)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEnvValue(tt.value)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for value %q but got none", tt.value)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for value %q: %v", tt.value, err)
				}
			}
		})
	}
}

// TestValidateContainerSpec tests full spec validation
func TestValidateContainerSpec(t *testing.T) {
	tests := []struct {
		name        string
		spec        ContainerCreateSpec
		expectError bool
		errorContains string
	}{
		{
			name: "valid spec",
			spec: ContainerCreateSpec{
				Name:  "test-app",
				Image: "nginx:latest",
				Ports: []PortMapping{{Host: 8080, Container: 80}},
				Environment: map[string]string{"NODE_ENV": "production"},
			},
			expectError: false,
		},
		{
			name: "invalid container name",
			spec: ContainerCreateSpec{
				Name:  "invalid!name",
				Image: "nginx:latest",
			},
			expectError: true,
			errorContains: "invalid container name",
		},
		{
			name: "invalid port",
			spec: ContainerCreateSpec{
				Name:  "test-app",
				Image: "nginx:latest",
				Ports: []PortMapping{{Host: 0, Container: 80}},
			},
			expectError: true,
			errorContains: "invalid host port",
		},
		{
			name: "invalid volume path",
			spec: ContainerCreateSpec{
				Name:  "test-app",
				Image: "nginx:latest",
				Volumes: []VolumeMapping{{Host: "relative/path", Container: "/data"}},
			},
			expectError: true,
			errorContains: "invalid host path",
		},
		{
			name: "invalid env key",
			spec: ContainerCreateSpec{
				Name:  "test-app",
				Image: "nginx:latest",
				Environment: map[string]string{"123KEY": "value"},
			},
			expectError: true,
			errorContains: "invalid environment key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateContainerSpec(tt.spec)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorContains != "" && !containsString(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain %q, got %q", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestBuildRunArgsIncludesReplaceForNamedContainers(t *testing.T) {
	spec := ContainerCreateSpec{
		Name:  "nginxdemo",
		Image: "docker.io/library/nginx:alpine",
	}
	args := buildRunArgs(spec)
	foundReplace := false
	for _, arg := range args {
		if arg == "--replace" {
			foundReplace = true
			break
		}
	}
	if !foundReplace {
		t.Fatalf("expected --replace flag in args, got %v", args)
	}
}

// TestPodmanCLI_CreateContainer tests container creation (requires Podman)
func TestPodmanCLI_CreateContainer(t *testing.T) {
	// Skip if running in CI without Podman
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	podman := &PodmanCLI{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use unique name with timestamp to avoid conflicts
	uniqueName := fmt.Sprintf("test-container-%d", time.Now().UnixNano())

	spec := ContainerCreateSpec{
		Name:  uniqueName,
		Image: "alpine:latest",
		Environment: map[string]string{
			"TEST_ENV": "integration-test",
		},
	}

	// Validate spec first
	if err := ValidateContainerSpec(spec); err != nil {
		t.Fatalf("Spec validation failed: %v", err)
	}

    // Create container
    containerID, err := podman.CreateContainer(ctx, spec)
    if err != nil {
        // If podman is not available, skip the test
        if containsString(err.Error(), "executable file not found") {
            t.Skip("Podman not available, skipping integration test")
        }
        t.Fatalf("Failed to create container: %v", err)
    }

	// Cleanup: remove container
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		
		// Stop container first
		_ = podman.StopContainer(cleanupCtx, containerID)
		
		// Remove container
		if err := podman.RemoveContainer(cleanupCtx, containerID); err != nil {
			t.Errorf("Failed to cleanup container %s: %v", containerID, err)
		}
	}()

	// Verify container ID format
	if !isValidContainerID(containerID) {
		t.Errorf("Invalid container ID format: %s", containerID)
	}

	// Test start container
	if err := podman.StartContainer(ctx, containerID); err != nil {
		t.Errorf("Failed to start container: %v", err)
	}

	// Test stop container
	if err := podman.StopContainer(ctx, containerID); err != nil {
		t.Errorf("Failed to stop container: %v", err)
	}
}

// TestIsValidContainerID tests container ID validation
func TestIsValidContainerID(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		expected bool
	}{
		{"valid full ID", "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef", true},
		{"valid short ID", "1234567890ab", true},
		{"too short", "123", false},
		{"too long", "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1", false},
		{"invalid chars", "123456789g", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidContainerID(tt.id)
			if result != tt.expected {
				t.Errorf("isValidContainerID(%q) = %v, expected %v", tt.id, result, tt.expected)
			}
		})
	}
}

// Helper function for string containment check
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && 
		   (s == substr || 
			len(s) > len(substr) && 
			(s[:len(substr)] == substr || 
			 s[len(s)-len(substr):] == substr ||
			 containsSubstring(s[1:len(s)-1], substr)))
}

func containsSubstring(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
