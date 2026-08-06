package launchpad

type screenFixture struct {
	cfg     Config
	vms     []VM
	volumes []HostVolume
	host    fakeHostEnvironment
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

func volumeSelectionFixture() screenFixture {
	fixture := workspaceRunFixture()
	fixture.volumes = []HostVolume{{
		ID:   "disk7s1",
		Path: "/Volumes/External SSD",
		Name: "External SSD",
		Size: 1000204886016,
	}, {
		ID:   "disk8s1",
		Path: "/Volumes/Backup",
		Name: "Backup",
		Size: 2000398934016,
	}}
	return fixture
}

func (f screenFixture) model() model {
	host := f.host
	if host.currentDirectory == "" {
		host.currentDirectory = "/tmp/project"
	}
	return newModelWithVolumesAndHost(f.cfg, "", f.vms, f.volumes, host)
}

type fakeHostEnvironment struct {
	currentDirectory string
	existingFiles    map[string]bool
}

func (f fakeHostEnvironment) CurrentDirectory() (string, error) {
	return f.currentDirectory, nil
}

func (f fakeHostEnvironment) FileExists(path string) (bool, error) {
	return f.existingFiles[path], nil
}
