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
	entries, err := os.ReadDir("/Volumes")
	if err != nil {
		return nil, fmt.Errorf("list mounted volumes: %w", err)
	}

	volumes := make([]HostVolume, 0, len(entries))
	for _, entry := range entries {
		path := filepath.Join("/Volumes", entry.Name())
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("inspect mounted volume %s: %w", path, err)
		}
		if !info.IsDir() {
			continue
		}

		infoJSON, err := diskutilPlistJSON("info", "-plist", path)
		if err != nil {
			return nil, fmt.Errorf("inspect mounted volume %s: %w", path, err)
		}
		volume, ok, err := hostVolumeFromInfoJSON(infoJSON)
		if err != nil {
			return nil, fmt.Errorf("parse mounted volume %s: %w", path, err)
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
