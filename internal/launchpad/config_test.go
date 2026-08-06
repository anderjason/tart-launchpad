package launchpad

import "testing"

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

	cfg.RenameVMKind("old", "new")

	if cfg.KindFor("old") != VMKindWorkspace {
		t.Fatalf("old kind = %q, want workspace", cfg.KindFor("old"))
	}
	if cfg.KindFor("new") != VMKindWorkspace {
		t.Fatalf("new kind = %q, want workspace", cfg.KindFor("new"))
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
