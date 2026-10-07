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

The local authentication experiment includes `POST /login`, `POST /auth/refresh`, `POST /logout`,
Bearer access-token middleware, protected `GET /api/me`, and public `/health`.
Login verifies the existing bcrypt hash, issues access/refresh JWTs, and inserts
a session. Refresh validates and rotates the stored session credentials. The
protected endpoint verifies an access JWT and reads the current user from SQLite.
Logout deletes stored refresh sessions and clears the refresh cookie. This local
authentication slice is functionally complete for the experiment; OIDC and full
response projection are not implemented.

## Layout

```
server-go/
  cmd/server/main.go        entry point: loads config, optionally opens SQLite, starts the server
  internal/config/          port, optional database path, signing key and token lifetimes
  internal/database/        existing SQLite connection; no schema creation/migrations
  internal/repository/      users, session insertion/lookup/rotation/deletion and stored signing-key read
  internal/services/        local login, access-token verification, refresh rotation and logout
  internal/middleware/      Bearer authentication for the protected endpoint
  internal/router/          gin route registration
  internal/handlers/        /health, /login, /auth/refresh, /logout, minimal /api/me and shared token response helpers
```

The user repository returns ID, username, password hash (`pash`), user type,
and active status. Its parameterized lookup matches Node's
`lower(username) = lowercased input` behaviour; it does not trim usernames or
filter out inactive users. A missing user returns `(nil, nil)`, while database
failures return an error. A NULL password hash maps to an empty string. The
protected-request lookup uses the current primary user ID and selects only
identity/active fields, without reading the password hash.

Login follows Handlers → Services → Repository → SQLite. The handler binds
credentials and selects token transport; the service verifies credentials and
creates tokens/session; repositories contain the SQL. For `/api/me`, middleware
calls the same authentication service, which verifies the JWT and reads the
user repository; the handler returns the authenticated identity. There is no
generic repository or unused interface hierarchy. Refresh follows the same
handler/service/repository layering and shares login's token response/cookie helpers.
Logout follows the same layers, with session deletion contained in the repository.

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

With no `GO_DATABASE_PATH`, `/health` works while `/login`, `/auth/refresh`, `/logout`, and
`/api/me` return `503`. To open an existing Audiobookshelf database, set
`GO_DATABASE_PATH` to its filesystem path
(Node stores it at `<ConfigPath>/absdatabase.sqlite`). Prefer a disposable copy
for this experiment. For example, in PowerShell:

```powershell
$env:GO_DATABASE_PATH = 'C:\abs-test\absdatabase.sqlite'
go run ./cmd/server
```

When configured, startup verifies that the database can be opened and fails if
it is missing or inaccessible. It opens in SQLite `mode=rw` (existing-file only),
with one connection and foreign keys enabled. No tables or schema are created
or migrated. Successful login inserts one row into the existing `sessions` table;
refresh updates that row and logout deletes matching session rows. User records
and settings are not modified. Use a database initialized
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
(`access` or `refresh`), and second-based `iat`/`exp`. The access token can be used
with `/api/me`; the refresh credential is used at `/auth/refresh` and `/logout`.

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

## Protected identity endpoint

Take `user.accessToken` from a successful login and send it as a Bearer token:

```sh
curl http://localhost:4000/api/me \
  -H 'Authorization: Bearer YOUR_ACCESS_TOKEN'
```

Success returns a direct, minimal user object (no login envelope or tokens):

```json
{"id":"...","username":"your-user","type":"user","isActive":true}
```

Authentication requires a valid HS256 signature, an unexpired `exp`,
`type: "access"`, and a nonempty string `userId`. Missing/malformed Bearer
headers, invalid/tampered/expired JWTs, refresh credentials, and missing/inactive
users return `401 {"error":"Unauthorized"}` with `WWW-Authenticate: Bearer`.
Database failures return a generic `500 {"error":"Authentication failed"}`;
the handler never runs after failed authentication. `/login` and `/health`
remain public.

Each authenticated request reloads the user by ID. The response uses the current
SQLite username, type and active status rather than the JWT's stored username.
Rename/type changes are visible immediately, and deactivated or deleted users
are rejected. Verification only reads data and does not create or rotate sessions;
it does not require a refresh-session row, matching Node's access-token flow.

This stage deliberately omits Node's full user projection, query-parameter
tokens, legacy non-expiring/untyped tokens, old-user-ID aliases, and API keys.
It accepts only Bearer access credentials for the current user ID; failures are
JSON rather than Passport's text response. OIDC remains unimplemented.

## Refresh

Send the refresh credential from login in `x-refresh-token`, or use its cookie:

```sh
curl -X POST http://localhost:4000/auth/refresh \
  -H 'x-refresh-token: YOUR_REFRESH_TOKEN'
```

For cookie mode, save login's cookies with curl's `-c cookies.txt`, then refresh
with `-b cookies.txt -c cookies.txt`. No request body or access token is required.

