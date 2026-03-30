# IPSwitch v0.1.0

## Overview

IPSwitch is a lightweight Windows desktop utility for quickly switching network adapters between static IP and DHCP.

This first release includes a graphical interface built with Wails v2 and Go, local profile persistence, current adapter configuration reading, and one-click network configuration updates.

## Included

- Adapter list loading
- Static IP and DHCP switching
- Current configuration reading
- Profile save, load, and delete
- Local JSON persistence
- Administrator check
- Operation logs
- Production Wails build

## Notes

- Windows only
- Run as Administrator when applying network settings
- If you build manually, use the `production` build tag or use `wails build`

## Download

Recommended executable:

- `build/bin/IPSwitch.exe`
