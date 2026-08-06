package launchpad

import (
	"reflect"
	"testing"
)

func TestLaunchpadIntentBuildsRunPlanFromHostEnvironment(t *testing.T) {
	intent := LaunchpadIntent{
		Kind:          IntentRunExisting,
		VM:            VM{Name: "dev", Kind: VMKindWorkspace},
		FolderAccess:  FolderReadHere,
		NetworkAccess: NetworkHost,
		VolumePaths:   []string{"/Volumes/External SSD"},
	}
	host := fakeHostEnvironment{
		currentDirectory: "/tmp/project",
	}

	plan, err := intent.BuildPlan(DefaultConfig(), host)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"tart", "run", "--no-clipboard", "--no-audio", "--dir=project:/tmp/project:ro", "--net-host", "--dir=volume-external-ssd:/Volumes/External SSD", "dev"}
	if !reflect.DeepEqual(plan.Steps[1].Args, want) {
		t.Fatalf("run args\n got %#v\nwant %#v", plan.Steps[1].Args, want)
	}
}

func TestLaunchpadIntentBuildsImportPlanFromHostEnvironment(t *testing.T) {
	intent := LaunchpadIntent{
		Kind:             IntentImportArchive,
		ImportSourcePath: "/tmp/dev.tvm",
		NameInput:        "restored-dev",
		ExistingVMs:      []VM{{Name: "existing", Kind: VMKindWorkspace}},
	}
	host := fakeHostEnvironment{
		currentDirectory: "/tmp/project",
	}

	plan, err := intent.BuildPlan(DefaultConfig(), host)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"tart", "import", "/tmp/dev.tvm", "restored-dev"}
	if !reflect.DeepEqual(plan.Steps[0].Args, want) {
		t.Fatalf("import args\n got %#v\nwant %#v", plan.Steps[0].Args, want)
	}
}
