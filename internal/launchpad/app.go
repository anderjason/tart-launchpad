package launchpad

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type App struct {
	cfg          Config
	cfgPath      string
	tart         Tart
	host         HostEnvironment
	volumeLister VolumeLister
}

func NewApp(cfg Config, cfgPath string, tart Tart) (*App, error) {
	cfg.Normalize()
	return &App{cfg: cfg, cfgPath: cfgPath, tart: tart, host: RealHostEnvironment{}, volumeLister: RealVolumeLister{}}, nil
}

func (a *App) Run() error {
	initialModel := newModelWithVolumesAndHost(a.cfg, a.cfgPath, nil, nil, a.host)
	initialModel.tart = a.tart
	initialModel.volumeLister = a.volumeLister
	initialModel.loadingVMs = true
	initialModel.loadingVolumes = true
	program := tea.NewProgram(initialModel, tea.WithAltScreen())
	final, err := program.Run()
	if err != nil {
		return err
	}
	if m, ok := final.(model); ok {
		if m.needsSave {
			if err := SaveConfig(m.cfgPath, m.cfg); err != nil {
				return err
			}
		}
	}
	return nil
}

type screen int

const (
	screenHome screen = iota
	screenVMAction
	screenTemplateAction
	screenRunDuration
	screenName
	screenFolder
	screenProjectFolderPath
	screenNetwork
	screenLANCIDR
	screenLANCIDRText
	screenVolumes
	screenImportPath
	screenExportPath
	screenReview
	screenExecute
	screenMessage
	screenMarkKind
)

type flow int

const (
	flowRunExisting flow = iota
	flowNewFromTemplate
	flowRunTemplateReadOnly
	flowExportVM
	flowImportArchive
)

type nameMode int

const (
	nameModeNewVM nameMode = iota
	nameModeRenameVM
	nameModeImportVM
)

type navigationEntry struct {
	screen screen
	cursor int
}

type homeItem struct {
	vm    VM
	index int
}

type vmsLoadedMsg []VM

type hostVolumesLoadedMsg []HostVolume

type dataLoadFailedMsg struct {
	err error
}

type executeStepFinishedMsg struct {
	index  int
	output string
	err    error
}

type homePollMsg struct{}
type executeSecondMsg struct{}

type softnetStatusMsg struct {
	ready bool
}

type executeStepState int

const (
	executeStepPending executeStepState = iota
	executeStepActive
	executeStepDone
	executeStepFailed
	executeStepSkipped
)

type model struct {
	cfg            Config
	cfgPath        string
	vms            []VM
	screen         screen
	flow           flow
	cursor         int
	width          int
	height         int
	message        string
	status         string
	needsSave      bool
	keysOverlay    bool
	mode           inputMode
	homeFilter     string
	homeFocusVM    string
	loadingVMs     bool
	loadingVolumes bool
	spinner        spinner.Model
	progress       progress.Model
	theme          theme

	// Host readiness is sampled once at startup instead of during View, so no
	// render path shells out while a boundary is being chosen.
	softnetIsReady     bool
	softnetStatusKnown bool

	backStack    []navigationEntry
	returnScreen screen

	selectedVM            VM
	selectedTemplate      VM
	runDuration           RunDuration
	nameMode              nameMode
	nameInput             string
	nameCursor            int
	lanInput              string
	lanCursor             int
	lanCIDRChoices        []lanCIDRChoice
	folderAccess          FolderAccess
	projectPath           string
	projectPathInput      string
	projectPathCursor     int
	networkAccess         NetworkAccess
	clipboard             bool
	guestAudio            bool
	hostVolumes           []HostVolume
	host                  HostEnvironment
	tart                  Tart
	volumeLister          VolumeLister
	selectedVolumePath    map[string]bool
	importSourcePath      string
	importPathCursor      int
	exportDestinationPath string
	exportPathCursor      int
	plan                  Plan
	reviewScroll          int
	deleteConfirmInput    string
	executeIndex          int
	executeStates         []executeStepState
	executeOutput         string
	executeErr            string
	executeDone           bool
	executeSummary        string
	temporaryVMExists     bool
	executeStartedAt      time.Time
	executeFinishedAt     time.Time
	stepStartedAt         []time.Time
	stepFinishedAt        []time.Time
	missingRunVolumes     []string
}

func newModel(cfg Config, cfgPath string, vms []VM) model {
	return newModelWithVolumes(cfg, cfgPath, vms, nil)
}

func newModelWithVolumes(cfg Config, cfgPath string, vms []VM, volumes []HostVolume) model {
	return newModelWithVolumesAndHost(cfg, cfgPath, vms, volumes, RealHostEnvironment{})
}

func newModelWithVolumesAndHost(cfg Config, cfgPath string, vms []VM, volumes []HostVolume, host HostEnvironment) model {
	if host == nil {
		host = RealHostEnvironment{}
	}
	appTheme := newTheme(cfg)
	spin := spinner.New(spinner.WithSpinner(appTheme.spinnerFrames()))
	bar := progress.New(progress.WithSolidFill("#5FD7D7"), progress.WithoutPercentage())
	return model{
		cfg:                cfg,
		cfgPath:            cfgPath,
		vms:                vms,
		hostVolumes:        volumes,
		host:               host,
		spinner:            spin,
		progress:           bar,
		theme:              appTheme,
		selectedVolumePath: map[string]bool{},
		screen:             screenHome,
		mode:               modeNormal,
		folderAccess:       FolderNoFolder,
		networkAccess:      NetworkOffline,
	}
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.loadDataCmd(), m.softnetStatusCmd(), m.homePollCmd()}
	if !m.cfg.Defaults.ReducedMotion {
		cmds = append(cmds, m.spinner.Tick)
	}
	return tea.Batch(cmds...)
}

// softnetStatusCmd samples Softnet readiness off the render path. Launchpad
// only reports what it finds: it never runs sudo and never downgrades a
// chosen network mode on the user's behalf.
func (m model) softnetStatusCmd() tea.Cmd {
	return func() tea.Msg {
		path, err := exec.LookPath("softnet")
		if err != nil {
			return softnetStatusMsg{ready: false}
		}
		return softnetStatusMsg{ready: softnetReady(path)}
	}
}

func (m model) loadDataCmd() tea.Cmd {
	return tea.Batch(m.loadVMsCmd(), m.loadVolumesCmd())
}

func (m model) loadVMsCmd() tea.Cmd {
	if m.tart != nil {
		tart := m.tart
		cfg := m.cfg
		cfg.VMs = cloneVMConfigs(m.cfg.VMs)
		return func() tea.Msg {
			vms, err := tart.ListVMs()
			if err != nil {
				return dataLoadFailedMsg{err: err}
			}
			return vmsLoadedMsg(MergeKinds(cfg, vms))
		}
	}
	return nil
}

func (m model) loadVolumesCmd() tea.Cmd {
	if m.volumeLister != nil {
		lister := m.volumeLister
		return func() tea.Msg {
			volumes, err := lister.ListHostVolumes()
			if err != nil {
				return dataLoadFailedMsg{err: err}
			}
			return hostVolumesLoadedMsg(volumes)
		}
	}
	return nil
}

func (m model) homePollCmd() tea.Cmd {
	if m.tart == nil {
		return nil
	}
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg { return homePollMsg{} })
}

func (m model) executeSecondCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return executeSecondMsg{} })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case vmsLoadedMsg:
		m.vms = MergeKinds(m.cfg, []VM(msg))
		if m.reconcileCompletedCleanup() {
			m.needsSave = true
		}
		m.loadingVMs = false
		if m.homeFocusVM != "" && m.focusHomeVM(m.homeFocusVM) {
			m.homeFocusVM = ""
		}
		m.cursor = clampCursor(m.cursor, len(m.filteredHomeItems()))
	case hostVolumesLoadedMsg:
		m.hostVolumes = []HostVolume(msg)
		m.loadingVolumes = false
	case dataLoadFailedMsg:
		m.loadingVMs = false
		m.loadingVolumes = false
		return m.showMessage(msg.err.Error(), screenHome), nil
	case executeStepFinishedMsg:
		return m.handleExecuteStepFinished(msg)
	case homePollMsg:
		if m.screen == screenHome && m.tart != nil {
			m.loadingVMs = true
			return m, tea.Batch(m.loadVMsCmd(), m.homePollCmd())
		}
		return m, m.homePollCmd()
	case executeSecondMsg:
		if m.screen == screenExecute && !m.executeDone {
			return m, m.executeSecondCmd()
		}
		return m, nil
	case softnetStatusMsg:
		m.softnetIsReady = msg.ready
		m.softnetStatusKnown = true
	case spinner.TickMsg:
		// Reduced motion keeps the frame static: the step list still reports
		// state, it just does not animate.
		if m.cfg.Defaults.ReducedMotion {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.reviewScroll = min(m.reviewScroll, m.reviewMaxScroll())
	case tea.KeyMsg:
		if m.status != "" && msg.String() != "c" {
			m.status = ""
		}
		// ctrl+c always quits. Plain q only quits from the fleet list in normal
		// mode, so it stays typeable inside any prompt or filter.
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			if m.screen == screenHome && m.mode == modeNormal && !m.keysOverlay {
				return m, tea.Quit
			}
		}
		return m.handleKey(msg.String())
	}
	return m, nil
}

func (m model) handleKey(key string) (tea.Model, tea.Cmd) {
	// The keys overlay owns the whole body while it is open, so it also owns
	// the keyboard: nothing reaches the screen underneath until it closes.
	if m.keysOverlay {
		switch key {
		case "?", "esc", "enter", "backspace", "q":
			m.keysOverlay = false
		}
		return m, nil
	}
	// A normal-mode "?" is always the keys overlay. In a text-capturing mode it
	// is just a character, which is why this check comes after the overlay.
	if key == "?" && m.mode == modeNormal {
		m.keysOverlay = true
		return m, nil
	}
	switch m.screen {
	case screenHome:
		return m.handleHomeKey(key)
	case screenVMAction:
		return m.handleVMActionKey(key)
	case screenTemplateAction:
		return m.handleTemplateActionKey(key)
	case screenRunDuration:
		return m.handleChoiceKey(key, len(RunDurations), func(m model) model {
			m.runDuration = RunDurations[m.cursor]
			if m.runDuration == RunDurationTemporaryRun {
				m.nameInput = "tmp-" + time.Now().Format("20060102-1504")
			} else {
				m.nameInput = ""
			}
			m.nameCursor = runeCount(m.nameInput)
			m.nameMode = nameModeNewVM
			m.message = ""
			return m.goToWithCursor(screenName, 0)
		})
	case screenName:
		return m.handleNameKey(key)
	case screenFolder:
		return m.handleHostConnectionsKey(key)
	case screenProjectFolderPath:
		return m.handleProjectFolderPathKey(key)
	case screenNetwork:
		return m.handleChoiceKey(key, len(NetworkAccesses), func(m model) model {
			m.networkAccess = NetworkAccesses[m.cursor]
			if m.networkNeedsCIDR() && len(m.cfg.Network.LANCIDRs) == 0 {
				m.lanCIDRChoices = defaultLANCIDRChoices()
				m.lanInput = ""
				m.lanCursor = 0
				m.message = ""
				return m.goToWithCursor(screenLANCIDR, 0)
			}
			return m.advanceToVolumesOrReviewModel()
		})
	case screenLANCIDR:
		return m.handleLANCIDRKey(key)
	case screenLANCIDRText:
		return m.handleLANCIDRTextKey(key)
	case screenVolumes:
		return m.handleVolumesKey(key)
	case screenImportPath:
		return m.handleImportPathKey(key)
	case screenExportPath:
		return m.handleExportPathKey(key)
	case screenReview:
		return m.handleReviewKey(key)
	case screenExecute:
		return m.handleExecuteKey(key)
	case screenMessage:
		if key == "enter" || key == "esc" || key == "backspace" {
			m.message = ""
			m.screen = m.returnScreen
			m.mode = m.modeFor(m.returnScreen)
			if m.screen == screenHome {
				m = m.returnHome()
			}
		}
	case screenMarkKind:
		return m.handleMarkKindKey(key)
	}
	return m, nil
}

