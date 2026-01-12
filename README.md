# Port-Gate

**Local Development Domain Router** - Access your services by name instead of port numbers.

```
http://taskflow.local/    → localhost:8081
http://mcp-hub.local/     → localhost:8084
http://kekuli.local/      → localhost:3000
```

## Features

- **No more port numbers** - Access services by domain name
- **Auto-discovers services** - Reads from `~/.system-index/services/*.json`
- **Live reload** - Picks up new services automatically
- **Single binary** - Pure Go, no runtime dependencies
- **Zero config** - Works out of the box with system-index

## Installation

```bash
cd /Users/birddigital/sources/standalone-projects/port-gate
go build -o port-gate
ln -s $PWD/port-gate ~/.local/bin/
```

## Quick Start

```bash
# Run on port 80 (requires sudo)
sudo port-gate

# Or run on port 8080 (no sudo required)
port-gate --port 8080
```

Add the printed entries to `/etc/hosts`:

```bash
# Add these lines to /etc/hosts:
127.0.0.1  taskflow.local
127.0.0.1  mcp-hub.local
127.0.0.1 kekuli.local
```

Then access services:
```bash
open http://taskflow.local/dashboard
open http://mcp-hub.local/tools
```

## Configuration

Optional config file at `~/.system-index/port-gate.json`:

```json
{
  "port": 80,
  "domain_suffix": ".local",
  "services": {
    "taskflow": 8081,
    "mcp-hub": 8084,
    "kekuli": 3000
  },
  "headers": {
    "X-Forwarded-Proto": "http",
    "X-Real-IP": "127.0.0.1"
  }
}
```

If `services` is empty, it auto-imports from system-index service configs.

## System Index Integration

Port-Gate reads service definitions from:

```
~/.system-index/services/
├── taskflow.json      # {"id": "taskflow-web", "port": 8081, ...}
├── mcp-hub.json       # {"id": "mcp-hub", "port": 8084, ...}
└── ...
```

The service `id` becomes the domain name (with `-web` suffix removed).

## Running on Login

### Using launchd (macOS)

Create `~/Library/LaunchAgents/com.birddigital.port-gate.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.birddigital.port-gate</string>
    <key>ProgramArguments</key>
    <array>
        <string>/Users/birddigital/.local/bin/port-gate</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
```

Load it:
```bash
launchctl load ~/Library/LaunchAgents/com.birddigital.port-gate.plist
launchctl start com.birddigital.port-gate
```

### With sudo (for port 80)

To bind to port 80, the process needs root. Use `sudo` or configure authless sudo for this specific binary:

```bash
# sudoers entry:
birddigital ALL=(root) NOPASSWD: /Users/birddigital/.local/bin/port-gate
```

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Port-Gate (Port 80)                   │
├─────────────────────────────────────────────────────────┤
│                                                           │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐       │
│  │   DNS      │  │   Config   │  │  Watcher   │       │
│  │  Routing   │  │   Loader   │  │            │       │
│  └────────────┘  └────────────┘  └────────────┘       │
│                                                           │
│  request:83.2ms  GET /dashboard                          │
│       ↓                                                   │
│  taskflow.local → reverse_proxy → localhost:8081        │
│  mcp-hub.local → reverse_proxy → localhost:8084         │
│                                                           │
└─────────────────────────────────────────────────────────┘
```

## Development

```bash
# Run tests
go test ./...

# Build for both architectures
GOARCH=amd64 go build -o port-gate-amd64
GOARCH=arm64 go build -o port-gate-arm64

# Universal binary
lipo -create port-gate-amd64 port-gate-arm64 -output port-gate-universal
```

## License

MIT
