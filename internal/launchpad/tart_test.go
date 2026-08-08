package launchpad

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
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

func TestExecuteTemporaryPlanCloneFailureDoesNotMarkPendingCleanup(t *testing.T) {
	cfg := DefaultConfig()
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	tart := &fakeTart{failAtLabel: "clone"}
	plan := temporaryExecutionPlan()

	err := ExecutePlan(io.Discard, io.Discard, tart, cfg, cfgPath, plan)
	if err == nil {
		t.Fatal("expected clone failure")
	}

	saved := mustLoadConfigFile(t, cfgPath)
	if len(saved.PendingCleanup) != 0 {
		t.Fatalf("pending cleanup = %#v, want none", saved.PendingCleanup)
	}
}

func TestExecuteTemporaryPlanRunFailureMarksPendingCleanup(t *testing.T) {
	cfg := DefaultConfig()
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	tart := &fakeTart{failAtLabel: "run"}
	plan := temporaryExecutionPlan()

	err := ExecutePlan(io.Discard, io.Discard, tart, cfg, cfgPath, plan)
	if err == nil {
		t.Fatal("expected run failure")
	}

	saved := mustLoadConfigFile(t, cfgPath)
	if len(saved.PendingCleanup) != 1 || saved.PendingCleanup[0] != "tmp-1" {
		t.Fatalf("pending cleanup = %#v, want tmp-1", saved.PendingCleanup)
	}
}

func TestExecuteTemporaryPlanDeleteFailureMarksPendingCleanup(t *testing.T) {
	cfg := DefaultConfig()
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	tart := &fakeTart{failAtLabel: "delete temporary VM"}
	plan := temporaryExecutionPlan()

	err := ExecutePlan(io.Discard, io.Discard, tart, cfg, cfgPath, plan)
	if err == nil {
		t.Fatal("expected delete failure")
	}

	saved := mustLoadConfigFile(t, cfgPath)
	if len(saved.PendingCleanup) != 1 || saved.PendingCleanup[0] != "tmp-1" {
		t.Fatalf("pending cleanup = %#v, want tmp-1", saved.PendingCleanup)
	}
}

func TestExecuteTemporaryPlanSuccessClearsPendingCleanup(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PendingCleanup = []string{"tmp-1"}
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	tart := &fakeTart{}
	plan := temporaryExecutionPlan()

	if err := ExecutePlan(io.Discard, io.Discard, tart, cfg, cfgPath, plan); err != nil {
		t.Fatal(err)
	}

	saved := mustLoadConfigFile(t, cfgPath)
	if len(saved.PendingCleanup) != 0 {
		t.Fatalf("pending cleanup = %#v, want none", saved.PendingCleanup)
	}
}

func TestExecuteTemporaryPlanUsesStepKindForLifecycle(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PendingCleanup = []string{"tmp-1"}
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	tart := &fakeTart{}
	plan := temporaryExecutionPlan()
	plan.Steps[0].Label = "copy"
	plan.Steps[2].Label = "remove"

	if err := ExecutePlan(io.Discard, io.Discard, tart, cfg, cfgPath, plan); err != nil {
		t.Fatal(err)
	}

	saved := mustLoadConfigFile(t, cfgPath)
	if len(saved.PendingCleanup) != 0 {
		t.Fatalf("pending cleanup = %#v, want none", saved.PendingCleanup)
	}
}

func TestExecuteRenameUpdatesKind(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SetKind("old", VMKindWorkspace)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	tart := &fakeTart{}
	plan, err := BuildRenamePlan("old", "new")
	if err != nil {
		t.Fatal(err)
	}

	if err := ExecutePlan(io.Discard, io.Discard, tart, cfg, cfgPath, plan); err != nil {
		t.Fatal(err)
	}

	saved := mustLoadConfigFile(t, cfgPath)
	if saved.KindFor("old") != VMKindWorkspace {
		t.Fatalf("old kind = %q, want workspace", saved.KindFor("old"))
	}
	if saved.KindFor("new") != VMKindWorkspace {
		t.Fatalf("new kind = %q, want workspace", saved.KindFor("new"))
	}
}

func TestExecuteDeleteClearsKindAndPendingCleanup(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SetKind("dev", VMKindWorkspace)
	cfg.PendingCleanup = []string{"dev"}
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	tart := &fakeTart{}
	plan, err := BuildDeletePlan("dev")
	if err != nil {
		t.Fatal(err)
	}

	if err := ExecutePlan(io.Discard, io.Discard, tart, cfg, cfgPath, plan); err != nil {
		t.Fatal(err)
	}

	saved := mustLoadConfigFile(t, cfgPath)
	if saved.KindFor("dev") != VMKindWorkspace {
		t.Fatalf("dev kind = %q, want workspace", saved.KindFor("dev"))
	}
	if len(saved.PendingCleanup) != 0 {
		t.Fatalf("pending cleanup = %#v, want none", saved.PendingCleanup)
	}
}

func TestExecutePlanReturnsSaveFailure(t *testing.T) {
	cfg := DefaultConfig()
	cfgPath := t.TempDir()
	tart := &fakeTart{failAtLabel: "run"}
	plan := temporaryExecutionPlan()

	err := ExecutePlan(io.Discard, io.Discard, tart, cfg, cfgPath, plan)
	if err == nil {
		t.Fatal("expected save failure")
	}
	if !strings.Contains(err.Error(), "save pending cleanup") {
		t.Fatalf("error = %q, want save pending cleanup context", err)
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
	if len(args) >= 3 && args[0] == "tart" && args[1] == "set" {
		return "configure"
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
			Kind:  CommandStepConfigure,
			Label: "configure",
			Args:  []string{"tart", "set", "tmp-1", "--cpu", "4"},
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
