<div align="center">

# 🔌 portwhy

**Why is this port taken — and can I free it?**
One command that explains who holds a port and stops it the right way.

<code>🔢 port</code> &nbsp;→&nbsp; <code>🧩 process · container · service</code> &nbsp;→&nbsp; <code>🛑 clean stop</code>

[![License](https://img.shields.io/badge/license-MIT-22c55e?style=flat-square)](LICENSE)
![Go](https://img.shields.io/badge/go-1.27%2B-3776ab?style=flat-square&logo=go&logoColor=white)
![Platforms](https://img.shields.io/badge/macOS%20·%20Linux-supported-0ea5e9?style=flat-square)
[![Release](https://img.shields.io/github/v/release/JtheGunner/portwhy?style=flat-square&color=8b5cf6)](https://github.com/JtheGunner/portwhy/releases)

</div>

---

## 🎯 What it is

When a server fails with `address already in use`, you normally combine `lsof` or `ss`, `docker ps`, `systemctl` and `ps` to find out who is responsible. `portwhy` answers it in one step: it follows the chain from the port to the process, its parents, and the container or service behind it — then offers to stop the cause, never silently.

```text
 port ──▶ process ──▶ parents ──▶ Docker │ systemd │ launchd │ plain process
                                              │
                          explanation + advice ──▶ [--kill] stop it, then verify the port is free
```

---

## 📦 Install

| | Method | Command |
| :-: | --- | --- |
| 🍺 | Homebrew (macOS) | `brew install JtheGunner/tap/portwhy` |
| 📥 | Binary (macOS, Linux) | Download the archive for your platform from [Releases](https://github.com/JtheGunner/portwhy/releases) and put `portwhy` on your `PATH` |
| 🐹 | From source | `go install github.com/JtheGunner/portwhy/cmd/portwhy@latest` |

portwhy is one static binary for macOS and Linux (amd64 and arm64).

---

## 🚀 Usage

```bash
portwhy 3000          # explain one port
portwhy 3000 5432     # several ports
portwhy 3000-3010     # a range
portwhy               # every listening port
portwhy --source docker  # only ports held by Docker containers
portwhy 3000 --kill   # explain, then stop the cause after confirmation
```

A busy port is explained, a free one says so:

```text
$ portwhy 9000 59999
:9000  TCP 127.0.0.1  → php-fpm: (PID 3884) · Homebrew service "php@8.4"
       Label homebrew.mxcl.php@8.4 · Process php-fpm: (PID 3884) · Uptime 6d 1h
       Stop: brew services stop php@8.4
:59999  free
```

When several processes hold the same port, each one gets its own block. Ranges and the all-ports list print only busy ports.

The all-ports list ends with a footer that names the sources currently in use, so you can see what to filter by:

```text
Sources: docker (2) · launchd (1) · process (4) — filter with --source <name>
```

`--source` accepts `docker`, `systemd`, `launchd` and `process` (a plain process without a service). Repeat the flag or separate values with commas to combine them. With `--kill`, the filter also limits which ports are stopped.

> [!TIP]
> Add `--dry-run` to `--kill` to see exactly what would be executed without stopping anything.

---

## 🚩 Flags

| | Flag | Effect |
| :-: | --- | --- |
| 🛑 | `-k`, `--kill` | stop the cause of a busy port after confirmation |
| ✅ | `-y`, `--yes` | skip the confirmation prompt (requires `--kill`) |
| 💥 | `--force` | send SIGKILL immediately instead of SIGTERM (only has an effect together with `--kill`) |
| 👀 | `--dry-run` | show what `--kill` would do without doing it |
| 🔎 | `--source <name>` | only show ports held by this source: `docker`, `systemd`, `launchd` or `process` (repeatable, comma-separated) |
| 🤖 | `--json` | print machine-readable JSON (`"version": 1`) |
| 🎨 | `--no-color` | disable coloured output (`NO_COLOR` and non-TTY output are honoured too) |
| 🏷️ | `-v`, `--version` | print the version |

---

## 🔢 Exit codes

| Code | Meaning |
| :-: | --- |
| `0` | at least one busy port was explained and nothing failed |
| `1` | all requested ports are free |
| `2` | error (invalid input, unreadable data, unsupported OS) |
| `3` | a kill failed |

When several apply, the precedence is `2` > `3` > `0` > `1`.

---

## 🛡️ Stopping safely

> [!IMPORTANT]
> Stopping is always an explicit, confirmed action. The prompt appears only when stdin and stdout are terminals.

- Each source is stopped the way it expects: `docker stop`, `systemctl stop`, `launchctl bootout`, `brew services stop`, or a signal for a plain process.
- portwhy never stops PID 1 or itself, re-checks the process start time before signalling (PID reuse), and verifies afterwards that the port is free.
- A process owned by another user is reported with a `sudo` hint. portwhy never escalates silently.
- A port held by a process that portwhy cannot see is never reported as free.
- Hints warn when the cause will come back, for example a Docker restart policy or a systemd `Restart=` setting.

---

## ⌨️ Shell completion

Generate a completion script for your shell:

```bash
portwhy completion bash   # or: zsh, fish
```

Run `portwhy completion <shell> --help` for the install steps of each shell.

---

## 🛠️ Development

Requires Go 1.27 or newer.

```bash
go build ./...
go vet ./...
golangci-lint run
go test ./...
go test -tags e2e ./e2e   # end-to-end tests against real local listeners
go run ./cmd/portwhy --version
```

---

## 📝 Changelog

See [CHANGELOG.md](./CHANGELOG.md).

---

## 🤝 Contributing

Issues and pull requests are welcome. Please open an issue first for larger changes, and run `go vet ./...`, `golangci-lint run` and `go test ./...` before submitting.

---

## 📄 License

MIT — see [LICENSE](./LICENSE)

---

<div align="center"><sub>Made for developers who just want their port back.</sub></div>
