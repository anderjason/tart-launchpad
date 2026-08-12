package launchpad

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestFolderChoiceAdvancesToNetwork(t *testing.T) {
	m := workspaceRunFixture().model()
	m.selectedVM = m.vms[0]
	m.flow = flowRunExisting
	m.screen = screenFolder
	m.cursor = indexFolder(FolderReadFolder)

	updated, _ := m.handleKey("enter")
	got := updated.(model)

	if got.screen != screenNetwork {
		t.Fatalf("screen = %v, want %v", got.screen, screenNetwork)
	}
	if got.folderAccess != FolderReadFolder {
		t.Fatalf("folderAccess = %q, want %q", got.folderAccess, FolderReadFolder)
	}
}

func TestHostConnectionsToggleClipboardAndGuestAudio(t *testing.T) {
	m := workspaceRunFixture().model()
	m.selectedVM = m.vms[0]
	m.flow = flowRunExisting
	m.screen = screenFolder

	updated, _ := m.handleKey("c")
	m = updated.(model)
	updated, _ = m.handleKey("a")
	m = updated.(model)

	if !m.clipboard || !m.guestAudio {
		t.Fatalf("connections = clipboard %t, guest audio %t; want both on", m.clipboard, m.guestAudio)
	}
}

func TestHostConnectionsCanChooseAnotherProjectFolder(t *testing.T) {
	m := workspaceRunFixture().model()
	m.selectedVM = m.vms[0]
	m.flow = flowRunExisting
	m.screen = screenFolder

	updated, _ := m.handleKey("p")
	m = updated.(model)
	if m.screen != screenProjectFolderPath {
		t.Fatalf("screen = %v, want project-folder path", m.screen)
	}
	m.projectPathInput = "/tmp/other-project"
	m.projectPathCursor = runeCount(m.projectPathInput)
	updated, _ = m.handleKey("enter")
	m = updated.(model)

	if m.screen != screenFolder || m.projectPath != "/tmp/other-project" {
		t.Fatalf("screen = %v, path = %q; want host connections with explicit path", m.screen, m.projectPath)
	}
}

func TestNewModelUsesLeastAccessScreenDefaults(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Defaults.FolderAccess = FolderReadFolder
	cfg.Defaults.NetworkAccess = NetworkInternet

	m := newModel(cfg, "", nil)

	if m.folderAccess != FolderNoFolder {
		t.Fatalf("folder default = %q, want %q", m.folderAccess, FolderNoFolder)
	}
	if m.networkAccess != NetworkOffline {
		t.Fatalf("network default = %q, want %q", m.networkAccess, NetworkOffline)
	}
}

func TestVMLoadedMessageMergesConfiguredKinds(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SetKind("base", VMKindTemplate)
	m := newModel(cfg, "", nil)
	m.loadingVMs = true

	updated, _ := m.Update(vmsLoadedMsg([]VM{{
		Name:  "base",
		State: "stopped",
	}}))
	got := updated.(model)

	if got.loadingVMs {
		t.Fatal("loadingVMs = true, want false")
	}
	if len(got.vms) != 1 {
		t.Fatalf("vms = %d, want 1", len(got.vms))
	}
	if got.vms[0].Kind != VMKindTemplate {
		t.Fatalf("kind = %q, want template", got.vms[0].Kind)
	}
}

func TestVMLoadedMessageClearsCleanupForAbsentVM(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PendingCleanup = []string{"tmp-gone"}
	m := newModel(cfg, "", nil)

	updated, _ := m.Update(vmsLoadedMsg([]VM{{Name: "dev"}}))
	got := updated.(model)

	if len(got.cfg.PendingCleanup) != 0 {
		t.Fatalf("pending cleanup = %#v, want empty", got.cfg.PendingCleanup)
	}
	if !got.needsSave {
		t.Fatal("needsSave = false, want reconciled cleanup persisted")
	}
}

func TestCleanupShortcutDoesNotDeleteVMWithReusedName(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PendingCleanup = []string{"tmp-reused"}
	m := newModel(cfg, "", []VM{{Name: "tmp-reused", Kind: VMKindWorkspace}})

	updated, cmd := m.handleKey("c")
	got := updated.(model)

	if cmd != nil {
		t.Fatal("cleanup shortcut returned a command")
	}
	if got.plan.DeleteVM != "" || got.screen != screenHome {
		t.Fatalf("cleanup shortcut prepared deletion of %q on screen %v", got.plan.DeleteVM, got.screen)
	}
	if !strings.Contains(got.status, "identity is ambiguous") {
		t.Fatalf("status = %q, want identity warning", got.status)
	}
}

func TestLoadVMsCommandUsesConfigSnapshot(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SetKind("base", VMKindTemplate)
	m := newModel(cfg, "", nil)
	m.tart = &fakeTart{listVMs: []VM{{Name: "base"}}}

	cmd := m.loadVMsCmd()
	m.cfg.SetKind("base", VMKindWorkspace)
	msg := cmd().(vmsLoadedMsg)

	if msg[0].Kind != VMKindTemplate {
		t.Fatalf("loaded kind = %q, want snapshot kind %q", msg[0].Kind, VMKindTemplate)
	}
}

func TestHomePollContinuesWhileOffHome(t *testing.T) {
	m := newModel(DefaultConfig(), "", nil)
	m.tart = &fakeTart{}
	m.screen = screenVMAction

	if m.homePollCmd() == nil {
		t.Fatal("home poll command is nil off the home screen")
	}
}

func TestNetworkChoiceAdvancesToReview(t *testing.T) {
	m := newModel(DefaultConfig(), "", []VM{{
		Name:  "dev",
		Kind:  VMKindWorkspace,
		State: "stopped",
	}})
	m.selectedVM = m.vms[0]
	m.flow = flowRunExisting
	m.screen = screenNetwork
	m.folderAccess = FolderReadFolder
	m.cursor = indexNetwork(NetworkInternet)

	updated, _ := m.handleKey("enter")
	got := updated.(model)

	if got.screen != screenReview {
		t.Fatalf("screen = %v, want %v", got.screen, screenReview)
	}
	if got.networkAccess != NetworkInternet {
		t.Fatalf("networkAccess = %q, want %q", got.networkAccess, NetworkInternet)
	}
	if len(got.plan.Steps) != 2 {
		t.Fatalf("plan steps = %d, want 2", len(got.plan.Steps))
	}
}

