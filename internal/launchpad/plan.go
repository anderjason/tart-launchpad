package launchpad

import (
	"fmt"
	"path/filepath"
	"strings"
)

const vmArchiveExtension = ".tvm"

const VMArchiveSensitiveStateWarning = "Exported VM files may contain secrets, credentials, browser sessions, source code, or other sensitive state."

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
	annotatedArgs, err := runArgsAnnotated(cfg, options.VMName, hostAccess, options.NetworkAccess, options.TemplateReadOnly)
	if err != nil {
		return Plan{}, err
	}
	args := annotatedArgValues(annotatedArgs)
	return Plan{
		Title: fmt.Sprintf("Run %s", options.VMName),
		Review: runPlanReview(PlanReview{
			FolderAccess:  hostAccess.FolderAccess,
			ProjectPath:   hostAccess.ProjectPath,
			NetworkAccess: options.NetworkAccess,
			Clipboard:     hostAccess.Clipboard,
			GuestAudio:    hostAccess.GuestAudio,
		}),
		Steps: []CommandStep{configureStep(options.VMName), {
			Kind:          CommandStepRun,
			Label:         "run",
			Args:          args,
			AnnotatedArgs: annotatedArgs,
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

func BuildExportPlan(options ExportOptions) (Plan, error) {
	vmName := strings.TrimSpace(options.VMName)
	if vmName == "" {
		return Plan{}, fmt.Errorf("%w: VM name is required", ErrUsage)
	}
	if options.VMKind != VMKindTemplate && options.VMKind != VMKindWorkspace {
		return Plan{}, fmt.Errorf("%w: export requires a template or workspace VM", ErrUsage)
	}
	destinationPath := strings.TrimSpace(options.DestinationPath)
	if destinationPath == "" {
		return Plan{}, fmt.Errorf("%w: export destination path is required", ErrUsage)
	}
	if options.DestinationExists {
		return Plan{}, fmt.Errorf("%w: export destination already exists: %s", ErrUsage, destinationPath)
	}
	temporaryPath, err := newExportTemporaryPath(destinationPath)
	if err != nil {
		return Plan{}, err
	}
	return Plan{
		Title:               fmt.Sprintf("Export %s", vmName),
		Review:              PlanReview{Verb: "export"},
		ExportPath:          destinationPath,
		ExportTemporaryPath: temporaryPath,
		Warnings:            []string{VMArchiveSensitiveStateWarning},
		Steps: []CommandStep{{
			Kind:  CommandStepExport,
			Label: "export",
			Args:  []string{"tart", "export", vmName, temporaryPath},
		}},
	}, nil
}

func BuildImportPlan(options ImportOptions) (Plan, error) {
	sourcePath := strings.TrimSpace(options.SourcePath)
	if sourcePath == "" {
		return Plan{}, fmt.Errorf("%w: import source path is required", ErrUsage)
	}
	if filepath.Ext(sourcePath) != vmArchiveExtension {
		return Plan{}, fmt.Errorf("%w: import source must be a .tvm file", ErrUsage)
	}
	destinationName := strings.TrimSpace(options.DestinationName)
	if destinationName == "" {
		return Plan{}, fmt.Errorf("%w: destination VM name is required", ErrUsage)
	}
	for _, vm := range options.ExistingVMs {
		if vm.Name == destinationName {
			return Plan{}, fmt.Errorf("%w: destination VM already exists: %s", ErrUsage, destinationName)
		}
	}
	return Plan{
		Title:        fmt.Sprintf("Import %s", destinationName),
		Review:       PlanReview{Verb: "import"},
		ImportPath:   sourcePath,
		ImportVMName: destinationName,
		Steps: []CommandStep{{
			Kind:  CommandStepImport,
			Label: "import",
			Args:  []string{"tart", "import", sourcePath, destinationName},
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
	if p.ExportPath != "" {
		return "export"
	}
	if p.ImportPath != "" {
		return "import"
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
	return p.RenameFrom == "" && p.DeleteVM == "" && p.ExportPath == "" && p.ImportPath == ""
}

func runArgs(cfg Config, vmName string, hostAccess HostAccessGrant, network NetworkAccess, templateReadOnly bool) ([]string, error) {
	annotatedArgs, err := runArgsAnnotated(cfg, vmName, hostAccess, network, templateReadOnly)
	if err != nil {
		return nil, err
	}
	return annotatedArgValues(annotatedArgs), nil
}

func runArgsAnnotated(cfg Config, vmName string, hostAccess HostAccessGrant, network NetworkAccess, templateReadOnly bool) ([]AnnotatedArg, error) {
	args := []AnnotatedArg{
		{Value: "tart"},
		{Value: "run"},
	}
	if !hostAccess.Clipboard {
		args = append(args, AnnotatedArg{Value: "--no-clipboard", Provenance: "clipboard off"})
	}
	if !hostAccess.GuestAudio {
		args = append(args, AnnotatedArg{Value: "--no-audio", Provenance: "guest audio off"})
	}

	folderArgs, err := hostAccess.FolderTartDirArgs()
	if err != nil {
		return nil, err
	}
	for _, arg := range folderArgs {
		args = append(args, AnnotatedArg{Value: arg, Provenance: string(hostAccess.FolderAccess)})
	}

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
		args = append(args, AnnotatedArg{Value: "--net-softnet-block=0.0.0.0/0", Provenance: string(NetworkOffline)})
	case NetworkInternet:
		args = append(args, AnnotatedArg{Value: "--net-softnet", Provenance: string(NetworkInternet)})
	case NetworkHost:
		args = append(args, AnnotatedArg{Value: "--net-host", Provenance: string(NetworkHost)})
	case NetworkLAN:
		if lanCIDRs == "" {
			return nil, fmt.Errorf("%w: lan requires at least one configured LAN CIDR", ErrUsage)
		}
		args = append(args,
			AnnotatedArg{Value: "--net-softnet-block=0.0.0.0/0", Provenance: string(NetworkLAN)},
			AnnotatedArg{Value: "--net-softnet-allow=" + lanCIDRs, Provenance: string(NetworkLAN)},
		)
	case NetworkLANAndInternet:
		if lanCIDRs == "" {
			return nil, fmt.Errorf("%w: lan-and-internet requires at least one configured LAN CIDR", ErrUsage)
		}
		args = append(args,
			AnnotatedArg{Value: "--net-softnet", Provenance: string(NetworkLANAndInternet)},
			AnnotatedArg{Value: "--net-softnet-allow=" + lanCIDRs, Provenance: string(NetworkLANAndInternet)},
		)
	default:
		return nil, fmt.Errorf("%w: invalid network access %q", ErrUsage, network)
	}

	if templateReadOnly {
		args = append(args, AnnotatedArg{Value: "--root-disk-opts=ro", Provenance: "template read-only"})
	}

	args = append(args, AnnotatedArg{Value: vmName})
	return args, nil
}

func annotatedArgValues(args []AnnotatedArg) []string {
	values := make([]string, 0, len(args))
	for _, arg := range args {
		values = append(values, arg.Value)
	}
	return values
}

func configureStep(vmName string) CommandStep {
	return CommandStep{
		Kind:  CommandStepConfigure,
		Label: "configure",
		Args: []string{
			"tart", "set", vmName,
			"--cpu", "4",
			"--memory", "8192",
			"--display", "1280x800",
		},
	}
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
