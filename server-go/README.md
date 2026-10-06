# server-go

Go rewrite of the Audiobookshelf backend (COMPX574 project). This package lives
alongside the existing Node.js backend (`server/`) and is not a replacement yet —
see "Status" below.

## Why Go, why gin

The team decided to rewrite the backend from Node.js/Express to Go, keeping the
existing frontend (Nuxt/Vue) and the existing SQLite database/schema unchanged.
Details and rationale are in `report-4/重写架构与实施方案.docx`.

For the HTTP framework we picked [gin](https://github.com/gin-gonic/gin): it's
lightweight, widely used, and the team already has experience with it.

## Status

Only the routing skeleton exists right now. No database connection, no real
endpoints yet, no contract-testing tooling against the Node.js service. This is
infrastructure work (milestone "Phase 1" in the architecture doc), not a
finished slice.

## Layout

```
server-go/
  cmd/server/main.go        entry point: loads config, builds the router, starts the server
  internal/config/          config loading (currently just the port)
  internal/router/          gin route registration
  internal/handlers/        request handlers (currently just /health)
```

`internal/services/`, `internal/repository/`, and `internal/middleware/` are not
created yet. They'll be added when the first real slice (e.g. login) is
implemented, following the target architecture's layering: Handlers → Services →
Repository → SQLite.

## Running it

```
cd server-go
go run ./cmd/server
```

Defaults to port 4000 (override with `GO_SERVER_PORT`). Verify it's up:

```
curl http://localhost:4000/health
# {"status":"ok"}
```

## Building

```
go build -o bin/server ./cmd/server
```
