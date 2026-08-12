package launchpad

import (
	"fmt"
	"strings"
)

func BuildRunPlan(cfg Config, options RunOptions) (Plan, error) {
	if options.VMName == "" {
		return Plan{}, fmt.Errorf("%w: --vm is required", ErrUsage)
	}
	if err := ValidateCanonicalTerms(options.FolderAccess, options.NetworkAccess); err != nil {
		return Plan{}, err
	}
	if options.FolderAccess != FolderNoFolder && strings.TrimSpace(options.ProjectPath) == "" {
		return Plan{}, fmt.Errorf("%w: project folder is required for %s", ErrUsage, options.FolderAccess)
	}
	projectPath := options.ProjectPath
	if options.FolderAccess == FolderNoFolder {
		projectPath = ""
	}

	hostAccess := HostAccessGrant{
		FolderAccess: options.FolderAccess,
		ProjectPath:  projectPath,
		Clipboard:    options.Clipboard,
		GuestAudio:   options.GuestAudio,
	}
	args, err := runArgs(cfg, options.VMName, hostAccess, options.NetworkAccess, options.TemplateReadOnly)
	if err != nil {
		return Plan{}, err
	}
	return Plan{
		Title: fmt.Sprintf("Run %s", options.VMName),
		Review: runPlanReview(PlanReview{
			FolderAccess:         hostAccess.FolderAccess,
			ProjectPath:          hostAccess.ProjectPath,
			NetworkAccess:        options.NetworkAccess,
			Clipboard:            hostAccess.Clipboard,
			GuestAudio:           hostAccess.GuestAudio,
			TemplateDiskReadOnly: options.TemplateReadOnly,
		}),
		Steps: []CommandStep{{
			Kind:  CommandStepRun,
			Label: "run",
			Args:  args,
		}},
	}, nil
}

func BuildNewFromTemplatePlan(cfg Config, options NewFromTemplateOptions) (Plan, error) {
	if options.TemplateName == "" {
		return Plan{}, fmt.Errorf("%w: template is required", ErrUsage)
	}
	if options.NewName == "" {
		return Plan{}, fmt.Errorf("%w: name is required", ErrUsage)
	}
	if !options.RunDuration.Valid() {
		return Plan{}, fmt.Errorf("%w: invalid run duration %q", ErrUsage, options.RunDuration)
	}
	runPlan, err := BuildRunPlan(cfg, RunOptions{
		VMName:        options.NewName,
		FolderAccess:  options.FolderAccess,
		ProjectPath:   options.ProjectPath,
		NetworkAccess: options.NetworkAccess,
		Clipboard:     options.Clipboard,
		GuestAudio:    options.GuestAudio,
	})
	if err != nil {
		return Plan{}, err
	}

	steps := []CommandStep{{
		Kind:  CommandStepClone,
		Label: "clone",
		Args:  []string{"tart", "clone", options.TemplateName, options.NewName},
	}}
	steps = append(steps, runPlan.Steps...)

	plan := Plan{
		Title:     fmt.Sprintf("New %s from %s", options.RunDuration, options.TemplateName),
		Review:    runPlan.Review,
		Steps:     steps,
		CreatedVM: options.NewName,
	}
	if options.RunDuration == RunDurationTemporaryRun {
		plan.TemporaryVM = options.NewName
		plan.HasTemporaryVM = true
		plan.Steps = append(plan.Steps, CommandStep{
			Kind:  CommandStepDeleteTemporaryVM,
			Label: "delete temporary VM",
			Args:  []string{"tart", "delete", options.NewName},
		})
	}
	return plan, nil
}

