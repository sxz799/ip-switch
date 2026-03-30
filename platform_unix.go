package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func isUnixAdmin() bool {
	return os.Geteuid() == 0
}

func listDarwinAdapters() ([]NetworkAdapter, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	adapters := []NetworkAdapter{}
	for _, iface := range interfaces {
		if !isDarwinManagedInterface(iface.Name) {
			continue
		}

		status := "Unknown"
		ifconfigOutput, ifconfigErr := runCommand("ifconfig", iface.Name)
		if ifconfigErr == nil {
			lowerOutput := strings.ToLower(ifconfigOutput)
			switch {
			case strings.Contains(lowerOutput, "status: active"):
				status = "Up"
			case strings.Contains(lowerOutput, "status: inactive"):
				status = "Down"
			case strings.Contains(lowerOutput, "status: no carrier"):
				status = "Down"
			}
		}

		adapters = append(adapters, NetworkAdapter{
			Name:        iface.Name,
			Description: darwinInterfaceDescription(iface.Name),
			Status:      status,
			MacAddress:  iface.HardwareAddr.String(),
			Index:       len(adapters) + 1,
		})
	}

	sort.Slice(adapters, func(i, j int) bool {
		if adapters[i].Status == adapters[j].Status {
			return strings.ToLower(adapters[i].Name) < strings.ToLower(adapters[j].Name)
		}
		return adapters[i].Status == "Up"
	})

	return adapters, nil
}

func getDarwinAdapterConfig(adapterName string) (NetworkConfig, error) {
	ifconfigOutput, err := runCommand("ifconfig", adapterName)
	if err != nil {
		return NetworkConfig{}, err
	}

	ipAddress, subnetMask := parseDarwinIPv4(ifconfigOutput)
	dnsServers := darwinDNSServers()
	mode := "static"
	if _, err := runCommand("ipconfig", "getpacket", adapterName); err == nil {
		mode = "dhcp"
	}

	config := NetworkConfig{
		AdapterName:  adapterName,
		Mode:         mode,
		IPAddress:    ipAddress,
		SubnetMask:   subnetMask,
		Gateway:      darwinDefaultGateway(adapterName),
		PrimaryDNS:   firstIPv4(dnsServers),
		SecondaryDNS: "",
		DNSServers:   dnsServers,
	}
	if len(dnsServers) > 1 {
		config.SecondaryDNS = dnsServers[1]
	}
	if config.SubnetMask != "" {
		config.PrefixLength, _ = subnetMaskToPrefixLength(config.SubnetMask)
	}

	return config, nil
}

func applyDarwinConfig(config NetworkConfig) (string, error) {
	if !isAdmin() {
		return applyDarwinConfigWithPrompt(config)
	}

	serviceName, err := darwinServiceName(config.AdapterName)
	if err != nil {
		return "", err
	}

	adapterName := serviceName
	if config.Mode == "dhcp" {
		if _, err := runDarwinPrivilegedBatch([][]string{
			{"networksetup", "-setdhcp", adapterName},
			{"networksetup", "-setdnsservers", adapterName, "empty"},
		}); err != nil {
			return "", fmt.Errorf("failed to enable DHCP: %w", err)
		}
		return fmt.Sprintf("Adapter [%s] is now using DHCP.", adapterName), nil
	}

	commands := [][]string{
		{"networksetup", "-setmanual", adapterName, config.IPAddress, config.SubnetMask, config.Gateway},
	}

	dnsCommand := []string{"networksetup", "-setdnsservers", adapterName, config.PrimaryDNS}
	if strings.TrimSpace(config.SecondaryDNS) != "" {
		dnsCommand = append(dnsCommand, config.SecondaryDNS)
	}
	commands = append(commands, dnsCommand)

	if _, err := runDarwinPrivilegedBatch(commands); err != nil {
		return "", fmt.Errorf("failed to set static IP: %w", err)
	}

	return fmt.Sprintf("Adapter [%s] is now using static IP %s.", adapterName, config.IPAddress), nil
}

