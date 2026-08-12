package launchpad

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type HostEnvironment interface {
	CurrentDirectory() (string, error)
	ResolveProjectDirectory(path string) (string, error)
}

type RealHostEnvironment struct{}

func (RealHostEnvironment) CurrentDirectory() (string, error) {
	return os.Getwd()
}

func (RealHostEnvironment) ResolveProjectDirectory(path string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if resolvedHome, resolveErr := filepath.EvalSymlinks(home); resolveErr == nil {
		home = resolvedHome
	}
	path = strings.TrimSpace(path)
	if path == "~" {
		path = home
	} else if strings.HasPrefix(path, "~/") {
		path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	if path == "" {
		return "", fmt.Errorf("%w: project folder is required", ErrUsage)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve project folder: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve project folder %q: %w", absolute, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect project folder %q: %w", resolved, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: project folder is not a directory: %s", ErrUsage, resolved)
	}
	if projectFolderIsForbidden(resolved, home) {
		return "", fmt.Errorf("%w: project folder is too broad or credential-bearing: %s", ErrUsage, resolved)
	}
	return filepath.Clean(resolved), nil
}

func projectFolderIsForbidden(path string, home string) bool {
	if strings.EqualFold(filepath.Clean(path), string(filepath.Separator)) ||
		strings.EqualFold(filepath.Clean(path), filepath.Clean(home)) ||
		pathAtOrWithin(home, path) {
		return true
	}
	forbidden := []string{
		filepath.Join(home, ".ssh"),
		filepath.Join(home, ".aws"),
		filepath.Join(home, ".gnupg"),
		filepath.Join(home, ".kube"),
		filepath.Join(home, ".docker"),
		filepath.Join(home, ".config", "gcloud"),
		filepath.Join(home, "Library", "Keychains"),
		filepath.Join(home, "Library", "Application Support", "1Password"),
	}
	for _, root := range forbidden {
		if pathAtOrWithin(path, root) || pathAtOrWithin(root, path) {
			return true
		}
	}
	return false
}

func pathAtOrWithin(path string, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	if strings.EqualFold(path, root) {
		return true
	}
	relative, err := filepath.Rel(strings.ToLower(root), strings.ToLower(path))
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
