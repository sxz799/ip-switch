package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
)

func listAdapters() ([]NetworkAdapter, error) {
	switch runtime.GOOS {
	case "windows":
		return listWindowsAdapters()
	case "darwin":
		return listDarwinAdapters()
	case "linux":
		return listLinuxAdapters()
	default:
		return nil, fmt.Errorf("unsupported platform: %s", platformDisplayName())
	}
}

func getAdapterConfig(adapterName string) (NetworkConfig, error) {
	adapterName = strings.TrimSpace(adapterName)
	if adapterName == "" {
		return NetworkConfig{}, errors.New("adapter is required")
	}

	switch runtime.GOOS {
	case "windows":
		return getWindowsAdapterConfig(adapterName)
	case "darwin":
		return getDarwinAdapterConfig(adapterName)
	case "linux":
		return getLinuxAdapterConfig(adapterName)
	default:
		return NetworkConfig{}, fmt.Errorf("unsupported platform: %s", platformDisplayName())
	}
}

func applyConfig(config NetworkConfig) (string, error) {
	switch runtime.GOOS {
	case "windows":
		return applyWindowsConfig(config)
	case "darwin":
		return applyDarwinConfig(config)
	case "linux":
		return applyLinuxConfig(config)
	default:
		return "", fmt.Errorf("unsupported platform: %s", platformDisplayName())
	}
}

func listWindowsAdapters() ([]NetworkAdapter, error) {
	script := `
$items = Get-NetAdapter |
  Where-Object { $_.InterfaceDescription -and $_.Name -notmatch 'Loopback' } |
  Sort-Object -Property Status, Name |
  Select-Object @{Name='name';Expression={$_.Name}},
                @{Name='description';Expression={$_.InterfaceDescription}},
                @{Name='status';Expression={$_.Status}},
                @{Name='macAddress';Expression={$_.MacAddress}},
                @{Name='index';Expression={[int]$_.ifIndex}}
$items | ConvertTo-Json -Depth 3 -Compress
`

	var adapters []NetworkAdapter
	if err := runPowerShellList(script, &adapters); err != nil {
		return nil, err
	}

	sort.Slice(adapters, func(i, j int) bool {
		if adapters[i].Status == adapters[j].Status {
			return strings.ToLower(adapters[i].Name) < strings.ToLower(adapters[j].Name)
		}
		return adapters[i].Status == "Up"
	})

	return adapters, nil
}

func getWindowsAdapterConfig(adapterName string) (NetworkConfig, error) {
	script := fmt.Sprintf(`
$cfg = Get-NetIPConfiguration -InterfaceAlias '%s' -ErrorAction Stop
$ip = ''
$prefix = 0
if ($cfg.IPv4Address) {
  $ip = $cfg.IPv4Address[0].IPAddress
  $prefix = [int]$cfg.IPv4Address[0].PrefixLength
}
$gateway = ''
if ($cfg.IPv4DefaultGateway) { $gateway = $cfg.IPv4DefaultGateway.NextHop }
$dns = @()
if ($cfg.DNSServer.ServerAddresses) {
  $dns = $cfg.DNSServer.ServerAddresses | Where-Object { $_ -match '^\d{1,3}(\.\d{1,3}){3}$' }
}
[PSCustomObject]@{
  adapterName = '%s'
  mode = $(if ($cfg.NetIPv4Interface.Dhcp -eq 'Enabled') { 'dhcp' } else { 'static' })
  ipAddress = $ip
  prefixLength = $prefix
  gateway = $gateway
  primaryDns = $(if ($dns.Count -gt 0) { $dns[0] } else { '' })
  secondaryDns = $(if ($dns.Count -gt 1) { $dns[1] } else { '' })
  dnsServers = $dns
} | ConvertTo-Json -Depth 4 -Compress
`, escapePowerShellString(adapterName), escapePowerShellString(adapterName))

	var config NetworkConfig
	if err := runPowerShellObject(script, &config); err != nil {
		return NetworkConfig{}, err
	}
	if config.PrefixLength > 0 {
		config.SubnetMask = prefixLengthToSubnetMask(config.PrefixLength)
	}

	return config, nil
}

func applyWindowsConfig(config NetworkConfig) (string, error) {
	adapterName := config.AdapterName
	if config.Mode == "dhcp" {
		if err := runNetsh("interface", "ip", "set", "address", fmt.Sprintf(`name="%s"`, adapterName), "source=dhcp"); err != nil {
			return "", fmt.Errorf("failed to enable DHCP: %w", err)
		}
		if err := runNetsh("interface", "ip", "set", "dns", fmt.Sprintf(`name="%s"`, adapterName), "source=dhcp"); err != nil {
			return "", fmt.Errorf("failed to switch DNS to DHCP: %w", err)
		}
		return fmt.Sprintf("Adapter [%s] is now using DHCP.", adapterName), nil
	}

	if err := runNetsh(
		"interface", "ip", "set", "address",
		fmt.Sprintf(`name="%s"`, adapterName),
		"source=static",
		fmt.Sprintf("addr=%s", config.IPAddress),
		fmt.Sprintf("mask=%s", config.SubnetMask),
		fmt.Sprintf("gateway=%s", config.Gateway),
		"gwmetric=1",
	); err != nil {
		return "", fmt.Errorf("failed to set static IP: %w", err)
	}

	if err := runNetsh(
		"interface", "ip", "set", "dns",
		fmt.Sprintf(`name="%s"`, adapterName),
		"source=static",
		fmt.Sprintf("addr=%s", config.PrimaryDNS),
		"register=primary",
	); err != nil {
		return "", fmt.Errorf("failed to set primary DNS: %w", err)
	}

	if strings.TrimSpace(config.SecondaryDNS) != "" {
		if err := runNetsh(
			"interface", "ip", "add", "dns",
			fmt.Sprintf(`name="%s"`, adapterName),
			fmt.Sprintf("addr=%s", config.SecondaryDNS),
			"index=2",
		); err != nil {
			return "", fmt.Errorf("failed to set secondary DNS: %w", err)
		}
	}

	return fmt.Sprintf("Adapter [%s] is now using static IP %s.", adapterName, config.IPAddress), nil
}

func isAdmin() bool {
	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command(
			"powershell.exe",
			"-NoProfile",
			"-NonInteractive",
			"-Command",
			"([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)",
		)
		hideCommandWindow(cmd)
		out, err := cmd.Output()
		if err != nil {
			return false
		}
		return strings.EqualFold(strings.TrimSpace(string(out)), "True")
	default:
		return isUnixAdmin()
	}
}

func runNetsh(args ...string) error {
	_, err := runCommand("netsh", args...)
	return err
}

func runPowerShellList(script string, target any) error {
	output, err := runPowerShell(script)
	if err != nil {
		return err
	}

	trimmed := strings.TrimSpace(output)
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	if strings.HasPrefix(trimmed, "{") {
		trimmed = "[" + trimmed + "]"
	}

	return json.Unmarshal([]byte(trimmed), target)
}

func runPowerShellObject(script string, target any) error {
	output, err := runPowerShell(script)
	if err != nil {
		return err
	}

	trimmed := strings.TrimSpace(output)
	if trimmed == "" || trimmed == "null" {
		return nil
	}

	return json.Unmarshal([]byte(trimmed), target)
}

func runPowerShell(script string) (string, error) {
	prefixedScript := "[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false); " + script
	return runCommand("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", prefixedScript)
}

func escapePowerShellString(value string) string {
	return strings.ReplaceAll(value, `'`, `''`)
}