func applyDarwinConfigWithPrompt(config NetworkConfig) (string, error) {
	if config.Mode == "dhcp" {
		if _, err := runDarwinPrivilegedForAdapter(config.AdapterName, [][]string{
			{"networksetup", "-setdhcp", darwinServicePlaceholder},
			{"networksetup", "-setdnsservers", darwinServicePlaceholder, "empty"},
		}); err != nil {
			return "", fmt.Errorf("failed to enable DHCP: %w", err)
		}
		return fmt.Sprintf("Adapter [%s] is now using DHCP.", config.AdapterName), nil
	}

	commands := [][]string{
		{"networksetup", "-setmanual", darwinServicePlaceholder, config.IPAddress, config.SubnetMask, config.Gateway},
	}
	dnsCommand := []string{"networksetup", "-setdnsservers", darwinServicePlaceholder, config.PrimaryDNS}
	if strings.TrimSpace(config.SecondaryDNS) != "" {
		dnsCommand = append(dnsCommand, config.SecondaryDNS)
	}
	commands = append(commands, dnsCommand)

	if _, err := runDarwinPrivilegedForAdapter(config.AdapterName, commands); err != nil {
		return "", fmt.Errorf("failed to set static IP: %w", err)
	}

	return fmt.Sprintf("Adapter [%s] is now using static IP %s.", config.AdapterName, config.IPAddress), nil
}

func isDarwinManagedInterface(name string) bool {
	if name == "lo0" {
		return false
	}
	allowedPrefixes := []string{"en", "bridge"}
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func darwinInterfaceDescription(name string) string {
	switch {
	case name == "en0":
		return "Primary Network Interface"
	case strings.HasPrefix(name, "en"):
		return "Ethernet/Wi-Fi Interface"
	case strings.HasPrefix(name, "bridge"):
		return "Bridge Interface"
	default:
		return "Network Interface"
	}
}

func parseDarwinIPv4(ifconfigOutput string) (string, string) {
	ipPattern := regexp.MustCompile(`inet (\d+\.\d+\.\d+\.\d+) netmask 0x([0-9a-fA-F]+)`)
	matches := ipPattern.FindStringSubmatch(ifconfigOutput)
	if len(matches) != 3 {
		return "", ""
	}

	maskValue, err := strconv.ParseUint(matches[2], 16, 32)
	if err != nil {
		return matches[1], ""
	}

	mask := fmt.Sprintf("%d.%d.%d.%d",
		(maskValue>>24)&0xff,
		(maskValue>>16)&0xff,
		(maskValue>>8)&0xff,
		maskValue&0xff,
	)

	return matches[1], mask
}

func darwinDefaultGateway(adapterName string) string {
	output, err := runCommand("route", "-n", "get", "default")
	if err != nil {
		return ""
	}

	lines := strings.Split(output, "\n")
	gateway := ""
	iface := ""
	for _, line := range lines {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "gateway:"):
			gateway = strings.TrimSpace(strings.TrimPrefix(line, "gateway:"))
		case strings.HasPrefix(line, "interface:"):
			iface = strings.TrimSpace(strings.TrimPrefix(line, "interface:"))
		}
	}

	if iface != "" && iface != adapterName {
		return ""
	}
	return gateway
}

func darwinDNSServers() []string {
	output, err := runCommand("scutil", "--dns")
	if err != nil {
		return nil
	}

	var dnsServers []string
	seen := map[string]bool{}
	pattern := regexp.MustCompile(`nameserver\[[0-9]+\] : (\d+\.\d+\.\d+\.\d+)`)
	for _, line := range strings.Split(output, "\n") {
		match := pattern.FindStringSubmatch(strings.TrimSpace(line))
		if len(match) != 2 || seen[match[1]] {
			continue
		}
		seen[match[1]] = true
		dnsServers = append(dnsServers, match[1])
	}

	return dnsServers
}

