package launchpad

import (
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

// This file owns the persistent frame: the header, the body area, the status
// row, and the key hint row. Screens supply a body and a hint set; everything
// else about the layout stays put between screens.

// inputMode says which component owns the keyboard. Every key is routed by
// mode first and screen second, so a printable rune can never be mistaken for
// a navigation command while a prompt is open.
type inputMode int

const (
	modeNormal inputMode = iota
	modeInsert
	modeFilter
	modeConfirm
)

func (mode inputMode) label() string {
	switch mode {
	case modeInsert:
		return "INSERT"
	case modeFilter:
		return "FILTER"
	case modeConfirm:
		return "CONFIRM"
	default:
		return "NORMAL"
	}
}

func (mode inputMode) capturesText() bool {
	return mode == modeInsert || mode == modeFilter || mode == modeConfirm
}

func modeChip(mode inputMode) string {
	switch mode {
	case modeInsert:
		return insertChipStyle.Render(mode.label())
	case modeFilter:
		return filterChipStyle.Render(mode.label())
	case modeConfirm:
		return confirmChipStyle.Render(mode.label())
	default:
		return normalChipStyle.Render(mode.label())
	}
}

const (
	frameDefaultWidth = 96
	frameMinWidth     = 48
	frameMaxWidth     = 118
	// title, context, spacing, and contextual keys
	frameChromeHeight = 4
	minBodyHeight     = 3
)

func (m model) frameWidth() int {
	if m.width <= 0 {
		return frameDefaultWidth
	}
	width := m.width - 2
	if width < frameMinWidth {
		return frameMinWidth
	}
	if width > frameMaxWidth {
		return frameMaxWidth
	}
	return width
}

// bodyHeight returns 0 when the terminal height is unknown, which means the
// body renders at its natural height instead of being padded or clipped.
func (m model) bodyHeight() int {
	if m.height <= 0 {
		return 0
	}
	height := m.height - frameChromeHeight
	if height < minBodyHeight {
		return minBodyHeight
	}
	return height
}

func (m model) mainPaneWidth() int {
	return m.frameWidth()
}

// framePage is the only thing a screen has to produce. The frame owns the
// header, the trail, the rules, the status row, and the key hints, so none of
// those move when the screen changes.
type framePage struct {
	Trail  []string
	Step   string
	Body   string
	Hints  []string
	Danger bool
}

// renderFrame keeps navigation context in one stable row. Dangerous screens
// use the same content in the danger style so the frame matches the action.
func (m model) renderFrame(page framePage) string {
	width := m.frameWidth()
	trail := strings.Join(page.Trail, " / ")
	contextStyle := mutedStyle
	if page.Danger {
		contextStyle = dangerStyle
	}
	context := rowBetween(contextStyle.Render(trail), contextStyle.Render(page.Step), width)
	lines := []string{titleStyle.Render("Tart Launchpad")}
	if trail != "" || page.Step != "" {
		lines = append(lines, context)
	}
	lines = append(lines, "", strings.TrimRight(page.Body, "\n"))
	if status := m.statusRow(width); status != "" {
		lines = append(lines, "", status)
	}
	if hints := m.hintRow(page.Hints, width); hints != "" {
		lines = append(lines, "", hints)
	}
	return framePadding.Render(strings.Join(lines, "\n"))
}

func (m model) glyphs() glyphSet {
	return m.theme.glyphs
}

// statusRow is the single place feedback appears: a validation message, the
// result of the last action, or a host condition that will bite at run time.
func (m model) statusRow(width int) string {
	glyphs := m.theme.glyphs
	if m.screen != screenMessage && m.message != "" {
		message := glyphs.Warn + " " + flattenLine(m.message)
		return clampPlainWidth(warningStyle.Render(message), message, width)
	}
	if m.status != "" {
		status := glyphs.Bullet + " " + flattenLine(m.status)
		return clampPlainWidth(successStyle.Render(status), status, width)
	}
	if advisory := m.advisory(); advisory != "" {
		advisory = glyphs.Warn + " " + advisory
		return clampPlainWidth(warningStyle.Render(advisory), advisory, width)
	}
	if len(m.cfg.PendingCleanup) > 0 && m.screen == screenHome {
		pending := glyphs.Warn + " pending cleanup: " + strings.Join(m.cfg.PendingCleanup, ", ")
		return clampPlainWidth(warningStyle.Render(pending), pending, width)
	}
	return ""
}

// advisory surfaces host conditions that will make a chosen boundary fail at
// run time. It never changes the plan; it only tells the truth early.
func (m model) advisory() string {
	if m.softnetStatusKnown && !m.softnetIsReady && m.screenTouchesSoftnet() {
		if m.screen == screenReview || m.screen == screenExecute {
			return "Set up Softnet before running."
		}
		return "Softnet needs setup for these options."
	}
	return ""
}

func (m model) screenTouchesSoftnet() bool {
	switch m.screen {
	case screenNetwork, screenLANCIDR, screenLANCIDRText:
		return true
	case screenReview, screenExecute:
		return planRequiresSoftnet(m.plan)
	default:
		return false
	}
}

// hintRow shows only keys that are useful at this moment. Input modes reveal
// themselves through the active text field instead of becoming another fixed
// visual object to parse.
func (m model) hintRow(items []string, width int) string {
	return helpLine(fitHints(items, width))
}

// fitHints drops trailing hints that do not fit instead of truncating styled
// text, so the hint row never ends in a half-written key name.
func fitHints(items []string, width int) []string {
	out := make([]string, 0, len(items))
	used := 0
	for _, item := range items {
		cost := lipgloss.Width(item) + 3
		if used+cost > width {
			break
		}
		out = append(out, item)
		used += cost
	}
	return out
}

func helpLine(items []string) string {
	bindings := make([]key.Binding, 0, len(items))
	for _, item := range items {
		keyName, label, _ := strings.Cut(item, " ")
		bindings = append(bindings, key.NewBinding(
			key.WithKeys(keyName),
			key.WithHelp(keyName, label),
		))
	}
	return helpStyle.Render(help.New().ShortHelpView(bindings))
}

func rowBetween(left string, right string, width int) string {
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	gap := width - leftWidth - rightWidth
	if gap < 1 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}

// flattenLine keeps multi-line errors from stretching the fixed status row.
func flattenLine(value string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(value, "\n", " ")), " ")
}

// clampPlainWidth drops a styled string when its plain source is too wide,
// rather than cutting through an escape sequence.
func clampPlainWidth(styled string, plain string, width int) string {
	if lipgloss.Width(plain) <= width {
		return styled
	}
	return mutedStyle.Render(truncate(plain, width))
}
