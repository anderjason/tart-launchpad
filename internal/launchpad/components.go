package launchpad

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// This file holds the interaction components every screen composes from: a
// cursor list, a text prompt, a grant choice list, and an action list. Screens
// describe what to show; components decide how a row, a cursor, or a prompt
// behaves.

const (
	choiceLabelColumn = 28
	actionLabelColumn = 24
	ledgerLabelColumn = 14
	stackedRowWidth   = 72
)

// navigateList is the shared cursor behavior for every list in the app.
// It reports whether the key belonged to the list so callers can fall through.
func navigateList(cursor int, count int, key string) (int, bool) {
	cursor = clampCursor(cursor, count)
	switch key {
	case "up", "k":
		if cursor > 0 {
			cursor--
		}
	case "down", "j":
		if cursor < count-1 {
			cursor++
		}
	default:
		return cursor, false
	}
	return cursor, true
}

// namedKeys are terminal key names that must never be typed into a prompt.
var namedKeys = map[string]bool{
	"up": true, "down": true, "left": true, "right": true,
	"enter": true, "esc": true, "escape": true, "tab": true,
	"backspace": true, "delete": true, "home": true, "end": true,
	"pgup": true, "pgdown": true, "space": true,
}

// editTextField is the shared prompt behavior. In a text-capturing mode every
// printable rune is content, so no prompt has to guess whether "h" meant back.
func editTextField(value string, cursor int, keyName string, allow func(rune) bool) (string, int, bool) {
	switch keyName {
	case "left":
		if cursor > 0 {
			cursor--
		}
		return value, cursor, true
	case "right":
		if cursor < runeCount(value) {
			cursor++
		}
		return value, cursor, true
	case "backspace":
		value, cursor = deleteBeforeCursor(value, cursor)
		return value, cursor, true
	case "ctrl+a":
		return value, 0, true
	case "ctrl+e":
		return value, runeCount(value), true
	case "ctrl+u":
		return substringRunes(value, cursor, runeCount(value)), 0, true
	case "ctrl+k":
		return substringRunes(value, 0, cursor), cursor, true
	case "ctrl+w":
		value, cursor = deleteWordBeforeCursor(value, cursor)
		return value, cursor, true
	}
	if namedKeys[keyName] || strings.Contains(keyName, "+") {
		return value, cursor, false
	}
	var b strings.Builder
	for _, r := range keyName {
		if allow(r) {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return value, cursor, false
	}
	value, cursor = insertAtCursor(value, cursor, b.String())
	return value, cursor, true
}

func anyPrintableRune(r rune) bool {
	return r >= 32 && r != 127
}

func cursorCell(selected bool) string {
	return cursorCellFor(unicodeGlyphSet, selected)
}

// cursorCellFor keeps the selected and unselected cursor cells the same width
// so rows never shift sideways as the cursor moves.
func cursorCellFor(glyphs glyphSet, selected bool) string {
	if selected {
		return selectedStyle.Render(glyphs.Cursor) + " "
	}
	return strings.Repeat(" ", lipgloss.Width(glyphs.Cursor)+1)
}

func sectionHeading(title string) string {
	return sectionLabelStyle.Render(title)
}

type accessChoice struct {
	Label       string
	Short       string
	Description string
	Danger      bool
	Attention   bool
}

// shortText is the concise explanation shown beside each choice. Description
// remains available to the review ledger.
func (c accessChoice) shortText() string {
	if c.Short != "" {
		return c.Short
	}
	return c.Description
}

func renderAccessChoice(choice accessChoice, selected bool) string {
	return accessChoiceRow(unicodeGlyphSet, choice, selected)
}

func accessChoiceRow(glyphs glyphSet, choice accessChoice, selected bool) string {
	return strings.Join(choiceRows(glyphs, choice.Label, choice.shortText(), selected, choice.Danger, frameDefaultWidth), "\n")
}

// choiceListBody is the shared choose template. At narrow widths, supporting
// text moves below the label instead of being truncated or squeezed.
func choiceListBody(glyphs glyphSet, prompt string, choices []accessChoice, cursor int, width int) string {
	cursor = clampCursor(cursor, len(choices))
	lines := []string{questionStyle.Render(prompt), ""}
	for i, choice := range choices {
		lines = append(lines, choiceRows(glyphs, choice.Label, choice.shortText(), i == cursor, choice.Danger, width)...)
	}
	return strings.Join(lines, "\n")
}

func choiceRows(glyphs glyphSet, label string, description string, selected bool, danger bool, width int) []string {
	prefix := cursorCellFor(glyphs, selected)
	labelStyle := textStyle
	if selected {
		labelStyle = selectedStyle
	} else if danger {
		labelStyle = dangerStyle
	}
	detailStyle := mutedStyle
	if selected {
		detailStyle = textStyle
	}
	if width < stackedRowWidth {
		return []string{
			prefix + labelStyle.Render(label),
			strings.Repeat(" ", lipgloss.Width(prefix)+2) + detailStyle.Render(description),
		}
	}
	return []string{prefix + labelStyle.Render(fixedDisplayWidth(label, choiceLabelColumn)) + " " + detailStyle.Render(description)}
}

type actionRow struct {
	ID          string
	Section     string
	Label       string
	Description string
	Danger      bool
	Attention   bool
}

func actionListBody(glyphs glyphSet, question string, rows []actionRow, cursor int, width int) string {
	cursor = clampCursor(cursor, len(rows))
	lines := []string{questionStyle.Render(question), ""}
	lastSection := ""
	for i, row := range rows {
		if row.Section != "" && row.Section != lastSection {
			if lastSection != "" {
				lines = append(lines, "")
			}
			lines = append(lines, sectionHeading(row.Section))
			lastSection = row.Section
		}
		lines = append(lines, actionRows(glyphs, row, i == cursor, width)...)
	}
	return strings.Join(lines, "\n")
}

func actionRows(glyphs glyphSet, row actionRow, selected bool, width int) []string {
	prefix := cursorCellFor(glyphs, selected)
	labelStyle := textStyle
	if selected {
		labelStyle = selectedStyle
	} else if row.Danger {
		labelStyle = dangerStyle
	}
	detailStyle := mutedStyle
	if row.Attention && !selected {
		detailStyle = attentionStyle
	} else if selected {
		detailStyle = textStyle
	}
	if width < stackedRowWidth {
		return []string{
			prefix + labelStyle.Render(row.Label),
			strings.Repeat(" ", lipgloss.Width(prefix)+2) + detailStyle.Render(row.Description),
		}
	}
	return []string{prefix + labelStyle.Render(fixedDisplayWidth(row.Label, actionLabelColumn)) + " " + detailStyle.Render(row.Description)}
}

// promptBody is the shared text-entry template: one question, one field, and
// one local hint or validation message.
func promptBody(question string, value string, cursor int, hint string, validation string) string {
	lines := []string{questionStyle.Render(question), "", "  " + renderTextInputValue(value, cursor), ""}
	if validation != "" {
		lines = append(lines, attentionStyle.Render("▲ "+validation))
	} else if hint != "" {
		lines = append(lines, mutedStyle.Render(hint))
	}
	return strings.Join(lines, "\n")
}

// boundaryLine is the ledger row shape used by the manifest everywhere it
// appears: a fixed-width label column followed by the granted value.
func boundaryLine(label string, value string) string {
	return mutedStyle.Render(fixedDisplayWidth(label, ledgerLabelColumn)) + " " + value
}

func folderAccessChoices() []accessChoice {
	return []accessChoice{
		{
			Label:       "None",
			Description: "Do not share a host folder",
		},
		{
			Label:       "Read only",
			Description: "The VM can read the project folder",
		},
		{
			Label:       "Read and write",
			Description: "The VM can change the project folder",
		},
	}
}

func networkAccessChoices(cfg Config) []accessChoice {
	lanDescription := summarizeCIDRs(cfg.Network.LANCIDRs)
	return []accessChoice{
		{
			Label:       "Block outbound IPv4",
			Description: "Not a guarantee of complete network isolation",
		},
		{
			Label:       "Internet",
			Description: "This Mac and local networks are blocked",
		},
		{
			Label:       "This Mac",
			Description: "Reach services on this Mac only",
		},
		{
			Label:       "Local network",
			Description: "Reach " + lanDescription + "; internet blocked",
		},
		{
			Label:       "Internet + local network",
			Description: "Reach the internet and " + lanDescription,
		},
	}
}

func folderAccessChoice(folder FolderAccess) accessChoice {
	choices := folderAccessChoices()
	if index := indexFolder(folder); index >= 0 && index < len(choices) {
		return choices[index]
	}
	return accessChoice{
		Label:       string(folder),
		Description: string(folder),
	}
}

func networkAccessChoice(cfg Config, network NetworkAccess) accessChoice {
	choices := networkAccessChoices(cfg)
	if index := indexNetwork(network); index >= 0 && index < len(choices) {
		return choices[index]
	}
	return accessChoice{
		Label:       string(network),
		Description: string(network),
	}
}

func runDurationChoices() []accessChoice {
	return []accessChoice{
		{
			Label:       "Temporary",
			Description: "Run a fresh VM, then delete it",
		},
		{
			Label:       "Keep as workspace",
			Description: "Create a persistent VM for ongoing work",
		},
	}
}

func vmKindChoices() []accessChoice {
	return []accessChoice{
		{
			Label:       "Template",
			Description: "Protected source for creating VMs",
		},
		{
			Label:       "Workspace",
			Description: "Persistent VM for normal runs",
		},
	}
}
