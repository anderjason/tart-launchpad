package launchpad

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanUsesSoftnet(t *testing.T) {
	plan := Plan{Steps: []CommandStep{{
		Args: []string{"tart", "run", "--net-softnet-block=0.0.0.0/0", "dev"},
	}}}
	if !planUsesSoftnet(plan) {
		t.Fatal("expected plan to use Softnet")
	}
}

func TestPlanUsesSoftnetIgnoresHostMode(t *testing.T) {
	plan := Plan{Steps: []CommandStep{{
		Args: []string{"tart", "run", "--net-host", "dev"},
	}}}
	if planUsesSoftnet(plan) {
		t.Fatal("host-only mode should not require Softnet")
	}
}

func TestHasFlagPrefix(t *testing.T) {
	if !hasFlagPrefix("--net-softnet-allow=192.168.1.0/24", "--net-softnet-allow=") {
		t.Fatal("expected prefix match")
	}
	if hasFlagPrefix("--net-softnet-allow", "--net-softnet-allow=") {
		t.Fatal("expected missing equals sign not to match")
	}
}

func TestResolveTrustedSoftnetPathRejectsExecutableOutsideHomebrew(t *testing.T) {
	path := filepath.Join(t.TempDir(), "softnet")
	if err := os.WriteFile(path, []byte("not really Softnet"), 0o555); err != nil {
		t.Fatal(err)
	}

	_, err := resolveTrustedSoftnetPath(path, []string{filepath.Join(t.TempDir(), "homebrew")})
	if err == nil || !strings.Contains(err.Error(), "outside a trusted Homebrew installation") {
		t.Fatalf("error = %v, want untrusted-installation rejection", err)
	}
}

func TestPlanPrerequisitesNeverSuggestElevatingPathSelectedSoftnet(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "softnet")
	if err := os.WriteFile(path, []byte("untrusted helper"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory)
	plan := Plan{Prerequisites: PlanPrerequisites{Softnet: true}}

	err := checkPlanPrerequisites(plan)
	if err == nil || !strings.Contains(err.Error(), "outside a trusted Homebrew installation") {
		t.Fatalf("error = %v, want untrusted-installation rejection", err)
	}
	if strings.Contains(err.Error(), "sudo chown") || strings.Contains(err.Error(), "sudo chmod") {
		t.Fatalf("error suggested elevating an untrusted helper: %v", err)
	}
}

func TestResolveTrustedSoftnetPathAcceptsProtectedHomebrewCellarHelper(t *testing.T) {
	prefix := filepath.Join(t.TempDir(), "homebrew")
	cellarBinary := filepath.Join(prefix, "Cellar", "softnet", "1.2.3", "bin", "softnet")
	binLink := filepath.Join(prefix, "bin", "softnet")
	if err := os.MkdirAll(filepath.Dir(cellarBinary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(binLink), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cellarBinary, []byte("trusted helper"), 0o555); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(cellarBinary, binLink); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for path := filepath.Dir(cellarBinary); pathAtOrWithin(path, prefix); path = filepath.Dir(path) {
			_ = os.Chmod(path, 0o755)
			if path == prefix {
				break
			}
		}
		_ = os.Chmod(filepath.Dir(binLink), 0o755)
	})
	if err := os.Chmod(filepath.Dir(binLink), 0o555); err != nil {
		t.Fatal(err)
	}
	for path := filepath.Dir(cellarBinary); pathAtOrWithin(path, prefix); path = filepath.Dir(path) {
		if err := os.Chmod(path, 0o555); err != nil {
			t.Fatal(err)
		}
		if path == prefix {
			break
		}
	}

	got, err := resolveTrustedSoftnetPath(binLink, []string{prefix})
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(cellarBinary)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("resolved path = %q, want %q", got, want)
	}
}

func TestResolveTrustedSoftnetPathRejectsWritableParent(t *testing.T) {
	prefix := filepath.Join(t.TempDir(), "homebrew")
	cellarBinary := filepath.Join(prefix, "Cellar", "softnet", "1.2.3", "bin", "softnet")
	if err := os.MkdirAll(filepath.Dir(cellarBinary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cellarBinary, []byte("replaceable helper"), 0o555); err != nil {
		t.Fatal(err)
	}

	_, err := resolveTrustedSoftnetPath(cellarBinary, []string{prefix})
	if err == nil || !strings.Contains(err.Error(), "writable by the current account") {
		t.Fatalf("error = %v, want writable-path rejection", err)
	}
}
