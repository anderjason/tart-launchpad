package launchpad

import (
	"reflect"
	"testing"
)

func TestBuildRunPlanReadHereInternet(t *testing.T) {
	cfg := DefaultConfig()
	plan, err := BuildRunPlan(cfg, RunOptions{
		VMName:        "dev",
		FolderAccess:  FolderReadFolder,
		NetworkAccess: NetworkInternet,
		ProjectPath:   "/tmp/project",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantConfigure := []string{"tart", "set", "dev", "--cpu", "4", "--memory", "8192", "--display", "1280x800"}
	if !reflect.DeepEqual(plan.Steps[0].Args, wantConfigure) {
		t.Fatalf("configure args\n got %#v\nwant %#v", plan.Steps[0].Args, wantConfigure)
	}
	wantRun := []string{"tart", "run", "--no-clipboard", "--no-audio", "--dir=project:/tmp/project:ro", "--net-softnet", "dev"}
	if !reflect.DeepEqual(plan.Steps[1].Args, wantRun) {
		t.Fatalf("run args\n got %#v\nwant %#v", plan.Steps[1].Args, wantRun)
	}
	if !plan.Prerequisites.Softnet {
		t.Fatal("Softnet prerequisite = false, want true")
	}
	if !plan.Review.ShowBoundaries || plan.Review.FolderAccess != FolderReadFolder || plan.Review.NetworkAccess != NetworkInternet {
		t.Fatalf("review = %#v, want run boundary review", plan.Review)
	}
}

func TestBuildRunPlanEditHereHost(t *testing.T) {
	cfg := DefaultConfig()
	plan, err := BuildRunPlan(cfg, RunOptions{
		VMName:        "dev",
		FolderAccess:  FolderEditFolder,
		NetworkAccess: NetworkHost,
		ProjectPath:   "/tmp/project",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantRun := []string{"tart", "run", "--no-clipboard", "--no-audio", "--dir=project:/tmp/project", "--net-host", "dev"}
	if !reflect.DeepEqual(plan.Steps[1].Args, wantRun) {
		t.Fatalf("run args\n got %#v\nwant %#v", plan.Steps[1].Args, wantRun)
	}
	if plan.Prerequisites.Softnet {
		t.Fatal("Softnet prerequisite = true, want false")
	}
}

func TestBuildRunPlanEnablesClipboardAndGuestAudioByOmittingDisableFlags(t *testing.T) {
	plan, err := BuildRunPlan(DefaultConfig(), RunOptions{
		VMName:        "dev",
		FolderAccess:  FolderReadFolder,
		ProjectPath:   "/tmp/project",
		NetworkAccess: NetworkHost,
		Clipboard:     true,
		GuestAudio:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantRun := []string{"tart", "run", "--dir=project:/tmp/project:ro", "--net-host", "dev"}
	if !reflect.DeepEqual(plan.Steps[1].Args, wantRun) {
		t.Fatalf("run args\n got %#v\nwant %#v", plan.Steps[1].Args, wantRun)
	}
	if !plan.Review.Clipboard || !plan.Review.GuestAudio || plan.Review.ProjectPath != "/tmp/project" {
		t.Fatalf("review = %#v, want enabled host connections and explicit project path", plan.Review)
	}
}

func TestBuildRunPlanRequiresProjectPathForFolderGrant(t *testing.T) {
	_, err := BuildRunPlan(DefaultConfig(), RunOptions{
		VMName:        "dev",
		FolderAccess:  FolderReadFolder,
		NetworkAccess: NetworkHost,
	})
	if err == nil {
		t.Fatal("expected missing project-folder error")
	}
}

func TestBuildRunPlanLANRequiresCIDR(t *testing.T) {
	cfg := DefaultConfig()
	_, err := BuildRunPlan(cfg, RunOptions{
		VMName:        "dev",
		FolderAccess:  FolderNoFolder,
		NetworkAccess: NetworkLAN,
		ProjectPath:   "/tmp/project",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildRunPlanLANAndInternet(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Network.LANCIDRs = []string{"192.168.1.0/24", "10.0.0.0/8"}
	plan, err := BuildRunPlan(cfg, RunOptions{
		VMName:        "dev",
		FolderAccess:  FolderNoFolder,
		NetworkAccess: NetworkLANAndInternet,
		ProjectPath:   "/tmp/project",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantRun := []string{"tart", "run", "--no-clipboard", "--no-audio", "--net-softnet", "--net-softnet-allow=192.168.1.0/24,10.0.0.0/8", "dev"}
	if !reflect.DeepEqual(plan.Steps[1].Args, wantRun) {
		t.Fatalf("run args\n got %#v\nwant %#v", plan.Steps[1].Args, wantRun)
	}
}

func TestBuildRunPlanRejectsNonPrivateConfiguredLANCIDR(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Network.LANCIDRs = []string{"0.0.0.0/0"}
	_, err := BuildRunPlan(cfg, RunOptions{
		VMName:        "dev",
		FolderAccess:  FolderNoFolder,
		NetworkAccess: NetworkLAN,
	})
	if err == nil {
		t.Fatal("internet-wide LAN CIDR succeeded; want rejection")
	}
}

func TestBuildRunPlanIgnoresUnusedInvalidLANCIDR(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Network.LANCIDRs = []string{"0.0.0.0/0"}
	_, err := BuildRunPlan(cfg, RunOptions{
		VMName:        "dev",
		FolderAccess:  FolderNoFolder,
		NetworkAccess: NetworkOffline,
	})
	if err != nil {
		t.Fatalf("offline plan failed because of unused LAN config: %v", err)
	}
}

func TestBuildRunPlanConfiguresResourcesBeforeRun(t *testing.T) {
	cfg := DefaultConfig()
	plan, err := BuildRunPlan(cfg, RunOptions{
		VMName:        "dev",
		FolderAccess:  FolderNoFolder,
		NetworkAccess: NetworkOffline,
		ProjectPath:   "/tmp/project",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Steps[0].Kind != CommandStepConfigure {
		t.Fatalf("first step kind = %q, want %q", plan.Steps[0].Kind, CommandStepConfigure)
	}
	want := []string{"tart", "set", "dev", "--cpu", "4", "--memory", "8192", "--display", "1280x800"}
	if !reflect.DeepEqual(plan.Steps[0].Args, want) {
		t.Fatalf("configure args\n got %#v\nwant %#v", plan.Steps[0].Args, want)
	}
}

func TestBuildRunPlanAddsSelectedVolumesReadWrite(t *testing.T) {
	cfg := DefaultConfig()
	plan, err := BuildRunPlan(cfg, RunOptions{
		VMName:        "dev",
		FolderAccess:  FolderNoFolder,
		NetworkAccess: NetworkOffline,
		ProjectPath:   "/tmp/project",
		VolumePaths:   []string{"/Volumes/External SSD", "/Volumes/Backup"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantRun := []string{"tart", "run", "--no-clipboard", "--no-audio", "--net-softnet-block=0.0.0.0/0", "--dir=volume-external-ssd:/Volumes/External SSD", "--dir=volume-backup:/Volumes/Backup", "dev"}
	if !reflect.DeepEqual(plan.Steps[1].Args, wantRun) {
		t.Fatalf("run args\n got %#v\nwant %#v", plan.Steps[1].Args, wantRun)
	}
	wantVolumes := []string{"/Volumes/External SSD", "/Volumes/Backup"}
	if !reflect.DeepEqual(plan.Review.VolumePaths, wantVolumes) {
		t.Fatalf("review volumes\n got %#v\nwant %#v", plan.Review.VolumePaths, wantVolumes)
	}
}

func TestHostAccessGrantBuildsFolderAndVolumeArgs(t *testing.T) {
	grant := HostAccessGrant{
		FolderAccess: FolderReadFolder,
		ProjectPath:  "/tmp/project",
		VolumePaths:  []string{"/Volumes/External SSD"},
	}

	got, err := grant.TartDirArgs()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--dir=project:/tmp/project:ro", "--dir=volume-external-ssd:/Volumes/External SSD"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dir args\n got %#v\nwant %#v", got, want)
	}
}

func TestBuildRunPlanRejectsBlankVolumePath(t *testing.T) {
	cfg := DefaultConfig()
	_, err := BuildRunPlan(cfg, RunOptions{
		VMName:        "dev",
		FolderAccess:  FolderNoFolder,
		NetworkAccess: NetworkOffline,
		ProjectPath:   "/tmp/project",
		VolumePaths:   []string{""},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestVolumeShareNameSanitizesPath(t *testing.T) {
	got := volumeShareName("/Volumes/My Backup!")
	if got != "volume-my-backup" {
		t.Fatalf("share name = %q, want volume-my-backup", got)
	}
}

func TestBuildRenamePlan(t *testing.T) {
	plan, err := BuildRenamePlan("old", "new")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"tart", "rename", "old", "new"}
	if !reflect.DeepEqual(plan.Steps[0].Args, want) {
		t.Fatalf("args\n got %#v\nwant %#v", plan.Steps[0].Args, want)
	}
	if plan.RenameFrom != "old" || plan.RenameTo != "new" {
		t.Fatalf("rename metadata = %#v", plan)
	}
}

func TestBuildRenamePlanRejectsSameName(t *testing.T) {
	_, err := BuildRenamePlan("same", "same")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildDeletePlan(t *testing.T) {
	plan, err := BuildDeletePlan("dev")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"tart", "delete", "dev"}
	if !reflect.DeepEqual(plan.Steps[0].Args, want) {
		t.Fatalf("args\n got %#v\nwant %#v", plan.Steps[0].Args, want)
	}
	if plan.DeleteVM != "dev" {
		t.Fatalf("delete metadata = %#v", plan)
	}
}

func TestBuildExportPlanForTemplate(t *testing.T) {
	destination := "/tmp/base.tvm"
	plan, err := BuildExportPlan(ExportOptions{
		VMName:          "base",
		VMKind:          VMKindTemplate,
		DestinationPath: destination,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"tart", "export", "base", destination}
	if !reflect.DeepEqual(plan.Steps[0].Args, want) {
		t.Fatalf("args\n got %#v\nwant %#v", plan.Steps[0].Args, want)
	}
	if plan.Steps[0].Kind != CommandStepExport {
		t.Fatalf("step kind = %q, want %q", plan.Steps[0].Kind, CommandStepExport)
	}
	if plan.ExportPath != destination {
		t.Fatalf("export path = %q, want %q", plan.ExportPath, destination)
	}
	if len(plan.Warnings) == 0 || plan.Warnings[0] != VMArchiveSensitiveStateWarning {
		t.Fatalf("warnings = %#v, want sensitive-state warning", plan.Warnings)
	}
}

func TestBuildExportPlanForWorkspace(t *testing.T) {
	destination := "/tmp/dev.tvm"
	plan, err := BuildExportPlan(ExportOptions{
		VMName:          "dev",
		VMKind:          VMKindWorkspace,
		DestinationPath: destination,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"tart", "export", "dev", destination}
	if !reflect.DeepEqual(plan.Steps[0].Args, want) {
		t.Fatalf("args\n got %#v\nwant %#v", plan.Steps[0].Args, want)
	}
}

func TestBuildExportPlanRejectsExistingDestination(t *testing.T) {
	_, err := BuildExportPlan(ExportOptions{
		VMName:            "dev",
		VMKind:            VMKindWorkspace,
		DestinationPath:   "/tmp/dev.tvm",
		DestinationExists: true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildExportPlanRejectsUnmarkedVM(t *testing.T) {
	_, err := BuildExportPlan(ExportOptions{
		VMName:          "dev",
		VMKind:          VMKindUnmarked,
		DestinationPath: "/tmp/dev.tvm",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildImportPlan(t *testing.T) {
	source := "/tmp/dev.tvm"
	plan, err := BuildImportPlan(ImportOptions{
		SourcePath:      source,
		DestinationName: "restored-dev",
		ExistingVMs: []VM{{
			Name: "dev",
			Kind: VMKindWorkspace,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"tart", "import", source, "restored-dev"}
	if !reflect.DeepEqual(plan.Steps[0].Args, want) {
		t.Fatalf("args\n got %#v\nwant %#v", plan.Steps[0].Args, want)
	}
	if plan.Steps[0].Kind != CommandStepImport {
		t.Fatalf("step kind = %q, want %q", plan.Steps[0].Kind, CommandStepImport)
	}
	if plan.ImportPath != source || plan.ImportVMName != "restored-dev" {
		t.Fatalf("import metadata = %#v", plan)
	}
}

func TestBuildImportPlanRejectsExistingVMName(t *testing.T) {
	_, err := BuildImportPlan(ImportOptions{
		SourcePath:      "/tmp/dev.tvm",
		DestinationName: "dev",
		ExistingVMs: []VM{{
			Name: "dev",
			Kind: VMKindWorkspace,
		}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildImportPlanRequiresTVMFile(t *testing.T) {
	_, err := BuildImportPlan(ImportOptions{
		SourcePath:      "/tmp/dev.zip",
		DestinationName: "dev",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildImportPlanAcceptsArchiveOutsideDesktop(t *testing.T) {
	plan, err := BuildImportPlan(ImportOptions{
		SourcePath:      "/tmp/downloads/dev.tvm",
		DestinationName: "dev",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Steps[0].Args[2]; got != "/tmp/downloads/dev.tvm" {
		t.Fatalf("import path = %q", got)
	}
}

func TestBuildTemporaryRunPlan(t *testing.T) {
	cfg := DefaultConfig()
	plan, err := BuildNewFromTemplatePlan(cfg, NewFromTemplateOptions{
		TemplateName:  "template",
		NewName:       "tmp-1",
		RunDuration:   RunDurationTemporaryRun,
		FolderAccess:  FolderNoFolder,
		NetworkAccess: NetworkOffline,
		ProjectPath:   "/tmp/project",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.HasTemporaryVM || plan.TemporaryVM != "tmp-1" {
		t.Fatalf("temporary metadata not set: %#v", plan)
	}
	if plan.CreatedVM != "tmp-1" {
		t.Fatalf("created VM = %q, want tmp-1", plan.CreatedVM)
	}
	if len(plan.Steps) != 4 {
		t.Fatalf("expected clone/configure/run/delete, got %d steps", len(plan.Steps))
	}
	if plan.Steps[1].Kind != CommandStepConfigure {
		t.Fatalf("second step kind = %q, want %q", plan.Steps[1].Kind, CommandStepConfigure)
	}
}
