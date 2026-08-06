package launchpad

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
)

// Launchpad reads as a boundary console: quiet chrome, loud grants.
// Frame colors stay muted so the eye lands on grant badges, generated command
// arguments, and anything that widens host access.
var (
	accentColor  = lipgloss.AdaptiveColor{Light: "31", Dark: "86"}
	mutedColor   = lipgloss.AdaptiveColor{Light: "240", Dark: "244"}
	faintColor   = lipgloss.AdaptiveColor{Light: "250", Dark: "238"}
	warnColor    = lipgloss.AdaptiveColor{Light: "166", Dark: "214"}
	dangerColor  = lipgloss.AdaptiveColor{Light: "160", Dark: "203"}
	successColor = lipgloss.AdaptiveColor{Light: "28", Dark: "42"}
	textColor    = lipgloss.AdaptiveColor{Light: "235", Dark: "252"}
	inkColor     = lipgloss.AdaptiveColor{Light: "231", Dark: "233"}
)

var (
	titleStyle        = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	brandStyle        = lipgloss.NewStyle().Bold(true).Foreground(inkColor).Background(accentColor).Padding(0, 1)
	headingStyle      = lipgloss.NewStyle().Bold(true).Foreground(textColor)
	crumbStyle        = lipgloss.NewStyle().Foreground(textColor)
	sectionLabelStyle = lipgloss.NewStyle().Bold(true).Foreground(mutedColor)
	paneTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(mutedColor)
	paneActiveStyle   = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	mutedStyle        = lipgloss.NewStyle().Foreground(mutedColor)
	ruleStyle         = lipgloss.NewStyle().Foreground(faintColor)
	helpStyle         = lipgloss.NewStyle().Foreground(mutedColor)
	helpKeyStyle      = lipgloss.NewStyle().Foreground(textColor)
	selectedStyle     = lipgloss.NewStyle().Foreground(accentColor).Bold(true)
	textStyle         = lipgloss.NewStyle().Foreground(textColor)
	warningStyle      = lipgloss.NewStyle().Foreground(warnColor)
	dangerStyle       = lipgloss.NewStyle().Foreground(dangerColor).Bold(true)
	successStyle      = lipgloss.NewStyle().Foreground(successColor)
	commandStyle      = lipgloss.NewStyle().Foreground(textColor)
	cursorStyle       = lipgloss.NewStyle().Background(accentColor)
	framePadding      = lipgloss.NewStyle().Padding(0, 1)
)

var (
	// Grant badges are identity tokens, not a risk scale. Keep adjacent choices visually distinct.
	noFolderBadgeStyle          = grantBadgeStyle("#F8FAFC", "#334155", "15", "24")
	readHereBadgeStyle          = grantBadgeStyle("#082F49", "#7DD3FC", "17", "117")
	editHereBadgeStyle          = grantBadgeStyle("#FFFFFF", "#A21CAF", "15", "127")
	offlineBadgeStyle           = grantBadgeStyle("#FFFFFF", "#52525B", "15", "240")
	internetBadgeStyle          = grantBadgeStyle("#FFFFFF", "#2563EB", "15", "33")
	hostBadgeStyle              = grantBadgeStyle("#FFFFFF", "#7C3AED", "15", "99")
	lanBadgeStyle               = grantBadgeStyle("#052E16", "#34D399", "22", "48")
	lanInternetStyle            = grantBadgeStyle("#FFFFFF", "#DB2777", "15", "162")
	unknownBadgeStyle           = grantBadgeStyle("#FAFAFA", "#3F3F46", "15", "238")
	templateBadgeStyle          = grantBadgeStyle("#FEF3C7", "#854D0E", "230", "94")
	workspaceKindBadgeStyle     = grantBadgeStyle("#E0F2FE", "#075985", "195", "24")
	temporaryRunBadgeStyle      = grantBadgeStyle("#FFFFFF", "#BE123C", "15", "161")
	workspaceDurationBadgeStyle = grantBadgeStyle("#E0F2FE", "#075985", "195", "24")

	// Mode chips announce which component owns the keyboard.
	normalChipStyle  = grantBadgeStyle("#E4E4E7", "#3F3F46", "15", "238")
	insertChipStyle  = grantBadgeStyle("#042F2E", "#5EEAD4", "16", "80")
	filterChipStyle  = grantBadgeStyle("#082F49", "#7DD3FC", "17", "117")
	confirmChipStyle = grantBadgeStyle("#FFFFFF", "#BE123C", "15", "161")
)

func grantBadge(choice accessChoice) string {
	return choice.Style.Render(choice.Label)
}

func grantBadgeStyle(foreground string, background string, foregroundANSI string, backgroundANSI string) lipgloss.Style {
	fg := lipgloss.CompleteColor{
		TrueColor: foreground,
		ANSI256:   foregroundANSI,
		ANSI:      foregroundANSI,
	}
	bg := lipgloss.CompleteColor{
		TrueColor: background,
		ANSI256:   backgroundANSI,
		ANSI:      backgroundANSI,
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.CompleteAdaptiveColor{Light: fg, Dark: fg}).
		Background(lipgloss.CompleteAdaptiveColor{Light: bg, Dark: bg}).
		Bold(true).
		Padding(0, 1)
}

