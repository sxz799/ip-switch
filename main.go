package main

import (
	"context"
	"embed"
	"errors"
	"fmt"

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
	IsAdmin       bool           `json:"isAdmin"`
	PrivilegeMode string         `json:"privilegeMode"`
	Profiles      []SavedProfile `json:"profiles"`
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
		IsAdmin:       isAdmin(),
		PrivilegeMode: privilegeMode(),
		Profiles:      profiles,
	}, nil
}

func (a *App) ListAdapters() ([]NetworkAdapter, error) {
	return listAdapters()
}

func (a *App) GetAdapterConfig(adapterName string) (NetworkConfig, error) {
	if adapterName == "" {
		return NetworkConfig{}, errors.New("adapter is required")
	}

	return getAdapterConfig(adapterName)
}

func (a *App) SaveProfile(name string, config NetworkConfig) ([]SavedProfile, error) {
	return saveProfile(name, config)
}

func (a *App) DeleteProfile(id string) ([]SavedProfile, error) {
	return deleteProfile(id)
}

func (a *App) LoadProfiles() ([]SavedProfile, error) {
	return loadProfiles()
}

func (a *App) ApplyConfig(config NetworkConfig) (string, error) {
	if !canApplyPrivilegedCommands() {
		return "", fmt.Errorf("run this app as Administrator/root before applying network settings on %s", platformDisplayName())
	}
	if err := validateConfig(config, true); err != nil {
		return "", err
	}

	return applyConfig(config)
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
