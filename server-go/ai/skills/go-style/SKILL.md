---
name: go-style
description: Go coding conventions for server-go, grounded in Effective Go and the Go Code Review Comments wiki, plus one project-specific rule for Node.js API parity.
---

# Go Style — server-go

Applies to all code under `server-go/`, not just the login module. Module-specific behavior rules live in their own skill (e.g. `go-rewrite-dev` for login).

Source of truth for everything below except the last section: [Effective Go](https://go.dev/doc/effective_go) and [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments). When in doubt, those two documents win over this file.

## Enforced, not optional

- `gofmt -l .` must return no output before a task is considered done.
- `goimports` run on every changed file (handles import grouping automatically: stdlib, then third-party, then internal).
- `go vet ./...` must be clean.

## Naming (Go Code Review Comments: Package Names, Initialisms, MixedCaps)

- `MixedCaps` or `mixedCaps`, never underscores.
- Package names: short, lowercase, one word, no underscores, no mixedCaps. Name the package for what it provides, not what it contains (e.g. `repository`, not `repository_utils`).
- Avoid stutter: don't repeat the package name in exported identifiers accessed from outside (`user.User` is bad, `user.Service` is fine since it reads as `user.Service`, not `UserService`).
- Initialisms keep one consistent case: `ID`, `URL`, `HTTP`, `JWT` — not `Id`, `Url`, `Http`, `Jwt`.
- No `Get` prefix on getters: a method returning a name is `Name()`, not `GetName()`.
- Single-method interfaces get an `-er` name from the method: a type with a `Close` method implements `Closer`.

## Receivers (Effective Go: Method Receivers)

- Short, usually one or two letters derived from the type name (`s *AuthService`, not `self`, `this`, or `svc`).
- Same receiver name on every method of the same type.
- Consistent pointer vs. value receiver per type — don't mix `(s *AuthService)` and `(s AuthService)` across methods of the same type.

## Errors (Go Code Review Comments: Error Strings; Effective Go: Errors)

- Error strings are lowercase and don't end in punctuation: `"creating session: %w"`, not `"Creating session: %w."` — because they often get wrapped into a larger error message.
- Wrap with `%w` (`fmt.Errorf("creating session for user %s: %w", userID, err)`) so callers can `errors.Is` / `errors.As`.
- Don't ignore an error silently (`_ = err`) unless there's a one-line comment saying why it's safe.
- Reserve `panic` for programmer errors (e.g. invariant violated at startup), never for an expected failure like bad user input or a DB miss — those return an `error`.
- `internal/services` and `internal/repository` return `error` and don't know about HTTP; only `internal/handlers` maps an error to a status code.

## Doc comments (Effective Go: Commentary)

- Every exported name gets a doc comment starting with the name itself, as a full sentence: `// Service handles login, refresh, and logout.`
- Comment the package itself once, in a `doc.go` or at the top of the main file, if the package's purpose isn't obvious from its name.

## Variable names (Effective Go: Names)

- Short names for short-lived, narrow-scope variables (`i`, `err`, `ctx`, `r` for a request).
- Longer, descriptive names as scope widens (package-level vars, exported names).

## Context (Go Code Review Comments: Contexts)

- `context.Context` is the first parameter, named `ctx`, on any function that does I/O (DB calls, outbound requests).
- Never store a `context.Context` inside a struct field.

## Project-specific rule (not a general Go convention — specific to this rewrite)

Go exported struct fields default to `UpperCamelCase`, but the Node.js API this project is replacing uses `camelCase` JSON keys (e.g. `userDefaultLibraryId`, `refreshToken`). Every struct marshaled into an API response or parsed from an API request must carry an explicit `json:"..."` tag matching the Node.js field name exactly. A missing tag compiles fine but silently breaks contract-test parity — see the `go-rewrite-test` skill.
