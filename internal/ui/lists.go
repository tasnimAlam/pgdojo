package ui

import (
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// newList configures a bubbles list for our own chrome: no title, status
// bar, pagination or help, substring filtering, and our key bindings.
func newList(d *Deps, items []list.Item, del list.ItemDelegate) list.Model {
	l := list.New(items, del, 0, 0)
	l.SetShowTitle(false)
	l.SetShowFilter(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()
	l.Filter = substringFilter
	l.Styles.NoItems = d.Theme.Dim

	k := d.Keys
	none := key.NewBinding(key.WithDisabled())
	l.KeyMap = list.KeyMap{
		CursorUp:             k.Up,
		CursorDown:           k.Down,
		PrevPage:             key.NewBinding(key.WithKeys("pgup")),
		NextPage:             key.NewBinding(key.WithKeys("pgdown")),
		GoToStart:            key.NewBinding(key.WithKeys("home")),
		GoToEnd:              key.NewBinding(key.WithKeys("end")),
		Filter:               k.Filter,
		ClearFilter:          k.Back,
		CancelWhileFiltering: k.Back,
		AcceptWhileFiltering: key.NewBinding(key.WithKeys("enter", "tab", "up", "down")),
		ShowFullHelp:         none,
		CloseFullHelp:        none,
		Quit:                 none,
		ForceQuit:            none,
	}
	return l
}

// substringFilter keeps items containing term (case-insensitive) and
// reports the matched rune range for underlining.
func substringFilter(term string, targets []string) []list.Rank {
	t := strings.ToLower(term)
	var ranks []list.Rank
	for i, s := range targets {
		lower := strings.ToLower(s)
		idx := strings.Index(lower, t)
		if idx < 0 {
			continue
		}
		start := utf8.RuneCountInString(lower[:idx])
		m := make([]int, utf8.RuneCountInString(t))
		for j := range m {
			m[j] = start + j
		}
		ranks = append(ranks, list.Rank{Index: i, MatchedIndexes: m})
	}
	return ranks
}

// filterLine renders "/ query▌" with "N matches" on the right.
func filterLine(t Theme, l list.Model, w int) string {
	q := l.FilterValue()
	if l.SettingFilter() {
		q = l.FilterInput.Value()
	}
	left := t.Accent.Render("/ ") + t.Text.Render(q)
	if l.SettingFilter() {
		left += t.Accent.Render("▌")
	}
	return spread(left, t.Dim.Render(fmt.Sprintf("%d matches", len(l.VisibleItems()))), w)
}

// highlight renders s in base, underlining the runes at matches.
func highlight(s string, matches []int, base lipgloss.Style) string {
	if len(matches) == 0 {
		return base.Render(s)
	}
	set := make(map[int]bool, len(matches))
	for _, m := range matches {
		set[m] = true
	}
	var b strings.Builder
	runes := []rune(s)
	for i := 0; i < len(runes); {
		j := i
		for j < len(runes) && set[j] == set[i] {
			j++
		}
		st := base
		if set[i] {
			st = st.Underline(true)
		}
		b.WriteString(st.Render(string(runes[i:j])))
		i = j
	}
	return b.String()
}

// chips renders values as small labels, wrapping to width w.
func (t Theme) chips(vals []string, w int) []string {
	var lines []string
	cur := ""
	for _, v := range vals {
		c := t.Chip.Render(" " + v + " ")
		if cur != "" && lipgloss.Width(cur)+1+lipgloss.Width(c) > w {
			lines = append(lines, cur)
			cur = ""
		}
		if cur != "" {
			cur += " "
		}
		cur += c
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// wrap word-wraps plain text to width w and styles it.
func wrap(s string, w int, st lipgloss.Style) []string {
	if w < 1 {
		return nil
	}
	// YAML block text has hard line breaks; keep only paragraph breaks.
	paras := strings.Split(strings.TrimSpace(s), "\n\n")
	for i, p := range paras {
		paras[i] = strings.Join(strings.Fields(p), " ")
	}
	out := lipgloss.NewStyle().Width(w).Render(strings.Join(paras, "\n\n"))
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		lines[i] = st.Render(strings.TrimRight(l, " "))
	}
	return lines
}

// ago formats a past time in words.
func ago(t time.Time, now time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return plural(int(d.Hours()), "hour") + " ago"
	case d < 48*time.Hour:
		return "yesterday"
	default:
		return plural(int(d.Hours()/24), "day") + " ago"
	}
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// tildePath shortens a path under $HOME to ~/...
func tildePath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home+"/") {
		return "~" + p[len(home):]
	}
	return p
}

// labelValue renders "label   value" with a fixed label column.
func (t Theme) labelValue(label, value string, labelW int) string {
	return t.Dim.Render(fmt.Sprintf("%-*s", labelW, label)) + value
}

// row renders a list row: a selection bar and background when selected.
// content must already be styled with on(..., selected) so the background
// covers every segment.
func (t Theme) row(content string, selected bool, w int) string {
	if lipgloss.Width(content) > w-1 {
		content = ansi.Truncate(content, w-1, "…")
	}
	pad := strings.Repeat(" ", max(w-1-lipgloss.Width(content), 0))
	if selected {
		return t.SelBar.Render("▌") + content + t.SelRow.Render(pad)
	}
	return " " + content + pad
}

// on adds the selected-row background to st when selected.
func (t Theme) on(st lipgloss.Style, selected bool) lipgloss.Style {
	if selected {
		return st.Background(t.P.SelBg)
	}
	return st
}

// fitOn is fit with padding that carries the selected-row background.
func (t Theme) fitOn(s string, w int, selected bool) string {
	if lipgloss.Width(s) > w {
		return ansi.Truncate(s, w, "…")
	}
	return s + t.on(t.Text, selected).Render(strings.Repeat(" ", w-lipgloss.Width(s)))
}
