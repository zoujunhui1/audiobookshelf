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

The Go server provides `/health` and handles `POST /login` (also under
`ROUTER_BASE_PATH`). It forwards every unmatched method/path to the
Node.js backend. Before each forward it checks Node's `/healthcheck`; if Node is
not running, it starts it and waits for readiness. Concurrent requests share one
startup. An exited child is restarted on the next request. Go verifies local
credentials against the existing SQLite database, signs compatible JWTs, and
persists refresh sessions. Refresh/logout, OpenID and other business endpoints
still run in Node.

## Layout

```
server-go/
  cmd/server/main.go        entry point and shutdown handling
  internal/config/          Go/Node ports, shared database and auth configuration
  internal/router/          gin route registration
  internal/handlers/        health and login HTTP handlers
  internal/services/auth.go password login, token generation and session creation
  internal/db/manager.go    connection checks, reconnection and SQLite settings
  internal/db/read.go       credential lookup/verification and auth settings
  internal/db/write.go      Node-compatible refresh session persistence
  internal/proxy/forward.go reverse proxy for unmatched requests
  internal/proxy/node.go    Node readiness, startup and process cleanup
```

Login follows Router → Handler → Auth service → DB → SQLite. The `db` package
currently serves as the repository layer.

Go verifies bcrypt passwords (including existing passwordless root behavior),
checks active users and whether local auth is enabled, then creates HS256 access
and refresh tokens with the same claims used by Node. Signing uses
`JWT_SECRET_KEY`, falling back to the existing server-settings `tokenSecret`.
Go does not generate a separate secret.

To preserve the existing frontend response, the auth service sends its new access
token to Node's `POST /api/authorize` to obtain user permissions, progress,
bookmarks, server settings and other business data. **The submitted username and
password are never sent to Node.** This first login slice still requires Node
for response enrichment; it is not a standalone replacement for all auth-related
business data. Go writes the refresh session only after response enrichment
succeeds, so failed enrichment leaves no orphan session.

`db/manager.go` checks the existing file and pings the connection before each
operation, reopening closed connections or replaced files. It uses one Go
connection, WAL, a 5-second busy timeout and foreign keys. Node continues to own
schema initialization/migration. Go will not create a missing database. On a new
installation, first use Node's `/status` and `/init` setup through the proxy.

## Running it

Prepare Node dependencies and compile its entry point once from the repository
root (the proxy starts the same `dist-server/index.js` used by `npm start`):

```
npm ci
npm run build:server
```

Then start Go:

```
cd server-go
go run ./cmd/server
```

Defaults to port 4000 (override with `GO_SERVER_PORT`). Verify it's up:

```
curl http://localhost:4000/health
# {"status":"ok"}
```

The health route does not start Node. A request to a Node route does:

```
curl http://localhost:4000/status
```

| Environment variable | Default | Purpose |
| --- | --- | --- |
| `GO_SERVER_PORT` | `4000` | Public Go HTTP port |
| `NODE_BACKEND_PORT` | `3333` | Local Node HTTP port; must differ from Go |
| `NODE_BACKEND_DIR` | `..` | Repository root, relative to the working directory |
| `NODE_EXECUTABLE` | `node` | Node executable name or path |
| `NODE_STARTUP_TIMEOUT` | `30s` | Maximum readiness wait for each forwarding request |
| `CONFIG_PATH` | `config` under Node's working directory | Shared database directory |
| `JWT_SECRET_KEY` | Existing database secret | Shared signing secret |
| `ACCESS_TOKEN_EXPIRY` | `3600` | Access token lifetime in seconds |
| `REFRESH_TOKEN_EXPIRY` | `2592000` | Refresh token lifetime in seconds |
| `ROUTER_BASE_PATH` | `/audiobookshelf` | Additional login route prefix; empty disables prefix |
| `RATE_LIMIT_AUTH_MAX` | `40` | Login attempts per client/window; `0` disables limiting |
| `RATE_LIMIT_AUTH_WINDOW` | `600000` | Login rate-limit window in milliseconds |
| `RATE_LIMIT_AUTH_MESSAGE` | `Too many authentication requests` | Login limit response message |

When running the compiled Go binary from another directory, set
`NODE_BACKEND_DIR` to the absolute repository path. Node inherits the environment
(including `CONFIG_PATH`, `METADATA_PATH`, and `JWT_SECRET_KEY`); the manager sets
production mode and explicit `127.0.0.1` host/port arguments.
Its output goes to the Go server's stdout/stderr. Existing Node runtime
requirements, such as ffmpeg/ffprobe, still apply.

Forwarding preserves paths, queries, request bodies, authentication headers,
response statuses and cookies, and supports streaming and WebSocket upgrades.
Gin path redirects are disabled so unmatched paths reach Node unchanged. Only
`GET /health` and `POST /login` (including the configured prefix) are handled
locally; other methods at those paths are forwarded.

Login accepts JSON or URL-encoded username/password fields. Missing or malformed
credentials return `400`, invalid/inactive credentials return `401`, and rate
limits return `429`. Database or response-enrichment failures return `503`.
Browsers receive the HTTP-only `refresh_token` cookie; requests with
`x-return-tokens: true` receive the refresh token in JSON instead. Node's
`/auth/refresh` and `/logout` use the Go-created session unchanged. Go uses the
direct peer IP for login limiting and session metadata; forwarded HTTPS cookie
metadata is accepted from a loopback upstream.

Startup/readiness failures return `503`; failures while forwarding return `502`.
A request canceled during startup does not stop the child. A readiness timeout
stops the unready owned child. On Go shutdown, owned Node processes receive
SIGINT and are killed after three seconds if necessary. A Node backend already
running outside Go is reused and never stopped by Go. Node's existing request
helpers consume the forwarded client IP/protocol headers.
The manager is intended for a single Go instance per Node port.

## Testing

```
go test -race ./...
```

Tests use isolated HTTP servers, helper subprocesses and temporary SQLite
databases. They cover routing, request/response compatibility, WebSocket upgrades,
concurrent startup, restart, cancellation, timeout, external process ownership,
credential rules, connection recovery, JWT/session compatibility, failed-login
cleanup and rate limiting. Integration with the full Node backend requires the
preparation above.

## Building

```
go build -o bin/server ./cmd/server
```

The SQLite driver uses CGO, so building requires CGO enabled and a C compiler.
