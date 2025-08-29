# Piccolo OS App Platform - Development TODO

## 🎯 **Current Goal: End-to-End REST API → Container Management**

**Vision:** Complete the plumbing from HTTP API endpoints through filesystem state management to container operations, enabling full app lifecycle management via REST API.

---

## **Phase 1: Filesystem State Management** ⏳
**Goal:** Replace in-memory app storage with filesystem + cache approach

### 1.1 Design Filesystem Structure ⭕
```bash
/var/lib/piccolod/
├── apps/                 # App definitions (source of truth)
│   ├── web-app/
│   │   ├── app.yaml     # Original app definition
│   │   └── metadata.json # Runtime metadata (status, container_id, etc.)
│   └── db-app/app.yaml
├── enabled/             # Systemctl-style enable/disable
│   ├── web-app -> ../apps/web-app
│   └── db-app -> ../apps/db-app  
└── cache/               # Optional performance cache
    └── port-mappings.json
```

### 1.2 Implement FilesystemStateManager ⭕
- [ ] Create `FilesystemStateManager` struct with filesystem operations
- [ ] Replace `map[string]*AppInstance` with filesystem-backed storage
- [ ] Add in-memory cache layer for performance (port lookups, app listings)
- [ ] Implement enable/disable functionality with symlinks
- [ ] Add file watching for external configuration changes
- [ ] Ensure thread safety with proper file locking mechanisms

### 1.3 Update App Manager Integration ⭕
- [ ] Modify existing `manager.go` to use `FilesystemStateManager` 
- [ ] Update all CRUD operations to persist to filesystem
- [ ] Update existing unit tests to work with filesystem backend
- [ ] Update integration tests to verify filesystem persistence
- [ ] Ensure existing interface compatibility (no breaking changes)

---

## **Phase 2: HTTP API Endpoints** ⭕
**Goal:** Expose app management via REST API (per PRD specification)

### 2.1 Define API Routes ⭕
```go
// Core app management endpoints
POST   /api/v1/apps           # Install app from app.yaml upload
GET    /api/v1/apps           # List all apps with status
GET    /api/v1/apps/{name}    # Get specific app details
DELETE /api/v1/apps/{name}    # Uninstall app completely

// App lifecycle control endpoints  
POST   /api/v1/apps/{name}/start   # Start app container
POST   /api/v1/apps/{name}/stop    # Stop app container
POST   /api/v1/apps/{name}/enable  # Enable app (start on boot)
POST   /api/v1/apps/{name}/disable # Disable app (manual start only)
```

### 2.2 Implement HTTP Handlers ⭕
- [ ] JSON request/response serialization with proper error handling
- [ ] Input validation for app names, YAML content, etc.
- [ ] HTTP status codes (201, 404, 409, 500) with meaningful responses
- [ ] Content-Type handling for app.yaml file uploads
- [ ] Request size limiting and timeout handling

### 2.3 Integration with Existing Server ⭕
- [ ] Add API routes to existing `internal/server/server.go`
- [ ] Wire up handlers with App Manager dependency injection
- [ ] Add structured logging for all API operations
- [ ] Integrate with existing health check system
- [ ] Add middleware foundation for future auth/rate limiting

---

## **Phase 3: End-to-End Integration Testing** ⭕
**Goal:** Verify complete REST API → Container workflow

### 3.1 API Integration Tests ⭕
```go
// Complete workflow validation
func TestAPI_FullAppLifecycle(t *testing.T) {
    // POST /api/v1/apps (install nginx from app.yaml)
    // POST /api/v1/apps/nginx/start (start container)  
    // GET /api/v1/apps (verify running status)
    // GET /api/v1/apps/nginx (check specific app details)
    // POST /api/v1/apps/nginx/stop (stop container)
    // DELETE /api/v1/apps/nginx (uninstall completely)
}
```

### 3.2 Real HTTP Server Testing ⭕
- [ ] Use `net/http/httptest` for isolated unit tests
- [ ] Add integration tests with real HTTP server instances
- [ ] Test with actual app.yaml files and real container creation
- [ ] Verify filesystem state persistence across service restarts
- [ ] Test concurrent API operations and race condition handling

---

## **Phase 4: Production Readiness** ⭕
**Goal:** Polish implementation for production deployment

### 4.1 Error Handling & Validation ⭕
- [ ] Comprehensive input validation with clear error messages
- [ ] Proper HTTP error response format with error codes
- [ ] Request rate limiting and size limits
- [ ] Graceful handling of container runtime failures
- [ ] Atomic operations with proper rollback on failures

### 4.2 Observability & Monitoring ⭕
- [ ] Structured request logging with correlation IDs
- [ ] Metrics collection for API endpoint performance
- [ ] Integration with existing health check endpoints
- [ ] Error rate monitoring and alerting capabilities

---

## **🚀 Immediate Next Steps**

### **Step 1: FilesystemStateManager** (Priority: High)
Start by implementing filesystem-based state management while keeping existing App Manager interface unchanged.

### **Step 2: Basic HTTP API** (Priority: High)  
Implement core CRUD endpoints with JSON request/response handling.

### **Step 3: Integration Testing** (Priority: Medium)
Add comprehensive end-to-end tests covering HTTP → Filesystem → Container flow.

---

## **Success Criteria** ✅

When this phase is complete, the following should work perfectly:

```bash
# Install app via HTTP API
curl -X POST http://localhost:8080/api/v1/apps \
  -H "Content-Type: application/x-yaml" \
  --data-binary @nginx-app.yaml

# List running apps
curl http://localhost:8080/api/v1/apps

# Start specific app
curl -X POST http://localhost:8080/api/v1/apps/nginx/start

# Stop and uninstall
curl -X POST http://localhost:8080/api/v1/apps/nginx/stop
curl -X DELETE http://localhost:8080/api/v1/apps/nginx
```

**Expected outcome:** Complete app lifecycle management via REST API with filesystem persistence and real container operations.

---

## **🔄 Development Guidelines**

- **Maintain backward compatibility** - existing tests should continue passing
- **Follow TDD methodology** - write tests first, especially for HTTP layer
- **Use integration tests** - verify real containers and filesystem operations
- **Keep interfaces clean** - filesystem implementation hidden behind abstractions
- **Error handling first** - robust error handling and recovery mechanisms

---

*Last updated: [Today's Date]*  
*Status: Ready to begin Phase 1 - FilesystemStateManager implementation*