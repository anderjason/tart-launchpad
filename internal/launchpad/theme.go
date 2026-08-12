package launchpad

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
)

// Color has one product-wide job: reinforce state or consequence. Every color
// also has a text, symbol, or positional counterpart so the interface remains
// understandable without color.
var (
	focusColor     = lipgloss.AdaptiveColor{Light: "31", Dark: "86"}
	textColor      = lipgloss.AdaptiveColor{Light: "235", Dark: "252"}
	mutedColor     = lipgloss.AdaptiveColor{Light: "240", Dark: "244"}
	subtleColor    = lipgloss.AdaptiveColor{Light: "250", Dark: "238"}
	attentionColor = lipgloss.AdaptiveColor{Light: "166", Dark: "214"}
	successColor   = lipgloss.AdaptiveColor{Light: "29", Dark: "48"}
	dangerColor    = lipgloss.AdaptiveColor{Light: "160", Dark: "203"}
	templateColor  = lipgloss.AdaptiveColor{Light: "55", Dark: "141"}
)

var (
	titleStyle        = lipgloss.NewStyle().Bold(true).Foreground(textColor)
	questionStyle     = lipgloss.NewStyle().Bold(true).Foreground(focusColor)
	sectionLabelStyle = lipgloss.NewStyle().Bold(true).Foreground(mutedColor)
	templateStyle     = lipgloss.NewStyle().Bold(true).Foreground(templateColor)
	mutedStyle        = lipgloss.NewStyle().Foreground(mutedColor)
	subtleStyle       = lipgloss.NewStyle().Foreground(subtleColor)
	helpStyle         = lipgloss.NewStyle().Foreground(mutedColor)
	selectedStyle     = lipgloss.NewStyle().Bold(true).Foreground(focusColor)
	textStyle         = lipgloss.NewStyle().Foreground(textColor)
	attentionStyle    = lipgloss.NewStyle().Foreground(attentionColor)
	dangerStyle       = lipgloss.NewStyle().Bold(true).Foreground(dangerColor)
	successStyle      = lipgloss.NewStyle().Foreground(successColor)
	cursorStyle       = lipgloss.NewStyle().Background(focusColor)
	framePadding      = lipgloss.NewStyle().Padding(0, 1)
)

// glyphSet keeps every drawn symbol in one place so the ascii_glyphs config
// switch can degrade the whole UI at once instead of screen by screen.
type glyphSet struct {
	Cursor  string
	Running string
	Idle    string
	Done    string
	Failed  string
	Warn    string
	Bullet  string
	Arrow   string
	Up      string
	Down    string
}

var unicodeGlyphSet = glyphSet{
	Cursor:  "▌",
	Running: "●",
	Idle:    "·",
	Done:    "✓",
	Failed:  "✗",
	Warn:    "▲",
	Bullet:  "•",
	Arrow:   "▸",
	Up:      "↑",
	Down:    "↓",
}

var asciiGlyphSet = glyphSet{
	Cursor:  ">",
	Running: "*",
	Idle:    ".",
	Done:    "+",
	Failed:  "x",
	Warn:    "!",
	Bullet:  "*",
	Arrow:   ">",
	Up:      "^",
	Down:    "v",
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

// spinnerFrames returns a still marker under reduced motion so a long clone
// does not animate.
func (t theme) spinnerFrames() spinner.Spinner {
	if t.reducedMotion {
		return spinner.Spinner{Frames: []string{t.glyphs.Bullet}, FPS: time.Second}
	}
	return spinner.MiniDot
}
