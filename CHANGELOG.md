# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog, and versioning can follow Semantic Versioning if you decide to formalize releases later.

## [0.1.0] - 2026-03-30

### Added

- Initial Wails v2 desktop application structure
- Windows network adapter enumeration
- Static IP and DHCP switching
- Current adapter configuration reader
- Local profile save, load, and delete support
- JSON-based persistent profile storage
- Administrator privilege check
- Real-time operation log panel
- Responsive Chinese UI
- Wails production build support

### Fixed

- Hidden PowerShell and `netsh` subprocess windows during runtime
- UTF-8 handling for PowerShell output to reduce adapter-name garbling
- Subnet mask conversion using Go-side prefix-length parsing
