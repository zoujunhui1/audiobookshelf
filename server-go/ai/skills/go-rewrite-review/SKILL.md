---
name: go-rewrite-review
description: Review checklist for a Go rewrite of an Audiobookshelf login-module endpoint, checking behavior parity against the Node.js original and Go code quality.
---

# Go Rewrite — Code Review Skill (Login Module)

Use this skill to review a login-module implementation in `server-go/`. This is meant for cross-review — do not review your own code.

## What to check, in order

1. **Parity with Node.js** — open `server/Auth.js` / `server/auth/TokenManager.js` / `server/controllers/MiscController.js` (whichever applies to the endpoint under review) side by side with the Go code. Check:
   - Status codes match on success and on every error path (401 vs 400 vs 500)
   - Response JSON has the same top-level keys as `getUserLoginResponsePayload` in `Auth.js`
   - Cookie name and flags (`httpOnly`, `sameSite`, `secure`, `maxAge`) match
   - Refresh token grace-period logic is present, not skipped (see the `go-rewrite-dev` skill for the exact 6-step flow it must follow)
   - The password hash column is `pash`, not a renamed or invented field

2. **Security**
   - Password compare uses bcrypt, not a plain string comparison
   - SQL is parameterized, no string-concatenated queries
   - JWT secret is read from an env var, not hardcoded in source
   - No password or token value appears in a log line

3. **Go code quality**
   - Errors are returned, not panicked, for any bad user input
   - No ignored errors (`_ = err`) on DB calls
   - Handlers stay thin: no SQL and no password-hash logic directly inside `internal/handlers`

4. **Scope**
   - Flag it if the change touches OIDC code or files outside what the task actually asked for

## Output format

Report findings as a list, most severe first. For each finding give: file + line, what is wrong, what the Node.js original actually does (with a file:line reference), and the concrete input that would expose the bug. If nothing is wrong, say so explicitly — do not invent findings just to look thorough.