func BuildRenamePlan(vmName string, newName string) (Plan, error) {
	if vmName == "" {
		return Plan{}, fmt.Errorf("%w: VM name is required", ErrUsage)
	}
	if strings.TrimSpace(newName) == "" {
		return Plan{}, fmt.Errorf("%w: new VM name is required", ErrUsage)
	}
	if strings.TrimSpace(newName) == vmName {
		return Plan{}, fmt.Errorf("%w: new VM name must differ from current name", ErrUsage)
	}
	return Plan{
		Title:      fmt.Sprintf("Rename %s", vmName),
		Review:     PlanReview{Verb: "apply"},
		RenameFrom: vmName,
		RenameTo:   strings.TrimSpace(newName),
		Steps: []CommandStep{{
			Kind:  CommandStepRename,
			Label: "rename",
			Args:  []string{"tart", "rename", vmName, strings.TrimSpace(newName)},
		}},
	}, nil
}

func BuildDeletePlan(vmName string) (Plan, error) {
	if vmName == "" {
		return Plan{}, fmt.Errorf("%w: VM name is required", ErrUsage)
	}
	return Plan{
		Title:    fmt.Sprintf("Delete %s", vmName),
		Review:   PlanReview{Verb: "apply"},
		DeleteVM: vmName,
		Steps: []CommandStep{{
			Kind:  CommandStepDelete,
			Label: "delete",
			Args:  []string{"tart", "delete", vmName},
		}},
	}, nil
}

func runPlanReview(review PlanReview) PlanReview {
	review.Verb = "run"
	review.ShowBoundaries = true
	return review
}

func (p Plan) ReviewVerb() string {
	if p.Review.Verb != "" {
		return p.Review.Verb
	}
	if p.ShowsBoundaries() {
		return "run"
	}
	return "apply"
}

func (p Plan) ShowsBoundaries() bool {
	if p.Review.ShowBoundaries {
		return true
	}
	return p.RenameFrom == "" && p.DeleteVM == ""
}

func runArgs(cfg Config, vmName string, hostAccess HostAccessGrant, network NetworkAccess, templateReadOnly bool) ([]string, error) {
	args := []string{"tart", "run"}
	if !hostAccess.Clipboard {
		args = append(args, "--no-clipboard")
	}
	if !hostAccess.GuestAudio {
		args = append(args, "--no-audio")
	}

	folderArgs, err := hostAccess.FolderTartDirArgs()
	if err != nil {
		return nil, err
	}
	args = append(args, folderArgs...)

	lanCIDRs := ""
	if network == NetworkLAN || network == NetworkLANAndInternet {
		configuredLANCIDRs, err := normalizePrivateIPv4CIDRs(cfg.Network.LANCIDRs)
		if err != nil {
			return nil, err
		}
		lanCIDRs = strings.Join(configuredLANCIDRs, ",")
	}
	switch network {
	case NetworkOffline:
		args = append(args, "--net-softnet-block=0.0.0.0/0")
	case NetworkInternet:
		args = append(args, "--net-softnet")
	case NetworkHost:
		args = append(args, "--net-host")
	case NetworkLAN:
		if lanCIDRs == "" {
			return nil, fmt.Errorf("%w: lan requires at least one configured LAN CIDR", ErrUsage)
		}
		args = append(args, "--net-softnet-block=0.0.0.0/0", "--net-softnet-allow="+lanCIDRs)
	case NetworkLANAndInternet:
		if lanCIDRs == "" {
			return nil, fmt.Errorf("%w: lan-and-internet requires at least one configured LAN CIDR", ErrUsage)
		}
		args = append(args, "--net-softnet", "--net-softnet-allow="+lanCIDRs)
	default:
		return nil, fmt.Errorf("%w: invalid network access %q", ErrUsage, network)
	}

	if templateReadOnly {
		args = append(args, "--root-disk-opts=ro")
	}

	args = append(args, vmName)
	return args, nil
}

func ShellQuote(args []string) string {
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		parts = append(parts, quoteArg(arg))
	}
	return strings.Join(parts, " ")
}

func quoteArg(arg string) string {
	if arg == "" {
		return "''"
	}
	if strings.IndexFunc(arg, func(r rune) bool {
		return !(r >= 'A' && r <= 'Z') &&
			!(r >= 'a' && r <= 'z') &&
			!(r >= '0' && r <= '9') &&
			!strings.ContainsRune("-_./:=,@%", r)
	}) == -1 {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}