func TestRenderFrameShowsTrailStepAndDangerContext(t *testing.T) {
	m := newModel(DefaultConfig(), "", nil)
	view := m.renderFrame(framePage{
		Trail:  []string{"VMs", "dev", "delete"},
		Step:   "review",
		Body:   "Delete dev?",
		Danger: true,
	})

	if !strings.Contains(view, "VMs / dev / delete") {
		t.Fatalf("frame does not show trail:\n%s", view)
	}
	if !strings.Contains(view, "review") {
		t.Fatalf("frame does not show step:\n%s", view)
	}
}

func TestHomeSelectionOpensVMActionMenu(t *testing.T) {
	m := newModel(DefaultConfig(), "", []VM{{
		Name:  "dev",
		Kind:  VMKindWorkspace,
		State: "stopped",
	}})

	updated, _ := m.handleKey("enter")
	got := updated.(model)

	if got.screen != screenVMAction {
		t.Fatalf("screen = %v, want %v", got.screen, screenVMAction)
	}
	if got.selectedVM.Name != "dev" {
		t.Fatalf("selected VM = %q, want dev", got.selectedVM.Name)
	}
}

func TestVMActionMenuExportPromptsForPath(t *testing.T) {
	m := newModel(DefaultConfig(), "", []VM{{
		Name:  "dev-action-export-test",
		Kind:  VMKindWorkspace,
		State: "stopped",
	}})
	m.selectedVM = m.vms[0]
	m.screen = screenVMAction
	m.cursor = indexString(vmActions(), "export")

	updated, _ := m.handleKey("enter")
	got := updated.(model)

	if got.screen != screenExportPath {
		t.Fatalf("screen = %v, want %v", got.screen, screenExportPath)
	}
}

func TestTemplateActionMenuExportPromptsForPath(t *testing.T) {
	m := newModel(DefaultConfig(), "", []VM{{
		Name:  "base-action-export-test",
		Kind:  VMKindTemplate,
		State: "stopped",
	}})
	m.selectedVM = m.vms[0]
	m.screen = screenTemplateAction
	m.cursor = indexString(templateActions(), "export")

	updated, _ := m.handleKey("enter")
	got := updated.(model)

	if got.screen != screenExportPath {
		t.Fatalf("screen = %v, want %v", got.screen, screenExportPath)
	}
}

func TestExportPathBuildsReviewPlan(t *testing.T) {
	m := screenFixture{cfg: DefaultConfig(), vms: []VM{{Name: "dev", Kind: VMKindWorkspace}}}.model()
	m.selectedVM = m.vms[0]
	m.flow = flowExportVM
	m.screen = screenExportPath
	m.mode = modeInsert
	m.exportDestinationPath = "/tmp/dev.tvm"
	m.exportPathCursor = runeCount(m.exportDestinationPath)

	updated, _ := m.handleKey("enter")
	got := updated.(model)
	if got.screen != screenReview {
		t.Fatalf("screen = %v, want %v", got.screen, screenReview)
	}
	if got.plan.ExportPath != "/tmp/dev.tvm" {
		t.Fatalf("export path = %q", got.plan.ExportPath)
	}
}

func TestImportPathPromptsForImportName(t *testing.T) {
	m := screenFixture{cfg: DefaultConfig()}.model()
	m.flow = flowImportArchive
	m.screen = screenImportPath
	m.mode = modeInsert
	m.importSourcePath = "/tmp/dev.tvm"
	m.importPathCursor = runeCount(m.importSourcePath)

	updated, _ := m.handleKey("enter")
	got := updated.(model)

	if got.screen != screenName {
		t.Fatalf("screen = %v, want %v", got.screen, screenName)
	}
	if got.nameMode != nameModeImportVM {
		t.Fatalf("name mode = %v, want %v", got.nameMode, nameModeImportVM)
	}
	if got.nameInput != "dev" {
		t.Fatalf("name input = %q, want dev", got.nameInput)
	}
	if got.importSourcePath != "/tmp/dev.tvm" {
		t.Fatalf("import source = %q", got.importSourcePath)
	}
}

func TestImportNameBuildsReviewPlan(t *testing.T) {
	source := "/tmp/dev.tvm"
	m := screenFixture{
		cfg: DefaultConfig(),
		vms: []VM{{
			Name: "existing",
			Kind: VMKindWorkspace,
		}},
		host: fakeHostEnvironment{currentDirectory: "/tmp/project"},
	}.model()
	m.flow = flowImportArchive
	m.screen = screenName
	m.nameMode = nameModeImportVM
	m.importSourcePath = source
	m.nameInput = "restored-dev"

	updated, _ := m.handleKey("enter")
	got := updated.(model)

	if got.screen != screenReview {
		t.Fatalf("screen = %v, want %v", got.screen, screenReview)
	}
	if got.plan.Steps[0].Kind != CommandStepImport {
		t.Fatalf("step kind = %q, want %q", got.plan.Steps[0].Kind, CommandStepImport)
	}
	want := []string{"tart", "import", source, "restored-dev"}
	if strings.Join(got.plan.Steps[0].Args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args\n got %#v\nwant %#v", got.plan.Steps[0].Args, want)
	}
}

func TestImportNameCollisionShowsMessage(t *testing.T) {
	m := screenFixture{
		cfg: DefaultConfig(),
		vms: []VM{{
			Name: "dev",
			Kind: VMKindWorkspace,
		}},
		host: fakeHostEnvironment{currentDirectory: "/tmp/project"},
	}.model()
	m.flow = flowImportArchive
	m.screen = screenName
	m.nameMode = nameModeImportVM
	m.importSourcePath = "/tmp/dev.tvm"
	m.nameInput = "dev"

	updated, _ := m.handleKey("enter")
	got := updated.(model)

	if got.screen != screenName {
		t.Fatalf("screen = %v, want %v", got.screen, screenName)
	}
	if !strings.Contains(got.message, "destination VM already exists") {
		t.Fatalf("message = %q, want collision error", got.message)
	}
}

