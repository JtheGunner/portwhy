# portwhy — project instructions

Go CLI: `portwhy <port>` explains why a local port is taken (process, container, service) and stops the cause after confirmation. macOS and Linux only. Public repo `github.com/JtheGunner/portwhy`, MIT. Only deltas from the global rules are listed here.

## Scope guards

- Out of scope: Windows, Kubernetes port-forward detection, remote hosts, daemon/history/monitoring, TUI, firewall analysis, port scanning. Do not add them without the owner's say-so.
- All user-facing output is **English** (overrides the German/English option in `language.md`).
- Design and plan live in `docs/superpowers/` and are **local-only** (excluded via `.git/info/exclude`). Never `git add` them, never mention them in commits or PRs. Work is ticketed in YouTrack; each plan task is one ticket.

## Architecture

`socket` → `proc` → `detect` (docker → systemd | launchd → generic) → `advise` → `report` → `render`; `kill` executes the `Terminator` a detector returns. OS-specific code only in `socket`, `proc`, `detect/systemd`, `detect/launchd`, `cli/deps_*.go` behind build tags and interfaces. Keep `cmd/portwhy/main.go` to startup and exit code.

## Hard rules

- Anything that shells out goes through `internal/execx.Runner` with a context timeout — no direct `exec.Command` elsewhere, so it stays fakeable.
- No cgo; one static binary. Runtime dependencies: cobra and `golang.org/x/term` only. Tests use stdlib `testing` — no assertion library. Ask before adding a dependency.
- Docker is spoken to via its Unix-socket HTTP API (2 s timeout, silent skip when unreachable), never via the `docker` CLI.
- Killing safety is non-negotiable: always confirmed (prompt only on a TTY), never PID 1 or own PID, re-check process start time before signalling (PID reuse), verify the port is free afterwards, never escalate with silent `sudo`. A port held by a process we cannot see must never be reported as free.
- Exit codes: `0` busy and explained, `1` all free, `2` error, `3` kill failed; precedence `2 > 3 > 0 > 1`. JSON output carries `"version": 1`; changing the schema needs a version bump.
- Parsers for external formats (`/proc`, `lsof -F`, `netstat`, `ps`, `launchctl`, Docker JSON) are untagged pure functions with fixtures in `testdata/`, so they run on both OSes.

## Testing

- Text and JSON output are covered by golden files in `internal/render/testdata`; refresh with `go test ./internal/render -update` and review the diff by eye.
- End-to-end tests carry the `e2e` build tag: `go test -tags e2e ./e2e`. They run on ubuntu and macOS CI. Docker tests run only when a daemon is reachable.

## Commands

```bash
go build ./... && go vet ./...
golangci-lint run
go test ./...
go test -tags e2e ./e2e
go run ./cmd/portwhy 3000
```

## Decisions on record

- macOS socket data comes from an own `lsof`/`netstat` parser (`internal/socket`); `gopsutil` is not adopted. Spike outcome: it was not added or run (no network access), but from memory its macOS connection listing also shells out to `lsof` (unverified), so it would add a dependency without removing the external tool, and without root `lsof` sees only own processes either way, which the `netstat` merge (PID 0 for invisible owners) covers. Revisit only if the parser proves fragile. Process data (`ps`) follows in the process-reader ticket.
- Podman and dev-server heuristics are v0.2; `wait`/`free` subcommands only if demand appears.
