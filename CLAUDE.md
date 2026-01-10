# CLAUDE.md - IRGSH-GO Project Guide

This document provides essential context for AI assistants working on the IRGSH-GO codebase.

## Project Overview

IRGSH-GO is a distributed Debian package building and repository management system written in Go. It automates the process of building, signing, and distributing Debian packages for the BlankOn Linux distribution.

## Architecture

The system follows a microservices architecture with Redis as the central message broker:

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  irgsh-cli  │────▶│ irgsh-chief │────▶│   Redis     │
└─────────────┘     └─────────────┘     └──────┬──────┘
                           │                   │
                           ▼                   ▼
                    ┌─────────────┐     ┌─────────────┐
                    │irgsh-builder│     │  irgsh-repo │
                    └─────────────┘     └─────────────┘
```

### Components

| Component | Port | Purpose |
|-----------|------|---------|
| **irgsh-chief** | 8080 | Central coordinator, API server, job scheduler |
| **irgsh-builder** | 8081 | Package build worker using pbuilder/Docker |
| **irgsh-repo** | 8082 | Repository manager using reprepro |
| **irgsh-iso** | 8083 | ISO image builder (minimal implementation) |
| **irgsh-cli** | N/A | Client tool for package maintainers |

## Directory Structure

```
/cmd/
├── chief/      # Central server (main.go, handler.go)
├── builder/    # Build worker (main.go, builder.go, init.go)
├── repo/       # Repository manager (main.go, repo.go)
├── iso/        # ISO builder (main.go, iso.go)
└── cli/        # Client CLI (main.go, cache.go, fs.go)

/internal/
├── config/       # Configuration management and validation
├── monitoring/   # Worker health tracking, metrics, job history
├── notification/ # Webhook notifications for job events
└── artifact/     # Artifact storage (repo/service/endpoint pattern)

/pkg/
├── httputil/   # JSON response helpers
└── systemutil/ # Command execution and log streaming

/utils/
├── config.yaml           # Main configuration template
├── scripts/              # Init and deployment scripts
├── systemctl/            # Systemd service files
├── reprepro-template/    # Repository configuration templates
└── docker/               # Dockerfile for pbuilder
```

## Build Commands

```bash
# Build all binaries
make build

# Build and run in development mode
make chief    # Runs with DEV=1
make builder
make repo

# Run tests with coverage
make test

# Build Debian package
make deb

# Initialize components
make builder-init
make repo-init
```

## Configuration

Configuration file: `/etc/irgsh/config.yaml` (or `./utils/config.yaml` for development)

Key sections:
- `redis`: Connection string for Redis broker
- `monitoring`: Worker heartbeat and cleanup settings
- `notification`: Webhook URL for job notifications
- `chief/builder/repo/iso`: Component-specific settings

**Special: irgsh-repo requires explicit config path:**
```bash
irgsh-repo -c /path/to/config.yaml
```

## Key Patterns

### Task Queue (Machinery)
Jobs are distributed via Redis using the machinery library:
- Tasks: `build`, `repo`
- Queue: `irgsh`
- Workers register handlers and process jobs asynchronously

### Monitoring
- Workers send heartbeats every 30 seconds
- Instances marked offline after 90 seconds without heartbeat
- Job history retained for 7 days
- Redis keys: `irgsh:instances:*`, `irgsh:jobs:*`

### Notifications
When `notification.webhook_url` is configured, POST requests are sent on job completion:
```json
{"title": "IRGSH Build Job SUCCESS", "message": "Job ID: xxx\nStatus: SUCCESS\n..."}
```

### Pipeline Flow
1. CLI validates and submits package (GPG signed)
2. Chief queues build task to Redis
3. Builder downloads, builds with pbuilder, uploads artifacts
4. Chief queues repo task
5. Repo downloads artifacts, injects into reprepro repository

## Testing

```bash
# Run all tests
make test

# Generate coverage report
make coverage

# Test files location
cmd/builder/builder_test.go
cmd/builder/init_test.go
cmd/repo/repo_test.go
pkg/httputil/response_test.go
```

## Common Development Tasks

### Adding a New Config Field
1. Add struct field to appropriate config type in `internal/config/config.go`
2. Add to `IrgshConfig` struct if new section
3. Update `utils/config.yaml` with example
4. Access via `irgshConfig.Section.Field`

### Adding a New API Endpoint (Chief)
1. Add handler function in `cmd/chief/handler.go`
2. Register route in `serve()` function in `cmd/chief/main.go`
3. Use `httputil.ResponseJSON()` for responses

### Adding Worker Functionality
1. Implement function in component's main package (e.g., `cmd/builder/builder.go`)
2. Register with machinery if it's a distributed task
3. Add notification call for job completion if needed

## Dependencies

Key libraries:
- `github.com/RichardKnop/machinery/v1` - Distributed task queue
- `github.com/go-redis/redis/v8` - Redis client
- `github.com/urfave/cli` - CLI framework
- `github.com/ghodss/yaml` - YAML parsing
- `gopkg.in/go-playground/validator.v9` - Struct validation
- `gopkg.in/src-d/go-git.v4` - Git operations

## Version Management

- Version stored in `/VERSION` file
- Injected at build time via `LDFLAGS`
- Debian changelog in `/debian/changelog`
- Bump both files when releasing

## Important Notes

1. **DEV mode**: Set `DEV=1` to redirect workdirs from `/var/lib/` to `./tmp/`
2. **Config validation**: All required fields must be present or startup fails
3. **GPG keys**: Chief and Repo require GPG keys for signing
4. **Redis required**: All components depend on Redis being available
5. **irgsh-repo isolation**: Each instance needs its own config for multi-arch support
