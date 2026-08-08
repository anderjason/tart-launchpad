package launchpad

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestListHostVolumesFailsWhenEntryCannotBeInspected(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"bad", "good"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	_, err := listHostVolumes(root, volumeDiscovery{
		readDir: os.ReadDir,
		stat:    os.Stat,
		info: func(args ...string) ([]byte, error) {
			if filepath.Base(args[len(args)-1]) == "bad" {
				return nil, errors.New("unreadable volume")
			}
			return []byte(`{
				"DeviceIdentifier": "disk7s1",
				"MountPoint": "/Volumes/good",
				"VolumeName": "good",
				"Internal": false,
				"APFSVolumeRoles": []
			}`), nil
		},
	})
	if err == nil {
		t.Fatal("volume discovery succeeded; want inspection failure")
	}
}

func TestHostVolumeFromInfoJSONKeepsExternalMountedVolume(t *testing.T) {
	volume, ok, err := hostVolumeFromInfoJSON([]byte(`{
		"DeviceIdentifier": "disk7s1",
		"MountPoint": "/Volumes/Backup Drive",
		"VolumeName": "Backup Drive",
		"TotalSize": 1000204886016,
		"Internal": false,
		"APFSVolumeRoles": []
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if volume.Path != "/Volumes/Backup Drive" {
		t.Fatalf("path = %q, want /Volumes/Backup Drive", volume.Path)
	}
	if volume.Name != "Backup Drive" {
		t.Fatalf("name = %q, want Backup Drive", volume.Name)
	}
	if volume.Size != 1000204886016 {
		t.Fatalf("size = %d, want 1000204886016", volume.Size)
	}
}

func TestHostVolumeFromInfoJSONExcludesInternalVolume(t *testing.T) {
	_, ok, err := hostVolumeFromInfoJSON([]byte(`{
		"DeviceIdentifier": "disk3s1",
		"MountPoint": "/Volumes/Macintosh HD",
		"VolumeName": "Macintosh HD",
		"Internal": true,
		"APFSVolumeRoles": []
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("ok = true, want false")
	}
}

func TestHostVolumeFromInfoJSONExcludesSystemVolumeRole(t *testing.T) {
	_, ok, err := hostVolumeFromInfoJSON([]byte(`{
		"DeviceIdentifier": "disk3s5",
		"MountPoint": "/Volumes/System",
		"VolumeName": "System",
		"Internal": false,
		"APFSVolumeRoles": ["System"]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("ok = true, want false")
	}
}

func TestHostVolumeFromInfoJSONExcludesUnmountedVolume(t *testing.T) {
	_, ok, err := hostVolumeFromInfoJSON([]byte(`{
		"DeviceIdentifier": "disk7s1",
		"VolumeName": "Backup Drive",
		"Internal": false,
		"APFSVolumeRoles": []
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("ok = true, want false")
	}
}
