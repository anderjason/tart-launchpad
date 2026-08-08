package launchpad

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	if err := validateExportPaths(temporaryPath, destinationPath); err != nil {
		return err
	}
	if err := os.Link(temporaryPath, destinationPath); err != nil {
		installErr := fmt.Errorf("install export archive: %w", err)
		if errors.Is(err, os.ErrExist) {
			installErr = fmt.Errorf("%w: export destination already exists: %s", ErrUsage, destinationPath)
		}
		if cleanupErr := discardExportTemporary(temporaryPath); cleanupErr != nil {
			return errors.Join(installErr, cleanupErr)
		}
		return installErr
	}
	if err := os.Remove(temporaryPath); err != nil {
		return fmt.Errorf("remove installed export temporary file: %w", err)
	}
	directory, err := os.Open(filepath.Dir(destinationPath))
	if err != nil {
		return fmt.Errorf("open export directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync export directory: %w", err)
	}
	return nil
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
