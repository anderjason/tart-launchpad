package launchpad

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

var trustedHomebrewPrefixes = []string{"/opt/homebrew", "/usr/local"}

func checkPlanPrerequisites(plan Plan) error {
	if !planRequiresSoftnet(plan) {
		return nil
	}
	softnetPath, err := trustedSoftnetPath()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUsage, err)
	}
	if softnetReady(softnetPath) {
		return nil
	}
	return fmt.Errorf("%w: Softnet needs one-time admin setup before this account can use offline, internet, lan, or lan-and-internet modes.\n\nLaunchpad validated this installed helper:\n\n  %s\n\nRun once from an admin shell using that exact path:\n\n  sudo chown root:wheel %s\n  sudo chmod 4755 %s\n\nThen rerun Tart Launchpad.", ErrUsage, softnetPath, quoteArg(softnetPath), quoteArg(softnetPath))
}

func trustedSoftnetPath() (string, error) {
	path, err := exec.LookPath("softnet")
	if err != nil {
		return "", fmt.Errorf("Softnet is required for this network mode but was not found in PATH")
	}
	return resolveTrustedSoftnetPath(path, trustedHomebrewPrefixes)
}

func resolveTrustedSoftnetPath(path string, prefixes []string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve Softnet path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve Softnet path %q: %w", absolute, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect Softnet helper %q: %w", resolved, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("Softnet helper is not a regular file: %s", resolved)
	}

	for _, prefix := range prefixes {
		absolutePrefix, err := filepath.Abs(prefix)
		if err != nil {
			continue
		}
		resolvedPrefix, err := filepath.EvalSymlinks(absolutePrefix)
		if err != nil || !isHomebrewSoftnetPath(resolved, resolvedPrefix) {
			continue
		}
		if err := rejectWritablePathChain(filepath.Dir(absolute), absolutePrefix); err != nil {
			return "", err
		}
		if err := rejectWritablePathChain(resolved, resolvedPrefix); err != nil {
			return "", err
		}
		return filepath.Clean(resolved), nil
	}
	return "", fmt.Errorf("Softnet in PATH is outside a trusted Homebrew installation: %s", resolved)
}

func isHomebrewSoftnetPath(path string, prefix string) bool {
	relative, err := filepath.Rel(filepath.Clean(prefix), filepath.Clean(path))
	if err != nil {
		return false
	}
	parts := strings.Split(relative, string(filepath.Separator))
	return len(parts) == 5 &&
		parts[0] == "Cellar" &&
		parts[1] == "softnet" &&
		parts[2] != "" &&
		parts[3] == "bin" &&
		parts[4] == "softnet"
}

func rejectWritablePathChain(path string, prefix string) error {
	current := filepath.Clean(path)
	prefix = filepath.Clean(prefix)
	for {
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("inspect Softnet path %q: %w", current, err)
		}
		if writableByCurrentUser(info) {
			return fmt.Errorf("Softnet path is writable by the current account: %s", current)
		}
		if current == prefix {
			return nil
		}
		parent := filepath.Dir(current)
		if parent == current || !pathAtOrWithin(parent, prefix) {
			return fmt.Errorf("Softnet path escaped trusted Homebrew prefix: %s", path)
		}
		current = parent
	}
}

func writableByCurrentUser(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return true
	}
	permissions := info.Mode().Perm()
	if uint32(os.Geteuid()) == stat.Uid {
		return permissions&0o200 != 0
	}
	if uint32(os.Getegid()) == stat.Gid && permissions&0o020 != 0 {
		return true
	}
	groups, err := os.Getgroups()
	if err != nil {
		return true
	}
	for _, group := range groups {
		if uint32(group) == stat.Gid {
			return permissions&0o020 != 0
		}
	}
	return permissions&0o002 != 0
}

func planRequiresSoftnet(plan Plan) bool {
	return plan.Prerequisites.Softnet || planUsesSoftnet(plan)
}

func planUsesSoftnet(plan Plan) bool {
	for _, step := range plan.Steps {
		for _, arg := range step.Args {
			if arg == "--net-softnet" ||
				hasFlagPrefix(arg, "--net-softnet-allow=") ||
				hasFlagPrefix(arg, "--net-softnet-block=") ||
				hasFlagPrefix(arg, "--net-softnet-expose=") {
				return true
			}
		}
	}
	return false
}

func hasFlagPrefix(value string, prefix string) bool {
	return len(value) >= len(prefix) && value[:len(prefix)] == prefix
}

func softnetReady(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if info.Mode()&os.ModeSetuid == 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	return stat.Uid == 0
}
