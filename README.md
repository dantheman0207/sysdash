# sysdash

`sysdash` is a keyboard-driven macOS terminal dashboard built with Bubble Tea, Bubbles, Lip Gloss, and gopsutil. It combines a compact dashboard with CPU and memory process drill-downs, connection inspection, and a lazy disk space browser.

## Features

- Main dashboard widgets for CPU, Memory, Disk I/O, Network, and Disk Browser
- CPU drill-down sorted by CPU descending
- Memory drill-down sorted by RSS descending
- Disk I/O throughput and network RX/TX throughput
- Network drill-down based on process-owned connections on macOS via `lsof -i -n -P`
- Disk browser rooted at `/`, sorted by size descending, with percentages and bars
- Cached recursive disk scanning for visited paths, with progress in the full disk browser
- Disk browser cleanup workflow with progress and saved-space reporting
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
    │   ├── cleanup.go
    │   └── process.go
    ├── app
    │   └── model.go
    ├── collect
    │   ├── connections_darwin.go
    │   ├── connections_stub.go
    │   └── sampler.go
    ├── diskscan
    │   ├── localbytes_darwin.go
    │   ├── localbytes_other.go
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
- `arrows`: move between dashboard panes, or move within lists
- `enter`: drill down, open process actions, or descend into a selected directory
- `left` / `backspace` / `ctrl+h`: go to the parent directory in the disk browser
- `home` / `end`: jump to top or bottom of the current list
- printable typing in CPU/memory views: filter the process list
- `f5`: refresh immediately, or rescan the current folder in the disk browser
- `f6`: open disk cleanup confirmation in the full disk browser
- `ctrl+c`: quit

## Notes

- The disk browser scanner avoids crossing device boundaries, skips symlinks, and treats Darwin `SF_DATALESS` cloud placeholders as zero local bytes.
- Root-level scans can still take time on large systems, but they run asynchronously, show progress, and cache completed paths.
- The disk cleanup workflow runs `brew cleanup -s`, `go clean -cache -modcache`, `docker system prune -f`, removes selected developer caches, and empties Trash, then reports free-space change.
- The app uses a 1-second heartbeat, refreshes data every 2 seconds while focused, and every 30 seconds while unfocused.
