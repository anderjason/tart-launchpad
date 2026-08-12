package launchpad

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type App struct {
	cfg     Config
	cfgPath string
	tart    Tart
	host    HostEnvironment
}

func NewApp(cfg Config, cfgPath string, tart Tart) (*App, error) {
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}
	return &App{cfg: cfg, cfgPath: cfgPath, tart: tart, host: RealHostEnvironment{}}, nil
}

func (a *App) Run() error {
	initialModel := newModelWithHost(a.cfg, a.cfgPath, nil, a.host)
	initialModel.tart = a.tart
	initialModel.loadingVMs = true
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
	screenTemplatePicker
	screenRunDuration
	screenName
	screenFolder
	screenFolderAccess
	screenProjectFolderPath
	screenNetwork
	screenLANCIDR
	screenLANCIDRText
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
)

type nameMode int

const (
	nameModeNewVM nameMode = iota
	nameModeRenameVM
)

type navigationEntry struct {
	screen screen
	cursor int
}

type homeItem struct {
	action string
	vm     VM
	index  int
}

type vmsLoadedMsg []VM

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

type executeStepState int

const (
	executeStepPending executeStepState = iota
	executeStepActive
	executeStepDone
	executeStepFailed
	executeStepSkipped
)

type model struct {
	cfg         Config
	cfgPath     string
	vms         []VM
	screen      screen
	flow        flow
	cursor      int
	width       int
	height      int
	message     string
	status      string
	statusKind  noticeKind
	needsSave   bool
	keysOverlay bool
	mode        inputMode
	homeFilter  string
	homeFocusVM string
	loadingVMs  bool
	spinner     spinner.Model
	theme       theme

	backStack    []navigationEntry
	returnScreen screen

	selectedVM                 VM
	selectedTemplate           VM
	runDuration                RunDuration
	nameMode                   nameMode
	nameInput                  string
	nameCursor                 int
	lanInput                   string
	lanCursor                  int
	lanCIDRChoices             []lanCIDRChoice
	folderAccess               FolderAccess
	projectPath                string
	projectPathInput           string
	projectPathCursor          int
	networkAccess              NetworkAccess
	clipboard                  bool
	guestAudio                 bool
	host                       HostEnvironment
	tart                       Tart
	plan                       Plan
	reviewScroll               int
	deleteConfirmInput         string
	deleteConfirmCursor        int
	executeIndex               int
	executeStates              []executeStepState
	executeOutput              string
	executeErr                 string
	executeDone                bool
	executeSummary             string
	temporaryVMCreationStarted bool
	executeStartedAt           time.Time
	executeFinishedAt          time.Time
	stepStartedAt              []time.Time
	stepFinishedAt             []time.Time
}

func newModel(cfg Config, cfgPath string, vms []VM) model {
	return newModelWithHost(cfg, cfgPath, vms, RealHostEnvironment{})
}

func newModelWithHost(cfg Config, cfgPath string, vms []VM, host HostEnvironment) model {
	if host == nil {
		host = RealHostEnvironment{}
	}
	appTheme := newTheme(cfg)
	spin := spinner.New(spinner.WithSpinner(appTheme.spinnerFrames()))
	return model{
		cfg:           cfg,
		cfgPath:       cfgPath,
		vms:           vms,
		host:          host,
		spinner:       spin,
		theme:         appTheme,
		screen:        screenHome,
		mode:          modeNormal,
		folderAccess:  FolderNoFolder,
		networkAccess: NetworkOffline,
	}
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.loadVMsCmd(), m.homePollCmd()}
	if !m.cfg.Defaults.ReducedMotion {
		cmds = append(cmds, m.spinner.Tick)
	}
	return tea.Batch(cmds...)
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
	case dataLoadFailedMsg:
		m.loadingVMs = false
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
		// A non-interactive step cannot be interrupted safely after it has
		// started. Interactive Tart runs own the terminal and receive ctrl+c
		// directly through ExecProcess.
		switch msg.String() {
		case "ctrl+c":
			if m.screen == screenExecute && !m.executeDone {
				m.status = "wait for the current step to finish before quitting"
				return m, nil
			}
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
	case screenTemplatePicker:
		return m.handleTemplatePickerKey(key)
	case screenRunDuration:
		return m.handleChoiceKey(key, len(RunDurations), func(m model) model {
			m.runDuration = RunDurations[m.cursor]
			if m.runDuration == RunDurationTemporaryRun {
				m.nameInput = m.availableTemporaryName(time.Now())
				m.nameCursor = runeCount(m.nameInput)
				m.nameMode = nameModeNewVM
				m.message = ""
				return m.goToWithCursor(screenFolder, 0)
			}
			m.nameInput = ""
			m.nameCursor = 0
			m.nameMode = nameModeNewVM
			m.message = ""
			return m.goToWithCursor(screenName, 0)
		})
	case screenName:
		return m.handleNameKey(key)
	case screenFolder:
		return m.handleHostConnectionsKey(key)
	case screenFolderAccess:
		return m.handleChoiceKey(key, len(FolderAccesses), func(m model) model {
			m.folderAccess = FolderAccesses[m.cursor]
			return m.goBack()
		})
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
			return m.advanceToReviewModel()
		})
	case screenLANCIDR:
		return m.handleLANCIDRKey(key)
	case screenLANCIDRText:
		return m.handleLANCIDRTextKey(key)
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
		return m, m.loadVMsCmd()
	case "n":
		return m.beginNewVM()
	case "enter":
		if len(items) == 0 {
			return m, nil
		}
		item := items[m.cursor]
		switch item.action {
		case "new":
			return m.beginNewVM()
		case "cleanup":
			return m.cleanUpPendingVM()
		}
		vm := item.vm
		m.selectedVM = vm
		if vm.Kind == VMKindTemplate {
			m = m.goToWithCursor(screenTemplateAction, 0)
			return m, nil
		}
		m = m.goToWithCursor(screenVMAction, 0)
	}
	return m, nil
}