func darwinServiceName(deviceName string) (string, error) {
	output, err := runCommand("/usr/sbin/networksetup", "-listnetworkserviceorder")
	if err != nil {
		return "", err
	}

	return parseDarwinServiceName(output, deviceName)
}

func parseDarwinServiceName(output, deviceName string) (string, error) {
	output = strings.ReplaceAll(output, "\r\n", "\n")
	output = strings.ReplaceAll(output, "\r", "\n")
	lines := strings.Split(output, "\n")
	for i := 0; i < len(lines)-1; i++ {
		serviceLine := strings.TrimSpace(lines[i])
		detailLine := strings.TrimSpace(lines[i+1])
		if !strings.HasPrefix(serviceLine, "(") || !strings.Contains(detailLine, "Device:") {
			continue
		}

		device := extractBetween(detailLine, "Device: ", ")")
		if device != deviceName {
			continue
		}

		closing := strings.Index(serviceLine, ")")
		if closing < 0 || closing+1 >= len(serviceLine) {
			continue
		}

		service := strings.TrimSpace(serviceLine[closing+1:])
		service = strings.TrimPrefix(service, "*")
		service = strings.TrimSpace(service)
		if service != "" {
			return service, nil
		}
	}

	return "", fmt.Errorf("no macOS network service found for adapter [%s]", deviceName)
}

const darwinServicePlaceholder = "__DARWIN_SERVICE__"

func runDarwinPrivilegedForAdapter(deviceName string, commands [][]string) (string, error) {
	scriptLines := []string{
		fmt.Sprintf("services=$(/usr/sbin/networksetup -listnetworkserviceorder | tr '\\r' '\\n')"),
		fmt.Sprintf("service=$(printf '%%s\\n' \"$services\" | awk -v dev=%s '", shellQuoteArg(deviceName)),
		"/^\\([0-9]+\\)/ { svc=$0; sub(/^\\([0-9]+\\)[[:space:]]*/, \"\", svc); sub(/^\\*[[:space:]]*/, \"\", svc); next }",
		"/Device:/ { if ($0 ~ \"Device: \" dev \"\\\\)\") { print svc; exit } }",
		"')",
		fmt.Sprintf("[ -n \"$service\" ] || { echo %s; exit 1; }", shellQuoteArg(fmt.Sprintf("no macOS network service found for adapter [%s]", deviceName))),
	}

	for _, command := range commands {
		if len(command) == 0 {
			continue
		}

		commandPath := command[0]
		if !strings.HasPrefix(commandPath, "/") {
			switch command[0] {
			case "networksetup":
				commandPath = "/usr/sbin/networksetup"
			default:
				return "", fmt.Errorf("unsupported privileged command: %s", command[0])
			}
		}

		parts := []string{shellQuoteArg(commandPath)}
		for _, arg := range command[1:] {
			if arg == darwinServicePlaceholder {
				parts = append(parts, `"$service"`)
				continue
			}
			parts = append(parts, shellQuoteArg(arg))
		}
		scriptLines = append(scriptLines, strings.Join(parts, " "))
	}

	script := fmt.Sprintf(`do shell script %q with administrator privileges`, strings.Join(scriptLines, "; "))
	return runCommand("/usr/bin/osascript", "-e", script)
}

func runDarwinPrivileged(name string, args ...string) (string, error) {
	if isAdmin() {
		return runCommand(name, args...)
	}

	commandPath := name
	if !strings.HasPrefix(commandPath, "/") {
		switch name {
		case "networksetup":
			commandPath = "/usr/sbin/networksetup"
		default:
			return "", fmt.Errorf("unsupported privileged command: %s", name)
		}
	}

	commandParts := []string{shellQuoteArg(commandPath)}
	for _, arg := range args {
		commandParts = append(commandParts, shellQuoteArg(arg))
	}
	commandLine := strings.Join(commandParts, " ")
	script := fmt.Sprintf(`do shell script %q with administrator privileges`, commandLine)

	return runCommand("/usr/bin/osascript", "-e", script)
}