func TestExportReviewShowsSensitiveStateWarning(t *testing.T) {
	plan, err := BuildExportPlan(ExportOptions{
		VMName:          "dev",
		VMKind:          VMKindWorkspace,
		DestinationPath: "/tmp/dev.tvm",
	})
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(DefaultConfig(), "", nil)
	m.plan = plan
	m.screen = screenReview

	view := lipgloss.NewRenderer(os.Stdout).NewStyle().Render(m.renderReview())
	if !strings.Contains(view, "may contain secrets") {
		t.Fatalf("review = %q, want sensitive-state warning", view)
	}
	if strings.Contains(view, "Folder") {
		t.Fatalf("review = %q, did not want run boundary summary for export", view)
	}
	if !strings.Contains(view, "FINAL ARCHIVE") || !strings.Contains(view, "/tmp/dev.tvm") {
		t.Fatalf("review = %q, want final export destination", view)
	}
}

func TestHomeKeyClampsStaleCursor(t *testing.T) {
	m := newModel(DefaultConfig(), "", []VM{{
		Name:  "one",
		Kind:  VMKindWorkspace,
		State: "stopped",
	}, {
		Name:  "two",
		Kind:  VMKindWorkspace,
		State: "stopped",
	}})
	m.cursor = 5

	updated, _ := m.handleKey("m")
	got := updated.(model)

	if got.cursor != 1 {
		t.Fatalf("cursor = %d, want clamped to 1", got.cursor)
	}
	if got.vms[1].Kind != VMKindTemplate {
		t.Fatalf("second VM kind = %q, want %q", got.vms[1].Kind, VMKindTemplate)
	}
}

func TestHomeFilterMapsActionsToOriginalVM(t *testing.T) {
	m := newModel(DefaultConfig(), "", []VM{{
		Name:  "base",
		Kind:  VMKindTemplate,
		State: "stopped",
	}, {
		Name:  "dev-special",
		Kind:  VMKindWorkspace,
		State: "stopped",
	}})
	m.homeFilter = "special"

	updated, _ := m.handleKey("m")
	got := updated.(model)

	if got.vms[0].Kind != VMKindTemplate {
		t.Fatalf("first VM kind = %q, want unchanged template", got.vms[0].Kind)
	}
	if got.vms[1].Kind != VMKindTemplate {
		t.Fatalf("filtered VM kind = %q, want cycled template", got.vms[1].Kind)
	}
}

func TestHomeWindowKeepsSelectionVisible(t *testing.T) {
	m := newModel(DefaultConfig(), "", []VM{
		{Name: "one"},
		{Name: "two"},
		{Name: "three"},
		{Name: "four"},
		{Name: "five"},
	})
	m.height = 10
	m.cursor = 4

	start, end := m.homeWindow(len(m.vms))

	if start != 0 {
		t.Fatalf("start = %d, want full window", start)
	}
	if end != len(m.vms) {
		t.Fatalf("end = %d, want %d", end, len(m.vms))
	}
}

func TestActionMenuEscapeRestoresSelectedVMCursor(t *testing.T) {
	m := newModel(DefaultConfig(), "", []VM{{
		Name:  "one",
		Kind:  VMKindWorkspace,
		State: "stopped",
	}, {
		Name:  "two",
		Kind:  VMKindWorkspace,
		State: "stopped",
	}})
	m.selectedVM = m.vms[1]
	m.screen = screenVMAction
	m.cursor = len(vmActions()) - 1

	updated, _ := m.handleKey("esc")
	got := updated.(model)

	if got.screen != screenHome {
		t.Fatalf("screen = %v, want %v", got.screen, screenHome)
	}
	if got.cursor != 1 {
		t.Fatalf("cursor = %d, want selected VM index 1", got.cursor)
	}
}

func TestNameEscapeReturnsHomeWithValidCursor(t *testing.T) {
	m := newModel(DefaultConfig(), "", []VM{{
		Name:  "one",
		Kind:  VMKindWorkspace,
		State: "stopped",
	}, {
		Name:  "two",
		Kind:  VMKindWorkspace,
		State: "stopped",
	}})
	m.selectedVM = m.vms[1]
	m.screen = screenName
	m.cursor = 9

	updated, _ := m.handleKey("esc")
	got := updated.(model)

	if got.screen != screenHome {
		t.Fatalf("screen = %v, want %v", got.screen, screenHome)
	}
	if got.cursor != 1 {
		t.Fatalf("cursor = %d, want selected VM index 1", got.cursor)
	}
}

func TestNameScreenAcceptsQAsText(t *testing.T) {
	m := newModel(DefaultConfig(), "", []VM{{
		Name: "base",
		Kind: VMKindTemplate,
	}})
	m.screen = screenName
	m.nameMode = nameModeNewVM
	m.nameInput = "se"
	m.nameCursor = runeCount(m.nameInput)

	updated, cmd := m.Update(teaKey("q"))
	got := updated.(model)

	if cmd != nil {
		t.Fatal("cmd is non-nil, want no quit command")
	}
	if got.nameInput != "seq" {
		t.Fatalf("name input = %q, want seq", got.nameInput)
	}
}

func TestHomeFilterAcceptsQAsText(t *testing.T) {
	m := newModel(DefaultConfig(), "", []VM{{
		Name: "quick-dev",
		Kind: VMKindWorkspace,
	}})
	m.mode = modeFilter

	updated, cmd := m.Update(teaKey("q"))
	got := updated.(model)

	if cmd != nil {
		t.Fatal("cmd is non-nil, want no quit command")
	}
	if got.homeFilter != "q" {
		t.Fatalf("home filter = %q, want q", got.homeFilter)
	}
}

func TestNameInputInsertsAtCursor(t *testing.T) {
	m := newModel(DefaultConfig(), "", nil)
	m.screen = screenName
	m.nameInput = "ab"
	m.nameCursor = 1

	updated, _ := m.handleKey("Z")
	got := updated.(model)

	if got.nameInput != "aZb" {
		t.Fatalf("name input = %q, want aZb", got.nameInput)
	}
	if got.nameCursor != 2 {
		t.Fatalf("name cursor = %d, want 2", got.nameCursor)
	}
}

