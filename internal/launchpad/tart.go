package launchpad

import (
	"encoding/json"
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
