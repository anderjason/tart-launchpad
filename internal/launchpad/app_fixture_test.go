package launchpad

import (
	"path/filepath"
	"strings"
)

type screenFixture struct {
	cfg  Config
	vms  []VM
	host fakeHostEnvironment
}

func workspaceRunFixture() screenFixture {
	return screenFixture{
		cfg: DefaultConfig(),
		vms: []VM{{
			Name:  "dev",
			Kind:  VMKindWorkspace,
			State: "stopped",
		}},
		host: fakeHostEnvironment{
			currentDirectory: "/tmp/project",
		},
	}
}

func templateRunFixture() screenFixture {
	return screenFixture{
		cfg: DefaultConfig(),
		vms: []VM{{
			Name:  "base",
			Kind:  VMKindTemplate,
			State: "stopped",
		}},
		host: fakeHostEnvironment{
			currentDirectory: "/tmp/project",
		},
	}
}

func (f screenFixture) model() model {
	host := f.host
	if host.currentDirectory == "" {
		host.currentDirectory = "/tmp/project"
	}
	return newModelWithHost(f.cfg, "", f.vms, host)
}

type fakeHostEnvironment struct {
	currentDirectory string
	existingFiles    map[string]bool
}

func (f fakeHostEnvironment) CurrentDirectory() (string, error) {
	return f.currentDirectory, nil
}

func (f fakeHostEnvironment) ResolveProjectDirectory(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = f.currentDirectory
	}
	return filepath.Clean(path), nil
}

func (f fakeHostEnvironment) FileExists(path string) (bool, error) {
	return f.existingFiles[path], nil
}
