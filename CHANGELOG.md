# Changelog

All notable changes to this project are documented in this file.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-11-12

First release.

### Added
- `portwhy [port|range]...` explains why a local port is taken: process, command line, user, uptime, working directory, parent chain, protocol and bind address. Without arguments it lists every listening port.
- Source detection with a source-appropriate way to stop it:
  - Docker containers, matched by published port through the Docker Unix-socket API (`docker stop` equivalent).
  - systemd services on Linux (`systemctl stop`, `--user` for user units).
  - launchd jobs on macOS (`launchctl bootout`) and Homebrew services (`brew services stop`).
  - Any other process (SIGTERM, then SIGKILL after confirmation).
- `-k`/`--kill` stops the cause after confirmation, `--yes` skips the prompt, `--force` sends SIGKILL at once, `--dry-run` only prints what would happen. Killing never targets PID 1 or portwhy itself, re-checks the process start time before signalling, verifies afterwards that the port is free and never escalates with a silent `sudo`.
- Advice for common situations: macOS AirPlay Receiver on ports 5000/7000, lingering `TIME_WAIT` sockets, listeners reachable from the network, and owners that are not visible without `sudo`.
- `--json` machine-readable output (schema `version` 1) and `--no-color`.
- Stable exit codes: `0` busy and explained, `1` all requested ports free, `2` error, `3` kill failed.
- Shell completion for bash, zsh and fish via `portwhy completion <shell>`.
- Pre-built binaries for macOS and Linux on amd64 and arm64, and a Homebrew tap (`JtheGunner/tap`).

[0.1.0]: https://github.com/JtheGunner/portwhy/releases/tag/v0.1.0
