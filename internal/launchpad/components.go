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
	choiceBadgeColumn = 20
	ledgerLabelColumn = 10
)

// navigateList is the shared cursor behavior for every list in the app.
// It reports whether the key belonged to the list so callers can fall through.
func navigateList(cursor int, count int, key string) (int, bool) {
	cursor = clampCursor(cursor, count)
	switch key {
	case "up", "k", "ctrl+p":
		if cursor > 0 {
			cursor--
		}
	case "down", "j", "ctrl+n":
		if cursor < count-1 {
			cursor++
		}
	case "g", "home":
		cursor = 0
	case "G", "end":
		cursor = max(0, count-1)
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

func checkboxCell(glyphs glyphSet, checked bool) string {
	if checked {
		return warningStyle.Render(glyphs.Checked)
	}
	return mutedStyle.Render(glyphs.Unchecked)
}

func sectionHeading(title string) string {
	return sectionLabelStyle.Render(title)
}

type accessChoice struct {
	Label       string
	Short       string
	Description string
	Style       lipgloss.Style
	Danger      bool
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
	prefix := cursorCellFor(glyphs, selected)
	labelCell := fixedDisplayWidth(grantBadge(choice), choiceBadgeColumn)
	text := choice.shortText()
	if choice.Danger {
		return prefix + labelCell + " " + dangerStyle.Render(text)
	}
	if selected {
		return prefix + labelCell + " " + textStyle.Render(text)
	}
	return prefix + labelCell + " " + mutedStyle.Render(text)
}

// choiceListBody renders one concise explanation for each choice.
func choiceListBody(glyphs glyphSet, prompt string, choices []accessChoice, cursor int) string {
	cursor = clampCursor(cursor, len(choices))
	lines := []string{textStyle.Render(prompt), ""}
	for i, choice := range choices {
		lines = append(lines, accessChoiceRow(glyphs, choice, i == cursor))
	}
	return strings.Join(lines, "\n")
}

type actionRow struct {
	Label       string
	Description string
	Danger      bool
}

func actionListBody(glyphs glyphSet, intro []string, rows []actionRow, cursor int) string {
	cursor = clampCursor(cursor, len(rows))
	lines := append([]string{}, intro...)
	if len(lines) > 0 {
		lines = append(lines, "")
	}
	for i, row := range rows {
		prefix := cursorCellFor(glyphs, i == cursor)
		label := fixedDisplayWidth(row.Label, 18)
		switch {
		case row.Danger:
			label = dangerStyle.Render(label)
		case i == cursor:
			label = selectedStyle.Render(label)
		}
		description := mutedStyle.Render(row.Description)
		if i == cursor {
			description = textStyle.Render(row.Description)
		}
		lines = append(lines, prefix+label+" "+description)
	}
	return strings.Join(lines, "\n")
}

// promptBody renders a labelled text prompt. The frame already shows INSERT, so
// the prompt only has to show the value, the caret, and what is allowed.
func promptBody(label string, value string, cursor int, context []string, allowed string) string {
	lines := append([]string{}, context...)
	if len(lines) > 0 {
		lines = append(lines, "")
	}
	lines = append(lines,
		sectionHeading(label),
		"",
		"  "+renderTextInputValue(value, cursor),
		"",
		mutedStyle.Render(allowed),
	)
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
			Label:       string(FolderNoFolder),
			Short:       "no host folder",
			Description: "no host folder mounted",
			Style:       noFolderBadgeStyle,
		},
		{
			Label:       string(FolderReadFolder),
			Short:       "project folder, read-only",
			Description: "selected project folder read-only",
			Style:       readHereBadgeStyle,
		},
		{
			Label:       string(FolderEditFolder),
			Short:       "project folder, read-write",
			Description: "selected project folder read-write",
			Style:       editHereBadgeStyle,
		},
	}
}

func networkAccessChoices(cfg Config) []accessChoice {
	lanDescription := summarizeCIDRs(cfg.Network.LANCIDRs)
	return []accessChoice{
		{
			Label:       string(NetworkOffline),
			Short:       "outbound IPv4 blocked",
			Description: "outbound IPv4 blocked",
			Style:       offlineBadgeStyle,
		},
		{
			Label:       string(NetworkInternet),
			Short:       "internet, no local networks",
			Description: "internet via Softnet; host/local private networks blocked",
			Style:       internetBadgeStyle,
		},
		{
			Label:       string(NetworkHost),
			Short:       "this Mac only",
			Description: "host-only network",
			Style:       hostBadgeStyle,
		},
		{
			Label:       string(NetworkLAN),
			Short:       "LAN only",
			Description: "LAN only: " + lanDescription,
			Style:       lanBadgeStyle,
		},
		{
			Label:       string(NetworkLANAndInternet),
			Short:       "internet plus LAN",
			Description: "internet plus LAN: " + lanDescription,
			Style:       lanInternetStyle,
		},
	}
}

func folderAccessChoice(folder FolderAccess) accessChoice {
	for _, choice := range folderAccessChoices() {
		if choice.Label == string(folder) {
			return choice
		}
	}
	return accessChoice{
		Label:       string(folder),
		Description: string(folder),
		Style:       unknownBadgeStyle,
	}
}

func networkAccessChoice(cfg Config, network NetworkAccess) accessChoice {
	for _, choice := range networkAccessChoices(cfg) {
		if choice.Label == string(network) {
			return choice
		}
	}
	return accessChoice{
		Label:       string(network),
		Description: string(network),
		Style:       unknownBadgeStyle,
	}
}

// networkReviewChoice spells out the configured CIDRs instead of summarizing
// them: review is the last place to read exactly what will be allowed.
func networkReviewChoice(cfg Config, network NetworkAccess) accessChoice {
	choice := networkAccessChoice(cfg, network)
	if len(cfg.Network.LANCIDRs) == 0 {
		return choice
	}
	switch network {
	case NetworkLAN:
		choice.Description = "LAN only: " + strings.Join(cfg.Network.LANCIDRs, ",")
	case NetworkLANAndInternet:
		choice.Description = "internet plus LAN: " + strings.Join(cfg.Network.LANCIDRs, ",")
	}
	return choice
}

func folderReviewDescription(folder FolderAccess) string {
	choice := folderAccessChoice(folder)
	return grantBadge(choice) + "  " + choice.Description
}

func networkReviewDescription(cfg Config, network NetworkAccess) string {
	choice := networkReviewChoice(cfg, network)
	return grantBadge(choice) + "  " + choice.Description
}

func runDurationChoices() []accessChoice {
	return []accessChoice{
		{
			Label:       string(RunDurationTemporaryRun),
			Short:       "clone, run, delete afterward",
			Description: "clone from the template, run, then delete the clone",
			Style:       temporaryRunBadgeStyle,
		},
		{
			Label:       string(RunDurationWorkspace),
			Short:       "keep for ongoing work",
			Description: "clone from the template and keep the VM",
			Style:       workspaceDurationBadgeStyle,
		},
	}
}

func vmKindChoices() []accessChoice {
	return []accessChoice{
		{
			Label:       string(VMKindTemplate),
			Short:       "kept clean; source for new VMs",
			Description: "kept clean; source for new VMs",
			Style:       templateBadgeStyle,
		},
		{
			Label:       string(VMKindWorkspace),
			Short:       "kept around; run directly",
			Description: "kept around; run directly for ongoing work",
			Style:       workspaceKindBadgeStyle,
		},
	}
}
