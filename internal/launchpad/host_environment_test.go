package launchpad

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveProjectDirectoryResolvesSymlink(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.Mkdir(project, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "project-link")
	if err := os.Symlink(project, link); err != nil {
		t.Fatal(err)
	}

	got, err := (RealHostEnvironment{}).ResolveProjectDirectory(link)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("resolved path = %q, want %q", got, want)
	}
}

func TestResolveProjectDirectoryRejectsBroadAndCredentialRoots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ssh := filepath.Join(home, ".ssh")
	if err := os.Mkdir(ssh, 0o700); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{string(filepath.Separator), home, ssh} {
		if _, err := (RealHostEnvironment{}).ResolveProjectDirectory(path); err == nil {
			t.Fatalf("ResolveProjectDirectory(%q) succeeded; want rejection", path)
		}
	}
}
