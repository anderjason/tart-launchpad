package launchpad

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	ProjectPath   string        `json:"project_path,omitempty"`
	NetworkAccess NetworkAccess `json:"network_access"`
	VolumePaths   []string      `json:"volume_paths,omitempty"`
	Clipboard     bool          `json:"clipboard,omitempty"`
	GuestAudio    bool          `json:"guest_audio,omitempty"`
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
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, path, nil
	}
	if err != nil {
		return Config{}, "", err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return Config{}, "", fmt.Errorf("read config: existing config is empty")
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, "", fmt.Errorf("read config: %w", err)
	}
	cfg.migratePreviouslyPersistedKinds()
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return Config{}, "", fmt.Errorf("read config: %w", err)
	}
	return cfg, path, nil
}

func SaveConfig(path string, cfg Config) error {
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	bytes, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	bytes = append(bytes, '\n')
	temporary, err := os.CreateTemp(directory, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure temporary config: %w", err)
	}
	if _, err := temporary.Write(bytes); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary config: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary config: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary config: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	removeTemporary = false
	directoryHandle, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("open config directory: %w", err)
	}
	defer directoryHandle.Close()
	if err := directoryHandle.Sync(); err != nil {
		return fmt.Errorf("sync config directory: %w", err)
	}
	return nil
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
	if c.Defaults.FolderAccess == "" {
		c.Defaults.FolderAccess = FolderNoFolder
	}
	if c.Defaults.NetworkAccess == "" {
		c.Defaults.NetworkAccess = NetworkOffline
	}
	for name, vm := range c.VMs {
		if vm.Kind == "" {
			vm.Kind = VMKindWorkspace
		}
		if vm.LastRun != nil {
			vm.LastRun.NetworkAccess = normalizeNetworkAccess(vm.LastRun.NetworkAccess)
			if vm.LastRun.FolderAccess == "" {
				vm.LastRun.FolderAccess = FolderNoFolder
			}
			if vm.LastRun.NetworkAccess == "" {
				vm.LastRun.NetworkAccess = NetworkOffline
			}
		}
		c.VMs[name] = vm
	}
}

func (c Config) Validate() error {
	if !c.Defaults.FolderAccess.Valid() {
		return fmt.Errorf("%w: invalid default folder access %q", ErrUsage, c.Defaults.FolderAccess)
	}
	if !c.Defaults.NetworkAccess.Valid() {
		return fmt.Errorf("%w: invalid default network access %q", ErrUsage, c.Defaults.NetworkAccess)
	}
	for name, vm := range c.VMs {
		if !vm.Kind.Valid() {
			return fmt.Errorf("%w: VM %q has invalid kind %q", ErrUsage, name, vm.Kind)
		}
		if vm.LastRun == nil {
			continue
		}
		if err := ValidateCanonicalTerms(vm.LastRun.FolderAccess, vm.LastRun.NetworkAccess); err != nil {
			return fmt.Errorf("VM %q last run: %w", name, err)
		}
		if vm.LastRun.FolderAccess != FolderNoFolder && strings.TrimSpace(vm.LastRun.ProjectPath) == "" {
			return fmt.Errorf("%w: VM %q last run grants folder access without a project path", ErrUsage, name)
		}
	}
	return nil
}

func (c *Config) migratePreviouslyPersistedKinds() {
	for name, vm := range c.VMs {
		if string(vm.Kind) == "unmarked" {
			vm.Kind = VMKindWorkspace
			c.VMs[name] = vm
		}
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
		return vm.Kind
	}
	return VMKindWorkspace
}

func (c *Config) SetKind(name string, kind VMKind) {
	c.Normalize()
	vm := c.VMs[name]
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
	c.Normalize()
	vm, ok := c.VMs[oldName]
	if !ok {
		return
	}
	delete(c.VMs, oldName)
	c.VMs[newName] = vm
	c.RemovePendingCleanup(oldName)
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
	if vm.Kind == "" {
		vm.Kind = VMKindWorkspace
	}
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