- A present `x-refresh-token` header takes precedence over `refresh_token`.
  In header mode the response includes the new `user.refreshToken`. Cookie mode
  returns `user.refreshToken: null`. Both modes set the new refresh cookie with
  the same security attributes/lifetime used by login, matching Node's refresh
  transport. The response otherwise uses the same minimal login user projection.
- Refresh requires a signed, unexpired HS256 JWT with `type: "refresh"` and a
  nonempty `userId`, an exact match to the current stored refresh token, a matching
  session user ID, an unexpired session, and an existing active user. Access tokens
  are rejected. Both JWT expiry and SQLite session expiry are checked.
- Success issues a fresh access/refresh pair and conditionally updates the same
  session's refresh token, expiry and update timestamp. Session ID, creation time,
  IP address and user agent stay unchanged. New token lifetimes run from refresh
  time. Only a successfully persisted pair is returned; no schema is changed.
- Missing credentials return `401 {"error":"No refresh token provided"}`.
  Other rejected credentials use the Node-style messages `Invalid token type`,
  `Invalid refresh token`, `Refresh token expired`, or `User not found or inactive`.
  Storage failures return generic `500 {"error":"Refresh failed"}` without tokens
  or cookie changes. Existing access JWTs remain valid after refresh rotation.

### Refresh compatibility limits

Refresh tokens are single-use. There is no Node-style previous-token grace period
or recovery of a lost rotation response: old tokens immediately return `401`.
The conditional SQLite update allows only one winner when two requests submit
the same token; the loser returns `401` rather than receiving the winner's token.
Previous-token fields are cleared on rotation. `REFRESH_TOKEN_GRACE_PERIOD` is
not used.

Expired session rows are rejected but not automatically removed; cleanup is
deferred. No login/refresh rate limiter or full browser response projection is
added. A present empty header rejects the request even with a valid cookie,
following this experiment's explicit header precedence; Node instead falls back
to the cookie for an empty header. OIDC remains outside the experiment's
implemented interfaces.

## Local logout

```sh
curl -X POST http://localhost:4000/logout \
  -H 'x-refresh-token: YOUR_REFRESH_TOKEN'
# {"redirect_url":null}
```

Cookie mode can send login/refresh's cookie with `-b cookies.txt -c cookies.txt`.
Matching Node's logout (which differs from refresh), a nonempty `refresh_token`
cookie takes precedence over `x-refresh-token`; an empty/missing cookie falls
back to the header. Every response clears the refresh cookie at `Path=/`, with
the same HttpOnly, SameSite and Secure attributes used when issuing it.

Normal logout deletes the exact current stored refresh session. With
`?allDevices=1`, one SQLite statement deletes all sessions belonging to the user
identified by that stored token; other users' sessions remain. No access token,
request body, or user ID is accepted as an alternative logout credential.
Missing, unknown, malformed or already-deleted tokens return `200` with
`{"redirect_url":null}`. A matching stored token is sufficient for deletion,
even if expired or the user is inactive, as in Node; logout does not validate
JWT claims or parse session expiry. Deletion failures return a generic
`500 {"error":"Logout failed"}` while still clearing the browser cookie.
Without authentication configured, logout clears the cookie and returns `503`.

Issued access JWTs remain valid until expiry (subject to the existing current
user/active checks), including after all-device logout. This experiment has no
global access-token revocation. As with refresh, only the current stored token
is recognized: Node's all-device logout can also match `lastRefreshToken`.
Passport/Express session logout, `auth_method` cookie cleanup and OIDC provider
logout are omitted because this Go slice does not create those sessions/cookies.
Unlike Node's normal logout, database deletion errors are reported as `500`
rather than silently returning success.

## Tests

```sh
go test ./...
```

Focused tests use temporary SQLite fixtures to check the foundation and the
main login cases: Node-generated bcrypt hashes (including a Unicode password),
case-insensitive lookup, inactive/invalid users, malformed/missing input,
passwordless root, JSON/form bodies, token signatures/claims, cookie/header
transport, persisted sessions, expiry configuration, failed session writes,
and `/health`. Protected-endpoint tests reuse those SQLite fixtures and actual
login tokens, checking rejection cases, current user identity, deactivation/
deletion, database failures, and no session writes during authentication. They
do not use a real user database or a large contract framework. Refresh tests cover
both transport modes and header precedence, token/session expiry, unknown or
inactive credentials, rotation/replay, persisted session metadata, new access
tokens reaching `/api/me`, failed writes, and concurrent single-use rotation.
Logout tests cover cookie/header precedence, cookie clearing, missing/invalid
credentials, repeated logout, expired sessions, all-device deletion with another
user preserved, storage failures, rejected refresh after logout, and continued
access-token/login/health behavior.

## Building

```
go build -o bin/server ./cmd/server
```
