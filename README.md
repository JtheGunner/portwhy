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
portwhy                  # overview of every listening port, grouped by service
portwhy 3000-3010        # overview of a range
portwhy --source docker  # overview of the ports held by Docker containers only
portwhy 3000             # explain one port in detail
portwhy 3000 5432        # several ports in detail
portwhy 3000 --kill      # explain, then stop the cause after confirmation
```

Without arguments, and for ranges, portwhy prints a compact table: one row per service or process, with its ports, protocol, bind address and owner. Free ports are left out. `⚠` marks sockets reachable from the network, and a summary line counts busy ports, services and hidden owners.

```text
$ portwhy
     Service / process                    Ports                   Proto  Bind       Owner
──────────────────────────────────────────────────────────────────────────────────────────────────────────────
  ⚠  Spotify                              1900, 5353              udp    0.0.0.0    PID 13250
     Antigravity                          3000, 3100, 3306, 3890  tcp    127.0.0.1  PID 3983
  ⚠  Docker container "db" (postgres:16)  5432                    tcp    0.0.0.0    postgres (PID 812)
     Homebrew service "php@8.4"           9000                    tcp    127.0.0.1  php-fpm (PID 3884) +2 more
  ⚠  Owner not visible                    22, 88, 137, 138, 445   tcp    0.0.0.0    run with sudo
──────────────────────────────────────────────────────────────────────────────────────────────────────────────
  13 busy ports · 4 services · ⚠ 8 reachable from the network · 5 ports with hidden owner
  ℹ  Details: portwhy 1900    Airy view: --expanded    Filter: --source docker|launchd|process
```

Add `--expanded` for an airier layout with a titled rule per service, one row per port and a summary box. It suits a short list better than a busy machine.

```text
$ portwhy --expanded
──────────────────────────────────────── Spotify · 2 ports ─────────────────────────────────────────

  ⚠  1900  udp  0.0.0.0    PID 13250
  ⚠  5353  udp  0.0.0.0    PID 13250
```

The hint under the all-ports overview names the sources currently in use, so you can see what to filter by. `--source` accepts `docker`, `systemd`, `launchd` and `process` (a plain process without a service). Repeat the flag or separate values with commas to combine them. With `--kill`, the filter also limits which ports are stopped.

A named port gets a detailed block that names the service, lists its facts and shows how it would be stopped. A free port says so:

```text
$ portwhy 9000 3000
───────────────────── :9000 · Homebrew service "php@8.4" ─────────────────────

  ℹ  tcp 0.0.0.0

     Process  php-fpm (PID 3884) +2 more
     Uptime   3d 0h
     Label    homebrew.mxcl.php@8.4

  →  Stop     brew services stop php@8.4

  ⚠  Reachable from the network (bound to all interfaces).

─────────────────────────────────── :3000 ────────────────────────────────────

  ✓  free
```

Several processes of one service on the same port (a master and its workers) share one block. On a terminal, lines fit the terminal width, capped at 100 columns, and a long command wraps over at most three lines. Piped output uses 78 columns and is never shortened. Use `--wide` to use the full terminal width and keep long values intact. Terminals without Unicode support (`TERM=dumb` or a non-UTF-8 locale) get plain ASCII symbols and lines.

With `--kill`, every port gets a status line (`✓` stopped, `ℹ` left untouched, `⚠` failed) on stderr, followed by a `Result` box.

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
| 🎨 | `--no-color` | disable coloured output (`NO_COLOR`, `TERM=dumb` and non-TTY output are honoured too) |
| 🪟 | `--expanded` | show the overview of all ports in the airy layout, one row per port |
| ↔️ | `--wide` | use the full terminal width and keep long values intact |
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

Tab completes flags and the values of `--source` (`portwhy --source d<Tab>` → `docker`).

With Homebrew the completions for bash, zsh and fish are installed automatically. Restart your shell to pick them up. For a manual install, load the script once:

| | Shell | Enable for the current session | Enable permanently |
| :-: | --- | --- | --- |
| 🐚 | bash | `source <(portwhy completion bash)` | `portwhy completion bash > ~/.local/share/bash-completion/completions/portwhy` |
| 🌀 | zsh | `source <(portwhy completion zsh)` | `portwhy completion zsh > "${fpath[1]}/_portwhy"` |
| 🐟 | fish | `portwhy completion fish \| source` | `portwhy completion fish > ~/.config/fish/completions/portwhy.fish` |

> [!NOTE]
> bash needs the `bash-completion` package, and zsh needs `compinit` enabled in `~/.zshrc`.

Run `portwhy completion <shell> --help` for more details on each shell.

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
