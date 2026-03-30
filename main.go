package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend
var assets embed.FS

type App struct {
	ctx context.Context
}

type NetworkAdapter struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	MacAddress  string `json:"macAddress"`
	Index       int    `json:"index"`
}

type NetworkConfig struct {
	AdapterName  string   `json:"adapterName"`
	Mode         string   `json:"mode"`
	IPAddress    string   `json:"ipAddress"`
	PrefixLength int      `json:"prefixLength"`
	SubnetMask   string   `json:"subnetMask"`
	Gateway      string   `json:"gateway"`
	PrimaryDNS   string   `json:"primaryDns"`
	SecondaryDNS string   `json:"secondaryDns"`
	DNSServers   []string `json:"dnsServers"`
}

type SavedProfile struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Config    NetworkConfig `json:"config"`
	UpdatedAt string        `json:"updatedAt"`
}

type AppState struct {
	IsAdmin  bool           `json:"isAdmin"`
	Profiles []SavedProfile `json:"profiles"`
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) GetAppState() (AppState, error) {
	profiles, err := loadProfiles()
	if err != nil {
		return AppState{}, err
	}

	return AppState{
		IsAdmin:  isAdmin(),
		Profiles: profiles,
	}, nil
}

func (a *App) ListAdapters() ([]NetworkAdapter, error) {
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

func (a *App) GetAdapterConfig(adapterName string) (NetworkConfig, error) {
	adapterName = strings.TrimSpace(adapterName)
	if adapterName == "" {
		return NetworkConfig{}, errors.New("adapter is required")
	}

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

func (a *App) SaveProfile(name string, config NetworkConfig) ([]SavedProfile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("profile name is required")
	}
	if err := validateConfig(config, false); err != nil {
		return nil, err
	}

	profiles, err := loadProfiles()
	if err != nil {
		return nil, err
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	id := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	if id == "" {
		id = fmt.Sprintf("profile-%d", time.Now().Unix())
	}

	updated := false
	for i := range profiles {
		if profiles[i].ID == id || strings.EqualFold(profiles[i].Name, name) {
			profiles[i].Name = name
			profiles[i].Config = config
			profiles[i].UpdatedAt = now
			updated = true
			break
		}
	}

	if !updated {
		profiles = append(profiles, SavedProfile{
			ID:        id,
			Name:      name,
			Config:    config,
			UpdatedAt: now,
		})
	}

	if err := saveProfiles(profiles); err != nil {
		return nil, err
	}

	return profiles, nil
}

func (a *App) DeleteProfile(id string) ([]SavedProfile, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("profile id is required")
	}

	profiles, err := loadProfiles()
	if err != nil {
		return nil, err
	}

	filtered := profiles[:0]
	for _, profile := range profiles {
		if profile.ID != id {
			filtered = append(filtered, profile)
		}
	}

	if err := saveProfiles(filtered); err != nil {
		return nil, err
	}

	return filtered, nil
}

func (a *App) LoadProfiles() ([]SavedProfile, error) {
	return loadProfiles()
}

func (a *App) ApplyConfig(config NetworkConfig) (string, error) {
	if !isAdmin() {
		return "", errors.New("run this app as Administrator before applying network settings")
	}
	if err := validateConfig(config, true); err != nil {
		return "", err
	}

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

func validateConfig(config NetworkConfig, requireAdapter bool) error {
	config.AdapterName = strings.TrimSpace(config.AdapterName)
	config.Mode = strings.TrimSpace(strings.ToLower(config.Mode))

	if requireAdapter && config.AdapterName == "" {
		return errors.New("adapter is required")
	}
	if config.Mode != "dhcp" && config.Mode != "static" {
		return errors.New("mode must be static or dhcp")
	}
	if config.Mode == "dhcp" {
		return nil
	}

	if !isValidIPv4(config.IPAddress) {
		return errors.New("invalid IP address")
	}
	if !isValidSubnetMask(config.SubnetMask) {
		return errors.New("invalid subnet mask")
	}
	if !isValidIPv4(config.Gateway) {
		return errors.New("invalid gateway")
	}
	if !isValidIPv4(config.PrimaryDNS) {
		return errors.New("invalid primary DNS")
	}
	if strings.TrimSpace(config.SecondaryDNS) != "" && !isValidIPv4(config.SecondaryDNS) {
		return errors.New("invalid secondary DNS")
	}

	return nil
}

func isValidIPv4(value string) bool {
	ip := net.ParseIP(strings.TrimSpace(value))
	return ip != nil && ip.To4() != nil
}

func isValidSubnetMask(mask string) bool {
	ip := net.ParseIP(strings.TrimSpace(mask))
	if ip == nil || ip.To4() == nil {
		return false
	}

	octets := ip.To4()
	bits := ""
	for _, octet := range octets {
		bits += fmt.Sprintf("%08b", octet)
	}

	if !strings.Contains(bits, "10") {
		return bits != strings.Repeat("0", 32)
	}

	firstZero := strings.Index(bits, "0")
	return firstZero >= 0 && !strings.Contains(bits[firstZero:], "1")
}

func prefixLengthToSubnetMask(prefixLength int) string {
	if prefixLength < 0 || prefixLength > 32 {
		return ""
	}
	mask := net.CIDRMask(prefixLength, 32)
	if len(mask) != 4 {
		return ""
	}
	return net.IP(mask).String()
}

func isAdmin() bool {
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
}

func runNetsh(args ...string) error {
	cmd := exec.Command("netsh", args...)
	hideCommandWindow(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
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
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", prefixedScript)
	hideCommandWindow(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("powershell execution failed: %s", strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func hideCommandWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

func escapePowerShellString(value string) string {
	return strings.ReplaceAll(value, `'`, `''`)
}

func profilesFilePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	baseDir := filepath.Join(dir, "IPSwitch")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return "", err
	}

	return filepath.Join(baseDir, "profiles.json"), nil
}

func loadProfiles() ([]SavedProfile, error) {
	filePath, err := profilesFilePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(filePath)
	if errors.Is(err, os.ErrNotExist) {
		return []SavedProfile{}, nil
	}
	if err != nil {
		return nil, err
	}

	var profiles []SavedProfile
	if err := json.Unmarshal(data, &profiles); err != nil {
		return nil, err
	}

	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].UpdatedAt > profiles[j].UpdatedAt
	})

	return profiles, nil
}

func saveProfiles(profiles []SavedProfile) error {
	filePath, err := profilesFilePath()
	if err != nil {
		return err
	}

	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].UpdatedAt > profiles[j].UpdatedAt
	})

	data, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0o644)
}

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:     "IP Switch Assistant",
		Width:     1240,
		Height:    860,
		MinWidth:  1080,
		MinHeight: 760,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: app.startup,
		Bind: []interface{}{
			app,
		},
		BackgroundColour: &options.RGBA{
			R: 241,
			G: 245,
			B: 249,
			A: 1,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
