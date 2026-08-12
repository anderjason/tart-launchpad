package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/anderjason/tart-launchpad/internal/launchpad"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(exitCode(err))
	}
}

func run() error {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-h", "--help", "help":
			fmt.Print(helpText())
			return nil
		case "plan":
			return runPlan(os.Args[2:])
		default:
			return fmt.Errorf("unknown command %q", os.Args[1])
		}
	}

	cfg, cfgPath, err := launchpad.LoadConfig()
	if err != nil {
		return err
	}

	app, err := launchpad.NewApp(cfg, cfgPath, launchpad.RealTart{})
	if err != nil {
		return err
	}

	return app.Run()
}

func runPlan(args []string) error {
	options, err := launchpad.ParsePlanArgs(args)
	if err != nil {
		return err
	}
	cfg, _, err := launchpad.LoadConfig()
	if err != nil {
		return err
	}
	if options.FolderAccess != launchpad.FolderNoFolder {
		options.ProjectPath, err = (launchpad.RealHostEnvironment{}).ResolveProjectDirectory(options.ProjectPath)
		if err != nil {
			return err
		}
	}
	plan, err := launchpad.BuildRunPlan(cfg, options)
	if err != nil {
		return err
	}
	for _, step := range plan.Steps {
		fmt.Println(launchpad.ShellQuote(step.Args))
	}
	return nil
}

func exitCode(err error) int {
	if errors.Is(err, launchpad.ErrUsage) {
		return 2
	}
	return 1
}

func helpText() string {
	return `Tart Launchpad

Usage:
  tart-launchpad
  tart-launchpad plan --vm <name> --folder-access <mode> --network-access <mode> [--project-folder <path>] [--clipboard] [--guest-audio] [--template-read-only]

VM kinds:         template, workspace
  folder access:  no-folder, read-folder, edit-folder
  network access: offline, internet, host, lan, lan-and-internet
`
}
