package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// helpOverlay draws the keyboard shortcuts modal centered over a dimmed
// copy of the current screen.
func (m Model) helpOverlay(under string) string {
	t := m.deps.Theme
	w := min(max(m.width*70/100, 76), m.width-2)
	inner := w - 4
	colW := (inner - 4) / 3

	var cols []string
	for _, g := range m.deps.Keys.HelpGroups() {
		keyW := 0
		for _, it := range g.items {
			keyW = max(keyW, lipgloss.Width(it.key))
		}
		lines := []string{t.Title.Render(g.title), ""}
		for _, it := range g.items {
			pad := strings.Repeat(" ", keyW-lipgloss.Width(it.key))
			lines = append(lines, fit(t.HelpKey.Render(it.key)+pad+"  "+t.Text.Render(it.desc), colW))
		}
		cols = append(cols, lipgloss.NewStyle().Width(colW).Render(strings.Join(lines, "\n")))
	}
	grid := lipgloss.JoinHorizontal(lipgloss.Top, cols[0], "  ", cols[1], "  ", cols[2])

	body := spread(t.Accent.Bold(true).Render("Keyboard shortcuts"), t.Dim.Render("esc or ? to close"), inner) +
		"\n\n" + grid + "\n\n" +
		strings.Join(wrap("Revealing the answer asks for confirmation and marks the exercise as revealed, not solved.", inner, t.Dim), "\n")
	h := min(lipgloss.Height(body)+2, m.height)
	modal := t.PaneColored("", body, w, h, t.P.Accent, t.Accent)

	return t.Overlay(under, modal, m.width, m.height)
}
