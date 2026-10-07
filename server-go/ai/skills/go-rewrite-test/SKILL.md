---
name: go-rewrite-test
description: How to verify a Go rewrite of an Audiobookshelf login-module endpoint behaves the same as the Node.js original, using golden-sample contract tests plus Go unit tests.
---

# Go Rewrite — Test Skill (Login Module)

## Setup — never touch the real demo library DB

1. Copy the current SQLite DB file into a scratch location before any test run. Both the Node reference server and the Go server under test must point at the *copy*, not the live file.
2. Clean up the copy after the run. Do not leave `repro-*` / `test-*` rows behind (this project has a history of leftover test rows from earlier bug investigations — do not repeat it).

## Golden-sample contract tests

Run the same request against both the running Node server (reference) and the Go server (under test), then diff the response: status code, JSON body shape, relevant cookie attributes.

Minimum case list (expand per the endpoint actually implemented):

- `POST /login` — correct credentials / wrong password / unknown username / missing fields
- `POST /auth/refresh` — valid refresh token / expired refresh token / token reused inside grace period / token reused after grace period expired
- `POST /logout` — single device / `?allDevices=1`
- `POST /authorize` — valid access token / expired access token / malformed token

## Go unit tests

- `internal/services` — token generation/parsing, and the grace-period branch logic (this is the part most likely to have an off-by-one bug — write a dedicated case for "second refresh using the old token, once before and once after grace period expiry")
- `internal/repository` — run against a throwaway temp-file SQLite seeded from the real schema, not against the shared demo DB

## Regression baseline

Pre-rewrite Node baseline: `npx mocha --recursive test/server` → 354/354 passing (recorded 2026-10-01). Re-running this is only useful if a change could have also touched Node code (it should not, for this module) — keep it as a reference number for when a reviewer asks whether the rewrite regressed anything on the Node side.

## Report

Summarize as: endpoint → case → match / mismatch, one line each. Call out any case where Go intentionally diverges from Node and why (e.g. "not yet implemented" is a fine diff to report; a silently wrong response is not).