func (m model) handleHomeKey(key string) (tea.Model, tea.Cmd) {
	items := m.filteredHomeItems()
	m.cursor = clampCursor(m.cursor, len(items))
	if m.mode == modeFilter {
		return m.handleHomeFilterKey(key)
	}
	if cursor, handled := navigateList(m.cursor, len(items), key); handled {
		m.cursor = cursor
		return m, nil
	}
	switch key {
	case "/":
		m.mode = modeFilter
		m.homeFilter = ""
		m.cursor = 0
	case "esc":
		if m.homeFilter != "" {
			m.homeFilter = ""
			m.cursor = 0
		}
	case "r":
		m.loadingVMs = true
		m.loadingVolumes = true
		return m, m.loadDataCmd()
	case "c":
		if len(m.cfg.PendingCleanup) == 0 {
			return m, nil
		}
		name := m.cfg.PendingCleanup[0]
		if m.focusHomeVM(name) {
			m.status = "verify " + name + " before deleting it; cleanup identity is ambiguous"
			return m, nil
		}
		m.cfg.RemovePendingCleanup(name)
		m.needsSave = true
		m.status = "cleanup already complete for " + name
	case ".":
		if len(items) == 0 {
			return m, nil
		}
		item := items[m.cursor]
		last, ok := m.cfg.LastRunFor(item.vm.Name)
		if !ok {
			m.status = "no last run for " + item.vm.Name
			return m, nil
		}
		m.selectedVM = item.vm
		nextFlow := flowRunExisting
		if item.vm.Kind == VMKindTemplate {
			nextFlow = flowRunTemplateReadOnly
		}
		m = m.beginRunFlow(nextFlow)
		m.folderAccess = last.FolderAccess
		m.projectPath = last.ProjectPath
		m.projectPathInput = last.ProjectPath
		m.projectPathCursor = runeCount(last.ProjectPath)
		m.networkAccess = last.NetworkAccess
		m.clipboard = last.Clipboard
		m.guestAudio = last.GuestAudio
		m.applyLastRunVolumes(last.VolumePaths)
		if err := m.preparePlan(); err != nil {
			return m.showMessage(err.Error(), screenHome), nil
		}
		for _, missing := range m.missingRunVolumes {
			m.plan.Warnings = append(m.plan.Warnings, "volume "+missing+" not mounted - removed from this run")
		}
		m = m.goToWithCursor(screenReview, 0)
	case "m":
		if len(items) == 0 {
			return m, nil
		}
		item := items[m.cursor]
		vm := item.vm
		next := nextKind(vm.Kind)
		m.cfg.SetKind(vm.Name, next)
		m.vms[item.index].Kind = next
		m.needsSave = true
		m.status = fmt.Sprintf("%s -> %s", vm.Name, next)
	case "n":
		templates := m.templates()
		if len(templates) == 0 {
			return m.showMessage("Mark a VM as template first with m.", screenHome), nil
		}
		selected := VM{}
		if len(items) > 0 && items[m.cursor].vm.Kind == VMKindTemplate {
			selected = items[m.cursor].vm
		} else if len(templates) == 1 {
			selected = templates[0]
		} else {
			m.status = "select a template before creating a VM"
			return m, nil
		}
		m = m.beginRunFlow(flowNewFromTemplate)
		m.selectedTemplate = selected
		m = m.goToWithCursor(screenRunDuration, 0)
	case "i":
		m.flow = flowImportArchive
		m.importSourcePath = ""
		m.importPathCursor = 0
		m.message = ""
		m = m.goToWithCursor(screenImportPath, 0)
	case "enter", "l", "right":
		if len(items) == 0 {
			return m, nil
		}
		vm := items[m.cursor].vm
		m.selectedVM = vm
		if vm.Kind == VMKindTemplate {
			m = m.goToWithCursor(screenTemplateAction, 0)
			return m, nil
		}
		m = m.goToWithCursor(screenVMAction, 0)
	}
	return m, nil
}

// handleHomeFilterKey runs while FILTER mode owns the keyboard: every
// printable rune narrows the fleet list instead of triggering an action.
func (m model) handleHomeFilterKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "enter":
		m.mode = modeNormal
	case "esc":
		m.mode = modeNormal
		m.homeFilter = ""
		m.cursor = 0
	case "up", "down":
		cursor, _ := navigateList(m.cursor, len(m.filteredHomeItems()), key)
		m.cursor = cursor
	default:
		value, _, handled := editTextField(m.homeFilter, runeCount(m.homeFilter), key, anyPrintableRune)
		if handled {
			m.homeFilter = value
			m.cursor = 0
		}
	}
	m.cursor = clampCursor(m.cursor, len(m.filteredHomeItems()))
	return m, nil
}

func (m model) handleVMActionKey(key string) (tea.Model, tea.Cmd) {
	m.cursor = clampCursor(m.cursor, len(vmActions()))
	if cursor, handled := navigateList(m.cursor, len(vmActions()), key); handled {
		m.cursor = cursor
		return m, nil
	}
	switch key {
	case "esc", "backspace", "h", "left":
		m = m.goBack()
	case "enter", "l", "right":
		switch vmActions()[m.cursor] {
		case "run":
			m = m.beginRunFlow(flowRunExisting)
			m = m.goToWithCursor(screenFolder, indexFolder(m.folderAccess))
		case "export":
			m.flow = flowExportVM
			m.exportDestinationPath = ""
			m.exportPathCursor = 0
			m.message = ""
			m = m.goToWithCursor(screenExportPath, 0)
		case "rename":
			m.nameMode = nameModeRenameVM
			m.nameInput = m.selectedVM.Name
			m.nameCursor = runeCount(m.nameInput)
			m.message = ""
			m = m.goToWithCursor(screenName, 0)
		case "delete":
			if err := m.prepareDeletePlan(); err != nil {
				return m.showMessage(err.Error(), screenVMAction), nil
			}
			m = m.goToWithCursor(screenReview, 0)
		case "mark kind":
			m = m.goToWithCursor(screenMarkKind, indexVMKind(m.selectedVM.Kind))
		}
	}
	return m, nil
}

func (m model) handleTemplateActionKey(key string) (tea.Model, tea.Cmd) {
	m.cursor = clampCursor(m.cursor, len(templateActions()))
	if cursor, handled := navigateList(m.cursor, len(templateActions()), key); handled {
		m.cursor = cursor
		return m, nil
	}
	switch key {
	case "esc", "backspace", "h", "left":
		m = m.goBack()
	case "enter", "l", "right":
		switch templateActions()[m.cursor] {
		case "new temporary run":
			m = m.beginRunFlow(flowNewFromTemplate)
			m.selectedTemplate = m.selectedVM
			m.runDuration = RunDurationTemporaryRun
			m.nameMode = nameModeNewVM
			m.nameInput = "tmp-" + time.Now().Format("20060102-1504")
			m.nameCursor = runeCount(m.nameInput)
			m.message = ""
			m = m.goToWithCursor(screenName, 0)
		case "new workspace":
			m = m.beginRunFlow(flowNewFromTemplate)
			m.selectedTemplate = m.selectedVM
			m.runDuration = RunDurationWorkspace
			m.nameMode = nameModeNewVM
			m.nameInput = ""
			m.nameCursor = 0
			m.message = ""
			m = m.goToWithCursor(screenName, 0)
		case "run read-only":
			m = m.beginRunFlow(flowRunTemplateReadOnly)
			m = m.goToWithCursor(screenFolder, indexFolder(m.folderAccess))
		case "export":
			m.flow = flowExportVM
			m.exportDestinationPath = ""
			m.exportPathCursor = 0
			m.message = ""
			m = m.goToWithCursor(screenExportPath, 0)
		case "rename":
			m.nameMode = nameModeRenameVM
			m.nameInput = m.selectedVM.Name
			m.nameCursor = runeCount(m.nameInput)
			m.message = ""
			m = m.goToWithCursor(screenName, 0)
		case "delete":
			if err := m.prepareDeletePlan(); err != nil {
				return m.showMessage(err.Error(), screenTemplateAction), nil
			}
			m = m.goToWithCursor(screenReview, 0)
		case "mark kind":
			m = m.goToWithCursor(screenMarkKind, indexVMKind(m.selectedVM.Kind))
		}
	}
	return m, nil
}

func (m model) handleChoiceKey(key string, count int, selectFn func(model) model) (tea.Model, tea.Cmd) {
	m.cursor = clampCursor(m.cursor, count)
	if cursor, handled := navigateList(m.cursor, count, key); handled {
		m.cursor = cursor
		return m, nil
	}
	switch key {
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		index := int(key[0] - '1')
		if index >= 0 && index < count {
			m.cursor = index
			m = selectFn(m)
		}
	case "enter", "l", "right":
		m = selectFn(m)
	case "esc", "backspace", "h", "left":
		m = m.goBack()
	}
	return m, nil
}

func (m model) handleHostConnectionsKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "c":
		m.clipboard = !m.clipboard
		return m, nil
	case "a":
		m.guestAudio = !m.guestAudio
		return m, nil
	case "p":
		m.projectPathInput = m.selectedProjectPath()
		m.projectPathCursor = runeCount(m.projectPathInput)
		m.message = ""
		m = m.goToWithCursor(screenProjectFolderPath, 0)
		return m, nil
	}
	return m.handleChoiceKey(key, len(FolderAccesses), func(m model) model {
		m.folderAccess = FolderAccesses[m.cursor]
		return m.goToWithCursor(screenNetwork, indexNetwork(m.networkAccess))
	})
}

func (m model) handleProjectFolderPathKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "enter":
		resolved, err := m.host.ResolveProjectDirectory(m.projectPathInput)
		if err != nil {
			m.message = err.Error()
			return m, nil
		}
		m.projectPath = resolved
		m.projectPathInput = resolved
		m.projectPathCursor = runeCount(resolved)
		m.message = ""
		m = m.goBack()
		return m, nil
	case "esc":
		m = m.goBack()
		return m, nil
	}
	value, cursor, handled := editTextField(m.projectPathInput, m.projectPathCursor, key, anyPrintableRune)
	if handled {
		m.projectPathInput = value
		m.projectPathCursor = cursor
		m.message = ""
	}
	return m, nil
}

// handleNameKey runs in INSERT mode: enter commits, esc leaves, and every
// other printable key is part of the VM name.
func (m model) handleNameKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "enter":
		if strings.TrimSpace(m.nameInput) == "" {
			m.message = "Name is required."
			return m, nil
		}
		switch m.nameMode {
		case nameModeRenameVM:
			if err := m.prepareRenamePlan(); err != nil {
				m.message = err.Error()
				return m, nil
			}
			m.message = ""
			m = m.goToWithCursor(screenReview, 0)
		case nameModeImportVM:
			if err := m.prepareImportPlan(); err != nil {
				m.message = err.Error()
				return m, nil
			}
			m.message = ""
			m = m.goToWithCursor(screenReview, 0)
		default:
			m.message = ""
			m = m.goToWithCursor(screenFolder, indexFolder(m.folderAccess))
		}
		return m, nil
	case "esc":
		m = m.goBack()
		return m, nil
	}
	value, cursor, handled := editTextField(m.nameInput, m.nameCursor, key, isNameRune)
	if handled {
		m.nameInput = value
		m.nameCursor = cursor
		m.message = ""
	}
	return m, nil
}

