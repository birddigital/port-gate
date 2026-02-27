# Port-Gate

**Local Development Domain Router** - Access your services by name instead of port numbers.

```
http://myapp.local/      → localhost:3000
http://api.local/        → localhost:8080
http://frontend.local/   → localhost:4200
```

## Features

- **No more port numbers** - Access services by domain name
- **Live reload** - Picks up config changes automatically
- **Single binary** - Pure Go, no runtime dependencies
- **Flexible config** - Works with a local `port-gate.json` or a configurable config directory
- **Optional service discovery** - Can auto-import service definitions from a directory (see below)

## Installation

```bash
git clone https://github.com/birddigital/port-gate.git
cd port-gate
go build -o port-gate

# Install to user binary directory
mkdir -p ~/.local/bin
cp port-gate ~/.local/bin/
# or system-wide
sudo cp port-gate /usr/local/bin/
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
127.0.0.1  myapp.local
127.0.0.1  api.local
127.0.0.1  frontend.local
```

Then access services:
```bash
open http://myapp.local/dashboard
open http://api.local/health
```

## Configuration

Port-Gate looks for configuration in the following order:

1. `./port-gate.json` (current directory)
2. `$PORT_GATE_CONFIG_DIR/port-gate.json` (if env var is set)
3. `~/.port-gate/port-gate.json` (default)

You can also specify the config directory with a flag:

```bash
port-gate --config-dir /path/to/config
```

Example `port-gate.json`:

```json
{
  "port": 80,
  "domain_suffix": ".local",
  "services": {
    "myapp": 3000,
    "api": 8080,
    "frontend": 4200
  },
  "headers": {
    "X-Forwarded-Proto": "http",
    "X-Real-IP": "127.0.0.1"
  }
}
```

See `port-gate.json.example` for a ready-to-use template.

## Optional: Service Auto-Discovery

Port-Gate can auto-import service definitions from JSON files in a `services/` subdirectory of the config directory. This feature is **disabled by default** and must be explicitly enabled:

```bash
port-gate --auto-import
```

When enabled, Port-Gate reads `*.json` files from `<config-dir>/services/` (e.g., `~/.port-gate/services/`). Each file should contain:

```json
{"id": "myapp", "port": 3000}
```

The service `id` (or `name` if `id` is empty) becomes the domain name (with `-web` suffix removed).

## Environment Variables

| Variable | Description |
|---|---|
| `PORT_GATE_CONFIG_DIR` | Override the configuration directory |

## Running on Login

### Using launchd (macOS)

Create `~/Library/LaunchAgents/local.port-gate.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>local.port-gate</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/port-gate</string>
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
launchctl load ~/Library/LaunchAgents/local.port-gate.plist
launchctl start local.port-gate
```

### With sudo (for port 80)

To bind to port 80, the process needs root. Use `sudo` or configure authless sudo for this specific binary:

```bash
# sudoers entry (replace 'youruser' with your actual username and adjust the path):
youruser ALL=(root) NOPASSWD: /usr/local/bin/port-gate
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
│  myapp.local    → reverse_proxy → localhost:3000        │
│  api.local      → reverse_proxy → localhost:8080        │
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
