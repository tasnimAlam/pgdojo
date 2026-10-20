package ui

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tasnimAlam/pgex/internal/exercise"
	"github.com/tasnimAlam/pgex/internal/progress"
)

// categories is screen 1 (home).
type categories struct {
	d    *Deps
	list list.Model
	w, h int

	resetting bool            // the "type reset" prompt is open
	input     textinput.Model // for the reset confirmation
	note      string          // result of the last reset
}

type catItem struct {
	index int
	cat   *exercise.Category
}

func (c catItem) FilterValue() string { return c.cat.Title }

func newCategories(d *Deps) *categories {
	items := make([]list.Item, len(d.Catalog.Categories))
	for i := range d.Catalog.Categories {
		items[i] = catItem{i, &d.Catalog.Categories[i]}
	}
	s := &categories{d: d, input: textinput.New()}
	s.input.Prompt = "> "
	s.input.Placeholder = "reset"
	s.list = newList(d, items, catDelegate{d})
	return s
}

func (s *categories) Init() tea.Cmd { return nil }

func (s *categories) Capturing() bool { return s.list.SettingFilter() || s.resetting }

func (s *categories) SetSize(w, h int) {
	s.w, s.h = w, h
	lw, _ := s.widths()
	// Inner pane height minus the summary/filter line.
	s.list.SetSize(lw-4, bodyHeight(h)-3)
}

func (s *categories) widths() (left, right int) {
	left = max(50, s.w*45/100)
	return left, max(s.w-left-1, 0)
}