func (m model) handleImportPathKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "enter":
		path := strings.TrimSpace(m.importSourcePath)
		if path == "" {
			m.message = "Path is required."
			return m, nil
		}
		if filepath.Ext(path) != vmArchiveExtension {
			m.message = "Import path must end in .tvm."
			return m, nil
		}
		m.importSourcePath = path
		m.nameMode = nameModeImportVM
		m.nameInput = strings.TrimSuffix(filepath.Base(path), vmArchiveExtension)
		m.nameCursor = runeCount(m.nameInput)
		m.message = ""
		m = m.goToWithCursor(screenName, 0)
	case "esc":
		m = m.goBack()
		return m, nil
	}
	value, cursor, handled := editTextField(m.importSourcePath, m.importPathCursor, key, anyPrintableRune)
	if handled {
		m.importSourcePath = value
		m.importPathCursor = cursor
		m.message = ""
	}
	return m, nil
}

func (m model) handleExportPathKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "enter":
		path := strings.TrimSpace(m.exportDestinationPath)
		if path == "" {
			m.message = "Path is required."
			return m, nil
		}
		if filepath.Ext(path) != vmArchiveExtension {
			m.message = "Export path must end in .tvm."
			return m, nil
		}
		m.exportDestinationPath = path
		if err := m.prepareExportPlan(); err != nil {
			m.message = err.Error()
			return m, nil
		}
		m.message = ""
		m = m.goToWithCursor(screenReview, 0)
		return m, nil
	case "esc":
		m = m.goBack()
		return m, nil
	}
	value, cursor, handled := editTextField(m.exportDestinationPath, m.exportPathCursor, key, anyPrintableRune)
	if handled {
		m.exportDestinationPath = value
		m.exportPathCursor = cursor
		m.message = ""
	}
	return m, nil
}

func (m model) handleLANCIDRKey(key string) (tea.Model, tea.Cmd) {
	if len(m.lanCIDRChoices) == 0 {
		m.lanCIDRChoices = defaultLANCIDRChoices()
	}
	m.cursor = clampCursor(m.cursor, len(m.lanCIDRChoices))
	if cursor, handled := navigateList(m.cursor, len(m.lanCIDRChoices), key); handled {
		m.cursor = cursor
		return m, nil
	}
	switch key {
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		index := int(key[0] - '1')
		if index >= 0 && index < len(m.lanCIDRChoices) {
			m.cursor = index
			return m.handleLANCIDRKey("enter")
		}
	case "enter", "l", "right":
		choice := m.lanCIDRChoices[m.cursor]
		if choice.FreeText {
			m.lanInput = ""
			m.lanCursor = 0
			m.message = ""
			m = m.goToWithCursor(screenLANCIDRText, 0)
			return m, nil
		}
		return m.addLANCIDRAndReview(choice.CIDR)
	case "esc", "backspace", "h", "left":
		m = m.goBack()
	}
	return m, nil
}

func (m model) handleLANCIDRTextKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "enter":
		cidr, err := normalizePrivateIPv4CIDR(m.lanInput)
		if err != nil {
			m.message = err.Error()
			return m, nil
		}
		return m.addLANCIDRAndReview(cidr)
	case "esc":
		m = m.goBack()
		return m, nil
	}
	value, cursor, handled := editTextField(m.lanInput, m.lanCursor, key, isCIDRRune)
	if handled {
		m.lanInput = value
		m.lanCursor = cursor
		m.message = ""
	}
	return m, nil
}

func (m model) addLANCIDRAndReview(cidr string) (tea.Model, tea.Cmd) {
	if m.cfg.AddLANCIDR(cidr) {
		m.needsSave = true
		m.status = fmt.Sprintf("LAN CIDR %s saved", cidr)
	}
	return m.advanceToVolumesOrReviewModel(), nil
}

func (m model) advanceToVolumesOrReviewModel() model {
	if len(m.hostVolumes) > 0 {
		return m.goToWithCursor(screenVolumes, 0)
	}
	if err := m.preparePlan(); err != nil {
		return m.showMessage(err.Error(), m.screen)
	}
	return m.goToWithCursor(screenReview, 0)
}

func (m model) handleVolumesKey(key string) (tea.Model, tea.Cmd) {
	m.cursor = clampCursor(m.cursor, len(m.hostVolumes))
	if cursor, handled := navigateList(m.cursor, len(m.hostVolumes), key); handled {
		m.cursor = cursor
		return m, nil
	}
	switch key {
	case " ":
		if len(m.hostVolumes) > 0 {
			if m.selectedVolumePath == nil {
				m.selectedVolumePath = map[string]bool{}
			}
			path := m.hostVolumes[m.cursor].Path
			m.selectedVolumePath[path] = !m.selectedVolumePath[path]
		}
	case "enter", "l", "right":
		if err := m.preparePlan(); err != nil {
			return m.showMessage(err.Error(), screenVolumes), nil
		}
		m = m.goToWithCursor(screenReview, 0)
	case "esc", "backspace", "h", "left":
		m = m.goBack()
	}
	return m, nil
}

func (m model) handleMarkKindKey(key string) (tea.Model, tea.Cmd) {
	kinds := []VMKind{VMKindTemplate, VMKindWorkspace}
	m.cursor = clampCursor(m.cursor, len(kinds))
	if cursor, handled := navigateList(m.cursor, len(kinds), key); handled {
		m.cursor = cursor
		return m, nil
	}
	switch key {
	case "1", "2":
		index := int(key[0] - '1')
		if index >= 0 && index < len(kinds) {
			m.cursor = index
			return m.handleMarkKindKey("enter")
		}
	case "enter", "l", "right":
		m = m.setSelectedVMKind(kinds[m.cursor])
		m.status = fmt.Sprintf("%s -> %s", m.selectedVM.Name, kinds[m.cursor])
		m = m.returnHomeToVM(m.selectedVM.Name)
	case "esc", "backspace", "h", "left":
		m = m.goBack()
	}
	return m, nil
}

func (m model) handleReviewKey(key string) (tea.Model, tea.Cmd) {
	// Deleting a template is the one action that captures the keyboard: the
	// typed name is the confirmation, so scroll and shortcut keys stand down.
	if m.planRequiresTypedConfirm() {
		return m.handleReviewConfirmKey(key)
	}
	switch key {
	case "up", "k":
		if m.reviewScroll > 0 {
			m.reviewScroll--
		}
	case "down", "j":
		if m.reviewScroll < m.reviewMaxScroll() {
			m.reviewScroll++
		}
	case "g":
		m.reviewScroll = 0
	case "c":
		m.status = "command copied"
		return m, copyTextCommand(planCommands(m.plan))
	case "enter":
		if m.planRequiresExplicitYes() {
			return m, nil
		}
		return m.confirmReview()
	case "y":
		return m.beginExecution()
	case "esc", "n", "backspace", "h", "left":
		m = m.goBack()
	}
	return m, nil
}

func (m model) handleReviewConfirmKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "enter":
		if m.deleteConfirmInput == m.plan.DeleteVM {
			return m.beginExecution()
		}
		return m, nil
	case "esc":
		m = m.goBack()
		return m, nil
	}
	// No shortcut steals a printable key here: every VM name character has to
	// be typeable, including c, y, and n.
	value, _, handled := editTextField(m.deleteConfirmInput, runeCount(m.deleteConfirmInput), key, anyPrintableRune)
	if handled {
		m.deleteConfirmInput = value
	}
	return m, nil
}

func (m model) confirmReview() (tea.Model, tea.Cmd) {
	return m.beginExecution()
}

func (m model) handleExecuteKey(key string) (tea.Model, tea.Cmd) {
	if !m.executeDone {
		return m, nil
	}
	switch key {
	case "enter":
		target := m.executionReturnName()
		m = m.returnHomeToVM(target)
		m.loadingVMs = true
		m.loadingVolumes = true
		return m, m.loadDataCmd()
	}
	return m, nil
}

func (m model) beginExecution() (tea.Model, tea.Cmd) {
	interactive := m.screen == screenReview
	if err := checkPlanPrerequisites(m.plan); err != nil {
		m.executeStates = make([]executeStepState, len(m.plan.Steps))
		m.executeErr = err.Error()
		m.executeDone = true
		m.screen = screenExecute
		m.mode = modeNormal
		return m, nil
	}
	m.executeIndex = 0
	m.executeStates = make([]executeStepState, len(m.plan.Steps))
	m.stepStartedAt = make([]time.Time, len(m.plan.Steps))
	m.stepFinishedAt = make([]time.Time, len(m.plan.Steps))
	now := time.Now()
	m.executeStartedAt = now
	m.executeFinishedAt = time.Time{}
	if len(m.executeStates) > 0 {
		m.executeStates[0] = executeStepActive
		m.stepStartedAt[0] = now
	}
	m.executeOutput = ""
	m.executeErr = ""
	m.executeDone = len(m.plan.Steps) == 0
	m.executeSummary = ""
	m.temporaryVMExists = false
	m.screen = screenExecute
	m.mode = modeNormal
	cmd := m.executeCurrentStepCmd()
	if interactive {
		cmd = tea.Batch(cmd, m.executeSecondCmd())
	}
	return m, cmd
}

func (m model) executeCurrentStepCmd() tea.Cmd {
	if m.executeIndex >= len(m.plan.Steps) || m.tart == nil {
		return nil
	}
	index := m.executeIndex
	step := m.plan.Steps[index]
	tart := m.tart
	if step.Kind == CommandStepRun && (m.plan.Review.FolderAccess == FolderReadFolder || m.plan.Review.FolderAccess == FolderEditFolder) {
		resolved, err := m.host.ResolveProjectDirectory(m.plan.Review.ProjectPath)
		if err != nil {
			return func() tea.Msg { return executeStepFinishedMsg{index: index, err: err} }
		}
		if resolved != m.plan.Review.ProjectPath {
			err := fmt.Errorf("project folder changed after review: %s", m.plan.Review.ProjectPath)
			return func() tea.Msg { return executeStepFinishedMsg{index: index, err: err} }
		}
	}
	if step.Kind == CommandStepRun && len(m.plan.Review.VolumePaths) > 0 {
		if m.volumeLister == nil {
			err := fmt.Errorf("mounted-volume validation is unavailable")
			return func() tea.Msg { return executeStepFinishedMsg{index: index, err: err} }
		}
		mounted, err := m.volumeLister.ListHostVolumes()
		if err != nil {
			return func() tea.Msg { return executeStepFinishedMsg{index: index, err: err} }
		}
		if err := verifyReviewedVolumes(m.plan.Review.VolumePaths, m.plan.Review.VolumeIDs, mounted); err != nil {
			return func() tea.Msg { return executeStepFinishedMsg{index: index, err: err} }
		}
	}
	if step.Kind == CommandStepRun && usesRealTart(tart) {
		cmd := exec.Command(step.Args[0], step.Args[1:]...)
		return tea.ExecProcess(cmd, func(err error) tea.Msg {
			return executeStepFinishedMsg{index: index, err: err}
		})
	}
	return func() tea.Msg {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		err := tart.RunStep(&stdout, &stderr, step)
		output := strings.TrimSpace(stdout.String() + stderr.String())
		return executeStepFinishedMsg{index: index, output: output, err: err}
	}
}

