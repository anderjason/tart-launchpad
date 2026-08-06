package launchpad

import "fmt"

type VMKind string

const (
	VMKindTemplate  VMKind = "template"
	VMKindWorkspace VMKind = "workspace"
	VMKindUnmarked  VMKind = "unmarked"
)

type FolderAccess string

const (
	FolderNoFolder FolderAccess = "no-folder"
	FolderReadHere FolderAccess = "read-here"
	FolderEditHere FolderAccess = "edit-here"
)

type NetworkAccess string

const (
	NetworkOffline        NetworkAccess = "offline"
	NetworkInternet       NetworkAccess = "internet"
	NetworkHost           NetworkAccess = "host"
	NetworkLAN            NetworkAccess = "lan"
	NetworkLANAndInternet NetworkAccess = "lan-and-internet"
)

type RunDuration string

const (
	RunDurationWorkspace    RunDuration = "workspace"
	RunDurationTemporaryRun RunDuration = "temporary-run"
)

var FolderAccesses = []FolderAccess{FolderNoFolder, FolderReadHere, FolderEditHere}
var NetworkAccesses = []NetworkAccess{NetworkOffline, NetworkInternet, NetworkHost, NetworkLAN, NetworkLANAndInternet}
var RunDurations = []RunDuration{RunDurationTemporaryRun, RunDurationWorkspace}

type VM struct {
	Name    string
	Source  string
	State   string
	Running bool
	Kind    VMKind
}

type HostVolume struct {
	ID   string
	Path string
	Name string
	Size uint64
}

type CommandStep struct {
	Kind          CommandStepKind
	Label         string
	Args          []string
	AnnotatedArgs []AnnotatedArg
}

type AnnotatedArg struct {
	Value      string
	Provenance string
}

type CommandStepKind string

const (
	CommandStepRun               CommandStepKind = "run"
	CommandStepConfigure         CommandStepKind = "configure"
	CommandStepClone             CommandStepKind = "clone"
	CommandStepRename            CommandStepKind = "rename"
	CommandStepDelete            CommandStepKind = "delete"
	CommandStepDeleteTemporaryVM CommandStepKind = "delete-temporary-vm"
	CommandStepExport            CommandStepKind = "export"
	CommandStepImport            CommandStepKind = "import"
)

type Plan struct {
	Title          string
	Steps          []CommandStep
	Review         PlanReview
	Prerequisites  PlanPrerequisites
	TemporaryVM    string
	HasTemporaryVM bool
	RenameFrom     string
	RenameTo       string
	DeleteVM       string
	ExportPath     string
	ImportPath     string
	ImportVMName   string
	Warnings       []string
}

type PlanPrerequisites struct {
	Softnet bool
}

type PlanReview struct {
	Verb           string
	ShowBoundaries bool
	FolderAccess   FolderAccess
	NetworkAccess  NetworkAccess
	VolumePaths    []string
	Clipboard      string
	Audio          string
}

type HostAccessGrant struct {
	FolderAccess FolderAccess
	ProjectPath  string
	VolumePaths  []string
}

type RunOptions struct {
	VMName           string
	FolderAccess     FolderAccess
	NetworkAccess    NetworkAccess
	CWD              string
	TemplateReadOnly bool
	VolumePaths      []string
}

type NewFromTemplateOptions struct {
	TemplateName  string
	NewName       string
	RunDuration   RunDuration
	FolderAccess  FolderAccess
	NetworkAccess NetworkAccess
	CWD           string
	VolumePaths   []string
}

type ExportOptions struct {
	VMName            string
	VMKind            VMKind
	DestinationPath   string
	DestinationExists bool
}

type ImportOptions struct {
	SourcePath      string
	DestinationName string
	ExistingVMs     []VM
}

func (k VMKind) Valid() bool {
	switch k {
	case VMKindTemplate, VMKindWorkspace, VMKindUnmarked:
		return true
	default:
		return false
	}
}

func (f FolderAccess) Valid() bool {
	switch f {
	case FolderNoFolder, FolderReadHere, FolderEditHere:
		return true
	default:
		return false
	}
}

func (n NetworkAccess) Valid() bool {
	switch n {
	case NetworkOffline, NetworkInternet, NetworkHost, NetworkLAN, NetworkLANAndInternet:
		return true
	default:
		return false
	}
}

func (d RunDuration) Valid() bool {
	switch d {
	case RunDurationWorkspace, RunDurationTemporaryRun:
		return true
	default:
		return false
	}
}

func ValidateCanonicalTerms(folder FolderAccess, network NetworkAccess) error {
	if !folder.Valid() {
		return fmt.Errorf("%w: invalid folder access %q", ErrUsage, folder)
	}
	if !network.Valid() {
		return fmt.Errorf("%w: invalid network access %q", ErrUsage, network)
	}
	return nil
}
