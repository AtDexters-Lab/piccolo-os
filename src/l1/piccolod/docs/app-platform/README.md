# Piccolo OS App Platform

The Piccolo OS App Platform enables users to easily install, manage, and run containerized applications with a mobile OS-like experience.

## Quick Start

### Minimal App
```yaml
name: blog
image: ghost:latest
```

### Custom Build  
```yaml
name: my-app
build:
  containerfile: |
    FROM node:18
    COPY . /app
    CMD ["npm", "start"]
```

## Documentation

- **[specification.yaml](./specification.yaml)** - Complete app.yaml specification with all fields documented
- **[examples/](./examples/)** - Common patterns and use cases

## Key Features

### 🏗️ **Flexible Container Sources**
- **Registry images**: Docker Hub, GitHub Container Registry, private registries
- **Custom builds**: Inline Containerfile or external build context  
- **Git builds**: Build directly from Git repositories

### 🔒 **Security by Default**
- **Network isolation**: No internet access by default
- **Storage sandboxing**: Each app gets isolated storage
- **Permission model**: Granular control over resources and capabilities
- **Federated storage**: User data synced across devices

### ⚡ **Developer Experience**
- **Progressive complexity**: Start simple, add features as needed
- **Smart defaults**: Minimal configuration for common cases
- **Extensibility**: Apps can read their own config for custom behavior
- **Hot builds**: Fast iteration with build caching

## Architecture

```
Container Sources:
├── Registry Images (ghost:latest, nginx:alpine)
├── Custom Builds (Containerfile + context)
└── Git Builds (clone → build → run)

Storage Architecture:
├── Persistent (/var/piccolo/storage/<app>/<volume>) → Federated Storage
├── Temporary (/tmp/piccolo/apps/<app>/<volume>) → Local /tmp
└── Filesystem (/var/piccolo/apps/<app>/filesystem) → Local overlay (optional)

Runtime:
├── Podman containers (rootless, daemonless)
├── systemd integration (proper lifecycle)
└── mDNS discovery (app.piccolo.local)
```

## Examples by Use Case

| Use Case | Example | Highlights |
|----------|---------|------------|
| **Custom code** | [custom-build.yaml](./examples/custom-build.yaml) | Inline Containerfile + build args |
| **Developer workstation** | [development.yaml](./examples/development.yaml) | Multiple listeners, persistent volumes |
| **Web service** | [web-service.yaml](./examples/web-service.yaml) | Health checks and remote publish |

## API Integration

Apps are deployed via HTTP API with flexible upload methods:

```bash
# Method 1: Inline YAML
curl -X POST /api/v1/apps \
  -H "Content-Type: application/yaml" \
  --data-binary @app.yaml

# Method 2: Multi-part upload  
curl -X POST /api/v1/apps \
  -F "app_definition=@app.yaml" \
  -F "containerfile=@Containerfile" \
  -F "context=@build-context.tar.gz"

# Method 3: Git deployment
curl -X POST /api/v1/apps \
  -H "Content-Type: application/json" \
  -d '{"git_url": "https://github.com/user/app.git", "path": "piccolo-app.yaml"}'
```

## Development

The app platform is implemented in the `piccolod` daemon using:
- **Podman** for OCI-compliant container runtime
- **systemd** for service lifecycle management  
- **SQLite** for app metadata persistence
- **Federated storage** for cross-device data sync

See [../../pre-beta-prd.md](../../pre-beta-prd.md) for current scope and architecture notes.