func usesRealTart(tart Tart) bool {
	switch tart.(type) {
	case RealTart, *RealTart:
		return true
	default:
		return false
	}
}

func (m *model) applyLastRunVolumes(paths []string) {
	mounted := map[string]bool{}
	for _, volume := range m.hostVolumes {
		mounted[volume.Path] = true
	}
	for _, path := range paths {
		if mounted[path] {
			m.selectedVolumePath[path] = true
			continue
		}
		m.missingRunVolumes = append(m.missingRunVolumes, path)
	}
}

func (m model) handleExecuteStepFinished(msg executeStepFinishedMsg) (tea.Model, tea.Cmd) {
	if msg.index < 0 || msg.index >= len(m.executeStates) {
		return m, nil
	}
	if msg.output != "" {
		m.executeOutput = msg.output
	}
	step := m.plan.Steps[msg.index]
	if msg.index < len(m.stepFinishedAt) {
		m.stepFinishedAt[msg.index] = time.Now()
	}
	if msg.err != nil {
		m.executeStates[msg.index] = executeStepFailed
		m.markSkippedAfter(msg.index)
		err := fmt.Errorf("%s: %w", step.Label, msg.err)
		if cleanupErr := m.recordPendingCleanupAfterFailure(); cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
		}
		m.executeErr = err.Error()
		m.executeDone = true
		m.executeFinishedAt = time.Now()
		return m, nil
	}
	if err := m.applySuccessfulStepEffects(step); err != nil {
		m.executeStates[msg.index] = executeStepFailed
		m.markSkippedAfter(msg.index)
		m.executeErr = err.Error()
		m.executeDone = true
		m.executeFinishedAt = time.Now()
		return m, nil
	}
	m.executeStates[msg.index] = executeStepDone
	m.executeIndex = msg.index + 1
	if m.executeIndex >= len(m.plan.Steps) {
		if err := m.applyCompletedPlanEffects(); err != nil {
			m.executeErr = err.Error()
		} else {
			m.executeSummary = m.completedExecutionSummary()
		}
		m.executeDone = true
		m.executeFinishedAt = time.Now()
		return m, nil
	}
	m.executeStates[m.executeIndex] = executeStepActive
	if m.executeIndex < len(m.stepStartedAt) {
		m.stepStartedAt[m.executeIndex] = time.Now()
	}
	return m, m.executeCurrentStepCmd()
}

func (m *model) applySuccessfulStepEffects(step CommandStep) error {
	if m.plan.HasTemporaryVM && step.Kind == CommandStepClone {
		m.temporaryVMExists = true
		m.cfg.AddPendingCleanup(m.plan.TemporaryVM)
		if err := SaveConfig(m.cfgPath, m.cfg); err != nil {
			return fmt.Errorf("save pending cleanup: %w", err)
		}
	}
	if m.plan.HasTemporaryVM && step.Kind == CommandStepDeleteTemporaryVM {
		m.cfg.RemovePendingCleanup(m.plan.TemporaryVM)
		m.temporaryVMExists = false
		if err := SaveConfig(m.cfgPath, m.cfg); err != nil {
			return fmt.Errorf("save completed temporary cleanup: %w", err)
		}
	}
	return nil
}

func (m *model) recordPendingCleanupAfterFailure() error {
	if !m.plan.HasTemporaryVM || !m.temporaryVMExists {
		return nil
	}
	m.cfg.AddPendingCleanup(m.plan.TemporaryVM)
	if err := SaveConfig(m.cfgPath, m.cfg); err != nil {
		return fmt.Errorf("save pending cleanup: %w", err)
	}
	return nil
}

func (m *model) applyCompletedPlanEffects() error {
	if m.plan.ShowsBoundaries() {
		name := m.executionReturnName()
		if name != "" {
			m.cfg.SetLastRun(name, LastRunConfig{
				FolderAccess:  m.plan.Review.FolderAccess,
				ProjectPath:   m.plan.Review.ProjectPath,
				NetworkAccess: m.plan.Review.NetworkAccess,
				VolumePaths:   append([]string(nil), m.plan.Review.VolumePaths...),
				Clipboard:     m.plan.Review.Clipboard,
				GuestAudio:    m.plan.Review.GuestAudio,
				At:            time.Now().Format(time.RFC3339),
			})
			if err := SaveConfig(m.cfgPath, m.cfg); err != nil {
				return fmt.Errorf("save last run grant: %w", err)
			}
		}
	}
	if m.plan.RenameFrom != "" && m.plan.RenameTo != "" {
		m.cfg.RenameVMKind(m.plan.RenameFrom, m.plan.RenameTo)
		if err := SaveConfig(m.cfgPath, m.cfg); err != nil {
			return fmt.Errorf("save renamed VM kind: %w", err)
		}
	}
	if m.plan.DeleteVM != "" {
		m.cfg.ForgetVM(m.plan.DeleteVM)
		if err := SaveConfig(m.cfgPath, m.cfg); err != nil {
			return fmt.Errorf("save deleted VM state: %w", err)
		}
	}
	return nil
}

func (m model) markSkippedAfter(index int) {
	for i := index + 1; i < len(m.executeStates); i++ {
		if m.executeStates[i] == executeStepPending {
			m.executeStates[i] = executeStepSkipped
		}
	}
}

func (m model) completedExecutionSummary() string {
	if m.plan.ExportPath != "" {
		return "Exported " + m.plan.ExportPath
	}
	if m.plan.ImportVMName != "" {
		return "Imported " + m.plan.ImportVMName
	}
	if m.plan.DeleteVM != "" {
		return "Deleted " + m.plan.DeleteVM
	}
	if m.plan.RenameTo != "" {
		return "Renamed " + m.plan.RenameTo
	}
	if m.plan.HasTemporaryVM {
		return fmt.Sprintf("Ran %s · temporary VM deleted · %d steps ✓", m.plan.TemporaryVM, len(m.plan.Steps))
	}
	return fmt.Sprintf("%s · %d steps ✓", m.plan.Title, len(m.plan.Steps))
}

func (m model) executionReturnName() string {
	if m.plan.RenameTo != "" {
		return m.plan.RenameTo
	}
	if m.plan.ImportVMName != "" {
		return m.plan.ImportVMName
	}
	if m.plan.DeleteVM != "" {
		return ""
	}
	if m.plan.HasTemporaryVM {
		return m.selectedTemplate.Name
	}
	if m.plan.CreatedVM != "" {
		return m.plan.CreatedVM
	}
	return m.selectedVM.Name
}

func (m *model) preparePlan() error {
	intent := m.launchpadIntent()
	var err error
	m.plan, err = intent.BuildPlan(m.cfg, m.host)
	return err
}

func (m model) launchpadIntent() LaunchpadIntent {
	intent := LaunchpadIntent{
		FolderAccess:  m.folderAccess,
		ProjectPath:   m.selectedProjectPath(),
		NetworkAccess: m.networkAccess,
		Clipboard:     m.clipboard,
		GuestAudio:    m.guestAudio,
		VolumePaths:   m.selectedVolumePaths(),
		VolumeIDs:     m.selectedVolumeIDs(),
		ExistingVMs:   m.vms,
	}
	switch m.flow {
	case flowRunExisting:
		intent.Kind = IntentRunExisting
		intent.VM = m.selectedVM
	case flowRunTemplateReadOnly:
		intent.Kind = IntentRunTemplateReadOnly
		intent.VM = m.selectedVM
	case flowNewFromTemplate:
		intent.Kind = IntentNewFromTemplate
		intent.Template = m.selectedTemplate
		intent.NameInput = m.nameInput
		intent.RunDuration = m.runDuration
	}
	return intent
}

func (m *model) prepareRenamePlan() error {
	var err error
	m.plan, err = LaunchpadIntent{
		Kind:      IntentRenameVM,
		VM:        m.selectedVM,
		NameInput: m.nameInput,
	}.BuildPlan(m.cfg, m.host)
	return err
}

func (m *model) prepareDeletePlan() error {
	var err error
	m.plan, err = LaunchpadIntent{
		Kind: IntentDeleteVM,
		VM:   m.selectedVM,
	}.BuildPlan(m.cfg, m.host)
	return err
}

func (m *model) prepareExportPlan() error {
	var err error
	m.plan, err = LaunchpadIntent{
		Kind:                  IntentExportVM,
		VM:                    m.selectedVM,
		ExportDestinationPath: m.exportDestinationPath,
	}.BuildPlan(m.cfg, m.host)
	return err
}

func (m *model) prepareImportPlan() error {
	var err error
	m.plan, err = LaunchpadIntent{
		Kind:             IntentImportArchive,
		ImportSourcePath: m.importSourcePath,
		NameInput:        m.nameInput,
		ExistingVMs:      m.vms,
	}.BuildPlan(m.cfg, m.host)
	return err
}

func (m model) View() string {
	content := m.render()
	if m.width <= 0 {
		return content
	}
	return lipgloss.Place(m.width, max(m.height, lipgloss.Height(content)), lipgloss.Left, lipgloss.Top, content)
}

func (m model) render() string {
	if m.keysOverlay {
		return m.renderKeysOverlay()
	}
	switch m.screen {
	case screenHome:
		return m.renderHome()
	case screenVMAction:
		return m.renderVMAction()
	case screenTemplateAction:
		return m.renderTemplateAction()
	case screenRunDuration:
		return m.renderRunDuration()
	case screenName:
		return m.renderName()
	case screenFolder:
		return m.renderFolder()
	case screenProjectFolderPath:
		return m.renderProjectFolderPath()
	case screenNetwork:
		return m.renderNetwork()
	case screenLANCIDR:
		return m.renderLANCIDR()
	case screenLANCIDRText:
		return m.renderLANCIDRText()
	case screenVolumes:
		return m.renderVolumes()
	case screenImportPath:
		return m.renderImportPath()
	case screenExportPath:
		return m.renderExportPath()
	case screenReview:
		return m.renderReview()
	case screenExecute:
		return m.renderExecute()
	case screenMessage:
		return m.renderMessage()
	case screenMarkKind:
		return m.renderMarkKind()
	default:
		return ""
	}
}

// renderHome lists the local Tart VMs as one scannable row per VM.
func (m model) renderHome() string {
	glyphs := m.glyphs()
	items := m.filteredHomeItems()
	cursorIndex := clampCursor(m.cursor, len(items))
	nameWidth := m.homeNameWidth()
	start, end := m.homeWindow(len(items))

	var lines []string
	if m.mode == modeFilter || m.homeFilter != "" {
		lines = append(lines, m.renderFilterRow(), "")
	}
	if start > 0 {
		lines = append(lines, mutedStyle.Render(glyphs.Up+" more"))
	}
	for i := start; i < end; i++ {
		if i == m.firstTemplateIndex(items) {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, sectionHeading("TEMPLATES"))
		}
		lines = append(lines, m.renderHomeRow(glyphs, items[i].vm, i == cursorIndex, nameWidth))
	}
	if end < len(items) {
		lines = append(lines, mutedStyle.Render(glyphs.Down+" more"))
	}
	switch {
	case len(m.vms) == 0 && m.loadingVMs:
		lines = append(lines, mutedStyle.Render("reading tart list…"))
	case len(m.vms) == 0:
		lines = append(lines,
			textStyle.Render("No local Tart VMs found."),
			mutedStyle.Render("i imports a .tvm from Desktop · or create one with tart"))
	case len(items) == 0:
		lines = append(lines, mutedStyle.Render("No VMs match the filter."))
	}

	return m.renderFrame(framePage{
		Trail: []string{"VMs"},
		Step:  m.homeCount(len(items)),
		Body:  strings.Join(lines, "\n"),
		Hints: m.homeHints(),
	})
}

