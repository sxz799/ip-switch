package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

func privilegeMode() string {
	switch runtime.GOOS {
	case "darwin":
		if isAdmin() {
			return "admin"
		}
		return "prompt"
	default:
		if isAdmin() {
			return "admin"
		}
		return "none"
	}
}

func canApplyPrivilegedCommands() bool {
	return isAdmin() || privilegeMode() == "prompt"
}

func saveProfile(name string, config NetworkConfig) ([]SavedProfile, error) {
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

func deleteProfile(id string) ([]SavedProfile, error) {
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

func subnetMaskToPrefixLength(mask string) (int, error) {
	ip := net.ParseIP(strings.TrimSpace(mask))
	if ip == nil || ip.To4() == nil {
		return 0, errors.New("invalid subnet mask")
	}
	ones, bits := net.IPMask(ip.To4()).Size()
	if bits != 32 || ones < 0 {
		return 0, errors.New("invalid subnet mask")
	}
	return ones, nil
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

func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	hideCommandWindow(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func shellQuoteArg(value string) string {
	return "'" + strings.ReplaceAll(value, `'`, `'\''`) + "'"
}

func parseKeyValueLines(output string) map[string]string {
	values := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return values
}

func extractBetween(value, start, end string) string {
	startIndex := strings.Index(value, start)
	if startIndex < 0 {
		return ""
	}
	startIndex += len(start)

	endIndex := strings.Index(value[startIndex:], end)
	if endIndex < 0 {
		return strings.TrimSpace(value[startIndex:])
	}

	return strings.TrimSpace(value[startIndex : startIndex+endIndex])
}

func firstIPv4(values []string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if isValidIPv4(value) {
			return value
		}
	}
	return ""
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func normalizeStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "up", "connected", "active":
		return "Up"
	case "down", "disconnected", "inactive", "unavailable":
		return "Down"
	default:
		if status == "" {
			return "Unknown"
		}
		return strings.Title(status)
	}
}

func platformDisplayName() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows"
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	default:
		return runtime.GOOS
	}
}

func parseInt(value string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(value))
	return n
}
