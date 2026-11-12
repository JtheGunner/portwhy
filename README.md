<div align="center">

# 🔌 portwhy

**Why is this port taken — and can I free it?**
One command that explains who holds a port and stops it the right way.

<code>🔢 port</code> &nbsp;→&nbsp; <code>🧩 process · container · service</code> &nbsp;→&nbsp; <code>🛑 clean stop</code>

![License](https://img.shields.io/badge/license-MIT-22c55e?style=flat-square)
![Go](https://img.shields.io/badge/go-1.27%2B-3776ab?style=flat-square&logo=go&logoColor=white)
![Platforms](https://img.shields.io/badge/macOS%20·%20Linux-supported-0ea5e9?style=flat-square)
![Status](https://img.shields.io/badge/status-in%20development-f59e0b?style=flat-square)

</div>

---

## 🎯 What it is

When a server fails with `address already in use`, you normally combine `lsof` or `ss`, `docker ps`, `systemctl` and `ps` to find out who is responsible. `portwhy` answers it in one step: it follows the chain from the port to the process, its parents, and the container, service or dev server behind it — then offers to stop the cause, never silently.

> [!NOTE]
> portwhy is in early development. The commands below describe the intended behavior and are being built ticket by ticket.

---

## 🚀 Usage

```bash
portwhy 3000          # explain one port
portwhy 3000 5432     # several ports
portwhy 3000-3010     # a range
portwhy               # every listening port
portwhy 3000 --kill   # explain, then stop the cause after confirmation
```

```bash
portwhy --version
```

---

## 🛠️ Development

Requires Go 1.27 or newer.

```bash
go build ./...
go vet ./...
go test ./...
go run ./cmd/portwhy --version
```

---

## 🤝 Contributing

Issues and pull requests are welcome. Please open an issue first for larger changes, and run `go vet ./...`, `golangci-lint run` and `go test ./...` before submitting.

---

## 📄 License

[MIT](LICENSE)

---

<div align="center"><sub>Made for developers who just want their port back.</sub></div>
