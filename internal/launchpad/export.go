package launchpad

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

func newExportTemporaryPath(destination string) (string, error) {
	token := make([]byte, 12)
	if _, err := rand.Read(token); err != nil {
		return "", fmt.Errorf("create export temporary path: %w", err)
	}
	directory := filepath.Dir(destination)
	name := "." + filepath.Base(destination) + ".tart-launchpad-" + hex.EncodeToString(token) + ".tmp"
	return filepath.Join(directory, name), nil
}

func reserveExportTemporary(temporaryPath string, destinationPath string) error {
	if err := validateExportPaths(temporaryPath, destinationPath); err != nil {
		return err
	}
	if _, err := os.Lstat(destinationPath); err == nil {
		return fmt.Errorf("%w: export destination already exists: %s", ErrUsage, destinationPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("check export destination: %w", err)
	}

	temporary, err := os.OpenFile(temporaryPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("reserve export temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("close export temporary file: %w", err)
	}
	return nil
}

func installExportTemporary(temporaryPath string, destinationPath string) error {
	return installExportTemporaryWithLink(temporaryPath, destinationPath, os.Link)
}

func installExportTemporaryWithLink(temporaryPath string, destinationPath string, link func(string, string) error) error {
	if err := validateExportPaths(temporaryPath, destinationPath); err != nil {
		return err
	}
	if err := link(temporaryPath, destinationPath); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%w: export destination already exists: %s; completed archive preserved at %s", ErrUsage, destinationPath, temporaryPath)
		}
		if err := copyExportNoClobber(temporaryPath, destinationPath); err != nil {
			return fmt.Errorf("install export archive without hard links: %w; completed archive preserved at %s", err, temporaryPath)
		}
	}
	if err := os.Remove(temporaryPath); err != nil {
		return fmt.Errorf("remove installed export temporary file: %w", err)
	}
	directory, err := os.Open(filepath.Dir(destinationPath))
	if err != nil {
		return fmt.Errorf("open export directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil && !errors.Is(err, syscall.EINVAL) && !errors.Is(err, syscall.ENOTSUP) {
		return fmt.Errorf("sync export directory: %w", err)
	}
	return nil
}

func copyExportNoClobber(sourcePath string, destinationPath string) (returnErr error) {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%w: export destination already exists: %s", ErrUsage, destinationPath)
		}
		return err
	}
	createdInfo, err := destination.Stat()
	if err != nil {
		_ = destination.Close()
		return err
	}
	defer func() {
		if closeErr := destination.Close(); returnErr == nil && closeErr != nil {
			returnErr = closeErr
		}
		if returnErr != nil {
			_ = removeFileIfSame(destinationPath, createdInfo)
		}
	}()

	if _, err := io.Copy(destination, source); err != nil {
		return err
	}
	if err := destination.Sync(); err != nil {
		return err
	}
	return nil
}

func removeFileIfSame(path string, openInfo os.FileInfo) error {
	pathInfo, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if !os.SameFile(openInfo, pathInfo) {
		return nil
	}
	return os.Remove(path)
}

func discardExportTemporary(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove export temporary file: %w", err)
	}
	return nil
}

func validateExportPaths(temporaryPath string, destinationPath string) error {
	if temporaryPath == "" || destinationPath == "" {
		return fmt.Errorf("%w: export paths are required", ErrUsage)
	}
	if filepath.Clean(temporaryPath) == filepath.Clean(destinationPath) {
		return fmt.Errorf("%w: export temporary path must differ from destination", ErrUsage)
	}
	if filepath.Clean(filepath.Dir(temporaryPath)) != filepath.Clean(filepath.Dir(destinationPath)) {
		return fmt.Errorf("%w: export temporary file must be beside the destination", ErrUsage)
	}
	return nil
}