func TestEmptyNameStaysInline(t *testing.T) {
	m := newModel(DefaultConfig(), "", nil)
	m.screen = screenName
	m.nameInput = ""

	updated, _ := m.handleKey("enter")
	got := updated.(model)

	if got.screen != screenName {
		t.Fatalf("screen = %v, want %v", got.screen, screenName)
	}
	if got.message != "Name is required." {
		t.Fatalf("message = %q, want required warning", got.message)
	}
}

func TestReviewCancelReturnsHomeWithValidCursor(t *testing.T) {
	m := newModel(DefaultConfig(), "", []VM{{
		Name:  "one",
		Kind:  VMKindWorkspace,
		State: "stopped",
	}, {
		Name:  "two",
		Kind:  VMKindWorkspace,
		State: "stopped",
	}})
	m.selectedVM = m.vms[1]
	m.screen = screenReview
	m.cursor = 99

	updated, _ := m.handleKey("esc")
	got := updated.(model)

	if got.screen != screenHome {
		t.Fatalf("screen = %v, want %v", got.screen, screenHome)
	}
	if got.cursor != 1 {
		t.Fatalf("cursor = %d, want selected VM index 1", got.cursor)
	}
}

func TestReviewBackReturnsToPreviousFlowScreen(t *testing.T) {
	m := workspaceRunFixture().model()
	m.selectedVM = m.vms[0]
	m.flow = flowRunExisting
	m.screen = screenNetwork
	m.folderAccess = FolderNoFolder
	m.networkAccess = NetworkOffline
	m.backStack = []navigationEntry{{screen: screenFolder, cursor: indexFolder(FolderNoFolder)}}

	updated, _ := m.handleKey("enter")
	got := updated.(model)
	if got.screen != screenReview {
		t.Fatalf("screen = %v, want %v", got.screen, screenReview)
	}

	updated, _ = got.handleKey("esc")
	got = updated.(model)
	if got.screen != screenNetwork {
		t.Fatalf("screen = %v, want %v", got.screen, screenNetwork)
	}
}

func TestMessageDismissReturnsHomeWithClampedCursor(t *testing.T) {
	m := newModel(DefaultConfig(), "", []VM{{
		Name:  "one",
		Kind:  VMKindWorkspace,
		State: "stopped",
	}})
	m.screen = screenMessage
	m.cursor = 12

	updated, _ := m.handleKey("enter")
	got := updated.(model)

	if got.screen != screenHome {
		t.Fatalf("screen = %v, want %v", got.screen, screenHome)
	}
	if got.cursor != 0 {
		t.Fatalf("cursor = %d, want clamped to 0", got.cursor)
	}
}

func TestDeleteReviewRequiresY(t *testing.T) {
	m := newModel(DefaultConfig(), "", []VM{{
		Name: "dev",
		Kind: VMKindWorkspace,
	}})
	m.selectedVM = m.vms[0]
	if err := m.prepareDeletePlan(); err != nil {
		t.Fatal(err)
	}
	m.screen = screenReview

	updated, _ := m.handleKey("enter")
	got := updated.(model)
	if got.screen != screenReview {
		t.Fatalf("screen after enter = %v, want review", got.screen)
	}

	updated, _ = got.handleKey("y")
	got = updated.(model)
	if got.screen != screenExecute {
		t.Fatalf("screen after y = %v, want execute", got.screen)
	}
}

func TestExecutionRecordsPendingCleanupAfterTemporaryRunFailure(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	m := newModel(DefaultConfig(), cfgPath, nil)
	m.plan = temporaryExecutionPlan()
	m.tart = &fakeTart{failAtLabel: "run"}

	updated, cmd := m.beginExecution()
	got := updated.(model)
	for cmd != nil && !got.executeDone {
		msg := cmd()
		updated, cmd = got.Update(msg)
		got = updated.(model)
	}

	if !got.executeDone {
		t.Fatal("executeDone = false, want true")
	}
	if !strings.Contains(got.executeErr, "run: boom") {
		t.Fatalf("executeErr = %q, want Tart run failure", got.executeErr)
	}
	if len(got.cfg.PendingCleanup) != 1 || got.cfg.PendingCleanup[0] != "tmp-1" {
		t.Fatalf("pending cleanup = %#v, want tmp-1", got.cfg.PendingCleanup)
	}
}

func TestExecutionRecordsPendingCleanupWhenCloneSucceeds(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	m := newModel(DefaultConfig(), cfgPath, nil)
	m.plan = temporaryExecutionPlan()
	m.tart = &fakeTart{}

	updated, cmd := m.beginExecution()
	got := updated.(model)

	if len(got.cfg.PendingCleanup) != 1 || got.cfg.PendingCleanup[0] != "tmp-1" {
		t.Fatalf("pending cleanup before clone = %#v, want tmp-1", got.cfg.PendingCleanup)
	}
	saved := mustLoadConfigFile(t, cfgPath)
	if len(saved.PendingCleanup) != 1 || saved.PendingCleanup[0] != "tmp-1" {
		t.Fatalf("saved pending cleanup before clone = %#v, want tmp-1", saved.PendingCleanup)
	}
	if cmd == nil {
		t.Fatal("clone command is nil")
	}
}

func TestExecutionKeepsPendingCleanupWhenCloneFails(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	m := newModel(DefaultConfig(), cfgPath, nil)
	m.plan = temporaryExecutionPlan()
	m.tart = &fakeTart{failAtLabel: "clone"}

	updated, cmd := m.beginExecution()
	got := updated.(model)
	updated, _ = got.Update(cmd())
	got = updated.(model)

	if got.executeErr == "" {
		t.Fatal("clone failure was not reported")
	}
	if len(got.cfg.PendingCleanup) != 1 || got.cfg.PendingCleanup[0] != "tmp-1" {
		t.Fatalf("pending cleanup after clone failure = %#v, want tmp-1", got.cfg.PendingCleanup)
	}
}

