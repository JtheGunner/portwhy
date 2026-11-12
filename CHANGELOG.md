# Changelog

All notable changes to this project are documented in this file.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `--expanded` shows the overview of all ports in an airy layout: a titled rule per service, one row per port and a summary box.
- The `--kill` run prints a status line per port (`✓` stopped, `ℹ` left untouched, `⚠` failed) and closes with a `Result` box.
- Plain ASCII symbols and lines for `TERM=dumb` and non-UTF-8 locales.

### Changed
- The overview is a compact table with one row per service, a summary line and a hint line. Hidden owners appear as rows. The `Sources:` line is now part of the hint.
- The detail view uses a titled rule, one fact per row and status symbols for scope, stop command and advice. Long commands wrap over up to three lines instead of being cut off.
- Rules and boxes follow the terminal width, capped at 100 columns; piped output uses 78. `--wide` uses the full width.
- JSON output, exit codes, the confirmation prompt and the stop behaviour are unchanged.

## [0.2.0] - 2026-11-12

### Added
- `--source <name>` limits the port list to ports held by `docker`, `systemd`, `launchd` or `process` (a plain process without a service). The flag is repeatable and accepts comma-separated, case-insensitive values. It applies to text and JSON output, and `--kill` only stops what the filter shows. An unknown value is an error that lists the available sources.
- The all-ports list ends with a `Sources:` line that names the sources currently in use.
- Grouped overview: `portwhy` without arguments, and with ranges, groups busy sockets by the service or process that owns them. The ports of one owner share a line, workers of one service collapse into one row, hidden owners are summarised in one line, and `!` marks sockets reachable from the network.
- Compact detail view for named ports and every `--kill` run: the header names the service, labelled rows are aligned, short facts share one `Facts` row, and each distinct stop command is shown once.
- `--wide` keeps long lines intact on a terminal. Without it, long values are shortened to the terminal width; piped output is never shortened.
- Tab completion for the values of `--source`, including comma-separated lists.
- The Homebrew cask installs the bash, zsh and fish completions automatically.

### Changed
- The default text output is the grouped overview or the compact detail view described above. JSON output, exit codes and the stop behaviour are unchanged.
- Colour is also disabled for `TERM=dumb`; ports, titles and labels use bold and dim styles.
- Worker processes of a launchd job, for example `php-fpm` workers under `brew services`, are attributed to the job that started them and show the same title and stop command. Only the process itself or its direct parent counts as the job.
- Port arguments no longer trigger file name suggestions in the shell.

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

[Unreleased]: https://github.com/JtheGunner/portwhy/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/JtheGunner/portwhy/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/JtheGunner/portwhy/releases/tag/v0.1.0