func (m model) renderHomeRow(glyphs glyphSet, vm VM, selected bool, nameWidth int) string {
	running := mutedStyle.Render(glyphs.Idle)
	if vm.Running {
		running = successStyle.Render(glyphs.Running)
	}
	name := fixedDisplayWidth(truncate(vm.Name, nameWidth), nameWidth)
	if selected {
		name = selectedStyle.Render(name)
	}
	return strings.Join([]string{cursorCellFor(glyphs, selected) + running, name, mutedStyle.Render(vm.State)}, " ")
}

// renderFilterRow shows the live query while FILTER mode owns the keyboard.
func (m model) renderFilterRow() string {
	if m.mode != modeFilter {
		return mutedStyle.Render("filter ") + textStyle.Render(m.homeFilter) + mutedStyle.Render("  esc clears")
	}
	return mutedStyle.Render("filter ") + renderTextInputValue(m.homeFilter, runeCount(m.homeFilter))
}

func (m model) homeCount(shown int) string {
	if m.homeFilter != "" || m.mode == modeFilter {
		return fmt.Sprintf("%d of %d shown", shown, len(m.vms))
	}
	return ""
}

func (m model) renderMessage() string {
	body := strings.Join(append(
		[]string{sectionHeading("LAUNCHPAD STOPPED HERE"), ""},
		wrapPlain(m.message, m.mainPaneWidth())...,
	), "\n")
	return m.renderFrame(framePage{
		Trail:  append(m.flowTrail(), "error"),
		Body:   dangerStyle.Render(body),
		Danger: true,
		Hints:  []string{"enter back"},
	})
}

// renderKeysOverlay replaces the body with the full key list for the current
// screen. It holds the keyboard while it is open, so no key both moves a
// cursor underneath and closes the overlay.
func (m model) renderKeysOverlay() string {
	var lines []string
	for _, group := range m.keyGroups() {
		lines = append(lines, sectionHeading(group.title))
		for _, hint := range group.hints {
			keyName, label, _ := strings.Cut(hint, " ")
			lines = append(lines, "  "+selectedStyle.Render(fixedDisplayWidth(keyName, 10))+mutedStyle.Render(label))
		}
		lines = append(lines, "")
	}
	return m.renderFrame(framePage{
		Trail: append(m.flowTrail(), "keys"),
		Body:  strings.TrimRight(strings.Join(lines, "\n"), "\n"),
		Hints: []string{"? close", "esc close"},
	})
}

func (m model) renderTemplateAction() string {
	intro := []string{
		textStyle.Render(m.selectedVM.Name + " is a template."),
		mutedStyle.Render("Templates stay clean: run them read-only, or create a VM from them."),
	}
	return m.renderFrame(framePage{
		Trail: []string{"VMs", m.selectedVM.Name, "actions"},
		Body:  actionListBody(m.glyphs(), intro, templateActionRows(), m.cursor),
		Hints: listHints(),
	})
}

func (m model) renderVMAction() string {
	intro := []string{textStyle.Render(m.selectedVM.Name)}
	if m.selectedVM.Running {
		intro = append(intro, warningStyle.Render("This VM is already running."))
	}
	return m.renderFrame(framePage{
		Trail: []string{"VMs", m.selectedVM.Name, "actions"},
		Body:  actionListBody(m.glyphs(), intro, vmActionRows(m.selectedVM), m.cursor),
		Hints: listHints(),
	})
}

func (m model) renderRunDuration() string {
	return m.renderFrame(framePage{
		Trail: append(m.flowTrail(), "duration"),
		Step:  m.stepIndicator(),
		Body:  choiceListBody(m.glyphs(), "How long should this VM live?", runDurationChoices(), m.cursor),
		Hints: listHints(),
	})
}

func (m model) renderName() string {
	crumb := "name"
	context := []string{textStyle.Render("Name the new VM.")}
	switch m.nameMode {
	case nameModeRenameVM:
		crumb = "rename"
		context = []string{
			textStyle.Render("Rename " + m.selectedVM.Name + "."),
			mutedStyle.Render("Launchpad keeps the VM kind and last run with the new name."),
		}
	case nameModeImportVM:
		crumb = "name"
		context = []string{
			textStyle.Render("Name the imported VM."),
			mutedStyle.Render("from " + truncateLeft(m.importSourcePath, max(24, m.mainPaneWidth()-6))),
		}
	}
	return m.renderFrame(framePage{
		Trail: append(m.flowTrail(), crumb),
		Step:  m.stepIndicator(),
		Body:  promptBody("VM NAME", m.nameInput, m.nameCursor, context, "allowed: a-z A-Z 0-9 - _ ."),
		Hints: promptHints(),
	})
}

func (m model) renderFolder() string {
	choices := folderAccessChoices()
	cursor := clampCursor(m.cursor, len(choices))
	glyphs := m.glyphs()
	lines := []string{
		textStyle.Render("What host connections should " + m.flowTargetName() + " receive?"),
		"",
		sectionHeading("PROJECT FOLDER"),
		"",
	}
	for i, choice := range choices {
		label := fixedDisplayWidth(choice.Label, 18)
		description := mutedStyle.Render(choice.Description)
		if i == cursor {
			label = selectedStyle.Render(label)
			description = textStyle.Render(choice.Description)
		}
		lines = append(lines, cursorCellFor(glyphs, i == cursor)+label+" "+description)
	}
	lines = append(lines,
		"",
		boundaryLine("path", truncateLeft(m.selectedProjectPath(), max(24, m.mainPaneWidth()-18))),
		boundaryLine("clipboard", connectionToggleValue(m.clipboard, "host and guest clipboard contents can cross the boundary")),
		boundaryLine("guest audio", connectionToggleValue(m.guestAudio, "guest audio plays through the host")),
	)
	return m.renderFrame(framePage{
		Trail: append(m.flowTrail(), "host connections"),
		Step:  m.stepIndicator(),
		Body:  strings.Join(lines, "\n"),
		Hints: []string{"1-3 choose folder access", "p choose folder", "c clipboard", "a guest audio", "enter continue", "esc back"},
	})
}

func (m model) renderProjectFolderPath() string {
	context := []string{
		textStyle.Render("Choose the one host project folder this VM may access."),
		mutedStyle.Render("Launchpad resolves symlinks and rejects broad or credential-bearing roots."),
	}
	return m.renderFrame(framePage{
		Trail: append(m.flowTrail(), "host connections", "project folder"),
		Step:  m.stepIndicator(),
		Body:  promptBody("PROJECT FOLDER", m.projectPathInput, m.projectPathCursor, context, "absolute path or ~/path"),
		Hints: promptHints(),
	})
}

func connectionToggleValue(enabled bool, consequence string) string {
	if !enabled {
		return mutedStyle.Render("off")
	}
	return warningStyle.Render("on") + "  " + textStyle.Render(consequence)
}

func (m model) renderNetwork() string {
	choices := networkAccessChoices(m.cfg)
	cursor := clampCursor(m.cursor, len(choices))
	return m.renderFrame(framePage{
		Trail: append(m.flowTrail(), "network"),
		Step:  m.stepIndicator(),
		Body:  choiceListBody(m.glyphs(), "What network access should "+m.flowTargetName()+" receive?", choices, cursor),
		Hints: listHints(),
	})
}

func (m model) renderLANCIDR() string {
	glyphs := m.glyphs()
	choices := m.lanCIDRChoices
	if len(choices) == 0 {
		choices = defaultLANCIDRChoices()
	}
	cursor := clampCursor(m.cursor, len(choices))
	lines := []string{textStyle.Render("Which local network should " + string(m.networkAccess) + " allow?"), ""}
	for i, choice := range choices {
		label := fixedDisplayWidth(choice.Label, 18)
		description := mutedStyle.Render(choice.Description)
		if i == cursor {
			label = selectedStyle.Render(label)
			description = textStyle.Render(choice.Description)
		}
		lines = append(lines, cursorCellFor(glyphs, i == cursor)+label+" "+description)
	}
	lines = append(lines, "", mutedStyle.Render("Launchpad saves the chosen CIDR for later runs."))
	return m.renderFrame(framePage{
		Trail: append(m.flowTrail(), "LAN CIDR"),
		Step:  m.stepIndicator(),
		Body:  strings.Join(lines, "\n"),
		Hints: listHints(),
	})
}

func (m model) renderLANCIDRText() string {
	context := []string{
		textStyle.Render("Enter the local network " + string(m.networkAccess) + " should allow."),
		mutedStyle.Render("example: 192.168.1.0/24"),
	}
	return m.renderFrame(framePage{
		Trail: append(m.flowTrail(), "LAN CIDR"),
		Step:  m.stepIndicator(),
		Body:  promptBody("LAN CIDR", m.lanInput, m.lanCursor, context, "allowed: 0-9 . /"),
		Hints: promptHints(),
	})
}

func (m model) renderVolumes() string {
	glyphs := m.glyphs()
	lines := []string{textStyle.Render("Which mounted host volumes should " + m.flowTargetName() + " share?"), ""}
	for i, volume := range m.hostVolumes {
		lines = append(lines, m.renderVolumeChoice(glyphs, volume, i == m.cursor))
	}
	lines = append(lines,
		"",
		mutedStyle.Render("Every volume starts off. Selected volumes are attached read-write."),
		mutedStyle.Render("Launchpad never mounts, unmounts, or prepares host storage."))
	return m.renderFrame(framePage{
		Trail: append(m.flowTrail(), "volumes"),
		Step:  m.stepIndicator(),
		Body:  strings.Join(lines, "\n"),
		Hints: []string{"space toggle", "enter continue", "esc back", "? keys"},
	})
}

func (m model) renderImportPath() string {
	lines := []string{
		textStyle.Render("Enter the path to a .tvm archive."),
		"",
		sectionHeading("ARCHIVE PATH"),
		"",
		"  " + renderTextInputValue(m.importSourcePath, m.importPathCursor),
		"",
		mutedStyle.Render("The archive may contain secrets or other sensitive state."),
	}
	return m.renderFrame(framePage{
		Trail: []string{"VMs", "import", "path"},
		Body:  strings.Join(lines, "\n"),
		Hints: []string{"enter continue", "esc cancel"},
	})
}

func (m model) renderExportPath() string {
	lines := []string{
		textStyle.Render("Enter where to save the .tvm archive."),
		"",
		sectionHeading("ARCHIVE PATH"),
		"",
		"  " + renderTextInputValue(m.exportDestinationPath, m.exportPathCursor),
		"",
		mutedStyle.Render("The archive may contain secrets or other sensitive state."),
	}
	return m.renderFrame(framePage{
		Trail: []string{"VMs", m.selectedVM.Name, "export", "path"},
		Body:  strings.Join(lines, "\n"),
		Hints: []string{"enter review", "esc back"},
	})
}

func (m model) renderMarkKind() string {
	choices := vmKindChoices()
	cursor := clampCursor(m.cursor, len(choices))
	return m.renderFrame(framePage{
		Trail: []string{"VMs", m.selectedVM.Name, "kind"},
		Body:  choiceListBody(m.glyphs(), "How should Launchpad treat "+m.selectedVM.Name+"?", choices, cursor),
		Hints: listHints(),
	})
}

