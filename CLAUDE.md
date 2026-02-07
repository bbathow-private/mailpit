# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Mailpit is a single-binary email testing tool for developers. It provides an SMTP server (default port 1025), a web UI with REST API (default port 8025), and an optional POP3 server (default port 1110). Written in Go with a Vue.js frontend, it has zero runtime dependencies.

## Build & Development Commands

### Go Backend
```bash
# Build binary (static, no CGO)
CGO_ENABLED=0 go build -ldflags "-s -w" -o mailpit

# Run all tests (must use -p 1 for serial execution)
go test -p 1 ./internal/storage ./server ./internal/smtpd ./internal/pop3 ./internal/tools ./internal/html2text ./internal/htmlcheck ./internal/linkcheck -v

# Run tests for a single package
go test ./internal/storage -v

# Run a single test function
go test ./internal/storage -run TestFunctionName -v

# Run benchmarks
go test -p 1 ./internal/storage ./internal/html2text -bench=.

# Check Go formatting (CI enforces this)
gofmt -s -w . && git diff --exit-code
```

### Frontend (Vue.js / esbuild)
```bash
npm install              # Install dependencies
npm run build            # Build minified assets
npm run watch            # Watch mode for development
npm run package          # Build minified assets (same as build)
npm run lint             # ESLint + Prettier check (zero warnings required)
npm run lint-fix         # Auto-fix linting issues
```

Frontend source is in `server/ui-src/`, compiled output goes to `server/ui/dist/`.

### Full Build (backend + frontend)
```bash
npm install && npm run package
CGO_ENABLED=0 go build -ldflags "-w -X github.com/axllent/mailpit/config.Version=dev" -o mailpit
```

### Docker
```bash
# Build image
docker build -t mailpit:dev .

# Run container (web UI on :8025, SMTP on :1025)
docker run -d --name mailpit -p 8025:8025 -p 1025:1025 mailpit:dev
```

### Testing with rqlite
Set `MP_DATABASE=http://localhost:4001` to run tests against rqlite instead of SQLite.

## Architecture

**Entry point**: `main.go` — routes to `cmd.Execute()` (mailpit server) or `sendmail.Run()` if the binary name contains "send".

**CLI layer** (`cmd/`): Uses Cobra. `root.go` starts the server: loads config → initializes DB → starts HTTP server → starts SMTP server.

**Core packages** (`internal/`):
- `storage/` — Database abstraction (SQLite or rqlite). Message CRUD, full-text search, tag management. Schema migrations in `storage/schemas/`.
- `smtpd/` — SMTP server implementation with relay/forwarding support and chaos testing features.
- `pop3/` — Optional POP3 server.
- `auth/` — htpasswd-based authentication.
- `html2text/`, `htmlcheck/`, `linkcheck/` — Email content processing utilities.
- `tools/` — Header parsing, snippet generation, and other utilities.

**HTTP layer** (`server/`):
- `server.go` — HTTP server setup, routing (gorilla/mux), embedded UI assets.
- `apiv1/` — REST API v1 endpoints with OpenAPI/Swagger docs.
- `websockets/` — Real-time updates via WebSocket.
- `handlers/` — HTTP route handlers including Kubernetes health probes.

**Frontend** (`server/ui-src/`):
- Vue 3 + Vue Router + Bootstrap 5, bundled with esbuild.
- Components in `components/`, views in `views/`, state in `stores/`.
- `MessageView.vue` is the largest component (~20KB) handling message display.

**Configuration** (`config/`): Global config struct populated from CLI flags and environment variables.

## Key Design Decisions

- **Static binary**: Always build with `CGO_ENABLED=0`. The SQLite driver is a pure Go implementation (`modernc.org/sqlite`).
- **Dual database support**: SQLite (default, single-file) and rqlite (distributed). Tests run against both in CI.
- **Embedded assets**: Frontend is compiled and embedded into the Go binary via `server/embed.go`.
- **Multi-tenant**: Storage supports tenant isolation via TenantID prefix.
- **Email parsing**: Uses `jhillyerd/enmime/v2` for MIME parsing.

## Code Style

- Go: `gofmt -s` formatting enforced in CI.
- JavaScript: ESLint + Prettier with zero warnings. Prettier config: tabs, width 4, print width 120.
- Contributions should be clean and well-commented. AI-assisted code is acceptable but "vibe coded" PRs are not.

## Branch Strategy

- Default branch: `develop`
- Feature branches: `feature/**`
- Releases: semantic version tags (v1.x.y)