func runDarwinPrivilegedBatch(commands [][]string) (string, error) {
	if len(commands) == 0 {
		return "", nil
	}
	if isAdmin() {
		var lastOutput string
		for _, command := range commands {
			if len(command) == 0 {
				continue
			}
			output, err := runCommand(command[0], command[1:]...)
			if err != nil {
				return "", err
			}
			lastOutput = output
		}
		return lastOutput, nil
	}

	commandLines := make([]string, 0, len(commands))
	for _, command := range commands {
		if len(command) == 0 {
			continue
		}

		commandPath := command[0]
		if !strings.HasPrefix(commandPath, "/") {
			switch command[0] {
			case "networksetup":
				commandPath = "/usr/sbin/networksetup"
			default:
				return "", fmt.Errorf("unsupported privileged command: %s", command[0])
			}
		}

		parts := []string{shellQuoteArg(commandPath)}
		for _, arg := range command[1:] {
			parts = append(parts, shellQuoteArg(arg))
		}
		commandLines = append(commandLines, strings.Join(parts, " "))
	}

	if len(commandLines) == 0 {
		return "", nil
	}

	script := fmt.Sprintf(`do shell script %q with administrator privileges`, strings.Join(commandLines, "; "))
	return runCommand("/usr/bin/osascript", "-e", script)
}

func listLinuxAdapters() ([]NetworkAdapter, error) {
	if !commandExists("nmcli") {
		return nil, errors.New("nmcli is required on Linux")
	}

	output, err := runCommand("nmcli", "-t", "-f", "DEVICE,TYPE,STATE,CONNECTION", "device", "status")
	if err != nil {
		return nil, err
	}

	var adapters []NetworkAdapter
	for index, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, ":")
		if len(parts) < 4 {
			continue
		}

		device := parts[0]
		if device == "" || device == "lo" {
			continue
		}

		macOutput, _ := runCommand("nmcli", "-g", "GENERAL.HWADDR", "device", "show", device)
		description := parts[1]
		if connectionName := parts[3]; connectionName != "" && connectionName != "--" {
			description = connectionName
		}

		adapters = append(adapters, NetworkAdapter{
			Name:        device,
			Description: description,
			Status:      normalizeStatus(parts[2]),
			MacAddress:  strings.TrimSpace(macOutput),
			Index:       index + 1,
		})
	}

	sort.Slice(adapters, func(i, j int) bool {
		if adapters[i].Status == adapters[j].Status {
			return strings.ToLower(adapters[i].Name) < strings.ToLower(adapters[j].Name)
		}
		return adapters[i].Status == "Up"
	})

	return adapters, nil
}

func getLinuxAdapterConfig(adapterName string) (NetworkConfig, error) {
	if !commandExists("nmcli") {
		return NetworkConfig{}, errors.New("nmcli is required on Linux")
	}

	connectionName, err := linuxConnectionName(adapterName)
	if err != nil {
		return NetworkConfig{}, err
	}

	deviceOutput, err := runCommand("nmcli", "-g", "IP4.ADDRESS,IP4.GATEWAY,IP4.DNS", "device", "show", adapterName)
	if err != nil {
		return NetworkConfig{}, err
	}

	methodOutput, err := runCommand("nmcli", "-g", "ipv4.method", "connection", "show", connectionName)
	if err != nil {
		return NetworkConfig{}, err
	}

	var ipAddress string
	var subnetMask string
	var prefixLength int
	var gateway string
	var dnsServers []string

	for _, line := range strings.Split(strings.TrimSpace(deviceOutput), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		switch {
		case strings.Contains(line, "/") && strings.Count(line, ".") >= 3:
			address, prefix, found := strings.Cut(line, "/")
			if found && isValidIPv4(address) {
				ipAddress = address
				prefixLength = parseInt(prefix)
				subnetMask = prefixLengthToSubnetMask(prefixLength)
			}
		case gateway == "" && isValidIPv4(line):
			gateway = line
		case isValidIPv4(line):
			dnsServers = append(dnsServers, line)
		}
	}

	config := NetworkConfig{
		AdapterName:  adapterName,
		Mode:         "static",
		IPAddress:    ipAddress,
		PrefixLength: prefixLength,
		SubnetMask:   subnetMask,
		Gateway:      gateway,
		DNSServers:   dnsServers,
		PrimaryDNS:   firstIPv4(dnsServers),
	}
	if len(dnsServers) > 1 {
		config.SecondaryDNS = dnsServers[1]
	}

	if strings.TrimSpace(methodOutput) == "auto" {
		config.Mode = "dhcp"
	}

	return config, nil
}