func badge(value string) string {
	switch value {
	case string(VMKindTemplate):
		return templateBadgeStyle.Render(value)
	case string(VMKindWorkspace):
		return workspaceKindBadgeStyle.Render(value)
	default:
		return unknownBadgeStyle.Render(value)
	}
}

// tintedGrantColor tints a generated command argument toward the grant that
// produced it, so the command block reads back as the manifest above it.
func tintedGrantColor(provenance string) lipgloss.TerminalColor {
	base := "#E5E7EB"
	hue := "#22D3EE"
	switch provenance {
	case string(FolderReadHere):
		hue = "#7DD3FC"
	case string(FolderEditHere), "volume read-write":
		hue = "#F59E0B"
	case string(NetworkOffline):
		hue = "#71717A"
	case string(NetworkInternet):
		hue = "#2563EB"
	case string(NetworkHost):
		hue = "#7C3AED"
	case string(NetworkLAN):
		hue = "#34D399"
	case string(NetworkLANAndInternet):
		hue = "#DB2777"
	case "clipboard off", "audio off":
		return mutedColor
	}
	baseColor, err := colorful.Hex(base)
	if err != nil {
		return commandStyle.GetForeground()
	}
	hueColor, err := colorful.Hex(hue)
	if err != nil {
		return commandStyle.GetForeground()
	}
	return lipgloss.Color(baseColor.BlendLuv(hueColor, 0.4).Hex())
}

// glyphSet keeps every drawn symbol in one place so the ascii_glyphs config
// switch can degrade the whole UI at once instead of screen by screen.
type glyphSet struct {
	Cursor      string
	Chevron     string
	Rule        string
	VRule       string
	Running     string
	Idle        string
	Check       string
	Cross       string
	Done        string
	Failed      string
	Warn        string
	Bullet      string
	Arrow       string
	Flow        string
	Up          string
	Down        string
	CheckboxOn  string
	CheckboxOff string
	Checked     string
	Unchecked   string
	GrantTop    string
	GrantBottom string
	ReadArrow   string
	WriteArrow  string
	Empty       string
	CornerTL    string
	CornerTR    string
	CornerBL    string
	CornerBR    string
	TeeLeft     string
	TeeRight    string
}

var unicodeGlyphSet = glyphSet{
	Cursor:      "▌",
	Chevron:     "›",
	Rule:        "─",
	VRule:       "│",
	Running:     "●",
	Idle:        "·",
	Check:       "✓",
	Cross:       "✗",
	Done:        "✓",
	Failed:      "✗",
	Warn:        "▲",
	Bullet:      "•",
	Arrow:       "▸",
	Flow:        "──▶",
	Up:          "↑",
	Down:        "↓",
	CheckboxOn:  "[x]",
	CheckboxOff: "[ ]",
	Checked:     "[x]",
	Unchecked:   "[ ]",
	GrantTop:    "⎧",
	GrantBottom: "⎩",
	ReadArrow:   "──ro──▶",
	WriteArrow:  "──rw──▶",
	Empty:       "—",
	CornerTL:    "╭",
	CornerTR:    "╮",
	CornerBL:    "╰",
	CornerBR:    "╯",
	TeeLeft:     "┤",
	TeeRight:    "├",
}

var asciiGlyphSet = glyphSet{
	Cursor:      ">",
	Chevron:     ">",
	Rule:        "-",
	VRule:       "|",
	Running:     "*",
	Idle:        ".",
	Check:       "+",
	Cross:       "x",
	Done:        "+",
	Failed:      "x",
	Warn:        "!",
	Bullet:      "*",
	Arrow:       ">",
	Flow:        "-->",
	Up:          "^",
	Down:        "v",
	CheckboxOn:  "[x]",
	CheckboxOff: "[ ]",
	Checked:     "[x]",
	Unchecked:   "[ ]",
	GrantTop:    "/",
	GrantBottom: "\\",
	ReadArrow:   "--ro-->",
	WriteArrow:  "--rw-->",
	Empty:       "-",
	CornerTL:    "+",
	CornerTR:    "+",
	CornerBL:    "+",
	CornerBR:    "+",
	TeeLeft:     "|",
	TeeRight:    "|",
}

// theme bundles the presentation choices the config can change, so screens
// never branch on preferences themselves.
type theme struct {
	glyphs        glyphSet
	reducedMotion bool
}

func newTheme(cfg Config) theme {
	glyphs := unicodeGlyphSet
	if cfg.Defaults.ASCIIGlyphs {
		glyphs = asciiGlyphSet
	}
	return theme{glyphs: glyphs, reducedMotion: cfg.Defaults.ReducedMotion}
}

// cursor keeps the selected and unselected cells the same width so rows never
// shift sideways as the cursor moves.
func (t theme) cursor(selected bool) string {
	if selected {
		return selectedStyle.Render(t.glyphs.Cursor) + " "
	}
	return strings.Repeat(" ", lipgloss.Width(t.glyphs.Cursor)+1)
}

func (t theme) ellipsis() string {
	if t.reducedMotion {
		return "..."
	}
	return "…"
}

// spinnerFrames returns a still marker under reduced motion so a long clone or
// import does not animate.
func (t theme) spinnerFrames() spinner.Spinner {
	if t.reducedMotion {
		return spinner.Spinner{Frames: []string{t.glyphs.Bullet}, FPS: time.Second}
	}
	return spinner.MiniDot
}