// renderVolumeChoice keeps the mount path and the read-write consequence on the
// row itself: a checked volume is the widest host grant Launchpad can produce.
func (m model) renderVolumeChoice(glyphs glyphSet, volume HostVolume, selected bool) string {
	checked := m.selectedVolumePath[volume.Path]
	mode := mutedStyle.Render("not shared")
	if checked {
		mode = warningStyle.Render("read-write")
	}
	name := fixedDisplayWidth(truncate(volume.Name, 16), 18)
	if selected {
		name = selectedStyle.Render(name)
	}
	path := mutedStyle.Render(fixedDisplayWidth(truncate(volume.Path, 22), 24))
	size := mutedStyle.Render(fixedDisplayWidth(formatBytes(volume.Size), 10))
	return cursorCellFor(glyphs, selected) + checkboxCell(glyphs, checked) + " " + name + path + size + mode
}

// renderReview is the last screen before anything runs. It answers two
// questions in a fixed order: what will this run touch, and what exact command
// produces that. Everything else on the screen is subordinate to those blocks.
func (m model) reviewLines() []string {
	glyphs := m.glyphs()
	var lines []string
	for _, warning := range m.plan.Warnings {
		for i, line := range wrapPlain(warning, max(24, m.mainPaneWidth()-4)) {
			prefix := "  "
			if i == 0 {
				prefix = glyphs.Arrow + " "
			}
			lines = append(lines, warningStyle.Render(prefix+line))
		}
	}
	if m.selectedVM.Running && m.flow == flowRunExisting {
		lines = append(lines, warningStyle.Render(glyphs.Arrow+" This VM is already running; tart run may fail."))
	}
	if len(lines) > 0 {
		lines = append(lines, "")
	}
	if m.plan.ShowsBoundaries() {
		lines = append(lines, sectionHeading("THIS RUN WILL TOUCH"), "")
		lines = append(lines, m.renderCompletedGrantLedger(), "")
	}
	lines = append(lines, sectionHeading("EXACT COMMAND"), "")
	for _, step := range m.plan.Steps {
		lines = append(lines, mutedStyle.Render(step.Label))
		lines = append(lines, m.renderCommandLedger(step)...)
	}
	if m.planRequiresTypedConfirm() {
		state := mutedStyle.Render("type the template name to arm delete")
		if m.deleteConfirmInput == m.plan.DeleteVM {
			state = dangerStyle.Render("armed " + glyphs.Chevron + " enter deletes " + m.plan.DeleteVM)
		}
		lines = append(lines,
			"",
			sectionHeading("CONFIRM TEMPLATE NAME"),
			"",
			"  "+renderTextInputValue(m.deleteConfirmInput, runeCount(m.deleteConfirmInput)),
			"  "+state)
	} else if m.planRequiresExplicitYes() {
		lines = append(lines, "", dangerStyle.Render("This cannot be undone. Press y to delete "+m.plan.DeleteVM+"."))
	}
	return lines
}

func (m model) renderReview() string {
	lines := m.reviewWindow(m.reviewLines())

	return m.renderFrame(framePage{
		Trail:  append(m.flowTrail(), "review"),
		Step:   m.stepIndicator(),
		Body:   strings.Join(lines, "\n"),
		Hints:  m.reviewHints(),
		Danger: m.planRequiresExplicitYes(),
	})
}

func (m model) renderExecute() string {
	var lines []string
	for i, step := range m.plan.Steps {
		state := executeStepPending
		if i < len(m.executeStates) {
			state = m.executeStates[i]
		}
		lines = append(lines, m.renderExecuteStep(state, step, i))
		for _, line := range wrapCommandArgs(step.Args, commandWrapWidth(m.width)-6) {
			lines = append(lines, "     "+mutedStyle.Render(line))
		}
	}
	if len(m.plan.Steps) > 0 {
		lines = append(lines, "", m.renderProgressBar())
	}
	glyphs := m.glyphs()
	if m.executeSummary != "" {
		lines = append(lines, "", successStyle.Render(glyphs.Done+" "+m.executeSummary+" · "+m.totalElapsedLabel()))
	}
	if m.executeErr != "" {
		lines = append(lines, "", dangerStyle.Render(glyphs.Failed+" "+m.executeErr))
		if tail := lastLines(m.executeOutput, 12); tail != "" {
			lines = append(lines, "", mutedStyle.Render(tail))
		}
	} else if m.executeDone && m.executeSummary == "" {
		lines = append(lines, "", successStyle.Render(glyphs.Done+" Done"))
	}

	hintSet := []string{"ctrl+c quit"}
	if m.executeDone {
		hintSet = []string{"enter back to VMs"}
	}
	return m.renderFrame(framePage{
		Trail:  append(m.flowTrail(), "execute"),
		Step:   m.executeStepLabel(),
		Body:   strings.Join(lines, "\n"),
		Hints:  hintSet,
		Danger: m.executeErr != "",
	})
}

func (m model) renderExecuteStep(state executeStepState, step CommandStep, index int) string {
	glyphs := m.glyphs()
	symbol := mutedStyle.Render(glyphs.Idle)
	label := step.Label
	switch state {
	case executeStepActive:
		symbol = selectedStyle.Render(m.spinner.View())
		label = selectedStyle.Render(label)
	case executeStepDone:
		symbol = successStyle.Render(glyphs.Done)
	case executeStepFailed:
		symbol = dangerStyle.Render(glyphs.Failed)
		label = dangerStyle.Render(label)
	case executeStepSkipped:
		symbol = mutedStyle.Render(glyphs.Idle)
		label = mutedStyle.Strikethrough(true).Render(label) + mutedStyle.Render(" never ran")
	}
	return " " + fixedDisplayWidth(symbol+" "+label, 34) + mutedStyle.Render(m.stepElapsedLabel(index))
}

// executeStepLabel is the right-hand step counter in the frame trail row.
func (m model) executeStepLabel() string {
	if len(m.plan.Steps) == 0 {
		return ""
	}
	if m.executeDone {
		return "finished in " + m.totalElapsedLabel()
	}
	return fmt.Sprintf("step %d of %d", min(m.executeIndex+1, len(m.plan.Steps)), len(m.plan.Steps))
}

func (m model) renderCommandLedger(step CommandStep) []string {
	if len(step.AnnotatedArgs) == 0 {
		out := []string{}
		for i, line := range wrapCommandArgs(step.Args, commandWrapWidth(m.width)) {
			prefix := "  "
			if i > 0 {
				prefix = "    "
			}
			out = append(out, prefix+highlightRiskFlags(line))
		}
		return out
	}
	out := []string{}
	if len(step.AnnotatedArgs) >= 2 {
		out = append(out, "  "+commandStyle.Render(step.AnnotatedArgs[0].Value+" "+step.AnnotatedArgs[1].Value))
	}
	for _, arg := range step.AnnotatedArgs[2:] {
		value := fixedDisplayWidth(quoteArg(arg.Value), 38)
		value = commandStyle.Foreground(tintedGrantColor(arg.Provenance)).Render(value)
		if arg.Provenance == "" {
			out = append(out, "    "+value)
			continue
		}
		out = append(out, "    "+value+" "+mutedStyle.Render("<- "+arg.Provenance))
	}
	return out
}

func (m model) renderProgressBar() string {
	done := 0
	for _, state := range m.executeStates {
		if state == executeStepDone {
			done++
		}
	}
	ratio := float64(done) / float64(len(m.plan.Steps))
	if m.executeDone && m.executeErr == "" {
		ratio = 1
	}
	return " " + m.progress.ViewAs(ratio) + "  " + mutedStyle.Render(fmt.Sprintf("%d of %d steps", done, len(m.plan.Steps)))
}

func (m model) stepElapsedLabel(index int) string {
	if index >= len(m.stepStartedAt) || m.stepStartedAt[index].IsZero() {
		return ""
	}
	end := time.Now()
	if index < len(m.stepFinishedAt) && !m.stepFinishedAt[index].IsZero() {
		end = m.stepFinishedAt[index]
	}
	return shortDuration(end.Sub(m.stepStartedAt[index]))
}

func (m model) totalElapsedLabel() string {
	if m.executeStartedAt.IsZero() {
		return ""
	}
	end := m.executeFinishedAt
	if end.IsZero() {
		end = time.Now()
	}
	return shortDuration(end.Sub(m.executeStartedAt))
}

// reviewWindow scrolls a long plan inside the body region instead of letting it
// grow past the frame: the command block must never be cut off silently.
func (m model) reviewWindow(lines []string) []string {
	available := m.bodyHeight()
	if available <= 0 || len(lines) <= available {
		return lines
	}
	maxStart := len(lines) - available
	start := m.reviewScroll
	if start < 0 {
		start = 0
	}
	if start > maxStart {
		start = maxStart
	}
	end := start + available
	glyphs := m.glyphs()
	window := append([]string(nil), lines[start:end]...)
	if start > 0 {
		window[0] = mutedStyle.Render(glyphs.Up + " more")
	}
	if end < len(lines) {
		window[len(window)-1] = mutedStyle.Render(glyphs.Down + " more")
	}
	return window
}

func (m model) reviewMaxScroll() int {
	available := m.bodyHeight()
	if available <= 0 {
		return 0
	}
	return max(0, len(m.reviewLines())-available)
}

func (m model) templates() []VM {
	var out []VM
	for _, vm := range m.vms {
		if vm.Kind == VMKindTemplate {
			out = append(out, vm)
		}
	}
	return out
}

func (m model) setSelectedVMKind(kind VMKind) model {
	for i, vm := range m.vms {
		if vm.Name != m.selectedVM.Name {
			continue
		}
		m.cfg.SetKind(vm.Name, kind)
		m.vms[i].Kind = kind
		m.selectedVM.Kind = kind
		m.needsSave = true
		return m
	}
	return m
}

func (m model) networkNeedsCIDR() bool {
	return m.networkAccess == NetworkLAN || m.networkAccess == NetworkLANAndInternet
}

func (m model) selectedVolumePaths() []string {
	paths := make([]string, 0, len(m.selectedVolumePath))
	for _, volume := range m.hostVolumes {
		if m.selectedVolumePath[volume.Path] {
			paths = append(paths, volume.Path)
		}
	}
	return paths
}

