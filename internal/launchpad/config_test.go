package launchpad

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfigUsesLeastAccessDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Defaults.FolderAccess != FolderNoFolder {
		t.Fatalf("folder default = %q, want %q", cfg.Defaults.FolderAccess, FolderNoFolder)
	}
	if cfg.Defaults.NetworkAccess != NetworkOffline {
		t.Fatalf("network default = %q, want %q", cfg.Defaults.NetworkAccess, NetworkOffline)
	}
}

func TestNormalizeUsesLeastAccessFallbacks(t *testing.T) {
	cfg := Config{}
	cfg.Normalize()
	if cfg.Defaults.FolderAccess != FolderNoFolder {
		t.Fatalf("folder default = %q, want %q", cfg.Defaults.FolderAccess, FolderNoFolder)
	}
	if cfg.Defaults.NetworkAccess != NetworkOffline {
		t.Fatalf("network default = %q, want %q", cfg.Defaults.NetworkAccess, NetworkOffline)
	}
}

func TestSaveConfigRejectsFolderReplayWithoutExplicitPath(t *testing.T) {
	cfg := DefaultConfig()
	cfg.VMs["dev"] = VMConfig{Kind: VMKindWorkspace, LastRun: &LastRunConfig{
		FolderAccess:  FolderReadFolder,
		NetworkAccess: NetworkOffline,
	}}

	if err := SaveConfig(filepath.Join(t.TempDir(), "config.json"), cfg); err == nil {
		t.Fatal("save succeeded with a folder replay that has no project path")
	}
}

func TestLoadConfigRejectsExistingEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TART_LAUNCHPAD_CONFIG", path)

	if _, _, err := LoadConfig(); err == nil {
		t.Fatal("empty existing config was accepted")
	}
}

func TestLoadConfigRejectsInvalidVMKind(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`{"vms":{"base":{"kind":"typo"}},"defaults":{"folder_access":"no-folder","network_access":"offline"}}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TART_LAUNCHPAD_CONFIG", path)

	if _, _, err := LoadConfig(); err == nil {
		t.Fatal("config with an invalid VM kind was accepted")
	}
}

func TestLoadConfigMigratesPreviouslyPersistedUncategorizedKind(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`{"vms":{"dev":{"kind":"unmarked"}},"defaults":{"folder_access":"no-folder","network_access":"offline"}}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TART_LAUNCHPAD_CONFIG", path)

	cfg, _, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.KindFor("dev"); got != VMKindWorkspace {
		t.Fatalf("migrated kind = %q, want workspace", got)
	}
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(saved), "unmarked") {
		t.Fatalf("legacy kind remained after save: %s", saved)
	}
}

func TestConfigMarkNextKindTogglesTemplateAndWorkspace(t *testing.T) {
	cfg := DefaultConfig()

	if got := cfg.MarkNextKind("dev"); got != VMKindTemplate {
		t.Fatalf("first kind = %q, want %q", got, VMKindTemplate)
	}
	if got := cfg.MarkNextKind("dev"); got != VMKindWorkspace {
		t.Fatalf("second kind = %q, want %q", got, VMKindWorkspace)
	}
	if got := cfg.MarkNextKind("dev"); got != VMKindTemplate {
		t.Fatalf("third kind = %q, want %q", got, VMKindTemplate)
	}
}

func TestConfigRenameVMKindMovesDurableKind(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SetKind("old", VMKindWorkspace)
	cfg.SetLastRun("old", LastRunConfig{FolderAccess: FolderReadFolder, ProjectPath: "/tmp/project", NetworkAccess: NetworkHost})
	cfg.PendingCleanup = []string{"old"}

	cfg.RenameVMKind("old", "new")

	if _, ok := cfg.VMs["old"]; ok {
		t.Fatal("old config entry still exists")
	}
	if cfg.KindFor("new") != VMKindWorkspace {
		t.Fatalf("new kind = %q, want workspace", cfg.KindFor("new"))
	}
	if _, ok := cfg.LastRunFor("new"); !ok {
		t.Fatal("new VM lost last-run settings")
	}
	if len(cfg.PendingCleanup) != 0 {
		t.Fatalf("pending cleanup = %#v, want none", cfg.PendingCleanup)
	}
}

func TestConfigForgetVMClearsKindAndPendingCleanup(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SetKind("dev", VMKindWorkspace)
	cfg.PendingCleanup = []string{"dev"}

	cfg.ForgetVM("dev")

	if cfg.KindFor("dev") != VMKindWorkspace {
		t.Fatalf("kind = %q, want workspace", cfg.KindFor("dev"))
	}
	if len(cfg.PendingCleanup) != 0 {
		t.Fatalf("pending cleanup = %#v, want none", cfg.PendingCleanup)
	}
}

func TestSaveConfigAtomicallyReplacesExistingFileWithPrivatePermissions(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "config.json")
	if err := os.WriteFile(path, []byte("old config\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	cfg.PendingCleanup = []string{"tmp-1"}
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}

	loaded := mustLoadConfigFile(t, path)
	if len(loaded.PendingCleanup) != 1 || loaded.PendingCleanup[0] != "tmp-1" {
		t.Fatalf("pending cleanup = %#v, want tmp-1", loaded.PendingCleanup)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %o, want 600", info.Mode().Perm())
	}
	matches, err := filepath.Glob(filepath.Join(directory, ".config-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary config files remain: %#v", matches)
	}
}