func (m model) beginNewVM() (tea.Model, tea.Cmd) {
	templates := m.templates()
	if len(templates) == 0 {
		return m.showMessage("No templates are available. Open a VM, choose Change role, then choose Template.", screenHome), nil
	}
	m = m.beginRunFlow(flowNewFromTemplate)
	if len(templates) == 1 {
		m.selectedTemplate = templates[0]
		return m.goToWithCursor(screenRunDuration, 0), nil
	}
	cursor := 0
	for i, template := range templates {
		if template.Name == m.selectedVM.Name {
			cursor = i
			break
		}
	}
	return m.goToWithCursor(screenTemplatePicker, cursor), nil
}

func (m model) cleanUpPendingVM() (tea.Model, tea.Cmd) {
	if len(m.cfg.PendingCleanup) == 0 {
		return m, nil
	}
	name := m.cfg.PendingCleanup[0]
	if m.focusHomeVM(name) {
		m.status = "Verify " + name + " before deleting it; its cleanup identity is ambiguous"
		m.statusKind = noticeAttention
		return m, nil
	}
	m.cfg.RemovePendingCleanup(name)
	m.needsSave = true
	m.status = "Cleanup already complete for " + name
	m.statusKind = noticeSuccess
	return m, nil
}

// handleHomeFilterKey runs while the filter owns the keyboard: every
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
	rows := m.vmActionRows()
	m.cursor = clampCursor(m.cursor, len(rows))
	if cursor, handled := navigateList(m.cursor, len(rows), key); handled {
		m.cursor = cursor
		return m, nil
	}
	switch key {
	case "esc":
		m = m.goBack()
	case "enter":
		switch rows[m.cursor].ID {
		case "run":
			m = m.beginRunFlow(flowRunExisting)
			m = m.goToWithCursor(screenFolder, 0)
		case "run-again":
			return m.repeatSelectedVMRun()
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
		case "change-role":
			m = m.goToWithCursor(screenMarkKind, indexVMKind(m.selectedVM.Kind))
		}
	}
	return m, nil
}

func (m model) handleTemplateActionKey(key string) (tea.Model, tea.Cmd) {
	rows := m.templateActionRows()
	m.cursor = clampCursor(m.cursor, len(rows))
	if cursor, handled := navigateList(m.cursor, len(rows), key); handled {
		m.cursor = cursor
		return m, nil
	}
	switch key {
	case "esc":
		m = m.goBack()
	case "enter":
		switch rows[m.cursor].ID {
		case "new-temporary":
			m = m.beginRunFlow(flowNewFromTemplate)
			m.selectedTemplate = m.selectedVM
			m.runDuration = RunDurationTemporaryRun
			m.nameMode = nameModeNewVM
			m.nameInput = m.availableTemporaryName(time.Now())
			m.nameCursor = runeCount(m.nameInput)
			m.message = ""
			m = m.goToWithCursor(screenFolder, 0)
		case "new-workspace":
			m = m.beginRunFlow(flowNewFromTemplate)
			m.selectedTemplate = m.selectedVM
			m.runDuration = RunDurationWorkspace
			m.nameMode = nameModeNewVM
			m.nameInput = ""
			m.nameCursor = 0
			m.message = ""
			m = m.goToWithCursor(screenName, 0)
		case "run-template":
			m = m.beginRunFlow(flowRunTemplateReadOnly)
			m = m.goToWithCursor(screenFolder, 0)
		case "run-again":
			return m.repeatSelectedVMRun()
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
		case "change-role":
			m = m.goToWithCursor(screenMarkKind, indexVMKind(m.selectedVM.Kind))
		}
	}
	return m, nil
}

func (m model) handleTemplatePickerKey(key string) (tea.Model, tea.Cmd) {
	templates := m.templates()
	return m.handleChoiceKey(key, len(templates), func(m model) model {
		m.selectedTemplate = templates[m.cursor]
		return m.goToWithCursor(screenRunDuration, 0)
	})
}