func (m model) selectedVolumeIDs() map[string]string {
	ids := map[string]string{}
	for _, volume := range m.hostVolumes {
		if m.selectedVolumePath[volume.Path] {
			ids[volume.Path] = volume.ID
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return ids
}

func verifyReviewedVolumes(paths []string, expectedIDs map[string]string, mounted []HostVolume) error {
	mountedByPath := make(map[string]HostVolume, len(mounted))
	for _, volume := range mounted {
		mountedByPath[volume.Path] = volume
	}
	for _, path := range paths {
		expectedID := expectedIDs[path]
		if expectedID == "" {
			return fmt.Errorf("mounted volume was not identified during review: %s", path)
		}
		volume, ok := mountedByPath[path]
		if !ok {
			return fmt.Errorf("mounted volume is no longer available: %s", path)
		}
		if volume.ID != expectedID {
			return fmt.Errorf("mounted volume changed after review: %s", path)
		}
	}
	return nil
}

func (m *model) reconcileCompletedCleanup() bool {
	present := make(map[string]bool, len(m.vms))
	for _, vm := range m.vms {
		present[vm.Name] = true
	}
	changed := false
	for _, name := range append([]string(nil), m.cfg.PendingCleanup...) {
		if !present[name] {
			m.cfg.RemovePendingCleanup(name)
			changed = true
		}
	}
	return changed
}

func (m model) filteredHomeItems() []homeItem {
	filter := strings.ToLower(strings.TrimSpace(m.homeFilter))
	main := make([]homeItem, 0, len(m.vms))
	templates := make([]homeItem, 0, len(m.vms))
	for i, vm := range m.vms {
		if filter != "" && !strings.Contains(strings.ToLower(vm.Name), filter) && !strings.Contains(strings.ToLower(string(vm.Kind)), filter) {
			continue
		}
		item := homeItem{vm: vm, index: i}
		if vm.Kind == VMKindTemplate {
			templates = append(templates, item)
			continue
		}
		main = append(main, item)
	}
	return append(main, templates...)
}

func (m model) firstTemplateIndex(items []homeItem) int {
	for i, item := range items {
		if item.vm.Kind == VMKindTemplate {
			return i
		}
	}
	return -1
}

// homeWindow keeps the highlighted VM inside the body region the frame gives
// the list, leaving room for the filter row when FILTER mode is open.
func (m model) homeWindow(count int) (int, int) {
	if count == 0 {
		return 0, 0
	}
	available := m.bodyHeight()
	if available <= 0 {
		available = 12
	}
	if m.mode == modeFilter || m.homeFilter != "" {
		available -= 2
	}
	if available < 1 {
		available = 1
	}
	if available > count {
		available = count
	}
	cursor := clampCursor(m.cursor, count)
	start := 0
	if cursor >= available {
		start = cursor - available + 1
	}
	end := start + available
	if end > count {
		end = count
		start = max(0, end-available)
	}
	return start, end
}

func (m model) volumeReviewDescription() string {
	paths := m.plan.Review.VolumePaths
	if len(paths) == 0 {
		return "none"
	}
	selected := map[string]bool{}
	for _, path := range paths {
		selected[path] = true
	}
	parts := make([]string, 0, len(paths))
	for _, volume := range m.hostVolumes {
		if selected[volume.Path] {
			parts = append(parts, fmt.Sprintf("%s (%s, read-write)", volume.Name, volume.Path))
		}
	}
	if len(parts) == 0 {
		for _, path := range paths {
			parts = append(parts, fmt.Sprintf("%s (read-write)", path))
		}
	}
	return strings.Join(parts, ", ")
}

func (m model) returnHome() model {
	m.screen = screenHome
	m.mode = modeNormal
	m.backStack = nil
	m.cursor = clampCursor(m.cursor, len(m.filteredHomeItems()))
	return m
}

func (m model) returnHomeToVM(name string) model {
	m.screen = screenHome
	m.mode = modeNormal
	m.backStack = nil
	m.homeFocusVM = name
	if m.focusHomeVM(name) {
		m.homeFocusVM = ""
	}
	items := m.filteredHomeItems()
	m.cursor = clampCursor(m.cursor, len(items))
	return m
}

func (m *model) focusHomeVM(name string) bool {
	if name == "" {
		return false
	}
	for i, item := range m.filteredHomeItems() {
		if item.vm.Name == name {
			m.cursor = i
			return true
		}
	}
	for _, vm := range m.vms {
		if vm.Name == name {
			m.homeFilter = ""
			return m.focusHomeVM(name)
		}
	}
	return false
}

func (m model) beginRunFlow(next flow) model {
	m.flow = next
	m.folderAccess = FolderNoFolder
	m.projectPath = m.currentPathLine()
	m.projectPathInput = m.projectPath
	m.projectPathCursor = runeCount(m.projectPath)
	m.networkAccess = NetworkOffline
	m.clipboard = false
	m.guestAudio = false
	m.selectedVolumePath = map[string]bool{}
	m.missingRunVolumes = nil
	m.reviewScroll = 0
	return m
}

func cloneVMConfigs(source map[string]VMConfig) map[string]VMConfig {
	clone := make(map[string]VMConfig, len(source))
	for name, vm := range source {
		if vm.LastRun != nil {
			lastRun := *vm.LastRun
			lastRun.VolumePaths = append([]string(nil), vm.LastRun.VolumePaths...)
			vm.LastRun = &lastRun
		}
		clone[name] = vm
	}
	return clone
}

func (m model) returnHomeForCurrentFlow() model {
	if m.flow == flowNewFromTemplate {
		return m.returnHomeToVM(m.selectedTemplate.Name)
	}
	if m.flow == flowImportArchive {
		return m.returnHome()
	}
	return m.returnHomeToVM(m.selectedVM.Name)
}

func (m model) goTo(next screen) model {
	m.backStack = append(m.backStack, navigationEntry{screen: m.screen, cursor: m.cursor})
	m.screen = next
	m.mode = m.modeFor(next)
	return m
}

func (m model) goToWithCursor(next screen, cursor int) model {
	m = m.goTo(next)
	m.cursor = cursor
	return m
}

func (m model) goBack() model {
	if len(m.backStack) == 0 {
		return m.returnHomeForCurrentFlow()
	}
	entry := m.backStack[len(m.backStack)-1]
	m.backStack = m.backStack[:len(m.backStack)-1]
	m.screen = entry.screen
	m.cursor = entry.cursor
	m.mode = m.modeFor(entry.screen)
	return m
}

// modeFor decides which component owns the keyboard on a screen. Keeping it in
// one place means a screen can never be entered with the wrong capture mode.
func (m model) modeFor(next screen) inputMode {
	switch next {
	case screenName, screenProjectFolderPath, screenImportPath, screenExportPath, screenLANCIDRText:
		return modeInsert
	case screenReview:
		if m.planRequiresTypedConfirm() {
			return modeConfirm
		}
		return modeNormal
	default:
		return modeNormal
	}
}

func (m model) showMessage(message string, returnScreen screen) model {
	m.message = message
	m.returnScreen = returnScreen
	m.screen = screenMessage
	m.mode = modeNormal
	return m
}

func vmActions() []string {
	return []string{"run", "export", "rename", "delete", "mark kind"}
}

func templateActions() []string {
	return []string{"new temporary run", "new workspace", "run read-only", "export", "rename", "delete", "mark kind"}
}

func indexFolder(value FolderAccess) int {
	for i, item := range FolderAccesses {
		if item == value {
			return i
		}
	}
	return 0
}

func indexNetwork(value NetworkAccess) int {
	for i, item := range NetworkAccesses {
		if item == value {
			return i
		}
	}
	return 0
}

func indexVM(vms []VM, name string) int {
	for i, vm := range vms {
		if vm.Name == name {
			return i
		}
	}
	return 0
}

func clampCursor(cursor int, count int) int {
	if count <= 0 || cursor < 0 {
		return 0
	}
	if cursor >= count {
		return count - 1
	}
	return cursor
}

func isNameRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.'
}

func isCIDRRune(r rune) bool {
	return (r >= '0' && r <= '9') || r == '.' || r == '/'
}

type lanCIDRChoice struct {
	Label       string
	Description string
	CIDR        string
	FreeText    bool
}

func defaultLANCIDRChoices() []lanCIDRChoice {
	detected := localLANCIDRs()
	choices := make([]lanCIDRChoice, 0, len(detected)+1)
	for _, cidr := range detected {
		choices = append(choices, lanCIDRChoice{
			Label:       cidr,
			Description: "detected from this Mac",
			CIDR:        cidr,
		})
	}
	choices = append(choices, lanCIDRChoice{
		Label:       "other...",
		Description: "enter a CIDR manually",
		FreeText:    true,
	})
	return choices
}

func vmActionRows(vm VM) []actionRow {
	runDescription := "choose boundaries and start"
	if vm.Running {
		runDescription += " (already running)"
	}
	return []actionRow{
		{Label: "run", Description: runDescription},
		{Label: "export", Description: "save an archive (may contain secrets)"},
		{Label: "rename", Description: "change the local VM name"},
		{Label: "delete", Description: "remove this VM permanently", Danger: true},
		{Label: "mark kind", Description: "set template/workspace/unmarked"},
	}
}

func templateActionRows() []actionRow {
	return []actionRow{
		{Label: "new temporary run", Description: "clone, run, then delete afterward"},
		{Label: "new workspace", Description: "clone and keep the VM"},
		{Label: "run read-only", Description: "start template with root disk read-only"},
		{Label: "export", Description: "save an archive (may contain secrets)"},
		{Label: "rename", Description: "change the local VM name"},
		{Label: "delete", Description: "remove this VM permanently", Danger: true},
		{Label: "mark kind", Description: "set template/workspace/unmarked"},
	}
}

// Key hints. Every screen advertises the same verbs in the same order, and the
// full list lives behind "?" so the footer never grows a second row.

func listHints() []string {
	return []string{"enter choose", "esc back", "? keys"}
}

func promptHints() []string {
	return []string{"enter continue", "esc back", "? keys"}
}

func (m model) homeHints() []string {
	if m.mode == modeFilter {
		return []string{"enter keep filter", "esc clear", "↑↓ move"}
	}
	hints := []string{"enter open", "/ filter", ". repeat", "n new", "i import", "? keys", "q quit"}
	if len(m.cfg.PendingCleanup) > 0 {
		hints = append([]string{"c clean up"}, hints...)
	}
	return hints
}

func (m model) reviewHints() []string {
	switch {
	case m.planRequiresTypedConfirm():
		return []string{"enter delete", "esc back"}
	case m.planRequiresExplicitYes():
		return []string{"y delete", "c copy", "esc back"}
	default:
		return []string{"enter " + m.plan.ReviewVerb(), "c copy", "esc back", "? keys"}
	}
}

type keyGroup struct {
	title string
	hints []string
}

// keyGroups is the full key reference shown by "?". It is grouped by what the
// keys do, not by when they were added.
func (m model) keyGroups() []keyGroup {
	groups := []keyGroup{{
		title: "MOVE",
		hints: []string{"j/↓ next", "k/↑ previous", "g first", "G last", "1-9 jump to choice"},
	}, {
		title: "ACT",
		hints: []string{"enter choose or confirm", "l/→ choose", "space toggle volume", "c copy command on review", "y confirm delete"},
	}, {
		title: "GO BACK",
		hints: []string{"esc back one step", "h/← back one step", "backspace back one step"},
	}}
	if m.screen == screenHome {
		groups = append(groups, keyGroup{
			title: "VMs",
			hints: []string{
				". run again with last boundaries",
				"n new VM from a template",
				"i import a .tvm from Desktop",
				"m cycle VM kind",
				"r refresh from tart",
				"/ filter by name or kind",
				"c clean up a pending temporary VM",
			},
		})
	}
	return append(groups, keyGroup{
		title: "ALWAYS",
		hints: []string{"? keys", "q quit from the VM list", "ctrl+c quit"},
	})
}

// renderCompletedGrantLedger is the boundary manifest shown on review.
func (m model) renderCompletedGrantLedger() string {
	glyphs := m.glyphs()
	return strings.Join([]string{
		boundaryLine(glyphs.GrantTop+" folder", folderReviewDescription(m.plan.Review.FolderAccess)),
		boundaryLine("  path", m.grantPathValue()),
		boundaryLine(glyphs.GrantBottom+" volumes", m.volumeReviewDescription()),
		boundaryLine("  network", networkReviewDescription(m.cfg, m.plan.Review.NetworkAccess)),
		boundaryLine("  clipboard", connectionToggleValue(m.plan.Review.Clipboard, "host and guest clipboard contents can cross the boundary")),
		boundaryLine("  guest audio", connectionToggleValue(m.plan.Review.GuestAudio, "guest audio plays through the host")),
	}, "\n")
}

// grantPathValue reports the host path a folder grant opens, or says plainly
// that no host folder is part of the grant.
func (m model) grantPathValue() string {
	if m.plan.Review.FolderAccess == FolderNoFolder {
		return mutedStyle.Render("no host folder in this grant")
	}
	path := m.plan.Review.ProjectPath
	if path == "" {
		return mutedStyle.Render("no project folder selected")
	}
	if m.plan.Review.FolderAccess == FolderEditFolder {
		return warningStyle.Render(path)
	}
	return textStyle.Render(path)
}

// flowTrail is the breadcrumb prefix for every screen inside a flow.
func (m model) flowTrail() []string {
	name := m.flowTargetName()
	verb := m.planActionLabel()
	if name == "" {
		return []string{"VMs", verb}
	}
	return []string{"VMs", name, verb}
}

// flowTargetName is the VM this flow is about: the new name while creating one,
// the selected VM otherwise.
func (m model) flowTargetName() string {
	switch m.flow {
	case flowNewFromTemplate:
		if strings.TrimSpace(m.nameInput) != "" {
			return strings.TrimSpace(m.nameInput)
		}
		if m.selectedTemplate.Name != "" {
			return "new VM"
		}
	case flowImportArchive:
		if strings.TrimSpace(m.nameInput) != "" {
			return strings.TrimSpace(m.nameInput)
		}
		return "imported VM"
	}
	return m.selectedVM.Name
}

func (m model) planActionLabel() string {
	switch m.flow {
	case flowNewFromTemplate, flowRunTemplateReadOnly, flowRunExisting:
		return "run"
	case flowExportVM:
		return "export"
	case flowImportArchive:
		return "import"
	default:
		return "review"
	}
}

func (m model) stepIndicator() string {
	total := 3
	if len(m.hostVolumes) > 0 {
		total++
	}
	if m.networkNeedsCIDR() && len(m.cfg.Network.LANCIDRs) == 0 {
		total++
	}
	current := 0
	switch m.screen {
	case screenFolder, screenProjectFolderPath:
		current = 1
	case screenNetwork:
		current = 2
	case screenLANCIDR, screenLANCIDRText:
		current = 3
	case screenVolumes:
		current = total - 1
	case screenReview:
		current = total
	}
	if current == 0 {
		return ""
	}
	return fmt.Sprintf("step %d of %d", current, total)
}

func truncate(value string, width int) string {
	if lipgloss.Width(value) <= width {
		return value
	}
	if width <= 1 {
		return "…"
	}
	var b strings.Builder
	used := 0
	limit := width - 1
	for _, r := range value {
		runeWidth := lipgloss.Width(string(r))
		if used+runeWidth > limit {
			break
		}
		b.WriteRune(r)
		used += runeWidth
	}
	return b.String() + "…"
}

func fixedDisplayWidth(value string, width int) string {
	displayWidth := lipgloss.Width(value)
	if displayWidth >= width {
		return value
	}
	return value + strings.Repeat(" ", width-displayWidth)
}

// truncateLeft keeps the end of a path visible, because the last folders say
// more about what a grant opens than the first ones do.
func truncateLeft(value string, width int) string {
	if width <= 1 || lipgloss.Width(value) <= width {
		return value
	}
	runes := []rune(value)
	used := 0
	start := len(runes)
	for i := len(runes) - 1; i >= 0; i-- {
		runeWidth := lipgloss.Width(string(runes[i]))
		if used+runeWidth > width-1 {
			break
		}
		used += runeWidth
		start = i
	}
	return "…" + string(runes[start:])
}

func countLabel(count int, singular string, plural string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, singular)
	}
	return fmt.Sprintf("%d %s", count, plural)
}

