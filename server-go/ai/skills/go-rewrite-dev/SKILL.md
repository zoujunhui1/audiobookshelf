---
name: go-rewrite-dev
description: Rules for implementing an Audiobookshelf login-module endpoint in Go (server-go), based on exact behavior of the existing Node.js Auth.js implementation.
---

# Go Rewrite — Developer Skill (Login Module)

Use this skill when implementing one or more login-module endpoints in `server-go/`. The goal is behavior parity with the existing Node.js implementation, not a redesign.

## Never improve the business logic — replicate it exactly

Do not fix, improve, simplify, or otherwise diverge from the existing Node.js
behavior, even when the Go idiom or a stricter/more "correct" behavior seems
better. Example: if Node returns 401 on an unexpected DB error during refresh,
Go returns 401 too — not 500, even though 500 is arguably more semantically
accurate. If you believe the Node behavior is actually a bug, do not silently
"fix" it — port it as-is and flag it in your report/commit message instead, and
let the reviewer/orchestrator decide whether to intentionally diverge.

Parity with the existing behavior is the entire point of this rewrite: this
project's golden-sample contract tests (`go-rewrite-test` skill) compare Go's
responses against Node's byte-for-byte, and a reviewer's job (`go-rewrite-review`
skill) is to catch exactly this kind of unrequested deviation.

## Ground truth source files (read before writing code)

- `server/Auth.js` — route definitions, login/logout/refresh/OIDC handlers
- `server/auth/TokenManager.js` — JWT generation, session rotation, refresh grace period
- `server/auth/LocalAuthStrategy.js` — password check (bcrypt), user lookup
- `server/routers/ApiRouter.js` (`/authorize` route) + `server/controllers/MiscController.js` — authorize handler
- `server/models/User.js`, `server/models/Session.js` — DB row shape

Always re-read the relevant file before implementing. This document is a summary, not the full spec.

## Database — no schema change

- SQLite file is shared with the Node service, unchanged schema. Do not add migrations.
- Users table: password hash is in column `pash`, not `password`.
- Session table columns: `id, userId, ipAddress, userAgent, refreshToken, expiresAt, lastRefreshToken, lastRefreshTokenExpiresAt`.

## Password check

- Node uses bcrypt, cost factor 8, via `bcrypt.compare(password, user.pash)`.
- Go side: use `golang.org/x/crypto/bcrypt`. The hash format is compatible, no re-hashing needed.

## JWT

- Secret: read from `JWT_SECRET_KEY` env var on both the Node and the Go side (team decision — do not read Node's `serverSettings.tokenSecret` DB row).
- Algorithm: HMAC (HS256, this is the `jsonwebtoken` default).
- Access token payload: `{ userId, username, jti, type: "access" }`, expiry from `ACCESS_TOKEN_EXPIRY` env (default 3600s).
- Refresh token payload: `{ userId, username, jti, type: "refresh" }`, expiry from `REFRESH_TOKEN_EXPIRY` env (default 30 days, i.e. 2592000s).
- `jti` is a random UUID, generate a new one for every token.

## Refresh token rotation — replicate exactly, this is the trickiest part

On `POST /auth/refresh`:

1. Verify JWT signature and check `type === "refresh"`.
2. Find the session row where `refreshToken = token` OR `lastRefreshToken = token`.
3. If matched by `lastRefreshToken` and `lastRefreshTokenExpiresAt` is still in the future → grace period hit: return the session's *current* `refreshToken` plus a new access token, do not rotate again.
4. If matched by `lastRefreshToken` but the grace period already expired → reject with `"Invalid refresh token"`.
5. If matched by `refreshToken` directly and `expiresAt` is in the past → delete the session, reject with `"Refresh token expired"`.
6. Otherwise → rotate: generate a new access + refresh token, move the old `refreshToken` value into `lastRefreshToken` with `lastRefreshTokenExpiresAt` set to now + `REFRESH_TOKEN_GRACE_PERIOD` (default 600s / 10 min), update the session row, set a new `refresh_token` cookie.

The grace period exists so a client that retries with a stale token (because it never saw the rotation response) can still recover. Do not skip step 3 — it changes the outcome of the "double refresh" test case.

## Cookie

- Name: `refresh_token`
- `httpOnly: true`
- `sameSite: lax`
- `secure`: true only when the request is over HTTPS
- `maxAge`: refresh token expiry, in milliseconds

## Response shape

`POST /login`, a successful `POST /auth/refresh`, and `POST /authorize` all return the same payload shape, built by `getUserLoginResponsePayload(user)` in `server/Auth.js`:

```
{
  user: <User.toOldJSONForBrowser()>,
  userDefaultLibraryId: string,
  serverSettings: <Setting.toJSONForBrowser()>,
  ereaderDevices: [...],
  Source: string
}
```

Read `toOldJSONForBrowser()` on `User` and `toJSONForBrowser()` on the settings model for the exact field list when you implement this. Do not guess the fields.

## Layering

`internal/handlers` (thin — gin binding, call a service, write the response) → `internal/services` (business logic lives here) → `internal/repository` (SQL against the shared SQLite file). Do not put SQL in handlers, and do not put business logic in repository.

## Before marking a task done

- `go build ./...` passes
- Manual curl smoke test for the happy path
- Do not write test assertions yourself — that's the test skill's job. Just confirm the server runs and responds.