func TestExecutionDoesNotCloneWhenPendingCleanupCannotBeSaved(t *testing.T) {
	m := newModel(DefaultConfig(), t.TempDir(), nil)
	m.plan = temporaryExecutionPlan()
	tart := &fakeTart{}
	m.tart = tart

	updated, cmd := m.beginExecution()
	got := updated.(model)

	if cmd != nil {
		t.Fatal("clone command was returned after pending cleanup save failed")
	}
	if !got.executeDone || !strings.Contains(got.executeErr, "save pending cleanup before clone") {
		t.Fatalf("execution state = done %v, error %q", got.executeDone, got.executeErr)
	}
	if len(tart.calls) != 0 {
		t.Fatalf("Tart calls = %#v, want none", tart.calls)
	}
}

func TestCtrlCDoesNotQuitDuringExecution(t *testing.T) {
	m := newModel(DefaultConfig(), filepath.Join(t.TempDir(), "config.json"), nil)
	m.plan = temporaryExecutionPlan()
	m.tart = &fakeTart{}
	updated, _ := m.beginExecution()
	m = updated.(model)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	got := updated.(model)
	if cmd != nil {
		t.Fatal("ctrl+c returned a quit command during active execution")
	}
	if got.screen != screenExecute || got.executeDone {
		t.Fatalf("execution state changed after ctrl+c: screen=%v done=%v", got.screen, got.executeDone)
	}
}

