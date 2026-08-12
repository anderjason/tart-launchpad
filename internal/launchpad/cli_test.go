package launchpad

import "testing"

func TestParsePlanArgsUsesLeastAccessDefaults(t *testing.T) {
	options, err := ParsePlanArgs([]string{"--vm", "dev"})
	if err != nil {
		t.Fatal(err)
	}
	if options.FolderAccess != FolderNoFolder {
		t.Fatalf("folder default = %q, want %q", options.FolderAccess, FolderNoFolder)
	}
	if options.NetworkAccess != NetworkOffline {
		t.Fatalf("network default = %q, want %q", options.NetworkAccess, NetworkOffline)
	}
}

func TestParsePlanArgsAcceptsExplicitHostConnections(t *testing.T) {
	options, err := ParsePlanArgs([]string{
		"--vm", "dev",
		"--folder-access", "read-folder",
		"--project-folder", "/tmp/other-project",
		"--clipboard",
		"--guest-audio",
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.ProjectPath != "/tmp/other-project" {
		t.Fatalf("project path = %q, want explicit path", options.ProjectPath)
	}
	if !options.Clipboard || !options.GuestAudio {
		t.Fatalf("host connections = clipboard %t, guest audio %t; want both on", options.Clipboard, options.GuestAudio)
	}
}
