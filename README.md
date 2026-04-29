# Atavus Agent

Desktop agent for remote device management — pair your Windows/macOS/Linux computer with Atavus AI and let your AI assistants manage files, organize folders, and more.

## Quick Start

1. **Download** the binary for your OS from [atavus.ai/devices](https://atavus.ai/devices)
2. **Run the agent** (terminal or double-click)
3. **Generate a pairing code** on [atavus.ai/devices](https://atavus.ai/devices)
4. **Enter the code** in the agent: `atavus-agent pair --code=XXXXXX`

## Commands

| Command | Description |
|---------|-------------|
| `atavus-agent version` | Show version |
| `atavus-agent pair --code=XXXXXX` | Pair with Atavus account |
| `atavus-agent connect` | Connect to server and start listening |
| `atavus-agent status` | Check connection status |
| `atavus-agent disconnect` | Disconnect from server |
| `atavus-agent uninstall` | Remove pairing config and stop at startup |
| `atavus-agent startup` | Install as startup service |

## Features

- **Secure WebSocket connection** to Atavus backend with auto-reconnect
- **Sandboxed file operations** — only allowed paths are accessible
- **File actions**: list, read, write, move, copy, delete, search, create folders
- **System info**: hostname, OS, disk usage, CPU count
- **Recycle/trash integration**: deleted files go to trash, recoverable
- **Heartbeat monitoring**: 30-second keepalive
- **Autostart**: Windows Registry (HKCU Run) or macOS LaunchAgent

## Architecture

```
┌─────────────┐     WebSocket      ┌────────────┐
│  Atavus AI   │ ◄─────────────── │   Agent     │
│  Backend     │   (wss://)        │  (Go App)   │
└─────────────┘                    └─────┬──────┘
                                         │
                                   ┌─────▼──────┐
                                   │  Sandbox    │
                                   │  (Filesys) │
                                   └────────────┘
```

The agent is a **single Go binary** (~5 MB, zero dependencies). It connects to the Atavus backend via WebSocket, authenticates with a pairing token, and waits for commands. All file operations go through a sandbox that enforces allowed/blocked paths.

## Build from Source

```bash
# Requirements: Go 1.18+
git clone git@github.com:atavusai/atavus-agent.git
cd atavus-agent

# Build for current platform
CGO_ENABLED=0 go build -ldflags="-s -w -X main.platform=linux" -o atavus-agent .

# Cross-compile all platforms
bash build.sh
```

## Security

- **Sandbox**: Hard-coded allowed paths (user home directory) and blocked paths (`.ssh`, `.gnupg`, `.aws`, `.config`)
- **No inbound ports**: Agent only makes outbound WebSocket connections
- **Token auth**: Paired via 6-digit code, 5-minute expiry
- **Audit logging**: All operations logged on the backend

## License

MIT
