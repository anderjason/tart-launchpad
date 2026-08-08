package launchpad

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type VolumeLister interface {
	ListHostVolumes() ([]HostVolume, error)
}

type RealVolumeLister struct{}

type volumeDiscovery struct {
	readDir func(string) ([]os.DirEntry, error)
	stat    func(string) (os.FileInfo, error)
	info    func(...string) ([]byte, error)
}

type diskutilVolumeInfoPayload struct {
	DeviceIdentifier string   `json:"DeviceIdentifier"`
	MountPoint       string   `json:"MountPoint"`
	VolumeName       string   `json:"VolumeName"`
	MediaName        string   `json:"MediaName"`
	TotalSize        uint64   `json:"TotalSize"`
	Internal         bool     `json:"Internal"`
	APFSVolumeRoles  []string `json:"APFSVolumeRoles"`
}

func (RealVolumeLister) ListHostVolumes() ([]HostVolume, error) {
	return listHostVolumes("/Volumes", volumeDiscovery{
		readDir: os.ReadDir,
		stat:    os.Stat,
		info:    diskutilPlistJSON,
	})
}

func listHostVolumes(root string, discovery volumeDiscovery) ([]HostVolume, error) {
	entries, err := discovery.readDir(root)
	if err != nil {
		return nil, fmt.Errorf("list mounted volumes: %w", err)
	}

	volumes := make([]HostVolume, 0, len(entries))
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		info, err := discovery.stat(path)
		if err != nil {
			return nil, fmt.Errorf("inspect mounted volume %q: %w", path, err)
		}
		if !info.IsDir() {
			continue
		}

		infoJSON, err := discovery.info("info", "-plist", path)
		if err != nil {
			return nil, fmt.Errorf("read mounted volume metadata %q: %w", path, err)
		}
		volume, ok, err := hostVolumeFromInfoJSON(infoJSON)
		if err != nil {
			return nil, fmt.Errorf("parse mounted volume metadata %q: %w", path, err)
		}
		if ok {
			volumes = append(volumes, volume)
		}
	}

	sort.Slice(volumes, func(i, j int) bool {
		return volumes[i].Path < volumes[j].Path
	})
	return volumes, nil
}

func diskutilPlistJSON(args ...string) ([]byte, error) {
	diskutil := exec.Command("diskutil", args...)
	plist, err := diskutil.Output()
	if err != nil {
		return nil, err
	}

	plutil := exec.Command("plutil", "-convert", "json", "-o", "-", "-")
	plutil.Stdin = bytes.NewReader(plist)
	out, err := plutil.Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}

func hostVolumeFromInfoJSON(data []byte) (HostVolume, bool, error) {
	var info diskutilVolumeInfoPayload
	if err := json.Unmarshal(data, &info); err != nil {
		return HostVolume{}, false, err
	}
	if info.Internal || hasSystemVolumeRole(info.APFSVolumeRoles) {
		return HostVolume{}, false, nil
	}

	path := strings.TrimSpace(info.MountPoint)
	if path == "" || !strings.HasPrefix(path, "/Volumes/") {
		return HostVolume{}, false, nil
	}
	id := strings.TrimSpace(info.DeviceIdentifier)
	if id == "" {
		return HostVolume{}, false, fmt.Errorf("missing device identifier")
	}
	name := firstNonEmpty(info.VolumeName, info.MediaName, filepath.Base(path), id)

	return HostVolume{
		ID:   id,
		Path: path,
		Name: name,
		Size: info.TotalSize,
	}, true, nil
}

func hasSystemVolumeRole(roles []string) bool {
	for _, role := range roles {
		switch strings.ToLower(strings.TrimSpace(role)) {
		case "system", "data", "preboot", "recovery", "vm", "update", "xart", "hardware":
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