func TestExportDoesNotReplaceDestinationCreatedAfterReview(t *testing.T) {
	directory := t.TempDir()
	destination := filepath.Join(directory, "dev.tvm")
	plan, err := BuildExportPlan(ExportOptions{
		VMName:          "dev",
		VMKind:          VMKindWorkspace,
		DestinationPath: destination,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(DefaultConfig(), filepath.Join(directory, "config.json"), nil)
	m.plan = plan
	m.tart = &fakeTart{exportContents: []byte("new archive")}

	updated, cmd := m.beginExecution()
	got := updated.(model)
	if err := os.WriteFile(destination, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	updated, _ = got.Update(cmd())
	got = updated.(model)

	contents, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "keep me" {
		t.Fatalf("destination contents = %q, want original contents", contents)
	}
	if got.executeErr == "" || !strings.Contains(got.executeErr, "already exists") {
		t.Fatalf("execution error = %q, want destination collision", got.executeErr)
	}
	temporaryContents, err := os.ReadFile(plan.ExportTemporaryPath)
	if err != nil {
		t.Fatalf("read preserved temporary export: %v", err)
	}
	if string(temporaryContents) != "new archive" {
		t.Fatalf("temporary export contents = %q, want completed archive", temporaryContents)
	}
}

func TestExportInstallsCompletedArchiveWithoutLeavingTemporaryFile(t *testing.T) {
	directory := t.TempDir()
	destination := filepath.Join(directory, "dev.tvm")
	plan, err := BuildExportPlan(ExportOptions{
		VMName:          "dev",
		VMKind:          VMKindWorkspace,
		DestinationPath: destination,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(DefaultConfig(), filepath.Join(directory, "config.json"), nil)
	m.plan = plan
	m.tart = &fakeTart{exportContents: []byte("archive")}

	updated, cmd := m.beginExecution()
	got := updated.(model)
	updated, _ = got.Update(cmd())
	got = updated.(model)

	contents, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "archive" {
		t.Fatalf("destination contents = %q, want archive", contents)
	}
	if got.executeErr != "" {
		t.Fatalf("execution error = %q", got.executeErr)
	}
	if _, err := os.Stat(plan.ExportTemporaryPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary export remains after success: %v", err)
	}
}

func TestCompletedWorkspaceFromTemplateRecordsCreatedVM(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	m := templateRunFixture().model()
	m.cfgPath = cfgPath
	m.selectedVM = m.vms[0]
	m.selectedTemplate = m.vms[0]
	m.flow = flowNewFromTemplate
	m.runDuration = RunDurationWorkspace
	m.nameInput = "newws"
	if err := m.preparePlan(); err != nil {
		t.Fatal(err)
	}
	if err := m.applyCompletedPlanEffects(); err != nil {
		t.Fatal(err)
	}

	if _, ok := m.cfg.LastRunFor("newws"); !ok {
		t.Fatal("created workspace has no last-run settings")
	}
	if _, ok := m.cfg.LastRunFor("base"); ok {
		t.Fatal("template unexpectedly received the workspace last-run settings")
	}
	if got := m.executionReturnName(); got != "newws" {
		t.Fatalf("return name = %q, want newws", got)
	}
}

func TestCompletedRenameMovesSavedVMState(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SetKind("old", VMKindTemplate)
	m := newModel(cfg, filepath.Join(t.TempDir(), "config.json"), nil)
	plan, err := BuildRenamePlan("old", "new")
	if err != nil {
		t.Fatal(err)
	}
	m.plan = plan

	if err := m.applyCompletedPlanEffects(); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.cfg.VMs["old"]; ok {
		t.Fatal("old VM state remains after rename")
	}
	if got := m.cfg.KindFor("new"); got != VMKindTemplate {
		t.Fatalf("new VM kind = %q, want template", got)
	}
}

func TestCompletedDeleteForgetsSavedVMState(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SetKind("dev", VMKindTemplate)
	cfg.AddPendingCleanup("dev")
	m := newModel(cfg, filepath.Join(t.TempDir(), "config.json"), nil)
	plan, err := BuildDeletePlan("dev")
	if err != nil {
		t.Fatal(err)
	}
	m.plan = plan

	if err := m.applyCompletedPlanEffects(); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.cfg.VMs["dev"]; ok {
		t.Fatal("deleted VM state remains")
	}
	if len(m.cfg.PendingCleanup) != 0 {
		t.Fatalf("pending cleanup = %#v, want empty", m.cfg.PendingCleanup)
	}
}

func TestExecutionClearsTemporaryCleanupAfterSuccess(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	m := newModel(DefaultConfig(), cfgPath, nil)
	m.plan = temporaryExecutionPlan()
	m.tart = &fakeTart{}

	updated, cmd := m.beginExecution()
	got := updated.(model)
	for cmd != nil && !got.executeDone {
		msg := cmd()
		updated, cmd = got.Update(msg)
		got = updated.(model)
	}

	if !got.executeDone {
		t.Fatal("executeDone = false, want true")
	}
	if got.executeErr != "" {
		t.Fatalf("executeErr = %q, want empty", got.executeErr)
	}
	if len(got.cfg.PendingCleanup) != 0 {
		t.Fatalf("pending cleanup = %#v, want empty", got.cfg.PendingCleanup)
	}
	if got.executeSummary == "" {
		t.Fatal("executeSummary is empty")
	}
}

func TestMessageDismissIgnoresStaleSelectedVM(t *testing.T) {
	m := newModel(DefaultConfig(), "", []VM{{
		Name:  "one",
		Kind:  VMKindWorkspace,
		State: "stopped",
	}, {
		Name:  "two",
		Kind:  VMKindWorkspace,
		State: "stopped",
	}})
	m.selectedVM = m.vms[0]
	m.screen = screenMessage
	m.cursor = 1

	updated, _ := m.handleKey("enter")
	got := updated.(model)

	if got.screen != screenHome {
		t.Fatalf("screen = %v, want %v", got.screen, screenHome)
	}
	if got.cursor != 1 {
		t.Fatalf("cursor = %d, want current cursor 1", got.cursor)
	}
}

func TestNewFromTemplateReviewCancelReturnsToTemplate(t *testing.T) {
	m := newModel(DefaultConfig(), "", []VM{{
		Name:  "old-selection",
		Kind:  VMKindWorkspace,
		State: "stopped",
	}, {
		Name:  "base",
		Kind:  VMKindTemplate,
		State: "stopped",
	}})
	m.selectedVM = m.vms[0]
	m.selectedTemplate = m.vms[1]
	m.flow = flowNewFromTemplate
	m.screen = screenReview
	m.cursor = 0

	updated, _ := m.handleKey("esc")
	got := updated.(model)

	if got.screen != screenHome {
		t.Fatalf("screen = %v, want %v", got.screen, screenHome)
	}
	if got.cursor != 1 {
		t.Fatalf("cursor = %d, want template index 1", got.cursor)
	}
}

func TestReturnHomeTargetsRenderedVMOrder(t *testing.T) {
	m := newModel(DefaultConfig(), "", []VM{
		{Name: "base", Kind: VMKindTemplate},
		{Name: "ws-a", Kind: VMKindWorkspace},
		{Name: "ws-b", Kind: VMKindWorkspace},
	})

	m = m.returnHomeToVM("ws-b")
	items := m.filteredHomeItems()

	if got := items[m.cursor].vm.Name; got != "ws-b" {
		t.Fatalf("selected VM = %q, want ws-b", got)
	}
}

func TestReturnHomeTargetsFilteredVMOrder(t *testing.T) {
	m := newModel(DefaultConfig(), "", []VM{
		{Name: "base", Kind: VMKindTemplate},
		{Name: "dev-a", Kind: VMKindWorkspace},
		{Name: "dev-b", Kind: VMKindWorkspace},
	})
	m.homeFilter = "dev-b"

	m = m.returnHomeToVM("dev-b")
	items := m.filteredHomeItems()

	if got := items[m.cursor].vm.Name; got != "dev-b" {
		t.Fatalf("selected VM = %q, want dev-b", got)
	}
}

func TestReturnHomeTargetsCreatedVMAfterRefresh(t *testing.T) {
	m := newModel(DefaultConfig(), "", []VM{{Name: "base", Kind: VMKindTemplate}})
	m = m.returnHomeToVM("newws")
	if m.homeFocusVM != "newws" {
		t.Fatalf("pending focus = %q, want newws", m.homeFocusVM)
	}

	updated, _ := m.Update(vmsLoadedMsg([]VM{
		{Name: "base", Kind: VMKindTemplate},
		{Name: "newws", Kind: VMKindWorkspace},
	}))
	m = updated.(model)
	items := m.filteredHomeItems()

	if got := items[m.cursor].vm.Name; got != "newws" {
		t.Fatalf("selected VM = %q, want newws", got)
	}
	if m.homeFocusVM != "" {
		t.Fatalf("pending focus = %q, want cleared", m.homeFocusVM)
	}
}

func TestRenderHomeWithStaleCursorStillShowsSelection(t *testing.T) {
	m := newModel(DefaultConfig(), "", []VM{{
		Name:  "one",
		Kind:  VMKindWorkspace,
		State: "stopped",
	}})
	m.cursor = 20

	view := m.renderHome()

	if !strings.Contains(view, "▌ ") {
		t.Fatalf("home view does not show selected row:\n%s", view)
	}
}

func TestReviewGroupsFolderAndPathAsHostAccessGrant(t *testing.T) {
	m := newModel(DefaultConfig(), "", nil)
	m.plan = Plan{Review: PlanReview{
		FolderAccess:  FolderReadFolder,
		NetworkAccess: NetworkOffline,
		Clipboard:     false,
		GuestAudio:    false,
	}}

	view := m.renderCompletedGrantLedger()

	if !strings.Contains(view, "⎧ folder") {
		t.Fatalf("review does not begin the host access grouping:\n%s", view)
	}
	if !strings.Contains(view, "⎩ path") {
		t.Fatalf("review does not end the host access grouping:\n%s", view)
	}
}

func TestMarkKindShowsEachKindExplanationOnce(t *testing.T) {
	m := newModel(DefaultConfig(), "", []VM{{
		Name: "base",
		Kind: VMKindTemplate,
	}})
	m.selectedVM = m.vms[0]
	m.screen = screenMarkKind

	view := m.renderMarkKind()
	if got := strings.Count(view, "kept clean; source for new VMs"); got != 1 {
		t.Fatalf("template explanation appears %d times, want once:\n%s", got, view)
	}
	if got := strings.Count(view, "kept around; run directly"); got != 1 {
		t.Fatalf("workspace explanation appears %d times, want once:\n%s", got, view)
	}
}

func TestVMActionRunAdvancesToFolder(t *testing.T) {
	m := newModel(DefaultConfig(), "", []VM{{
		Name:  "dev",
		Kind:  VMKindWorkspace,
		State: "stopped",
	}})
	m.selectedVM = m.vms[0]
	m.screen = screenVMAction
	m.cursor = 0

	updated, _ := m.handleKey("enter")
	got := updated.(model)

	if got.screen != screenFolder {
		t.Fatalf("screen = %v, want %v", got.screen, screenFolder)
	}
	if got.flow != flowRunExisting {
		t.Fatalf("flow = %v, want %v", got.flow, flowRunExisting)
	}
}

func TestRepeatTemplateRunIsReadOnly(t *testing.T) {
	fixture := templateRunFixture()
	fixture.cfg.SetLastRun("base", LastRunConfig{FolderAccess: FolderNoFolder, NetworkAccess: NetworkOffline})
	m := fixture.model()

	updated, _ := m.handleKey(".")
	got := updated.(model)

	if got.screen != screenReview {
		t.Fatalf("screen = %v, want %v", got.screen, screenReview)
	}
	if got.flow != flowRunTemplateReadOnly {
		t.Fatalf("flow = %v, want %v", got.flow, flowRunTemplateReadOnly)
	}
	if !slices.Contains(got.plan.Steps[1].Args, "--root-disk-opts=ro") {
		t.Fatalf("run args = %#v, want read-only root disk", got.plan.Steps[1].Args)
	}
}

func TestRepeatRunRestoresExactHostConnections(t *testing.T) {
	fixture := workspaceRunFixture()
	fixture.cfg.SetLastRun("dev", LastRunConfig{
		FolderAccess:  FolderEditFolder,
		ProjectPath:   "/tmp/other-project",
		NetworkAccess: NetworkHost,
		Clipboard:     true,
		GuestAudio:    true,
	})
	m := fixture.model()

	updated, _ := m.handleKey(".")
	got := updated.(model)

	if got.screen != screenReview {
		t.Fatalf("screen = %v, want review", got.screen)
	}
	if got.plan.Review.ProjectPath != "/tmp/other-project" || !got.plan.Review.Clipboard || !got.plan.Review.GuestAudio {
		t.Fatalf("review = %#v, want exact replayed host connections", got.plan.Review)
	}
	if slices.Contains(got.plan.Steps[1].Args, "--no-clipboard") || slices.Contains(got.plan.Steps[1].Args, "--no-audio") {
		t.Fatalf("run args = %#v, want clipboard and guest audio enabled", got.plan.Steps[1].Args)
	}
}

func TestNewShortcutUsesSelectedTemplate(t *testing.T) {
	m := newModel(DefaultConfig(), "", []VM{
		{Name: "base-a", Kind: VMKindTemplate},
		{Name: "dev", Kind: VMKindWorkspace},
		{Name: "base-b", Kind: VMKindTemplate},
	})
	m.cursor = 2

	updated, _ := m.handleKey("n")
	got := updated.(model)

	if got.selectedTemplate.Name != "base-b" {
		t.Fatalf("selected template = %q, want base-b", got.selectedTemplate.Name)
	}
}

func TestLANChoiceWithoutCIDROpensCIDRPrompt(t *testing.T) {
	m := newModel(DefaultConfig(), "", []VM{{
		Name:  "dev",
		Kind:  VMKindWorkspace,
		State: "stopped",
	}})
	m.selectedVM = m.vms[0]
	m.flow = flowRunExisting
	m.screen = screenNetwork
	m.folderAccess = FolderReadFolder
	m.cursor = indexNetwork(NetworkLAN)

	updated, _ := m.handleKey("enter")
	got := updated.(model)

	if got.screen != screenLANCIDR {
		t.Fatalf("screen = %v, want %v", got.screen, screenLANCIDR)
	}
	if len(got.lanCIDRChoices) == 0 {
		t.Fatal("LAN CIDR choices are empty")
	}
	if !got.lanCIDRChoices[len(got.lanCIDRChoices)-1].FreeText {
		t.Fatalf("last LAN CIDR choice = %#v, want free-text option", got.lanCIDRChoices[len(got.lanCIDRChoices)-1])
	}
	if got.message != "" {
		t.Fatalf("message = %q, want empty", got.message)
	}
}

func TestLANCIDRChoiceSavesAndAdvancesToReview(t *testing.T) {
	m := newModel(DefaultConfig(), "", []VM{{
		Name:  "dev",
		Kind:  VMKindWorkspace,
		State: "stopped",
	}})
	m.selectedVM = m.vms[0]
	m.flow = flowRunExisting
	m.screen = screenLANCIDR
	m.folderAccess = FolderReadFolder
	m.networkAccess = NetworkLAN
	m.lanCIDRChoices = []lanCIDRChoice{{
		Label:       "192.168.1.0/24",
		Description: "detected from this Mac",
		CIDR:        "192.168.1.0/24",
	}}

	updated, _ := m.handleKey("enter")
	got := updated.(model)

	if got.screen != screenReview {
		t.Fatalf("screen = %v, want %v", got.screen, screenReview)
	}
	if len(got.cfg.Network.LANCIDRs) != 1 || got.cfg.Network.LANCIDRs[0] != "192.168.1.0/24" {
		t.Fatalf("LAN CIDRs = %#v, want one chosen CIDR", got.cfg.Network.LANCIDRs)
	}
	if !got.needsSave {
		t.Fatal("needsSave = false, want true")
	}
}

func TestLANCIDRFreeTextChoiceOpensTextPrompt(t *testing.T) {
	m := newModel(DefaultConfig(), "", nil)
	m.screen = screenLANCIDR
	m.lanCIDRChoices = []lanCIDRChoice{{
		Label:       "other...",
		Description: "enter a CIDR manually",
		FreeText:    true,
	}}

	updated, _ := m.handleKey("enter")
	got := updated.(model)

	if got.screen != screenLANCIDRText {
		t.Fatalf("screen = %v, want %v", got.screen, screenLANCIDRText)
	}
}

func TestLANCIDRTextPromptSavesAndAdvancesToReview(t *testing.T) {
	m := newModel(DefaultConfig(), "", []VM{{
		Name:  "dev",
		Kind:  VMKindWorkspace,
		State: "stopped",
	}})
	m.selectedVM = m.vms[0]
	m.flow = flowRunExisting
	m.screen = screenLANCIDRText
	m.folderAccess = FolderReadFolder
	m.networkAccess = NetworkLAN
	m.lanInput = "192.168.1.0/24"

	updated, _ := m.handleKey("enter")
	got := updated.(model)

	if got.screen != screenReview {
		t.Fatalf("screen = %v, want %v", got.screen, screenReview)
	}
	if len(got.cfg.Network.LANCIDRs) != 1 || got.cfg.Network.LANCIDRs[0] != "192.168.1.0/24" {
		t.Fatalf("LAN CIDRs = %#v, want one entered CIDR", got.cfg.Network.LANCIDRs)
	}
	if !got.needsSave {
		t.Fatal("needsSave = false, want true")
	}
}

func TestInvalidLANCIDRStaysOnPrompt(t *testing.T) {
	m := newModel(DefaultConfig(), "", nil)
	m.screen = screenLANCIDRText
	m.lanInput = "192.168.1"

	updated, _ := m.handleKey("enter")
	got := updated.(model)

	if got.screen != screenLANCIDRText {
		t.Fatalf("screen = %v, want %v", got.screen, screenLANCIDRText)
	}
	if got.message == "" {
		t.Fatal("message is empty, want validation message")
	}
}

func TestCursorCellsHaveStableWidth(t *testing.T) {
	selected := lipgloss.Width(cursorCell(true))
	unselected := lipgloss.Width(cursorCell(false))
	if selected != unselected {
		t.Fatalf("cursor widths selected=%d unselected=%d", selected, unselected)
	}
}

func TestAccessChoicesUseCanonicalGrantLabels(t *testing.T) {
	folder := folderAccessChoice(FolderEditFolder)
	if folder.Label != string(FolderEditFolder) {
		t.Fatalf("edit-folder label = %q", folder.Label)
	}

	cfg := DefaultConfig()
	network := networkAccessChoice(cfg, NetworkLANAndInternet)
	if network.Label != string(NetworkLANAndInternet) {
		t.Fatalf("lan-and-internet label = %q", network.Label)
	}
}

func TestAccessChoiceRowsHaveStableBadgeColumn(t *testing.T) {
	choices := append(folderAccessChoices(), networkAccessChoices(DefaultConfig())...)
	for _, choice := range choices {
		row := renderAccessChoice(choice, false)
		if lipgloss.Width(row) < 20 {
			t.Fatalf("row too narrow for %q: %q", choice.Label, row)
		}
	}
}

func TestHomeGroupsTemplatesAfterMainVMs(t *testing.T) {
	m := newModel(DefaultConfig(), "", []VM{{
		Name:  "base",
		Kind:  VMKindTemplate,
		State: "stopped",
	}, {
		Name:  "dev",
		Kind:  VMKindWorkspace,
		State: "stopped",
	}, {
		Name:  "old",
		Kind:  VMKindWorkspace,
		State: "stopped",
	}})

	items := m.filteredHomeItems()
	want := []string{"dev", "old", "base"}
	for i, name := range want {
		if items[i].vm.Name != name {
			t.Fatalf("item %d = %q, want %q", i, items[i].vm.Name, name)
		}
	}

	view := m.renderHome()
	if !strings.Contains(view, "TEMPLATES") {
		t.Fatalf("home view does not label the template section:\n%s", view)
	}
	if strings.Contains(m.renderHomeRow(m.glyphs(), m.vms[1], false, 20), "workspace") {
		t.Fatalf("workspace row should not carry a type label")
	}
}

func TestBoundaryLabelsHaveStableWidth(t *testing.T) {
	lines := []string{
		boundaryLine("Folder", "x"),
		boundaryLine("Network", "x"),
		boundaryLine("Clipboard", "x"),
	}
	for _, line := range lines {
		if got := lipgloss.Width(strings.TrimSuffix(line, "x")); got != 11 {
			t.Fatalf("label prefix width for %q = %d, want 11", line, got)
		}
	}
}

func TestReviewWindowShowsMoreMarkers(t *testing.T) {
	m := newModel(DefaultConfig(), "", nil)
	m.height = 11
	m.reviewScroll = 2
	lines := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"}

	window := m.reviewWindow(lines)

	if len(window) >= len(lines) {
		t.Fatalf("window length = %d, want clipped below %d", len(window), len(lines))
	}
	if !strings.Contains(window[0], "more") {
		t.Fatalf("first window line = %q, want more marker", window[0])
	}
	if !strings.Contains(window[len(window)-1], "more") {
		t.Fatalf("last window line = %q, want more marker", window[len(window)-1])
	}
}

func TestReviewScrollStopsAtEnd(t *testing.T) {
	m := newModel(DefaultConfig(), "", nil)
	m.height = 11
	m.screen = screenReview
	m.plan = Plan{Steps: []CommandStep{
		{Label: "one", Args: []string{"tart", "run", "one"}},
		{Label: "two", Args: []string{"tart", "run", "two"}},
		{Label: "three", Args: []string{"tart", "run", "three"}},
	}}

	for range 100 {
		updated, _ := m.handleReviewKey("j")
		m = updated.(model)
	}
	maxScroll := m.reviewMaxScroll()
	if m.reviewScroll != maxScroll {
		t.Fatalf("review scroll = %d, want max %d", m.reviewScroll, maxScroll)
	}
	updated, _ := m.handleReviewKey("k")
	m = updated.(model)
	if maxScroll > 0 && m.reviewScroll != maxScroll-1 {
		t.Fatalf("review scroll after k = %d, want %d", m.reviewScroll, maxScroll-1)
	}
}

func TestWrapCommandSplitsLongArguments(t *testing.T) {
	command := "tart run --dir=project:/this/is/a/very/long/path/that/does/not/have/spaces:ro demo"
	lines := wrapCommand(command, 24)
	if len(lines) < 2 {
		t.Fatalf("expected wrapped command, got %q", lines)
	}
	for _, line := range lines {
		if len(line) > 24 {
			t.Fatalf("line %q has length %d, want <= 24", line, len(line))
		}
	}
}

func teaKey(value string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(value)}
}

func TestNetworkAccessOrderStartsOfflineThenInternet(t *testing.T) {
	if NetworkAccesses[0] != NetworkOffline {
		t.Fatalf("first network access = %q, want %q", NetworkAccesses[0], NetworkOffline)
	}
	if NetworkAccesses[1] != NetworkInternet {
		t.Fatalf("second network access = %q, want %q", NetworkAccesses[1], NetworkInternet)
	}
}

func indexString(values []string, value string) int {
	for i, existing := range values {
		if existing == value {
			return i
		}
	}
	return 0
}
