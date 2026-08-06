package launchpad

import (
	"fmt"
	"strings"
)

func (g HostAccessGrant) TartDirArgs() ([]string, error) {
	folderArgs, err := g.FolderTartDirArgs()
	if err != nil {
		return nil, err
	}
	volumeArgs, err := g.VolumeTartDirArgs()
	if err != nil {
		return nil, err
	}
	return append(folderArgs, volumeArgs...), nil
}

func (g HostAccessGrant) FolderTartDirArgs() ([]string, error) {
	var args []string
	switch g.FolderAccess {
	case FolderNoFolder:
	case FolderReadHere:
		args = append(args, "--dir=project:"+g.ProjectPath+":ro")
	case FolderEditHere:
		args = append(args, "--dir=project:"+g.ProjectPath)
	default:
		return nil, fmt.Errorf("%w: invalid folder access %q", ErrUsage, g.FolderAccess)
	}
	return args, nil
}

func (g HostAccessGrant) VolumeTartDirArgs() ([]string, error) {
	var args []string
	shareNames := map[string]int{}
	for _, path := range g.VolumePaths {
		if strings.TrimSpace(path) == "" {
			return nil, fmt.Errorf("%w: volume path is required", ErrUsage)
		}
		args = append(args, "--dir="+uniqueVolumeShareName(path, shareNames)+":"+path)
	}
	return args, nil
}

func uniqueVolumeShareName(path string, used map[string]int) string {
	base := volumeShareName(path)
	used[base]++
	if used[base] == 1 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, used[base])
}

func volumeShareName(path string) string {
	base := strings.TrimSpace(path)
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	base = strings.ToLower(base)
	var b strings.Builder
	lastDash := false
	for _, r := range base {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if valid {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" {
		name = "volume"
	}
	return "volume-" + name
}
