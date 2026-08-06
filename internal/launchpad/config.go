package launchpad

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	VMs            map[string]VMConfig `json:"vms"`
	Network        NetworkConfig       `json:"network"`
	Defaults       DefaultsConfig      `json:"defaults"`
	PendingCleanup []string            `json:"pending_cleanup,omitempty"`
}

type VMConfig struct {
	Kind    VMKind         `json:"kind"`
	LastRun *LastRunConfig `json:"last_run,omitempty"`
}

type NetworkConfig struct {
	LANCIDRs []string `json:"lan_cidrs"`
}

type DefaultsConfig struct {
	FolderAccess  FolderAccess  `json:"folder_access"`
	NetworkAccess NetworkAccess `json:"network_access"`
	ReducedMotion bool          `json:"reduced_motion,omitempty"`
	ASCIIGlyphs   bool          `json:"ascii_glyphs,omitempty"`
}

type LastRunConfig struct {
	FolderAccess  FolderAccess  `json:"folder_access"`
	NetworkAccess NetworkAccess `json:"network_access"`
	VolumePaths   []string      `json:"volume_paths,omitempty"`
	At            string        `json:"at"`
}

func DefaultConfig() Config {
	return Config{
		VMs: map[string]VMConfig{},
		Defaults: DefaultsConfig{
			FolderAccess:  FolderNoFolder,
			NetworkAccess: NetworkOffline,
		},
	}
}

func LoadConfig() (Config, string, error) {
	path, err := ConfigPath()
	if err != nil {
		return Config{}, "", err
	}

	cfg := DefaultConfig()
	bytes, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, path, nil
	}
	if err != nil {
		return Config{}, "", err
	}
	if len(bytes) == 0 {
		return cfg, path, nil
	}
	if err := json.Unmarshal(bytes, &cfg); err != nil {
		return Config{}, "", fmt.Errorf("read config: %w", err)
	}
	cfg.Normalize()
	return cfg, path, nil
}

func SaveConfig(path string, cfg Config) error {
	cfg.Normalize()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	bytes, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	bytes = append(bytes, '\n')
	return os.WriteFile(path, bytes, 0o600)
}

func ConfigPath() (string, error) {
	if path := os.Getenv("TART_LAUNCHPAD_CONFIG"); path != "" {
		return path, nil
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "tart-launchpad", "config.json"), nil
}

func (c *Config) Normalize() {
	if c.VMs == nil {
		c.VMs = map[string]VMConfig{}
	}
	c.Defaults.NetworkAccess = normalizeNetworkAccess(c.Defaults.NetworkAccess)
	if !c.Defaults.FolderAccess.Valid() {
		c.Defaults.FolderAccess = FolderNoFolder
	}
	if !c.Defaults.NetworkAccess.Valid() {
		c.Defaults.NetworkAccess = NetworkOffline
	}
	for name, vm := range c.VMs {
		if !vm.Kind.Valid() {
			vm.Kind = VMKindUnmarked
		}
		if vm.LastRun != nil {
			vm.LastRun.NetworkAccess = normalizeNetworkAccess(vm.LastRun.NetworkAccess)
			if !vm.LastRun.FolderAccess.Valid() || !vm.LastRun.NetworkAccess.Valid() {
				vm.LastRun = nil
			}
		}
		c.VMs[name] = vm
	}
}

func normalizeNetworkAccess(network NetworkAccess) NetworkAccess {
	switch network {
	case "online":
		return NetworkInternet
	case "host-only":
		return NetworkHost
	case "lan-only":
		return NetworkLAN
	case "lan-online":
		return NetworkLANAndInternet
	default:
		return network
	}
}

func (c Config) KindFor(name string) VMKind {
	if vm, ok := c.VMs[name]; ok && vm.Kind.Valid() {
		if vm.Kind == VMKindUnmarked {
			return VMKindWorkspace
		}
		return vm.Kind
	}
	return VMKindWorkspace
}

func (c *Config) SetKind(name string, kind VMKind) {
	c.Normalize()
	vm := c.VMs[name]
	if kind == VMKindUnmarked {
		if vm.LastRun == nil {
			delete(c.VMs, name)
			return
		}
		vm.Kind = VMKindUnmarked
		c.VMs[name] = vm
		return
	}
	vm.Kind = kind
	c.VMs[name] = vm
}

func (c *Config) MarkNextKind(name string) VMKind {
	next := nextKind(c.KindFor(name))
	c.SetKind(name, next)
	return next
}

func nextKind(kind VMKind) VMKind {
	switch kind {
	case VMKindTemplate:
		return VMKindWorkspace
	default:
		return VMKindTemplate
	}
}

func (c *Config) RenameVMKind(oldName string, newName string) {
	kind := c.KindFor(oldName)
	c.SetKind(oldName, VMKindUnmarked)
	c.SetKind(newName, kind)
}

func (c *Config) ForgetVM(name string) {
	delete(c.VMs, name)
	c.RemovePendingCleanup(name)
}

func (c Config) LastRunFor(name string) (LastRunConfig, bool) {
	if vm, ok := c.VMs[name]; ok && vm.LastRun != nil {
		return *vm.LastRun, true
	}
	return LastRunConfig{}, false
}

func (c *Config) SetLastRun(name string, grant LastRunConfig) {
	c.Normalize()
	vm := c.VMs[name]
	vm.LastRun = &grant
	c.VMs[name] = vm
}

func (c *Config) AddLANCIDR(cidr string) bool {
	for _, existing := range c.Network.LANCIDRs {
		if existing == cidr {
			return false
		}
	}
	c.Network.LANCIDRs = append(c.Network.LANCIDRs, cidr)
	return true
}

func (c *Config) AddPendingCleanup(name string) {
	for _, existing := range c.PendingCleanup {
		if existing == name {
			return
		}
	}
	c.PendingCleanup = append(c.PendingCleanup, name)
}

func (c *Config) RemovePendingCleanup(name string) {
	next := c.PendingCleanup[:0]
	for _, existing := range c.PendingCleanup {
		if existing != name {
			next = append(next, existing)
		}
	}
	c.PendingCleanup = next
}