func applyLinuxConfig(config NetworkConfig) (string, error) {
	if !commandExists("nmcli") {
		return "", errors.New("nmcli is required on Linux")
	}

	connectionName, err := linuxConnectionName(config.AdapterName)
	if err != nil {
		return "", err
	}

	if config.Mode == "dhcp" {
		if _, err := runCommand("nmcli", "connection", "modify", connectionName, "ipv4.method", "auto", "ipv4.addresses", "", "ipv4.gateway", "", "ipv4.dns", ""); err != nil {
			return "", fmt.Errorf("failed to enable DHCP: %w", err)
		}
		if _, err := runCommand("nmcli", "connection", "up", connectionName); err != nil {
			return "", fmt.Errorf("failed to reload connection: %w", err)
		}
		return fmt.Sprintf("Adapter [%s] is now using DHCP.", config.AdapterName), nil
	}

	prefixLength, err := subnetMaskToPrefixLength(config.SubnetMask)
	if err != nil {
		return "", err
	}

	dnsValue := config.PrimaryDNS
	if strings.TrimSpace(config.SecondaryDNS) != "" {
		dnsValue += "," + config.SecondaryDNS
	}

	if _, err := runCommand(
		"nmcli", "connection", "modify", connectionName,
		"ipv4.method", "manual",
		"ipv4.addresses", fmt.Sprintf("%s/%d", config.IPAddress, prefixLength),
		"ipv4.gateway", config.Gateway,
		"ipv4.dns", dnsValue,
	); err != nil {
		return "", fmt.Errorf("failed to set static IP: %w", err)
	}

	if _, err := runCommand("nmcli", "connection", "up", connectionName); err != nil {
		return "", fmt.Errorf("failed to reload connection: %w", err)
	}

	return fmt.Sprintf("Adapter [%s] is now using static IP %s.", config.AdapterName, config.IPAddress), nil
}

func linuxConnectionName(adapterName string) (string, error) {
	statusOutput, err := runCommand("nmcli", "-t", "-f", "DEVICE,CONNECTION", "device", "status")
	if err != nil {
		return "", err
	}

	for _, line := range strings.Split(strings.TrimSpace(statusOutput), "\n") {
		parts := strings.Split(line, ":")
		if len(parts) < 2 || parts[0] != adapterName {
			continue
		}
		if parts[1] != "" && parts[1] != "--" {
			return parts[1], nil
		}
	}

	connectionOutput, err := runCommand("nmcli", "-t", "-f", "NAME,DEVICE", "connection", "show")
	if err != nil {
		return "", err
	}

	for _, line := range strings.Split(strings.TrimSpace(connectionOutput), "\n") {
		parts := strings.Split(line, ":")
		if len(parts) < 2 || parts[1] != adapterName {
			continue
		}
		if parts[0] != "" {
			return parts[0], nil
		}
	}

	return "", fmt.Errorf("no NetworkManager connection found for adapter [%s]", adapterName)
}