func (s *categories) Update(msg tea.Msg) (Screen, tea.Cmd) {
	if n, ok := msg.(noteMsg); ok {
		s.note = n.text
		return s, nil
	}
	if s.resetting {
		return s, s.updateReset(msg)
	}
	if k, ok := msg.(tea.KeyPressMsg); ok && !s.list.SettingFilter() {
		switch {
		case key.Matches(k, s.d.Keys.Reset):
			s.resetting, s.note = true, ""
			s.input.SetValue("")
			return s, s.input.Focus()
		case key.Matches(k, s.d.Keys.Open):
			if it, ok := s.list.SelectedItem().(catItem); ok {
				return s, func() tea.Msg { return openCategoryMsg{it.index} }
			}
			return s, nil
		case key.Matches(k, s.d.Keys.Continue):
			if last, ok := s.d.Progress.Last(); ok {
				return s, func() tea.Msg { return openExerciseMsg{key: last.Category + "/" + last.Exercise} }
			}
			return s, nil
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, cmd
}

// updateReset handles the reset prompt: typing "reset" and enter erases
// all progress; anything else cancels.
func (s *categories) updateReset(msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(msg)
		return cmd
	}
	switch k.String() {
	case "esc":
		s.resetting, s.note = false, "Reset cancelled."
		return nil
	case "enter":
		s.resetting = false
		s.input.Blur()
		if s.input.Value() != "reset" {
			s.note = "Reset cancelled: type the word reset to confirm."
			return nil
		}
		p := s.d.Progress
		return func() tea.Msg {
			if err := p.Reset(); err != nil {
				return noteMsg{"Could not reset progress: " + err.Error()}
			}
			return noteMsg{"All progress was reset."}
		}
	}
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	return cmd
}

func (s *categories) View() string {
	out := s.view()
	if !s.resetting {
		return out
	}
	t := s.d.Theme
	w := min(s.w-4, 60)
	lines := wrap("This erases solved status, attempts and hints for every exercise. Your saved drafts are kept.", w-4, t.Text)
	lines = append(lines, "")
	lines = append(lines, wrap("Type reset and press enter to confirm, or esc to cancel.", w-4, t.Dim)...)
	lines = append(lines, "", s.input.View())
	modal := t.PaneColored("Reset all progress", strings.Join(lines, "\n"), w, len(lines)+2, t.P.Fail, t.Fail.Bold(true))
	return t.Overlay(out, modal, s.w, s.h)
}

func (s *categories) view() string {
	t := s.d.Theme
	solved, total := 0, 0
	for _, c := range s.d.Catalog.Categories {
		solved += countSolved(s.d.Progress, c)
		total += len(c.Exercises)
	}

	bh := bodyHeight(s.h)
	lw, rw := s.widths()

	top := t.Dim.Render(fmt.Sprintf("%d categories · %d of %d solved", len(s.d.Catalog.Categories), solved, total))
	if s.list.FilterState() != list.Unfiltered {
		top = filterLine(t, s.list, lw-4)
	}
	left := t.Pane("Categories", top+"\n"+s.list.View(), lw, bh, true)

	right := ""
	if rw >= 10 {
		last, hasLast := s.d.Progress.Last()
		detailH := bh
		var cont string
		if hasLast {
			detailH = bh - 4
			cont = "\n" + t.Pane("Continue where you left off", s.continueText(last, rw-4), rw, 4, false)
		}
		right = s.detail(rw, detailH) + cont
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)

	conn := t.Dim.Render("● connecting")
	switch s.d.conn {
	case connUp:
		conn = t.Accent.Render("● connected")
	case connDown:
		conn = t.Fail.Render("● offline")
	}
	header := t.Header([]string{"PostgreSQL exercises in your terminal"},
		t.Dim.Render("db ")+t.Text.Render(s.d.DBLabel)+" "+conn, s.w)

	k := s.d.Keys
	help := []helpItem{{"↑/↓", "move"}, short(k.Open, "open"), short(k.Continue, "continue"),
		short(k.Filter, "filter"), short(k.Help, "help"), short(k.Quit, "quit")}
	if s.list.SettingFilter() {
		help = []helpItem{{"type", "to filter"}, short(k.Open, "apply"), short(k.Back, "clear")}
	}
	pct := 0
	if total > 0 {
		pct = solved * 100 / total
	}
	status := t.Dim.Render(fmt.Sprintf("%d%%", pct))
	if s.note != "" {
		status = t.Warn.Render(s.note) + "  " + status
	}
	return joinScreen(header, body, t.Footer(help, status, s.w))
}

func (s *categories) detail(w, h int) string {
	t := s.d.Theme
	it, ok := s.list.SelectedItem().(catItem)
	if !ok {
		return t.Pane("", t.Dim.Render("No category matches the filter."), w, h, false)
	}
	c := it.cat
	inner := w - 4

	hints, revealed := 0, 0
	var lastRun time.Time
	tables := map[string]bool{}
	for _, ex := range c.Exercises {
		e := s.d.Progress.Get(ex.Key())
		hints += e.HintsUsed
		if e.Revealed {
			revealed++
		}
		if e.UpdatedAt.After(lastRun) {
			lastRun = e.UpdatedAt
		}
		for _, tb := range ex.Tables {
			tables[tb] = true
		}
	}
	names := make([]string, 0, len(tables))
	for tb := range tables {
		names = append(names, tb)
	}
	sort.Strings(names)

	lines := wrap(c.Description, inner, t.Text)
	lines = append(lines, "")
	col := func(label, val string) string { return t.labelValue(label, t.Text.Render(val), 9) }
	stats := []string{
		col("solved", fmt.Sprintf("%d/%d", countSolved(s.d.Progress, *c), len(c.Exercises))),
		col("hints", fmt.Sprintf("%d used", hints)),
		col("answers", fmt.Sprintf("%d rev.", revealed)),
		col("last run", ago(lastRun, s.d.Now())),
	}
	if inner >= 44 {
		stats = []string{fit(stats[0], 22) + stats[1], fit(stats[2], 22) + stats[3]}
	}
	lines = append(lines, stats...)
	lines = append(lines,
		"",
		t.Dim.Render("Tables used"),
	)
	lines = append(lines, t.chips(names, inner)...)
	return t.Pane(c.Title, strings.Join(lines, "\n"), w, h, false)
}

func (s *categories) continueText(last progress.Last, w int) string {
	t := s.d.Theme
	for _, c := range s.d.Catalog.Categories {
		if c.ID != last.Category {
			continue
		}
		for i, ex := range c.Exercises {
			if ex.ID == last.Exercise {
				line := t.Accent.Render("→ ") + t.Dim.Render(c.Title+" › ") + t.Text.Render(fmt.Sprintf("%d. %s", i+1, ex.Title))
				return line + "\n" + t.Dim.Render("press ") + t.Key.Render("c") + t.Dim.Render(" to continue")
			}
		}
	}
	return t.Dim.Render("The last exercise no longer exists.")
}

func countSolved(p *progress.Store, c exercise.Category) int {
	n := 0
	for _, ex := range c.Exercises {
		if p.Get(ex.Key()).Status == progress.Solved {
			n++
		}
	}
	return n
}

type catDelegate struct{ d *Deps }

func (catDelegate) Height() int                         { return 1 }
func (catDelegate) Spacing() int                        { return 0 }
func (catDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (c catDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	it, ok := item.(catItem)
	if !ok {
		return
	}
	t := c.d.Theme
	sel := index == m.Index()
	solved, total := countSolved(c.d.Progress, *it.cat), len(it.cat.Exercises)

	width := m.Width()
	nameW := max(width-1-2-1-10-1-6, 4)
	arrow, name := t.on(t.Text, sel).Render("  "), t.on(t.Text, sel)
	if sel {
		arrow, name = t.on(t.Accent, true).Render("▸ "), t.on(t.Bright, true)
	}
	title := highlight(it.cat.Title, m.MatchesForItem(index), name)
	title = t.fitOn(title, nameW, sel)
	sp := t.on(t.Text, sel).Render(" ")
	count := t.on(t.Dim, sel).Render(fmt.Sprintf("%6s", fmt.Sprintf("%d/%d", solved, total)))
	fmt.Fprint(w, t.row(arrow+title+sp+t.ProgressBar(solved, total)+sp+count, sel, width))
}
