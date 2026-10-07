# Login Module Rewrite — Agent Workflow

This describes the multi-agent pipeline used to implement the Go rewrite of the
login module (`POST /login`, `POST /auth/refresh`, `POST /logout`, `POST /authorize`)
on the `zoujunhui1-go-rewrite` branch. Six roles, five of them spawned agents, one
(the prep step) done directly by the orchestrating session.

## Roles

| # | Role | Spawned agent? | Skill |
|---|------|-----------------|-------|
| 0 | Prep | No — orchestrator | — |
| 1 | Dev 1 | Yes, worktree-isolated | `go-rewrite-dev`, `go-style` |
| 2 | Dev 2 | Yes, worktree-isolated | `go-rewrite-dev`, `go-style` |
| 3 | Review 1 (reviews Dev 2) | Yes | `go-rewrite-review` |
| 4 | Review 2 (reviews Dev 1) | Yes | `go-rewrite-review` |
| 5 | Merge | Yes | this document (no dedicated skill — the job is narrow enough to specify inline, see below) |
| 6 | Test | Yes | `go-rewrite-test` |

## Sequence

1. **Prep** (orchestrator, before any agent is spawned): commit the shared skeleton
   to `zoujunhui1-go-rewrite` — the `AuthService` struct definition (fields only, no
   business-logic methods), `UserRepository`, `SessionRepository` (both wired to
   `config.DBPath`). This is the contract both devs build on top of.
2. Create two git worktrees off that commit, one branch per dev
   (e.g. `zoujunhui1-go-rewrite-dev1`, `zoujunhui1-go-rewrite-dev2`).
3. **Dev 1** and **Dev 2** run in parallel, each in their own worktree, each commits
   to their own branch when done.
4. **Review 1** and **Review 2** run in parallel, each reviewing the *other* dev's
   branch (cross-review, never self-review).
5. Fix round: review findings get applied (by the orchestrator or by re-invoking the
   relevant dev agent) before moving on.
6. **Merge**: merge `dev1` branch into `zoujunhui1-go-rewrite`, then `dev2` branch.
   Because of the file-ownership split below, this should be conflict-free except for
   `go.mod`/`go.sum` if either dev added a dependency. After both are merged in, wire
   the four new handlers into `internal/router/router.go` (this file is intentionally
   untouched by either dev). Finish with `go build ./...` — if it does not pass, the
   merge is not done.
7. **Test**: once the merged branch builds, run the golden-sample contract tests and
   `go test ./...` per the `go-rewrite-test` skill.

## File ownership (how parallel dev work avoids conflicts)

| Path | Owner |
|------|-------|
| `internal/repository/user.go`, `internal/repository/session.go` | Prep (shared, read-only to devs) |
| `internal/services/auth.go` (struct + shared fields only) | Prep (shared, read-only to devs) |
| `internal/services/auth_login.go` (`Login`, `Refresh` methods) | Dev 1 |
| `internal/handlers/login.go`, `internal/handlers/refresh.go` | Dev 1 |
| `internal/services/auth_logout.go` (`Logout`, `Authorize` methods) | Dev 2 |
| `internal/handlers/logout.go`, `internal/handlers/authorize.go` | Dev 2 |
| `internal/router/router.go` | Merge agent only |

Go allows a struct's methods to live in separate files within the same package, which
is why `auth_login.go` and `auth_logout.go` can both add methods to `*AuthService`
without touching the same file.

## Open items not yet decided

- Whether to re-run this same pipeline for the OIDC endpoints, or treat that as a
  separate future round (see `go-rewrite-dev` skill scope note).
- How the Test agent starts the Node reference server for golden-sample comparison
  (port, who starts/stops it) — not yet specified.
