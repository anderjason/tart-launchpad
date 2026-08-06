package launchpad

import (
	"fmt"
	"os"
)

func ParsePlanArgs(args []string) (RunOptions, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return RunOptions{}, err
	}
	options := RunOptions{
		FolderAccess:  FolderNoFolder,
		NetworkAccess: NetworkOffline,
		CWD:           cwd,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		next := func() (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("%w: %s requires a value", ErrUsage, arg)
			}
			i++
			return args[i], nil
		}
		switch arg {
		case "--vm":
			value, err := next()
			if err != nil {
				return RunOptions{}, err
			}
			options.VMName = value
		case "--folder-access":
			value, err := next()
			if err != nil {
				return RunOptions{}, err
			}
			options.FolderAccess = FolderAccess(value)
		case "--network-access":
			value, err := next()
			if err != nil {
				return RunOptions{}, err
			}
			options.NetworkAccess = NetworkAccess(value)
		case "--cwd":
			value, err := next()
			if err != nil {
				return RunOptions{}, err
			}
			options.CWD = value
		case "--template-read-only":
			options.TemplateReadOnly = true
		case "--volume":
			value, err := next()
			if err != nil {
				return RunOptions{}, err
			}
			options.VolumePaths = append(options.VolumePaths, value)
		default:
			return RunOptions{}, fmt.Errorf("%w: unknown plan option %q", ErrUsage, arg)
		}
	}
	return options, nil
}
