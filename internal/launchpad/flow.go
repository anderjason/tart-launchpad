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
	Kind                  LaunchpadIntentKind
	VM                    VM
	Template              VM
	NameInput             string
	ImportSourcePath      string
	ExportDestinationPath string
	RunDuration           RunDuration
	FolderAccess          FolderAccess
	ProjectPath           string
	NetworkAccess         NetworkAccess
	Clipboard             bool
	GuestAudio            bool
	ExistingVMs           []VM
}

func (i LaunchpadIntent) BuildPlan(cfg Config, host HostEnvironment) (Plan, error) {
	switch i.Kind {
	case IntentRunExisting:
		if i.VM.Kind == VMKindTemplate {
			return Plan{}, fmt.Errorf("%w: %s is a template; run it read-only or create a workspace", ErrUsage, i.VM.Name)
		}
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
	projectPath, err := i.resolvedProjectPath(host)
	if err != nil {
		return Plan{}, err
	}
	return BuildRunPlan(cfg, RunOptions{
		VMName:           vmName,
		FolderAccess:     i.FolderAccess,
		ProjectPath:      projectPath,
		NetworkAccess:    i.NetworkAccess,
		Clipboard:        i.Clipboard,
		GuestAudio:       i.GuestAudio,
		TemplateReadOnly: templateReadOnly,
	})
}

func (i LaunchpadIntent) buildNewFromTemplatePlan(cfg Config, host HostEnvironment) (Plan, error) {
	projectPath, err := i.resolvedProjectPath(host)
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
		ProjectPath:   projectPath,
		NetworkAccess: i.NetworkAccess,
		Clipboard:     i.Clipboard,
		GuestAudio:    i.GuestAudio,
	})
}

func (i LaunchpadIntent) resolvedProjectPath(host HostEnvironment) (string, error) {
	if i.FolderAccess == FolderNoFolder {
		return "", nil
	}
	path := strings.TrimSpace(i.ProjectPath)
	if path == "" {
		var err error
		path, err = host.CurrentDirectory()
		if err != nil {
			return "", err
		}
	}
	return host.ResolveProjectDirectory(path)
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
