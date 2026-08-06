package launchpad

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type Tart interface {
	ListVMs() ([]VM, error)
	RunStep(stdout io.Writer, stderr io.Writer, step CommandStep) error
}

type RealTart struct{}

type tartListItem struct {
	Name    string
	Source  string
	State   string
	Running bool
}

func (RealTart) ListVMs() ([]VM, error) {
	cmd := exec.Command("tart", "list", "--format", "json")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("tart list: %w", err)
	}
	var items []tartListItem
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, fmt.Errorf("parse tart list: %w", err)
	}
	vms := make([]VM, 0, len(items))
	for _, item := range items {
		if !isRunnableVMSource(item.Source) {
			continue
		}
		vms = append(vms, VM{
			Name:    item.Name,
			Source:  item.Source,
			State:   item.State,
			Running: item.Running,
		})
	}
	return vms, nil
}

func isRunnableVMSource(source string) bool {
	return strings.EqualFold(source, "local")
}

func (RealTart) RunStep(stdout io.Writer, stderr io.Writer, step CommandStep) error {
	return runCommand(stdout, stderr, step.Args)
}

func runCommand(stdout io.Writer, stderr io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("empty command")
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func MergeKinds(cfg Config, vms []VM) []VM {
	for i := range vms {
		vms[i].Kind = cfg.KindFor(vms[i].Name)
	}
	return vms
}

func ExecutePlan(stdout io.Writer, stderr io.Writer, tart Tart, cfg Config, cfgPath string, plan Plan) error {
	if err := checkPlanPrerequisites(plan); err != nil {
		return err
	}
	temporaryVMExists := false
	for _, step := range plan.Steps {
		fmt.Fprintf(stderr, "%s\n", ShellQuote(step.Args))
		if err := tart.RunStep(stdout, stderr, step); err != nil {
			commandErr := fmt.Errorf("%s: %w", step.Label, err)
			if plan.HasTemporaryVM && temporaryVMExists {
				cfg.AddPendingCleanup(plan.TemporaryVM)
				if saveErr := SaveConfig(cfgPath, cfg); saveErr != nil {
					return errors.Join(commandErr, fmt.Errorf("save pending cleanup: %w", saveErr))
				}
			}
			return commandErr
		}
		if plan.HasTemporaryVM && step.Kind == CommandStepClone {
			temporaryVMExists = true
		}
		if plan.HasTemporaryVM && step.Kind == CommandStepDeleteTemporaryVM {
			cfg.RemovePendingCleanup(plan.TemporaryVM)
			temporaryVMExists = false
			if err := SaveConfig(cfgPath, cfg); err != nil {
				return fmt.Errorf("save completed temporary cleanup: %w", err)
			}
		}
	}
	if plan.RenameFrom != "" && plan.RenameTo != "" {
		cfg.RenameVMKind(plan.RenameFrom, plan.RenameTo)
		if err := SaveConfig(cfgPath, cfg); err != nil {
			return fmt.Errorf("save renamed VM kind: %w", err)
		}
	}
	if plan.DeleteVM != "" {
		cfg.ForgetVM(plan.DeleteVM)
		if err := SaveConfig(cfgPath, cfg); err != nil {
			return fmt.Errorf("save deleted VM state: %w", err)
		}
	}
	return nil
}
