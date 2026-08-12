package launchpad

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"
)

func TestIsRunnableVMSource(t *testing.T) {
	if !isRunnableVMSource("local") {
		t.Fatal("local source should be runnable")
	}
	if !isRunnableVMSource("LOCAL") {
		t.Fatal("local source check should be case-insensitive")
	}
	if isRunnableVMSource("OCI") {
		t.Fatal("OCI cache entries should not appear as runnable VMs")
	}
}

type fakeTart struct {
	failAtLabel string
	calls       [][]string
	listVMs     []VM
}

func (f *fakeTart) ListVMs() ([]VM, error) {
	return append([]VM(nil), f.listVMs...), nil
}

func (f *fakeTart) RunStep(stdout io.Writer, stderr io.Writer, step CommandStep) error {
	f.calls = append(f.calls, step.Args)
	if commandLabel(step.Args) == f.failAtLabel {
		return errors.New("boom")
	}
	return nil
}

func commandLabel(args []string) string {
	if len(args) >= 3 && args[0] == "tart" && args[1] == "clone" {
		return "clone"
	}
	if len(args) >= 3 && args[0] == "tart" && args[1] == "delete" {
		return "delete temporary VM"
	}
	return "run"
}

func temporaryExecutionPlan() Plan {
	return Plan{
		Title:          "temporary",
		TemporaryVM:    "tmp-1",
		HasTemporaryVM: true,
		Steps: []CommandStep{{
			Kind:  CommandStepClone,
			Label: "clone",
			Args:  []string{"tart", "clone", "template", "tmp-1"},
		}, {
			Kind:  CommandStepRun,
			Label: "run",
			Args:  []string{"tart", "run", "tmp-1"},
		}, {
			Kind:  CommandStepDeleteTemporaryVM,
			Label: "delete temporary VM",
			Args:  []string{"tart", "delete", "tmp-1"},
		}},
	}
}

func mustLoadConfigFile(t *testing.T, path string) Config {
	t.Helper()
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return DefaultConfig()
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal(bytes, &cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Normalize()
	return cfg
}
