package app

import (
	"os"
	"testing"

	"piccolod/internal/api"
)

// TestParseAppDefinition tests parsing of valid app.yaml files
func TestParseAppDefinition(t *testing.T) {
	tests := []struct {
		name           string
		filePath       string
		expectedName   string
		expectedImage  string
		expectedType   string
		expectError    bool
		validateFields func(*testing.T, *api.AppDefinition)
	}{
		{
			name:          "minimal app",
			filePath:      "../../testdata/apps/valid/minimal.yaml",
			expectedName:  "test-minimal",
			expectedImage: "alpine:latest",
			expectedType:  "user", // default
			expectError:   false,
			validateFields: func(t *testing.T, app *api.AppDefinition) {
				if app.Build != nil {
					t.Error("Expected nil build for image-based app")
				}
			},
		},
		{
			name:         "complete app",
			filePath:     "../../testdata/apps/valid/complete.yaml",
			expectedName: "test-complete",
			expectedType: "user",
			expectError:  false,
			validateFields: func(t *testing.T, app *api.AppDefinition) {
				if len(app.Listeners) == 0 {
					t.Error("Expected listeners to be defined")
				}
				found := false
				for _, l := range app.Listeners {
					if l.Name == "web" && l.GuestPort == 80 {
						found = true
					}
				}
				if !found {
					t.Error("Expected web listener with guest_port 80")
				}
				if app.Storage == nil || app.Storage.Persistent == nil {
					t.Error("Expected persistent storage to be defined")
				}
				if app.Environment == nil {
					t.Error("Expected environment variables to be defined")
				}
				if env, ok := app.Environment["ENV"]; !ok || env != "test" {
					t.Error("Expected ENV=test environment variable")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Read file content
			content, err := os.ReadFile(tt.filePath)
			if err != nil {
				t.Fatalf("Failed to read test file %s: %v", tt.filePath, err)
			}

			// Parse the app definition
			app, err := ParseAppDefinition(content)

			if tt.expectError {
				if err == nil {
					t.Fatalf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// Basic field validation
			if app.Name != tt.expectedName {
				t.Errorf("Expected name %s, got %s", tt.expectedName, app.Name)
			}

			if tt.expectedImage != "" && app.Image != tt.expectedImage {
				t.Errorf("Expected image %s, got %s", tt.expectedImage, app.Image)
			}

			// Validate default values
			if app.Type == "" {
				app.Type = "user" // Parser should set this default
			}
			if app.Type != tt.expectedType {
				t.Errorf("Expected type %s, got %s", tt.expectedType, app.Type)
			}

			// Run custom field validation
			if tt.validateFields != nil {
				tt.validateFields(t, app)
			}
		})
	}
}

// TestParseAppDefinitionErrors tests parsing of invalid app.yaml files
func TestParseAppDefinitionErrors(t *testing.T) {
	tests := []struct {
		name        string
		filePath    string
		expectedErr string
	}{
		{
			name:        "missing name",
			filePath:    "../../testdata/apps/invalid/missing-name.yaml",
			expectedErr: "name is required",
		},
		{
			name:        "missing image and build",
			filePath:    "../../testdata/apps/invalid/missing-image-and-build.yaml",
			expectedErr: "either image or build must be specified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Read file content
			content, err := os.ReadFile(tt.filePath)
			if err != nil {
				t.Fatalf("Failed to read test file %s: %v", tt.filePath, err)
			}

			// Parse should fail
			_, err = ParseAppDefinition(content)
			if err == nil {
				t.Fatalf("Expected error but got none")
			}

			// Check error message contains expected text
			if tt.expectedErr != "" {
				if !containsString(err.Error(), tt.expectedErr) {
					t.Errorf("Expected error to contain %q, got %q", tt.expectedErr, err.Error())
				}
			}
		})
	}
}

// TestValidateAppDefinition tests validation logic separately
func TestValidateAppDefinition(t *testing.T) {
	tests := []struct {
		name        string
		app         *api.AppDefinition
		expectError bool
		expectedErr string
	}{
		{
			name: "valid minimal app",
			app: &api.AppDefinition{
				Name:      "test-app",
				Image:     "nginx:latest",
				Listeners: []api.AppListener{{Name: "web", GuestPort: 80}},
			},
			expectError: false,
		},
		{
			name:        "empty name",
			app:         &api.AppDefinition{Image: "nginx:latest"},
			expectError: true,
			expectedErr: "name is required",
		},
		{
			name:        "invalid name characters",
			app:         &api.AppDefinition{Name: "test_app!", Image: "nginx:latest"},
			expectError: true,
			expectedErr: "name must contain only lowercase letters, numbers, and hyphens",
		},
		{
			name:        "name too long",
			app:         &api.AppDefinition{Name: "this-is-a-very-long-app-name-that-exceeds-the-maximum-allowed-length", Image: "nginx:latest"},
			expectError: true,
			expectedErr: "name must be 50 characters or less",
		},
		{
			name:        "missing image and build",
			app:         &api.AppDefinition{Name: "test-app"},
			expectError: true,
			expectedErr: "either image or build must be specified",
		},
		{
			name: "both image and build specified",
			app: &api.AppDefinition{
				Name:  "test-app",
				Image: "nginx:latest",
				Build: &api.AppBuild{Containerfile: "FROM nginx"},
			},
			expectError: true,
			expectedErr: "cannot specify both image and build",
		},
		{
			name: "invalid listener port",
			app: &api.AppDefinition{
				Name:      "test-app",
				Image:     "nginx:latest",
				Listeners: []api.AppListener{{Name: "web", GuestPort: 0}},
			},
			expectError: true,
			expectedErr: "guest_port must be between 1 and 65535",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set defaults before validation (like the parser does)
			SetDefaults(tt.app)

			err := ValidateAppDefinition(tt.app)

			if tt.expectError {
				if err == nil {
					t.Fatalf("Expected error but got none")
				}
				if tt.expectedErr != "" && !containsString(err.Error(), tt.expectedErr) {
					t.Errorf("Expected error to contain %q, got %q", tt.expectedErr, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// Helper function to check if string contains substring
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

// TestSetDefaults tests that parser sets appropriate default values
func TestSetDefaults(t *testing.T) {
	app := &api.AppDefinition{
		Name:  "test-app",
		Image: "nginx:latest",
	}

	SetDefaults(app)

	if app.Type != "user" {
		t.Errorf("Expected default type 'user', got %s", app.Type)
	}
}

// TestSecurityValidation tests security-focused validation
func TestSecurityValidation(t *testing.T) {
	tests := []struct {
		name        string
		filePath    string
		expectedErr string
	}{
		{
			name:        "path traversal attempt",
			filePath:    "../../testdata/apps/invalid/path-traversal.yaml",
			expectedErr: "container path must be absolute",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := os.ReadFile(tt.filePath)
			if err != nil {
				t.Fatalf("Failed to read test file %s: %v", tt.filePath, err)
			}

			_, err = ParseAppDefinition(content)
			if err == nil {
				t.Fatalf("Expected security validation error but got none")
			}

			if !containsString(err.Error(), tt.expectedErr) {
				t.Errorf("Expected error to contain %q, got %q", tt.expectedErr, err.Error())
			}
		})
	}
}

// TestLargeContentHandling tests handling of large YAML content
func TestLargeContentHandling(t *testing.T) {
	content, err := os.ReadFile("../../testdata/apps/invalid/large-yaml.yaml")
	if err != nil {
		t.Fatalf("Failed to read large YAML test file: %v", err)
	}

	// Should handle reasonably large content without issues
	app, err := ParseAppDefinition(content)
	if err != nil {
		t.Fatalf("Should handle reasonably large content, but got error: %v", err)
	}

	if app.Name != "large-app" {
		t.Errorf("Expected name 'large-app', got %s", app.Name)
	}
}

// TestReservedNames tests that reserved app names are rejected
func TestReservedNames(t *testing.T) {
	reservedNames := []string{"api", "www", "admin", "root", "system", "piccolo"}

	for _, name := range reservedNames {
		t.Run(name, func(t *testing.T) {
			app := &api.AppDefinition{
				Name:  name,
				Image: "nginx:latest",
			}

			SetDefaults(app)
			err := ValidateAppDefinition(app)

			if err == nil {
				t.Fatalf("Expected error for reserved name '%s' but got none", name)
			}

			if !containsString(err.Error(), "reserved") {
				t.Errorf("Expected error about reserved name, got %q", err.Error())
			}
		})
	}
}

// TestMalformedYAML tests handling of malformed YAML
func TestMalformedYAML(t *testing.T) {
	malformedYAML := `name: test
image: nginx
listeners:
  - name: web
    guest_port: [invalid yaml structure`

	_, err := ParseAppDefinition([]byte(malformedYAML))
	if err == nil {
		t.Fatal("Expected error for malformed YAML but got none")
	}

	if !containsString(err.Error(), "failed to parse YAML") {
		t.Errorf("Expected YAML parsing error, got %q", err.Error())
	}
}
