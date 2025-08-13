# mDNS Testing TODO

## Overview
Comprehensive unit testing strategy for the refactored mDNS implementation with 7 semantic modules.

## Test Strategy

### 1. Unit Tests by Module

#### `types.go` (151 lines)
- [ ] Test struct initialization and validation
- [ ] Test data structure relationships
- [ ] Test constants and configuration defaults

#### `manager.go` (212 lines) 
- [ ] Test `NewManager()` initialization with all components
- [ ] Test `Start()` sequence and error handling
- [ ] Test `Stop()` cleanup and graceful shutdown
- [ ] Test machine ID generation logic
- [ ] Test deterministic machine ID from various sources

#### `security.go` (193 lines)
- [ ] Test rate limiting per client IP
- [ ] Test client blocking and unblocking logic
- [ ] Test packet size validation
- [ ] Test DNS message validation (query bombs, malformed packets)
- [ ] Test concurrent query slot management
- [ ] Test security cleanup and metrics

#### `resilience.go` (228 lines)
- [ ] Test interface health score calculations
- [ ] Test exponential backoff duration calculations
- [ ] Test interface failure marking and recovery
- [ ] Test health monitoring and recovery mode
- [ ] Test system health aggregation
- [ ] Test interface resurrection logic

#### `conflict.go` (194 lines)
- [ ] Test conflict detection probing
- [ ] Test conflict response handling
- [ ] Test deterministic name resolution
- [ ] Test periodic conflict monitoring
- [ ] Test startup conflict detection

#### `interface.go` (398 lines)
- [ ] Test interface discovery and filtering
- [ ] Test dual-stack IPv4/IPv6 socket creation
- [ ] Test interface change detection
- [ ] Test IP address change detection
- [ ] Test socket binding and multicast joining
- [ ] Test interface cleanup on removal

#### `protocol.go` (349 lines)
- [ ] Test dual-stack query responders (IPv4/IPv6)
- [ ] Test DNS query parsing and validation
- [ ] Test A and AAAA record generation
- [ ] Test response size limits and security
- [ ] Test announcement scheduling and sending
- [ ] Test multicast announcement formatting

### 2. Integration Tests

#### Network Layer Integration
- [ ] Test full dual-stack operation (IPv4 + IPv6)
- [ ] Test multi-interface operation
- [ ] Test interface hot-plug scenarios
- [ ] Test network partition recovery

#### Security Integration
- [ ] Test rate limiting under load
- [ ] Test DoS attack scenarios
- [ ] Test malformed packet handling
- [ ] Test concurrent query limits

#### Conflict Resolution Integration
- [ ] Test conflict detection with real responses
- [ ] Test deterministic naming with multiple conflicting hosts
- [ ] Test name change announcements

### 3. Performance Tests

#### Load Testing
- [ ] Test high query volume handling
- [ ] Test memory usage under load
- [ ] Test concurrent client handling
- [ ] Test interface scaling (many interfaces)

#### Stress Testing
- [ ] Test interface failure/recovery cycles
- [ ] Test network instability scenarios
- [ ] Test resource exhaustion handling

### 4. Mock Strategy

#### External Dependencies
- [ ] Mock `net.Interfaces()` for interface discovery
- [ ] Mock UDP connections for network I/O
- [ ] Mock system files for machine ID generation
- [ ] Mock time functions for deterministic testing

#### Test Utilities
- [ ] Create mock network interfaces
- [ ] Create mock DNS packets and responses
- [ ] Create test harness for multi-interface scenarios
- [ ] Create performance benchmarking utilities

### 5. Test Organization

```
internal/mdns/
├── manager_test.go           # Manager core tests
├── security_test.go          # Security module tests  
├── resilience_test.go        # Resilience module tests
├── conflict_test.go          # Conflict detection tests
├── interface_test.go         # Interface management tests
├── protocol_test.go          # DNS protocol tests
├── types_test.go             # Types and data structure tests
├── integration_test.go       # Cross-module integration tests
├── testutils/               # Test utilities and mocks
│   ├── mock_interfaces.go   # Mock network interfaces
│   ├── mock_dns.go          # Mock DNS packets
│   └── test_harness.go      # Common test setup
└── benchmarks/              # Performance benchmarks
    ├── load_test.go         # Load testing
    └── stress_test.go       # Stress testing
```

### 6. Coverage Goals

- [ ] **Unit Test Coverage**: >90% line coverage per module
- [ ] **Integration Coverage**: All major interaction paths
- [ ] **Error Path Coverage**: All error conditions and recovery paths
- [ ] **Concurrency Coverage**: All race conditions and thread safety

### 7. Test Scenarios

#### Real-world Scenarios
- [ ] Android device resolution testing
- [ ] Windows VM resolution testing  
- [ ] Multiple Piccolo instances on same network
- [ ] Interface up/down events
- [ ] DHCP IP address changes
- [ ] Network roaming scenarios

#### Edge Cases
- [ ] No network interfaces available
- [ ] All interfaces fail simultaneously
- [ ] Conflicting hostnames with identical machine IDs
- [ ] DNS response packet corruption
- [ ] Extremely high query rates

### 8. Continuous Integration

#### Test Automation
- [ ] Set up automated test runs on commit
- [ ] Set up performance regression detection
- [ ] Set up coverage reporting
- [ ] Set up race condition detection (`go test -race`)

## Implementation Priority

1. **Phase 1**: Core unit tests (types, manager, security)
2. **Phase 2**: Complex module tests (resilience, conflict, interface) 
3. **Phase 3**: Protocol and integration tests
4. **Phase 4**: Performance and stress testing
5. **Phase 5**: Real-world scenario testing

## Notes

- Use table-driven tests for multiple input scenarios
- Leverage Go's built-in race detector
- Create comprehensive test documentation
- Use mocks judiciously to avoid testing implementation details
- Focus on testing behavior and contracts, not internal implementation