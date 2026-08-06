package launchpad

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func checkPlanPrerequisites(plan Plan) error {
	if !planRequiresSoftnet(plan) {
		return nil
	}
	softnetPath, err := exec.LookPath("softnet")
	if err != nil {
		return fmt.Errorf("%w: Softnet is required for this network mode but was not found in PATH", ErrUsage)
	}
	if softnetReady(softnetPath) {
		return nil
	}
	return fmt.Errorf("%w: Softnet needs one-time admin setup before this account can use offline, internet, lan, or lan-and-internet modes.\n\nRun once from an admin shell:\n\n  softnet_path=\"$(realpath \"$(command -v softnet)\")\"\n  sudo chown root:wheel \"$softnet_path\"\n  sudo chmod 4755 \"$softnet_path\"\n\nThen rerun Tart Launchpad.", ErrUsage)
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
