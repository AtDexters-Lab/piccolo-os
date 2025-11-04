# Testing Piccolo OS

Comprehensive testing ensures system reliability and security across all components.

## Testing Strategy

### Test Levels
1. **Unit Tests** - Individual component functionality
2. **Integration Tests** - Component interaction testing  
3. **System Tests** - End-to-end OS functionality
4. **Security Tests** - Vulnerability and hardening validation

### Automation Philosophy
- All tests should be automatable and repeatable
- CI/CD pipeline integration for continuous validation
- VM-based testing for complete system validation

## Layer 1 (piccolod) Testing

### Unit Tests

```bash
cd src/l1/piccolod

# Run all unit tests
go test ./...

# Run specific package tests
go test ./internal/server
go test ./internal/container

# Run tests with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

### Integration Tests

```bash
# Test with Docker dependency
docker run --rm hello-world  # Verify Docker works
go test ./internal/container -integration

# Test HTTP API endpoints
go test ./internal/server -integration
```

### Component-Specific Testing

```bash
# Container manager tests
go test ./internal/container -v

# Storage manager tests  
go test ./internal/storage -v

# Trust agent tests (requires TPM simulator)
go test ./internal/trust -v
```

# Web UI (Layer 1 / piccolod web-src)

The Svelte/Tailwind portal ships with a Playwright suite that exercises primary flows on desktop and mobile breakpoints.

```bash
cd src/l1/piccolod/web-src

# Build the UI bundle (vite)
npm run build

# Run the entire Playwright test suite (headless)
npx playwright test

# Run only the mobile layout smoke tests (includes logout reachability)
npx playwright test tests/mobile.spec.ts --project mobile-chromium

# Record new screenshots for visual regression tests when intentional UI changes land
npx playwright test tests/visual.spec.ts --update-snapshots
```

Key scenarios include:
- `mobile.spec.ts` – verifies touch targets, mobile navigation, and ensures a logout control is available on phones.
- `navigation.spec.ts` – checks primary desktop navigation and quick settings access.
- `remote_*` specs – cover remote publish configuration, certificate inventory, and connectivity flows.

If you add new UI capabilities, extend the relevant spec or introduce a new one under `src/l1/piccolod/web-src/tests/` so the behaviour is enforced in CI.

## System-Level Testing

### Automated VM Testing

The primary system test runs the complete OS in a VM:

```bash
cd src/l0

# Build and test complete system
./build.sh
./test_piccolo_os_image.sh --build-dir ./build/output/1.0.0 --version 1.0.0
```

### Test Coverage

The automated test script validates:

1. **Binary Presence** - `piccolod` binary exists and is executable
2. **Service Status** - `piccolod.service` is active and running  
3. **Process Security** - `piccolod` runs as root user (hardening verification)
4. **HTTP API** - Version endpoint responds correctly
5. **Container Runtime** - Docker integration functional

### Test Output Example

```
[2024-08-06 07:45:00] ### Step 1: Preparing the test environment...
[2024-08-06 07:45:01] ### Step 2: Booting Piccolo Live ISO in QEMU...
[2024-08-06 07:45:30] ### Step 3: Waiting for SSH to become available...
--- CHECK 1: piccolod binary ---
PASS: piccolod binary is present and executable.
--- CHECK 2: piccolod service status ---  
PASS: piccolod service is active.
--- CHECK 3: piccolod process runs as root ---
PASS: piccolod process is running as root user.
--- CHECK 4: piccolod version via HTTP ---
PASS: piccolod version is correct.
--- CHECK 5: Container runtime ---
PASS: Container runtime is functional.
[2024-08-06 07:46:00] ✅ ✅ ✅ ALL CHECKS PASSED ✅ ✅ ✅
```

## Manual Testing Procedures

### Interactive VM Testing

For exploratory testing and debugging:

```bash
# Start VM with console access
qemu-system-x86_64 \
    -name "Piccolo-Manual-Test" \
    -m 2048 \
    -machine q35,accel=kvm \
    -cpu host \
    -netdev user,id=eth0,hostfwd=tcp::2222-:22,hostfwd=tcp::8080-:8080 \
    -device virtio-net-pci,netdev=eth0 \
    -drive file=build/output/1.0.0/piccolo-os-live-1.0.0.iso,media=cdrom,format=raw \
    -boot order=d \
    -nographic
