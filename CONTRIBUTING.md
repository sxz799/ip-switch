# Contributing

Thanks for your interest in contributing.

## Development Setup

### Prerequisites

- Windows 10 or Windows 11
- Go 1.21+
- Node.js
- Wails CLI
- WebView2 Runtime

### Install Wails CLI

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

### Install dependencies

```powershell
go mod tidy
```

## Run in development mode

```powershell
wails dev
```

## Build

Preferred:

```powershell
wails build
```

Manual production build:

```powershell
go build -tags production -ldflags='-H windowsgui' -o IPSwitch.exe .
```

## Contribution Guidelines

- Keep changes focused and small when possible
- Preserve the current Windows-first scope unless the change explicitly expands platform support
- Avoid introducing heavy frontend frameworks unless there is a clear project decision to do so
- Keep UI labels and user-facing flows simple
- Validate network-related input carefully
- Do not break Wails production builds

## Code Style

- Go code should be formatted with `gofmt`
- Prefer clear, direct naming over abstraction for its own sake
- Keep comments short and useful
- Keep frontend code framework-free unless discussed first

## Testing Expectations

Before opening a pull request, please verify:

- The project builds successfully
- The app opens normally
- Adapter list can be loaded
- Static IP and DHCP flows do not regress
- Saved profiles can be loaded after restart

## Pull Requests

Please include:

- A short summary of the change
- Why the change is needed
- Screenshots if the UI changed
- Notes about any manual testing performed

## Issue Reports

If you report a bug, include:

- Windows version
- App version
- Reproduction steps
- Expected result
- Actual result
- Screenshot or log output if available