// wrapPlain wraps unstyled text to a width without breaking words.
func wrapPlain(value string, width int) []string {
	if width <= 0 || strings.TrimSpace(value) == "" {
		return []string{value}
	}
	var lines []string
	for _, paragraph := range strings.Split(value, "\n") {
		if strings.TrimSpace(paragraph) == "" {
			lines = append(lines, "")
			continue
		}
		current := ""
		for _, word := range strings.Fields(paragraph) {
			switch {
			case current == "":
				current = word
			case lipgloss.Width(current)+1+lipgloss.Width(word) <= width:
				current += " " + word
			default:
				lines = append(lines, current)
				current = word
			}
		}
		lines = append(lines, current)
	}
	return lines
}

func (m model) currentPathLine() string {
	cwd, err := m.host.CurrentDirectory()
	if err != nil {
		return ""
	}
	return cwd
}

func (m model) selectedProjectPath() string {
	if strings.TrimSpace(m.projectPath) != "" {
		return strings.TrimSpace(m.projectPath)
	}
	return m.currentPathLine()
}

func commandWrapWidth(terminalWidth int) int {
	if terminalWidth <= 0 {
		return 88
	}
	width := terminalWidth - 10
	if width < 32 {
		return 32
	}
	return width
}

func wrapCommand(command string, width int) []string {
	if width <= 0 || lipgloss.Width(command) <= width {
		return []string{command}
	}
	words := strings.Fields(command)
	return wrapCommandTokens(words, width)
}

func wrapCommandArgs(args []string, width int) []string {
	if len(args) == 0 {
		return nil
	}
	words := make([]string, 0, len(args))
	for _, arg := range args {
		words = append(words, quoteArg(arg))
	}
	return wrapCommandTokens(words, width)
}

func wrapCommandTokens(words []string, width int) []string {
	if len(words) == 0 {
		return nil
	}
	var lines []string
	current := words[0]
	for _, word := range words[1:] {
		if lipgloss.Width(current)+1+lipgloss.Width(word) <= width {
			current += " " + word
			continue
		}
		lines = append(lines, current+" \\")
		current = word
	}
	lines = append(lines, current)
	return splitLongLines(lines, width)
}

func splitLongLines(lines []string, width int) []string {
	var out []string
	for _, line := range lines {
		if strings.HasPrefix(line, "'") {
			out = append(out, line)
			continue
		}
		for lipgloss.Width(line) > width {
			chunk, rest := splitDisplayWidth(line, max(1, width-2))
			out = append(out, chunk+"\\")
			line = rest
		}
		out = append(out, line)
	}
	return out
}

func splitDisplayWidth(value string, width int) (string, string) {
	var b strings.Builder
	used := 0
	for index, r := range value {
		runeWidth := lipgloss.Width(string(r))
		if used+runeWidth > width {
			return b.String(), value[index:]
		}
		b.WriteRune(r)
		used += runeWidth
	}
	return value, ""
}

func renderTextInputValue(value string, cursor int) string {
	cursor = clampCursorForText(cursor, value)
	before := substringRunes(value, 0, cursor)
	after := substringRunes(value, cursor, runeCount(value))
	return before + cursorStyle.Render(" ") + after
}

func insertAtCursor(value string, cursor int, insert string) (string, int) {
	cursor = clampCursorForText(cursor, value)
	before := substringRunes(value, 0, cursor)
	after := substringRunes(value, cursor, runeCount(value))
	return before + insert + after, cursor + runeCount(insert)
}

func deleteBeforeCursor(value string, cursor int) (string, int) {
	cursor = clampCursorForText(cursor, value)
	if cursor == 0 {
		return value, 0
	}
	before := substringRunes(value, 0, cursor-1)
	after := substringRunes(value, cursor, runeCount(value))
	return before + after, cursor - 1
}

func deleteWordBeforeCursor(value string, cursor int) (string, int) {
	cursor = clampCursorForText(cursor, value)
	if cursor == 0 {
		return value, 0
	}
	runes := []rune(value)
	start := cursor
	for start > 0 && runes[start-1] == ' ' {
		start--
	}
	for start > 0 && runes[start-1] != ' ' {
		start--
	}
	return string(runes[:start]) + string(runes[cursor:]), start
}

func substringRunes(value string, start int, end int) string {
	runes := []rune(value)
	if start < 0 {
		start = 0
	}
	if end > len(runes) {
		end = len(runes)
	}
	if start > end {
		start = end
	}
	return string(runes[start:end])
}

func runeCount(value string) int {
	return len([]rune(value))
}

func clampCursorForText(cursor int, value string) int {
	if cursor < 0 {
		return 0
	}
	count := runeCount(value)
	if cursor > count {
		return count
	}
	return cursor
}

func dropLastRune(value string) string {
	if value == "" {
		return ""
	}
	runes := []rune(value)
	return string(runes[:len(runes)-1])
}

func formatBytes(bytes uint64) string {
	if bytes == 0 {
		return "unknown size"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	value := float64(bytes)
	unit := 0
	for value >= 1000 && unit < len(units)-1 {
		value /= 1000
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d %s", bytes, units[unit])
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}

func formatArchiveTime(t time.Time) string {
	if t.IsZero() {
		return "unknown date"
	}
	return t.Format("2006-01-02 15:04")
}

func summarizeCIDRs(values []string) string {
	if len(values) == 0 {
		return "configured LAN CIDRs"
	}
	if len(values) <= 2 {
		return strings.Join(values, ",")
	}
	return strings.Join(values[:2], ",") + fmt.Sprintf(" +%d more", len(values)-2)
}

func (m model) planRequiresExplicitYes() bool {
	return m.plan.DeleteVM != ""
}

func (m model) planRequiresTypedConfirm() bool {
	return m.plan.DeleteVM != "" && m.selectedVM.Kind == VMKindTemplate
}

func planCommands(plan Plan) string {
	lines := make([]string, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		lines = append(lines, ShellQuote(step.Args))
	}
	return strings.Join(lines, "\n")
}

func copyTextCommand(value string) tea.Cmd {
	return tea.Printf("\x1b]52;c;%s\a", base64.StdEncoding.EncodeToString([]byte(value)))
}

func highlightRiskFlags(line string) string {
	if strings.Contains(line, "'") {
		return commandStyle.Render(line)
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return commandStyle.Render(line)
	}
	for i, field := range fields {
		if strings.HasPrefix(field, "--dir=") && !strings.HasSuffix(field, ":ro") {
			fields[i] = warningStyle.Render(field)
			continue
		}
		if field == "--net-host" {
			fields[i] = warningStyle.Render(field)
			continue
		}
		fields[i] = commandStyle.Render(field)
	}
	return strings.Join(fields, " ")
}

func shortDuration(value time.Duration) string {
	if value < time.Second {
		return fmt.Sprintf("%.1fs", float64(value)/float64(time.Second))
	}
	if value < time.Minute {
		return fmt.Sprintf("%ds", int(value.Seconds()))
	}
	return fmt.Sprintf("%dm %02ds", int(value.Minutes()), int(value.Seconds())%60)
}

func humanAge(value time.Duration) string {
	if value < time.Hour {
		return fmt.Sprintf("%dm", max(1, int(value.Minutes())))
	}
	if value < 48*time.Hour {
		return fmt.Sprintf("%dh", int(value.Hours()))
	}
	return fmt.Sprintf("%dd", int(value.Hours()/24))
}

func lastLines(value string, count int) string {
	if count <= 0 || strings.TrimSpace(value) == "" {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(value), "\n")
	if len(lines) <= count {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-count:], "\n")
}

func (m model) homeNameWidth() int {
	width := 48
	if m.width > 0 {
		width = m.width - 26
	}
	if width < 20 {
		return 20
	}
	if width > 48 {
		return 48
	}
	return width
}

func indexVMKind(kind VMKind) int {
	kinds := []VMKind{VMKindTemplate, VMKindWorkspace}
	for i, item := range kinds {
		if item == kind {
			return i
		}
	}
	return 1
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