```

### SSH Access Testing

```bash
# Generate test SSH key
ssh-keygen -t rsa -b 4096 -f test_key -N ""

# Access via SSH  
ssh -p 2222 -i test_key core@localhost

# Verify system state
sudo systemctl status piccolod.service
sudo journalctl -u piccolod.service -f
ps aux | grep piccolod
```

### API Testing

```bash
# Test version endpoint
curl -v http://localhost:8080/api/v1/version

# Test container endpoints (when implemented)
curl -v http://localhost:8080/api/v1/containers

# Test health endpoint  
curl -v http://localhost:8080/api/v1/health
```

## Security Testing

### Systemd Security Validation

Verify hardening directives are properly applied:

```bash
# Check service properties
systemctl show piccolod.service | grep -E "(NoNewPrivileges|CapabilityBoundingSet|DeviceAllow)"

# Verify process capabilities  
sudo grep -E "(CapInh|CapPrm|CapEff|CapBnd)" /proc/$(pgrep piccolod)/status

# Check device access
sudo ls -la /dev/tpm0 2>/dev/null || echo "TPM not available"
sudo ls -la /dev/sd* 2>/dev/null || echo "No SATA devices"
```

### TPM Testing

When TPM hardware is available:

```bash
# Check TPM accessibility  
sudo ls -la /dev/tpm0

# Test TPM operations (if implemented)
curl -X POST http://localhost:8080/api/v1/trust/attest

# Verify TPM-based encryption readiness
sudo cryptsetup luksDump /dev/sda1 2>/dev/null || echo "No LUKS volumes"
```

## Performance Testing

### Resource Usage Validation

```bash
# Monitor resource usage
top -p $(pgrep piccolod)
free -h
df -h

# Memory usage over time
while true; do
    ps -o pid,rss,vsz,cmd -p $(pgrep piccolod)
    sleep 5
done
```

### API Performance

```bash
# Basic load testing
for i in {1..100}; do
    curl -s http://localhost:8080/api/v1/version >/dev/null &
done
wait

# Response time measurement
time curl http://localhost:8080/api/v1/version
```

## Continuous Integration

### GitHub Actions Integration

Example `.github/workflows/test.yml`:

```yaml
name: Test Piccolo OS
on: [push, pull_request]

jobs:
  test-l1:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v3
        with:
          go-version: '1.21'
      - name: Run L1 tests
        run: |
          cd src/l1/piccolod
          go test ./...

  test-system:
    runs-on: ubuntu-latest
    needs: test-l1
    steps:
      - uses: actions/checkout@v3  
      - name: Install dependencies
        run: |
          sudo apt-get update
          sudo apt-get install -y qemu-system-x86 openssh-client
      - name: Build and test system
        run: |
          cd src/l0
          ./build.sh
          ./test_piccolo_os_image.sh --build-dir ./build/output/1.0.0 --version 1.0.0
```

## Troubleshooting Test Issues

### Common Build Test Failures

```bash
# Docker daemon not running
sudo systemctl start docker

# KVM not available
sudo modprobe kvm-intel  # or kvm-amd
ls -la /dev/kvm

# Port conflicts
sudo netstat -tlnp | grep :2222
sudo fuser -k 2222/tcp
```

### VM Test Debugging

```bash
# Increase test timeout
SSH_TIMEOUT=300 ./test_piccolo_os_image.sh --build-dir ./build/output/1.0.0 --version 1.0.0

# Verbose SSH debugging
SSH_OPTS="-vvv" ./test_piccolo_os_image.sh --build-dir ./build/output/1.0.0 --version 1.0.0

# Keep VM running for debugging
KEEP_VM=1 ./test_piccolo_os_image.sh --build-dir ./build/output/1.0.0 --version 1.0.0
```

For more information, see:
- [Building Guide](building.md)
- [Architecture Overview](../architecture/overview.md)
- [Troubleshooting](../operations/troubleshooting.md)