func (m model) repeatSelectedVMRun() (tea.Model, tea.Cmd) {
	last, ok := m.cfg.LastRunFor(m.selectedVM.Name)
	if !ok {
		m.status = "No previous run for " + m.selectedVM.Name
		m.statusKind = noticeNeutral
		return m, nil
	}
	nextFlow := flowRunExisting
	if m.selectedVM.Kind == VMKindTemplate {
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
	if err := m.preparePlan(); err != nil {
		return m.showMessage(err.Error(), m.screen), nil
	}
	return m.goToWithCursor(screenReview, 0), nil
}

func (m model) handleChoiceKey(key string, count int, selectFn func(model) model) (tea.Model, tea.Cmd) {
	m.cursor = clampCursor(m.cursor, count)
	if cursor, handled := navigateList(m.cursor, count, key); handled {
		m.cursor = cursor
		return m, nil
	}
	switch key {
	case "enter":
		m = selectFn(m)
	case "esc":
		m = m.goBack()
	}
	return m, nil
}

func (m model) handleHostConnectionsKey(key string) (tea.Model, tea.Cmd) {
	const connectionRows = 5
	m.cursor = clampCursor(m.cursor, connectionRows)
	if cursor, handled := navigateList(m.cursor, connectionRows, key); handled {
		m.cursor = cursor
		return m, nil
	}
	switch key {
	case "enter":
		switch m.cursor {
		case 0:
			m = m.goToWithCursor(screenFolderAccess, indexFolder(m.folderAccess))
		case 1:
			m.projectPathInput = m.selectedProjectPath()
			m.projectPathCursor = runeCount(m.projectPathInput)
			m.message = ""
			m = m.goToWithCursor(screenProjectFolderPath, 0)
		case 2:
			m.clipboard = !m.clipboard
		case 3:
			m.guestAudio = !m.guestAudio
		case 4:
			m = m.goToWithCursor(screenNetwork, indexNetwork(m.networkAccess))
		}
	case "esc":
		m = m.goBack()
	}
	return m, nil
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

// handleNameKey runs while the field owns the keyboard: enter commits, esc leaves, and every
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
		default:
			m.message = ""
			m = m.goToWithCursor(screenFolder, 0)
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
	case "enter":
		choice := m.lanCIDRChoices[m.cursor]
		if choice.FreeText {
			m.lanInput = ""
			m.lanCursor = 0
			m.message = ""
			m = m.goToWithCursor(screenLANCIDRText, 0)
			return m, nil
		}
		return m.addLANCIDRAndReview(choice.CIDR)
	case "esc":
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
	return m.advanceToReviewModel(), nil
}

func (m model) advanceToReviewModel() model {
	if err := m.preparePlan(); err != nil {
		return m.showMessage(err.Error(), m.screen)
	}
	return m.goToWithCursor(screenReview, 0)
}

func (m model) handleMarkKindKey(key string) (tea.Model, tea.Cmd) {
	kinds := []VMKind{VMKindTemplate, VMKindWorkspace}
	m.cursor = clampCursor(m.cursor, len(kinds))
	if cursor, handled := navigateList(m.cursor, len(kinds), key); handled {
		m.cursor = cursor
		return m, nil
	}
	switch key {
	case "enter":
		m = m.setSelectedVMKind(kinds[m.cursor])
		m.status = fmt.Sprintf("%s is now a %s", m.selectedVM.Name, reviewVMKind(kinds[m.cursor]))
		m.statusKind = noticeSuccess
		m = m.returnHomeToVM(m.selectedVM.Name)
	case "esc":
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
	case "c":
		m.status = "Command copied"
		m.statusKind = noticeNeutral
		return m, copyTextCommand(planCommands(m.plan))
	case "enter":
		return m.confirmReview()
	case "esc":
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
	value, cursor, handled := editTextField(m.deleteConfirmInput, m.deleteConfirmCursor, key, anyPrintableRune)
	if handled {
		m.deleteConfirmInput = value
		m.deleteConfirmCursor = cursor
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
		return m, m.loadVMsCmd()
	}
	return m, nil
}

func (m model) beginExecution() (tea.Model, tea.Cmd) {
	interactive := m.screen == screenReview
	if err := m.validateReviewedProjectFolder(); err != nil {
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
	m.temporaryVMCreationStarted = false
	m.screen = screenExecute
	m.mode = modeNormal
	if len(m.plan.Steps) > 0 {
		if err := m.prepareStepExecution(m.plan.Steps[0]); err != nil {
			m.executeStates[0] = executeStepFailed
			m.markSkippedAfter(0)
			m.executeErr = fmt.Errorf("%s: %w", m.plan.Steps[0].Label, err).Error()
			m.executeDone = true
			m.executeFinishedAt = time.Now()
			return m, nil
		}
	}
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
	if step.Kind == CommandStepRun {
		if err := m.validateReviewedProjectFolder(); err != nil {
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
	if err := m.prepareStepExecution(m.plan.Steps[m.executeIndex]); err != nil {
		m.executeStates[m.executeIndex] = executeStepFailed
		m.markSkippedAfter(m.executeIndex)
		m.executeErr = fmt.Errorf("%s: %w", m.plan.Steps[m.executeIndex].Label, err).Error()
		m.executeDone = true
		m.executeFinishedAt = time.Now()
		return m, nil
	}
	return m, m.executeCurrentStepCmd()
}

func (m *model) prepareStepExecution(step CommandStep) error {
	if m.plan.HasTemporaryVM && step.Kind == CommandStepClone {
		m.cfg.AddPendingCleanup(m.plan.TemporaryVM)
		if err := SaveConfig(m.cfgPath, m.cfg); err != nil {
			m.cfg.RemovePendingCleanup(m.plan.TemporaryVM)
			return fmt.Errorf("save pending cleanup before clone: %w", err)
		}
		m.temporaryVMCreationStarted = true
	}
	return nil
}

func (m *model) applySuccessfulStepEffects(step CommandStep) error {
	if m.plan.HasTemporaryVM && step.Kind == CommandStepDeleteTemporaryVM {
		m.cfg.RemovePendingCleanup(m.plan.TemporaryVM)
		m.temporaryVMCreationStarted = false
		if err := SaveConfig(m.cfgPath, m.cfg); err != nil {
			return fmt.Errorf("save completed temporary cleanup: %w", err)
		}
	}
	return nil
}

func (m *model) recordPendingCleanupAfterFailure() error {
	if !m.plan.HasTemporaryVM || !m.temporaryVMCreationStarted {
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
	if m.plan.DeleteVM != "" {
		return "Deleted " + m.plan.DeleteVM
	}
	if m.plan.RenameTo != "" {
		return "Renamed " + m.plan.RenameTo
	}
	if m.plan.HasTemporaryVM {
		return "Temporary run finished; " + m.plan.TemporaryVM + " deleted"
	}
	if m.plan.CreatedVM != "" {
		return "Created and ran " + m.plan.CreatedVM
	}
	return "Ran " + m.executionReturnName()
}

func (m model) executionReturnName() string {
	if m.plan.RenameTo != "" {
		return m.plan.RenameTo
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
	m.deleteConfirmInput = ""
	m.deleteConfirmCursor = 0
	var err error
	m.plan, err = LaunchpadIntent{
		Kind: IntentDeleteVM,
		VM:   m.selectedVM,
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
	case screenTemplatePicker:
		return m.renderTemplatePicker()
	case screenRunDuration:
		return m.renderRunDuration()
	case screenName:
		return m.renderName()
	case screenFolder:
		return m.renderFolder()
	case screenFolderAccess:
		return m.renderFolderAccess()
	case screenProjectFolderPath:
		return m.renderProjectFolderPath()
	case screenNetwork:
		return m.renderNetwork()
	case screenLANCIDR:
		return m.renderLANCIDR()
	case screenLANCIDRText:
		return m.renderLANCIDRText()
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
		item := items[i]
		if i == m.firstActionIndex(items) {
			lines = append(lines, sectionHeading("Actions"))
		}
		if i == m.firstWorkspaceIndex(items) {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, sectionHeading("Workspaces"))
		}
		if i == m.firstTemplateIndex(items) {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, templateStyle.Render("Templates"))
		}
		if item.action != "" {
			lines = append(lines, m.renderHomeActionRows(glyphs, item.action, i == cursorIndex)...)
			continue
		}
		lines = append(lines, m.renderHomeRow(glyphs, item.vm, i == cursorIndex, nameWidth))
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
			mutedStyle.Render("Create one with Tart, then refresh this list."))
	case len(items) == 0:
		lines = append(lines, mutedStyle.Render("No VMs match the filter."))
	}

	return m.renderFrame(framePage{
		Trail: []string{"VMs"},
		Step:  m.homeCount(m.visibleVMCount(items)),
		Body:  strings.Join(lines, "\n"),
		Hints: m.homeHints(),
	})
}

func (m model) renderHomeActionRows(glyphs glyphSet, action string, selected bool) []string {
	label := "New VM"
	description := "Create from a template"
	if action == "cleanup" {
		label = "Clean up temporary VM"
		description = "Resolve " + m.cfg.PendingCleanup[0]
	}
	return actionRows(glyphs, actionRow{Label: label, Description: description}, selected, m.mainPaneWidth())
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

// renderFilterRow shows the live query while the filter owns the keyboard.
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
	lines := []string{dangerStyle.Render(m.glyphs().Failed + " Couldn't continue"), ""}
	for _, line := range wrapPlain(m.message, m.mainPaneWidth()) {
		lines = append(lines, textStyle.Render(line))
	}
	return m.renderFrame(framePage{
		Trail:  append(m.flowTrail(), "Error"),
		Body:   strings.Join(lines, "\n"),
		Danger: true,
		Hints:  []string{"enter back", "esc back"},
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
	return m.renderFrame(framePage{
		Trail: []string{"VMs", m.selectedVM.Name},
		Body:  actionListBody(m.glyphs(), "What would you like to do with "+m.selectedVM.Name+"?", m.templateActionRows(), m.cursor, m.mainPaneWidth()),
		Hints: listHints(),
	})
}

func (m model) renderVMAction() string {
	question := "What would you like to do with " + m.selectedVM.Name + "?"
	return m.renderFrame(framePage{
		Trail: []string{"VMs", m.selectedVM.Name},
		Body:  actionListBody(m.glyphs(), question, m.vmActionRows(), m.cursor, m.mainPaneWidth()),
		Hints: listHints(),
	})
}

func (m model) renderTemplatePicker() string {
	templates := m.templates()
	choices := make([]accessChoice, 0, len(templates))
	for _, template := range templates {
		choices = append(choices, accessChoice{Label: template.Name, Description: "Protected source"})
	}
	return m.renderFrame(framePage{
		Trail: []string{"VMs", "New VM", "Template"},
		Body:  choiceListBody(m.glyphs(), "Which template should the new VM use?", choices, m.cursor, m.mainPaneWidth()),
		Hints: listHints(),
	})
}

func (m model) renderRunDuration() string {
	return m.renderFrame(framePage{
		Trail: append(m.flowTrail(), "Duration"),
		Body:  choiceListBody(m.glyphs(), "What should happen after this VM runs?", runDurationChoices(), m.cursor, m.mainPaneWidth()),
		Hints: listHints(),
	})
}

func (m model) renderName() string {
	crumb := "Name"
	question := "What should the new workspace be called?"
	switch m.nameMode {
	case nameModeRenameVM:
		crumb = "Rename"
		question = "What should " + m.selectedVM.Name + " be called?"
	}
	return m.renderFrame(framePage{
		Trail: append(m.flowTrail(), crumb),
		Body:  promptBody(question, m.nameInput, m.nameCursor, "Letters, numbers, hyphens, underscores, and periods.", m.message),
		Hints: promptHints(),
	})
}

func (m model) renderFolder() string {
	rows := []actionRow{
		{Label: "Folder access", Description: reviewFolderAccess(m.folderAccess), Attention: m.folderAccess == FolderEditFolder},
		{Label: "Project folder", Description: truncateLeft(m.selectedProjectPath(), max(24, m.mainPaneWidth()-34)), Attention: m.folderAccess != FolderNoFolder},
		{Label: "Clipboard", Description: connectionLabel(m.clipboard, "Shared with VM"), Attention: m.clipboard},
		{Label: "Audio", Description: connectionLabel(m.guestAudio, "Plays on this Mac"), Attention: m.guestAudio},
		{Label: "Continue", Description: "Choose network access next"},
	}
	return m.renderFrame(framePage{
		Trail: append(m.flowTrail(), "Connections"),
		Body:  actionListBody(m.glyphs(), "How may "+m.flowTargetName()+" connect to this Mac?", rows, m.cursor, m.mainPaneWidth()),
		Hints: listHints(),
	})
}

func (m model) renderFolderAccess() string {
	return m.renderFrame(framePage{
		Trail: append(m.flowTrail(), "Connections", "Folder access"),
		Body:  choiceListBody(m.glyphs(), "What may the VM do with the project folder?", folderAccessChoices(), m.cursor, m.mainPaneWidth()),
		Hints: listHints(),
	})
}

func (m model) renderProjectFolderPath() string {
	return m.renderFrame(framePage{
		Trail: append(m.flowTrail(), "Connections", "Project folder"),
		Body:  promptBody("Which project folder may the VM access?", m.projectPathInput, m.projectPathCursor, "Absolute path or ~/path. Broad and credential-bearing roots are rejected.", m.message),
		Hints: promptHints(),
	})
}

func connectionLabel(enabled bool, consequence string) string {
	if !enabled {
		return "Off"
	}
	return consequence
}

func (m model) renderNetwork() string {
	choices := networkAccessChoices(m.cfg)
	cursor := clampCursor(m.cursor, len(choices))
	return m.renderFrame(framePage{
		Trail: append(m.flowTrail(), "Network"),
		Body:  choiceListBody(m.glyphs(), "What may "+m.flowTargetName()+" reach?", choices, cursor, m.mainPaneWidth()),
		Hints: listHints(),
	})
}

func (m model) renderLANCIDR() string {
	choices := m.lanCIDRChoices
	if len(choices) == 0 {
		choices = defaultLANCIDRChoices()
	}
	rows := make([]accessChoice, 0, len(choices))
	for _, choice := range choices {
		rows = append(rows, accessChoice{Label: choice.Label, Description: choice.Description})
	}
	return m.renderFrame(framePage{
		Trail: append(m.flowTrail(), "Local network"),
		Body:  choiceListBody(m.glyphs(), "Which local network may the VM reach?", rows, m.cursor, m.mainPaneWidth()),
		Hints: listHints(),
	})
}

func (m model) renderLANCIDRText() string {
	return m.renderFrame(framePage{
		Trail: append(m.flowTrail(), "Local network"),
		Body:  promptBody("Which local network may the VM reach?", m.lanInput, m.lanCursor, "IPv4 CIDR, such as 192.168.1.0/24.", m.message),
		Hints: promptHints(),
	})
}

func (m model) renderMarkKind() string {
	choices := vmKindChoices()
	cursor := clampCursor(m.cursor, len(choices))
	return m.renderFrame(framePage{
		Trail: []string{"VMs", m.selectedVM.Name, "Role"},
		Body:  choiceListBody(m.glyphs(), "What role should "+m.selectedVM.Name+" have?", choices, cursor, m.mainPaneWidth()) + "\n\n" + mutedStyle.Render("Changing the role does not inspect or clean the VM."),
		Hints: listHints(),
	})
}

// renderReview is the last screen before anything runs. The decision comes
// first, followed by the access summary and the exact commands in quiet text.
func (m model) reviewLines() []string {
	glyphs := m.glyphs()
	lines := []string{questionStyle.Render(m.reviewQuestion()), ""}
	hasWarning := false
	if m.selectedVM.Running && m.flow == flowRunExisting {
		hasWarning = true
		lines = append(lines, attentionStyle.Render(glyphs.Warn+" This VM is already running; Tart may refuse another run."))
	}
	if hasWarning {
		lines = append(lines, "")
	}
	if m.plan.ShowsBoundaries() {
		lines = append(lines, m.renderRunSummary()...)
		lines = append(lines, "")
	}
	commandHeading := "Command"
	if len(m.plan.Steps) != 1 {
		commandHeading = "Commands"
	}
	lines = append(lines, sectionHeading(commandHeading), "")
	for _, step := range m.plan.Steps {
		lines = append(lines, m.renderReviewCommand(step)...)
	}
	if m.planRequiresTypedConfirm() {
		state := mutedStyle.Render("Type " + m.plan.DeleteVM + " to confirm. This cannot be undone.")
		if m.deleteConfirmInput == m.plan.DeleteVM {
			state = dangerStyle.Render("Press enter to delete " + m.plan.DeleteVM + ".")
		}
		lines = append(lines,
			"",
			sectionHeading("Confirm"),
			"",
			"  "+renderTextInputValue(m.deleteConfirmInput, m.deleteConfirmCursor),
			"  "+state)
	}
	return lines
}

func (m model) renderReview() string {
	lines := m.reviewWindow(m.reviewLines())

	return m.renderFrame(framePage{
		Trail:  m.flowTrail(),
		Body:   strings.Join(lines, "\n"),
		Hints:  m.reviewHints(),
		Danger: m.plan.DeleteVM != "",
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
		if state == executeStepFailed {
			for _, line := range wrapCommandArgs(step.Args, commandWrapWidth(m.width)-6) {
				lines = append(lines, "     "+mutedStyle.Render(line))
			}
		}
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

	hintSet := []string{"wait for current step"}
	if m.executeDone {
		hintSet = []string{"enter back to VMs"}
	}
	return m.renderFrame(framePage{
		Trail:  append(m.flowTrail(), "Run"),
		Step:   m.executeStepLabel(),
		Body:   strings.Join(lines, "\n"),
		Hints:  hintSet,
		Danger: m.executeErr != "",
	})
}

func (m model) renderExecuteStep(state executeStepState, step CommandStep, index int) string {
	glyphs := m.glyphs()
	symbol := mutedStyle.Render(glyphs.Idle)
	label := m.executionStepLabel(step)
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

func (m model) executionStepLabel(step CommandStep) string {
	target := m.executionReturnName()
	if m.plan.HasTemporaryVM {
		target = m.plan.TemporaryVM
	}
	switch step.Kind {
	case CommandStepClone:
		return "Create " + target
	case CommandStepRun:
		return "Start " + target
	case CommandStepDeleteTemporaryVM:
		return "Delete temporary VM"
	case CommandStepRename:
		return "Rename " + m.plan.RenameFrom
	case CommandStepDelete:
		return "Delete " + m.plan.DeleteVM
	default:
		return step.Label
	}
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

func (m model) renderReviewCommand(step CommandStep) []string {
	out := []string{}
	for i, line := range wrapCommandArgs(step.Args, commandWrapWidth(m.width)-2) {
		prefix := "  "
		if i > 0 {
			prefix = "    "
		}
		out = append(out, mutedStyle.Render(prefix+line))
	}
	return out
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

func (m model) validateReviewedProjectFolder() error {
	if m.plan.Review.FolderAccess == FolderReadFolder || m.plan.Review.FolderAccess == FolderEditFolder {
		resolved, err := m.host.ResolveProjectDirectory(m.plan.Review.ProjectPath)
		if err != nil {
			return err
		}
		if resolved != m.plan.Review.ProjectPath {
			return fmt.Errorf("project folder changed after review: %s", m.plan.Review.ProjectPath)
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
	actions := []homeItem{}
	if filter == "" {
		actions = append(actions, homeItem{action: "new"})
		if len(m.cfg.PendingCleanup) > 0 {
			actions = append(actions, homeItem{action: "cleanup"})
		}
	}
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
	return append(actions, append(main, templates...)...)
}

func (m model) firstActionIndex(items []homeItem) int {
	for i, item := range items {
		if item.action != "" {
			return i
		}
	}
	return -1
}

func (m model) firstWorkspaceIndex(items []homeItem) int {
	for i, item := range items {
		if item.action == "" && item.vm.Kind != VMKindTemplate {
			return i
		}
	}
	return -1
}

func (m model) firstTemplateIndex(items []homeItem) int {
	for i, item := range items {
		if item.vm.Kind == VMKindTemplate {
			return i
		}
	}
	return -1
}

func (m model) visibleVMCount(items []homeItem) int {
	count := 0
	for _, item := range items {
		if item.action == "" {
			count++
		}
	}
	return count
}

// homeWindow keeps the highlighted VM inside the body region the frame gives
// the list, leaving room for the filter row when filtering is active.
func (m model) homeWindow(count int) (int, int) {
	if count == 0 {
		return 0, 0
	}
	available := m.bodyHeight()
	if available <= 0 {
		available = 12
	}
	if m.mode == modeFilter || m.homeFilter != "" {
		// The filter, a section label, spacing, and scroll markers are not
		// represented in the item count.
		available -= 5
	} else {
		// Reserve rows for section labels, spacing, scroll markers, and the
		// stacked New VM description at the supported minimum width.
		available -= 7
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
	m.reviewScroll = 0
	return m
}

func cloneVMConfigs(source map[string]VMConfig) map[string]VMConfig {
	clone := make(map[string]VMConfig, len(source))
	for name, vm := range source {
		if vm.LastRun != nil {
			lastRun := *vm.LastRun
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
	case screenName, screenProjectFolderPath, screenLANCIDRText:
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

func reviewVMKind(kind VMKind) string {
	if kind == VMKindTemplate {
		return "Template"
	}
	if kind == VMKindWorkspace {
		return "Workspace"
	}
	return string(kind)
}

func (m model) availableTemporaryName(now time.Time) string {
	base := "tmp-" + now.Format("20060102-150405")
	exists := func(name string) bool {
		for _, vm := range m.vms {
			if vm.Name == name {
				return true
			}
		}
		return false
	}
	if !exists(base) {
		return base
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s-%d", base, suffix)
		if !exists(candidate) {
			return candidate
		}
	}
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

func (m model) vmActionRows() []actionRow {
	runDescription := "Choose connections and network"
	if m.selectedVM.Running {
		runDescription += "; already running"
	}
	rows := []actionRow{{ID: "run", Section: "Run", Label: "Choose settings and run", Description: runDescription}}
	if _, ok := m.cfg.LastRunFor(m.selectedVM.Name); ok {
		rows = append(rows, actionRow{ID: "run-again", Label: "Run again", Description: "Use the previous connections and network"})
	}
	return append(rows,
		actionRow{ID: "rename", Section: "Manage", Label: "Rename", Description: "Change the VM name"},
		actionRow{ID: "change-role", Label: "Change role", Description: "Choose Workspace or Template"},
		actionRow{ID: "delete", Section: "Delete", Label: "Delete permanently", Description: "Remove this VM; cannot be undone", Danger: true},
	)
}

func (m model) templateActionRows() []actionRow {
	rows := []actionRow{
		{ID: "new-temporary", Section: "Create", Label: "Temporary VM", Description: "Create, run, then delete automatically"},
		{ID: "new-workspace", Label: "Workspace", Description: "Create a persistent VM"},
		{ID: "run-template", Section: "Run", Label: "Run template", Description: "Template disk stays read-only; choose connections next"},
	}
	if _, ok := m.cfg.LastRunFor(m.selectedVM.Name); ok {
		rows = append(rows, actionRow{ID: "run-again", Label: "Run again", Description: "Use the previous connections and network"})
	}
	return append(rows,
		actionRow{ID: "rename", Section: "Manage", Label: "Rename", Description: "Change the VM name"},
		actionRow{ID: "change-role", Label: "Change role", Description: "Choose Workspace or Template"},
		actionRow{ID: "delete", Section: "Delete", Label: "Delete permanently", Description: "Remove this VM; cannot be undone", Danger: true},
	)
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
	return []string{"↑↓ move", "enter open", "/ filter", "? keys", "q quit"}
}

func (m model) reviewHints() []string {
	switch {
	case m.planRequiresTypedConfirm():
		return []string{"enter delete", "esc back"}
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
		title: "Move",
		hints: []string{"↓ or j next", "↑ or k previous"},
	}, {
		title: "Act",
		hints: []string{"enter open, choose, or confirm", "c copy command on review"},
	}, {
		title: "Go back",
		hints: []string{"esc back one step"},
	}}
	if m.screen == screenHome {
		groups = append(groups, keyGroup{
			title: "VMs",
			hints: []string{
				"n new VM from a template",
				"r refresh from tart",
				"/ filter by name or kind",
			},
		})
	}
	return append(groups, keyGroup{
		title: "Always",
		hints: []string{"? keys", "q quit from the VM list", "ctrl+c quit"},
	})
}

func (m model) reviewQuestion() string {
	switch {
	case m.plan.RenameFrom != "":
		return fmt.Sprintf("Rename %s to %s?", m.plan.RenameFrom, m.plan.RenameTo)
	case m.plan.DeleteVM != "":
		return "Delete " + m.plan.DeleteVM + "?"
	case m.plan.ShowsBoundaries():
		target := m.flowTargetName()
		if target != "" {
			return "Run " + target + "?"
		}
		return strings.TrimSpace(m.plan.Title) + "?"
	default:
		return strings.TrimSpace(m.plan.Title) + "?"
	}
}

// renderRunSummary translates internal mode names into the four decisions a
// person needs to confirm. Details get their own line instead of becoming a
// sentence beside the value.
func (m model) renderRunSummary() []string {
	lines := []string{}
	if m.plan.Review.TemplateDiskReadOnly {
		lines = append(lines, boundaryLine("Template disk", textStyle.Render("Read only")))
	}
	lines = append(lines, boundaryLine("Folder", reviewFolderValue(m.plan.Review.FolderAccess)))
	if m.plan.Review.FolderAccess != FolderNoFolder && m.plan.Review.ProjectPath != "" {
		lines = append(lines, boundaryLine("", mutedStyle.Render(m.plan.Review.ProjectPath)))
	}

	network, detail := reviewNetworkAccess(m.cfg, m.plan.Review.NetworkAccess)
	lines = append(lines, boundaryLine("Network", reviewNetworkValue(m.plan.Review.NetworkAccess, network)))
	if detail != "" {
		lines = append(lines, boundaryLine("", mutedStyle.Render(detail)))
	}
	lines = append(lines,
		boundaryLine("Clipboard", reviewConnectionValue(m.plan.Review.Clipboard, "Shared with VM")),
		boundaryLine("Audio", reviewConnectionValue(m.plan.Review.GuestAudio, "Plays on this Mac")),
	)
	return lines
}

func reviewFolderValue(folder FolderAccess) string {
	value := reviewFolderAccess(folder)
	switch folder {
	case FolderNoFolder:
		return mutedStyle.Render(value)
	case FolderReadFolder:
		return attentionStyle.Render(value)
	case FolderEditFolder:
		return attentionStyle.Render(value)
	default:
		return textStyle.Render(value)
	}
}

func reviewNetworkValue(network NetworkAccess, value string) string {
	switch network {
	case NetworkOffline:
		return mutedStyle.Render(value)
	case NetworkInternet, NetworkHost, NetworkLAN, NetworkLANAndInternet:
		return attentionStyle.Render(value)
	default:
		return textStyle.Render(value)
	}
}

func reviewFolderAccess(folder FolderAccess) string {
	switch folder {
	case FolderNoFolder:
		return "None"
	case FolderReadFolder:
		return "Read only"
	case FolderEditFolder:
		return "Read and write"
	default:
		return string(folder)
	}
}

func reviewNetworkAccess(cfg Config, network NetworkAccess) (string, string) {
	switch network {
	case NetworkOffline:
		return "Block outbound IPv4", "Not complete network isolation"
	case NetworkInternet:
		return "Internet", "This Mac and local network blocked"
	case NetworkHost:
		return "This Mac", ""
	case NetworkLAN:
		return "Local network", strings.Join(cfg.Network.LANCIDRs, ", ")
	case NetworkLANAndInternet:
		return "Internet + local network", strings.Join(cfg.Network.LANCIDRs, ", ")
	default:
		return string(network), ""
	}
}

func reviewConnectionValue(enabled bool, enabledText string) string {
	if enabled {
		return attentionStyle.Render(enabledText)
	}
	return mutedStyle.Render("Off")
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
	}
	return m.selectedVM.Name
}

func (m model) planActionLabel() string {
	switch m.flow {
	case flowNewFromTemplate, flowRunTemplateReadOnly, flowRunExisting:
		return "run"
	default:
		return "review"
	}
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

func (m model) planRequiresTypedConfirm() bool {
	return m.plan.DeleteVM != ""
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
