package launchpad

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestInstallExportFallsBackWhenHardLinksAreUnsupported(t *testing.T) {
	for _, linkError := range []error{syscall.EPERM, syscall.ENOTSUP} {
		t.Run(linkError.Error(), func(t *testing.T) {
			directory := t.TempDir()
			temporary := filepath.Join(directory, ".dev.tvm.tmp")
			destination := filepath.Join(directory, "dev.tvm")
			if err := os.WriteFile(temporary, []byte("archive"), 0o600); err != nil {
				t.Fatal(err)
			}
			unsupportedLink := func(string, string) error {
				return &os.LinkError{Op: "link", Old: temporary, New: destination, Err: linkError}
			}

			if err := installExportTemporaryWithLink(temporary, destination, unsupportedLink); err != nil {
				t.Fatal(err)
			}
			contents, err := os.ReadFile(destination)
			if err != nil {
				t.Fatal(err)
			}
			if string(contents) != "archive" {
				t.Fatalf("destination contents = %q, want archive", contents)
			}
			if _, err := os.Stat(temporary); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("temporary export remains after fallback: %v", err)
			}
		})
	}
}

func TestInstallExportFallbackDoesNotReplaceDestination(t *testing.T) {
	directory := t.TempDir()
	temporary := filepath.Join(directory, ".dev.tvm.tmp")
	destination := filepath.Join(directory, "dev.tvm")
	if err := os.WriteFile(temporary, []byte("new archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	unsupportedLink := func(string, string) error {
		return &os.LinkError{Op: "link", Old: temporary, New: destination, Err: syscall.ENOTSUP}
	}

	err := installExportTemporaryWithLink(temporary, destination, unsupportedLink)
	if err == nil || !errors.Is(err, ErrUsage) {
		t.Fatalf("error = %v, want destination collision", err)
	}
	contents, readErr := os.ReadFile(destination)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(contents) != "keep me" {
		t.Fatalf("destination contents = %q, want original", contents)
	}
	if _, statErr := os.Stat(temporary); statErr != nil {
		t.Fatalf("completed temporary export was not preserved: %v", statErr)
	}
}
