# sysdash

`sysdash` is a keyboard-driven macOS terminal dashboard built with Bubble Tea, Bubbles, Lip Gloss, and gopsutil. It combines a compact dashboard with CPU and memory process drill-downs, connection inspection, and a lazy disk space browser.

## Features

- Main dashboard widgets for CPU, Memory, Disk I/O, Network, and Disk Browser
- CPU drill-down sorted by CPU descending
- Memory drill-down sorted by RSS descending
- Disk I/O throughput and network RX/TX throughput
- Network drill-down based on process-owned connections on macOS via `lsof -i -n -P`
- Disk browser rooted at `/`, sorted by size descending, with percentages and bars
- Lazy, async, cached disk scanning that only scans visited paths
- Process actions for `SIGTERM`, `SIGKILL`, `SIGSTOP`, and `SIGCONT`
- `SIGKILL` confirmation prompt

## Repo Layout

```text
.
├── go.mod
├── main.go
├── README.md
└── internal
    ├── actions
    │   └── process.go
    ├── app
    │   └── model.go
    ├── collect
    │   ├── connections_darwin.go
    │   ├── connections_stub.go
    │   └── sampler.go
    ├── diskscan
    │   └── scanner.go
    ├── domain
    │   └── types.go
    └── ui
        ├── render.go
        └── styles.go
```

## Build

```bash
go mod tidy
go build ./...
```

## Run

```bash
go run .
```

The app is intended for macOS Apple Silicon. On macOS, the network drill-down uses `lsof` for process-owned connections because per-process network throughput attribution is out of scope for this MVP.

## Keys

- `tab` / `shift+tab`: focus dashboard widgets
- `enter`: drill down, or descend into a selected directory in the disk browser
- `esc`: return to dashboard
- `backspace`: go to parent directory in the disk browser
- `j` / `k`: move selection
- `g` / `G`: jump to top or bottom
- `r`: refresh immediately
- `q`: quit
- `t`: send `SIGTERM` to selected process
- `x`: request `SIGKILL` confirmation for selected process views
- `s`: send `SIGSTOP`
- `c`: send `SIGCONT`

## Notes

- The disk browser scanner avoids crossing device boundaries and marks partial sizes with `*` when a path could not be scanned fully.
- Root-level scans can still take time on large systems, but they run asynchronously and cache completed paths.
- The dashboard refreshes once per second.
