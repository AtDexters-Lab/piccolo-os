# App Manager Testing

This document describes how to run tests for the App Manager component.

## Test Types

### Unit Tests (Default)
- **Fast** (~0.002s total)
- **No dependencies** - uses mocks
- **Always run** in CI/CD

```bash
# Run unit tests only
go test ./internal/app -short

# Run with verbose output
go test ./internal/app -v -short
```

### Integration Tests
- **Slower** (~30-60s total)
- **Requires Podman** installed and running
- **Tests real containers** and network connectivity

```bash
# Run integration tests (requires Podman)
go test ./internal/app -tags=integration

# Run with verbose output
go test ./internal/app -tags=integration -v

# Run specific integration test
go test ./internal/app -tags=integration -run TestAppManager_FullLifecycle
```

### All Tests
```bash
# Run both unit and integration tests
go test ./internal/app

# Run all tests with verbose output
go test ./internal/app -v
```

## Integration Test Requirements

### Prerequisites
1. **Podman installed** and accessible in PATH
2. **Network access** to pull container images
3. **Available ports** 8081-8090 for testing
4. **Sufficient disk space** for container images

### What Integration Tests Verify

**Real Container Operations:**
- Container creation from app.yaml definitions
- Actual port mapping and HTTP connectivity
- Environment variable injection
- Container lifecycle (start/stop/remove)
- Error handling with real Podman failures

**End-to-End Scenarios:**
- Full app lifecycle: Install → Start → Stop → Uninstall
- YAML parsing → Container creation → Network testing
- Port conflict detection
- Invalid image handling

### Test Data Files

Integration tests use YAML files in `testdata/integration/`:
- `simple-nginx.yaml` - Basic nginx app with port mapping
- `alpine-with-env.yaml` - Alpine container with environment variables

## Continuous Integration

```yaml
# Example CI configuration
test-unit:
  run: go test ./internal/app -short -v

test-integration:
  needs: [test-unit]
  services: [podman]
  run: go test ./internal/app -tags=integration -v
```

## Troubleshooting

**Integration tests skipped:**
- Ensure Podman is installed: `podman --version`
- Check Podman service: `systemctl --user status podman.socket`

**Port conflicts:**
- Tests use ports 8081-8090
- Stop any services using these ports
- Or modify port numbers in test files

**Container pull failures:**
- Ensure network connectivity
- Pre-pull images: `podman pull nginx:alpine alpine:latest`

**Permission errors:**
- Ensure user can run podman without sudo
- Configure rootless Podman properly