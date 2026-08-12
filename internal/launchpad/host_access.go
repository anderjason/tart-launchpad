package launchpad

import (
	"fmt"
)

func (g HostAccessGrant) TartDirArgs() ([]string, error) {
	return g.FolderTartDirArgs()
}

func (g HostAccessGrant) FolderTartDirArgs() ([]string, error) {
	var args []string
	switch g.FolderAccess {
	case FolderNoFolder:
	case FolderReadFolder:
		args = append(args, "--dir=project:"+g.ProjectPath+":ro")
	case FolderEditFolder:
		args = append(args, "--dir=project:"+g.ProjectPath)
	default:
		return nil, fmt.Errorf("%w: invalid folder access %q", ErrUsage, g.FolderAccess)
	}
	return args, nil
}
