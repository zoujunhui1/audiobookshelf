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

The routing skeleton and minimal SQLite/user repository foundation exist.
The only HTTP endpoint is `/health`. Authentication routes, password checks,
tokens, middleware, response projection, and Node.js contract tooling are not
implemented yet.

## Layout

```
server-go/
  cmd/server/main.go        entry point: loads config, optionally opens SQLite, starts the server
  internal/config/          port and optional existing database path
  internal/database/        existing SQLite connection; no schema creation/migrations
  internal/repository/      minimal user lookup for the future local-auth service
  internal/router/          gin route registration
  internal/handlers/        request handlers (currently just /health)
```

The user repository returns ID, username, password hash (`pash`), user type,
and active status. Its parameterized lookup matches Node's
`lower(username) = lowercased input` behaviour; it does not trim usernames or
filter out inactive users. A missing user returns `(nil, nil)`, while database
failures return an error. A NULL password hash maps to an empty string.

Services and authentication middleware will be added with login. Future
handlers will call services, which use the repository, following
Handlers → Services → Repository → SQLite. No unused service/interface layer
is introduced in this foundation stage.

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

With no `GO_DATABASE_PATH`, the server runs in health-only mode. To open an
existing Audiobookshelf database, set `GO_DATABASE_PATH` to its filesystem path
(Node stores it at `<ConfigPath>/absdatabase.sqlite`). Prefer a disposable copy
for this experiment. For example, in PowerShell:

```powershell
$env:GO_DATABASE_PATH = 'C:\abs-test\absdatabase.sqlite'
go run ./cmd/server
```

When configured, startup verifies that the database can be opened and fails if
it is missing or inaccessible. It opens in SQLite `mode=rw` (existing-file only),
with one connection, and closes the connection when the server exits. No tables,
schema, or user data are created or changed by this stage; later login work will
need session writes. `/health` stays independent of database queries and is not
a database readiness endpoint. User-table compatibility is checked by repository
queries, not by startup migrations.

SQLite's built-in `lower()` is used, as in the Node query. Non-ASCII case folding
is limited by SQLite; no Unicode extension is added here.

## Tests

```sh
go test ./...
```

Focused tests use temporary SQLite fixtures to check existing-file opening,
schema/data preservation, missing-file rejection, case-insensitive lookup,
NULL hashes, inactive users, query parameterization, database errors, and `/health`.

## Building

```
go build -o bin/server ./cmd/server
```
