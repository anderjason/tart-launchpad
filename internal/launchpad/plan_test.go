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
	wantRun := []string{"tart", "run", "--no-clipboard", "--no-audio", "--dir=project:/tmp/project:ro", "--net-softnet", "dev"}
	if !reflect.DeepEqual(plan.Steps[0].Args, wantRun) {
		t.Fatalf("run args\n got %#v\nwant %#v", plan.Steps[0].Args, wantRun)
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
	if !reflect.DeepEqual(plan.Steps[0].Args, wantRun) {
		t.Fatalf("run args\n got %#v\nwant %#v", plan.Steps[0].Args, wantRun)
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
	if !reflect.DeepEqual(plan.Steps[0].Args, wantRun) {
		t.Fatalf("run args\n got %#v\nwant %#v", plan.Steps[0].Args, wantRun)
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
	if !reflect.DeepEqual(plan.Steps[0].Args, wantRun) {
		t.Fatalf("run args\n got %#v\nwant %#v", plan.Steps[0].Args, wantRun)
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
	if len(plan.Steps) != 3 {
		t.Fatalf("expected clone/run/delete, got %d steps", len(plan.Steps))
	}
	if plan.Steps[1].Kind != CommandStepRun {
		t.Fatalf("second step kind = %q, want %q", plan.Steps[1].Kind, CommandStepRun)
	}
}
