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

The SQLite foundation and first functional local `POST /login` endpoint exist,
alongside `/health`. Login verifies the existing bcrypt hash, issues access and
refresh JWTs, and inserts a session into the existing database. Refresh/logout
routes, protected endpoints/middleware, OIDC, and full response projection are
not implemented.

## Layout

```
server-go/
  cmd/server/main.go        entry point: loads config, optionally opens SQLite, starts the server
  internal/config/          port, optional database path, signing key and token lifetimes
  internal/database/        existing SQLite connection; no schema creation/migrations
  internal/repository/      user lookup, session insertion and stored signing-key read
  internal/services/        local credential checks and token/session creation
  internal/router/          gin route registration
  internal/handlers/        /health and /login HTTP handlers
```

The user repository returns ID, username, password hash (`pash`), user type,
and active status. Its parameterized lookup matches Node's
`lower(username) = lowercased input` behaviour; it does not trim usernames or
filter out inactive users. A missing user returns `(nil, nil)`, while database
failures return an error. A NULL password hash maps to an empty string.

Login follows Handlers → Services → Repository → SQLite. The handler binds
credentials and selects token transport; the service verifies credentials and
creates tokens/session; repositories contain the SQL. No generic repository,
unused interface hierarchy, or authentication middleware is introduced.

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

With no `GO_DATABASE_PATH`, `/health` works and `/login` returns `503`. To open an
existing Audiobookshelf database, set `GO_DATABASE_PATH` to its filesystem path
(Node stores it at `<ConfigPath>/absdatabase.sqlite`). Prefer a disposable copy
for this experiment. For example, in PowerShell:

```powershell
$env:GO_DATABASE_PATH = 'C:\abs-test\absdatabase.sqlite'
go run ./cmd/server
```

When configured, startup verifies that the database can be opened and fails if
it is missing or inaccessible. It opens in SQLite `mode=rw` (existing-file only),
with one connection and foreign keys enabled. No tables or schema are created
or migrated. Successful login inserts one row into the existing `sessions`
table; user records and settings are not modified. Use a database initialized
by Node with the authentication tables already present. `/health` stays
independent of database queries and is not a database readiness endpoint.

Login's signing key comes from `JWT_SECRET_KEY`, or, if unset, from
`settings['server-settings'].value.tokenSecret`. The stored string is used
directly, without base64-decoding. Startup fails when neither provides a key;
it does not generate an ephemeral key or update settings. When using an override,
use the same key as Node if token interoperability is required.

`ACCESS_TOKEN_EXPIRY` and `REFRESH_TOKEN_EXPIRY` accept positive integer seconds,
defaulting to 3600 and 2592000 respectively. Invalid overrides fail startup.

SQLite's built-in `lower()` is used, as in the Node query. Non-ASCII case folding
is limited by SQLite; no Unicode extension is added here.

## Local login

```sh
curl -X POST http://localhost:4000/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"your-user","password":"your-password"}'
```

JSON and URL-encoded request bodies are supported. Username lookup is
case-insensitive and does not trim whitespace. Inactive users, unknown users,
invalid passwords, and invalid/missing non-root hashes receive `401`. Matching
Node, an active root without a password hash can log in with an explicit empty
password; an omitted password receives `400`.

Successful login returns this deliberately reduced projection:

```json
{"user":{"id":"...","username":"...","type":"user","isActive":true,"accessToken":"...","refreshToken":null}}
```

Both JWTs use HS256 and contain `userId`, `username`, UUIDv4 `jti`, `type`
(`access` or `refresh`), and second-based `iat`/`exp`. The access token is ready
for the next milestone's protected-endpoint test. Issuing a refresh credential
here does not implement a refresh route.

By default the refresh token is delivered in an `HttpOnly`, `SameSite=Lax`,
`Path=/` cookie with the configured refresh lifetime. `Secure` is set for TLS
or an `https` entry in `x-forwarded-proto`, matching Node. With the exact header
`x-return-tokens: true`, the refresh token is instead returned in
`user.refreshToken` and no refresh cookie is set. Each successful login stores
that refresh token with the user ID, peer IP, user agent, UUID session ID, and
UTC Sequelize-compatible creation/update/expiry timestamps. Previous-token
fields remain NULL. A session insertion failure returns `500` with no tokens.

### Experiment compatibility limits

- The response omits the legacy `user.token`, permissions, progress, bookmarks,
  email, library selection, settings, e-reader devices, and `Source`. It is not
  a complete replacement for the existing frontend login payload.
- Errors are JSON: `400` for missing/invalid credentials input, `401` with
  `{"error":"Invalid credentials"}`, `415` for unsupported content types, and
  a generic `500` for backend failures. Node's Passport failure bodies are text;
  an empty username also returns `400` here instead of Node's `401`.
- Only body credentials are accepted. No Passport/Express session cookie,
  login rate limiter, router-base-path aliases, or configured-auth-method switch
  is implemented. This experiment exposes local login when a database/key is configured.
- Session IP metadata records the direct peer, rather than Node's broader
  forwarded-IP selection. Missing signing keys must be configured explicitly;
  Node's automatic secret generation is deferred. Invalid expiry overrides are
  rejected rather than applying Node's permissive parsing/fallback.

## Tests

```sh
go test ./...
```

Focused tests use temporary SQLite fixtures to check the foundation and the
main login cases: Node-generated bcrypt hashes (including a Unicode password),
case-insensitive lookup, inactive/invalid users, malformed/missing input,
passwordless root, JSON/form bodies, token signatures/claims, cookie/header
transport, persisted sessions, expiry configuration, failed session writes,
and `/health`. They do not use a real user database or a large contract framework.

## Building

```
go build -o bin/server ./cmd/server
```
