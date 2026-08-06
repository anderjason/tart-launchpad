package launchpad

import (
	"fmt"
	"strings"
)

type LaunchpadIntentKind string

const (
	IntentRunExisting         LaunchpadIntentKind = "run-existing"
	IntentRunTemplateReadOnly LaunchpadIntentKind = "run-template-read-only"
	IntentNewFromTemplate     LaunchpadIntentKind = "new-from-template"
	IntentRenameVM            LaunchpadIntentKind = "rename-vm"
	IntentDeleteVM            LaunchpadIntentKind = "delete-vm"
	IntentExportVM            LaunchpadIntentKind = "export-vm"
	IntentImportArchive       LaunchpadIntentKind = "import-archive"
)

type LaunchpadIntent struct {
	Kind             LaunchpadIntentKind
	VM               VM
	Template         VM
	NameInput             string
	ImportSourcePath      string
	ExportDestinationPath string
	RunDuration      RunDuration
	FolderAccess     FolderAccess
	NetworkAccess    NetworkAccess
	VolumePaths      []string
	ExistingVMs      []VM
}

func (i LaunchpadIntent) BuildPlan(cfg Config, host HostEnvironment) (Plan, error) {
	switch i.Kind {
	case IntentRunExisting:
		return i.buildRunPlan(cfg, host, i.VM.Name, false)
	case IntentRunTemplateReadOnly:
		return i.buildRunPlan(cfg, host, i.VM.Name, true)
	case IntentNewFromTemplate:
		return i.buildNewFromTemplatePlan(cfg, host)
	case IntentRenameVM:
		return BuildRenamePlan(i.VM.Name, i.NameInput)
	case IntentDeleteVM:
		return BuildDeletePlan(i.VM.Name)
	case IntentExportVM:
		return i.buildExportPlan(host)
	case IntentImportArchive:
		return i.buildImportPlan(host)
	default:
		return Plan{}, fmt.Errorf("%w: unknown launchpad intent %q", ErrUsage, i.Kind)
	}
}

func (i LaunchpadIntent) buildRunPlan(cfg Config, host HostEnvironment, vmName string, templateReadOnly bool) (Plan, error) {
	cwd, err := host.CurrentDirectory()
	if err != nil {
		return Plan{}, err
	}
	return BuildRunPlan(cfg, RunOptions{
		VMName:           vmName,
		FolderAccess:     i.FolderAccess,
		NetworkAccess:    i.NetworkAccess,
		CWD:              cwd,
		TemplateReadOnly: templateReadOnly,
		VolumePaths:      i.VolumePaths,
	})
}

func (i LaunchpadIntent) buildNewFromTemplatePlan(cfg Config, host HostEnvironment) (Plan, error) {
	cwd, err := host.CurrentDirectory()
	if err != nil {
		return Plan{}, err
	}
	templateName := i.Template.Name
	if templateName == "" {
		return Plan{}, fmt.Errorf("no template selected")
	}
	return BuildNewFromTemplatePlan(cfg, NewFromTemplateOptions{
		TemplateName:  templateName,
		NewName:       strings.TrimSpace(i.NameInput),
		RunDuration:   i.RunDuration,
		FolderAccess:  i.FolderAccess,
		NetworkAccess: i.NetworkAccess,
		CWD:           cwd,
		VolumePaths:   i.VolumePaths,
	})
}

func (i LaunchpadIntent) buildExportPlan(host HostEnvironment) (Plan, error) {
	destinationPath := strings.TrimSpace(i.ExportDestinationPath)
	exists, err := host.FileExists(destinationPath)
	if err != nil {
		return Plan{}, err
	}
	return BuildExportPlan(ExportOptions{
		VMName:            i.VM.Name,
		VMKind:            i.VM.Kind,
		DestinationPath:   destinationPath,
		DestinationExists: exists,
	})
}

func (i LaunchpadIntent) buildImportPlan(host HostEnvironment) (Plan, error) {
	return BuildImportPlan(ImportOptions{
		SourcePath:      i.ImportSourcePath,
		DestinationName: i.NameInput,
		ExistingVMs:     i.ExistingVMs,
	})
}
