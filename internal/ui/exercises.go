package ui

import (
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tasnimAlam/pgex/internal/exercise"
	"github.com/tasnimAlam/pgex/internal/progress"
)

// exerciseList is screen 2: the exercises of one category with a preview.
type exerciseList struct {
	d    *Deps
	cat  *exercise.Category
	list list.Model
	w, h int
}

type exItem struct {
	num int // 1-based position in the category
	ex  exercise.Exercise
}

func (e exItem) FilterValue() string { return e.ex.Title }

func newExerciseList(d *Deps, index int) *exerciseList {
	cat := &d.Catalog.Categories[index]
	items := make([]list.Item, len(cat.Exercises))
	for i, ex := range cat.Exercises {
		items[i] = exItem{i + 1, ex}
	}
	s := &exerciseList{d: d, cat: cat}
	s.list = newList(d, items, exDelegate{d})
	return s
}

func (s *exerciseList) Init() tea.Cmd { return s.fetchSelected() }

func (s *exerciseList) Capturing() bool { return s.list.SettingFilter() }

func (s *exerciseList) widths() (left, right int) {
	left = max(40, s.w*45/100)
	return left, max(s.w-left-1, 0)
}

func (s *exerciseList) SetSize(w, h int) {
	s.w, s.h = w, h
	lw, _ := s.widths()
	// Inner height minus the filter/description line and the legend.
	s.list.SetSize(lw-4, bodyHeight(h)-2-3)
}

func (s *exerciseList) fetchSelected() tea.Cmd {
	if it, ok := s.list.SelectedItem().(exItem); ok {
		return s.d.fetchColumns(it.ex)
	}
	return nil
}

func (s *exerciseList) Update(msg tea.Msg) (Screen, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok && !s.list.SettingFilter() {
		switch {
		case key.Matches(k, s.d.Keys.Open):
			if it, ok := s.list.SelectedItem().(exItem); ok {
				return s, func() tea.Msg { return openExerciseMsg{it.ex.Key()} }
			}
			return s, nil
		case key.Matches(k, s.d.Keys.Back) && s.list.FilterState() == list.Unfiltered,
			k.String() == "backspace" && s.list.FilterState() == list.Unfiltered:
			return s, func() tea.Msg { return backMsg{} }
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, tea.Batch(cmd, s.fetchSelected())
}

func (s *exerciseList) View() string {
	t := s.d.Theme
	bh := bodyHeight(s.h)
	lw, rw := s.widths()
	solved := countSolved(s.d.Progress, *s.cat)

	top := t.Dim.Render(s.cat.Description)
	if s.list.FilterState() != list.Unfiltered {
		top = filterLine(t, s.list, lw-4)
	}
	legend := t.Accent.Render("✓") + t.Dim.Render(" solved  ") + t.Warn.Render("●") +
		t.Dim.Render(" attempted  ") + t.Faint.Render("○") + t.Dim.Render(" new")
	listView := lipgloss.NewStyle().Height(bh - 5).Render(s.list.View())
	left := t.Pane("Exercises", top+"\n"+listView+"\n\n"+legend, lw, bh, true)

	right := ""
	if rw >= 10 {
		right = s.preview(rw, bh)
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)

	header := t.Header([]string{"Categories", s.cat.Title},
		t.Dim.Render(fmt.Sprintf("%d / %d solved", solved, len(s.cat.Exercises))), s.w)

	k := s.d.Keys
	help := []helpItem{{"↑/↓", "move"}, short(k.Open, "open"), short(k.Filter, "filter"),
		short(k.Back, "back"), short(k.Help, "help"), short(k.Quit, "quit")}
	if s.list.SettingFilter() {
		help = []helpItem{{"type", "to filter"}, short(k.Open, "apply"), short(k.Back, "clear")}
	}
	return joinScreen(header, body, t.Footer(help, "", s.w))
}

func (s *exerciseList) preview(w, h int) string {
	t := s.d.Theme
	it, ok := s.list.SelectedItem().(exItem)
	if !ok {
		return t.Pane("Preview", t.Dim.Render("No exercise matches the filter."), w, h, false)
	}
	ex := it.ex
	inner := w - 4
	const lw = 18

	lines := wrap(ex.Title, inner, t.Title)
	lines = append(lines, "")
	lines = append(lines, wrap(ex.Question, inner, t.Text)...)
	lines = append(lines, "")

	var cols string
	switch c, ok := s.d.columns[ex.Key()]; {
	case !ok || (c.cols == nil && c.err == nil):
		cols = t.Dim.Render("loading…")
	case c.err != nil:
		cols = t.Dim.Render("unavailable (database offline)")
	default:
		cols = strings.Join(t.chips(c.cols, inner-lw), " ")
	}
	lines = append(lines, t.labelValue("Expected columns", cols, lw))

	order := "no"
	if ex.Ordered {
		order = "yes"
	}
	lines = append(lines, t.labelValue("Order matters", t.Text.Render(order), lw))
	tables := t.Dim.Render("none")
	if len(ex.Tables) > 0 {
		tables = strings.Join(t.chips(ex.Tables, inner-lw), " ")
	}
	lines = append(lines, t.labelValue("Tables", tables, lw))

	e := s.d.Progress.Get(ex.Key())
	attempts := t.Dim.Render("none yet")
	if e.Attempts > 0 {
		outcome := t.Fail.Render("failed")
		if e.LastPass {
			outcome = t.Accent.Render("passed")
		}
		attempts = t.Text.Render(fmt.Sprint(e.Attempts)) + t.Dim.Render(" · last run ") + outcome
	}
	lines = append(lines, t.labelValue("Attempts", attempts, lw))

	// Draft path pinned to the bottom of the pane.
	innerH := h - 2
	for len(lines) < innerH-1 {
		lines = append(lines, "")
	}
	lines = append(lines[:innerH-1], t.labelValue("Draft", t.Dim.Render(tildePath(s.d.Drafts.Path(ex.Key()))), 6))
	return t.Pane("Preview", strings.Join(lines, "\n"), w, h, false)
}

type exDelegate struct{ d *Deps }

func (exDelegate) Height() int                         { return 1 }
func (exDelegate) Spacing() int                        { return 0 }
func (exDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (e exDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	it, ok := item.(exItem)
	if !ok {
		return
	}
	t := e.d.Theme
	sel := index == m.Index()
	status := e.d.Progress.Get(it.ex.Key()).Status
	base := t.on(t.Text, sel)
	if sel {
		base = t.on(t.Bright, true)
	}
	sp := t.on(t.Text, sel).Render(" ")
	content := t.on(markerStyle(t, status), sel).Render(markerGlyph(status)) + sp +
		t.on(t.Dim, sel).Render(fmt.Sprintf("%2d.", it.num)) + sp +
		highlight(it.ex.Title, m.MatchesForItem(index), base)
	fmt.Fprint(w, t.row(t.fitOn(content, m.Width()-1, sel), sel, m.Width()))
}

func markerGlyph(s progress.Status) string {
	switch s {
	case progress.Solved:
		return "✓"
	case progress.Attempted:
		return "●"
	case progress.Revealed:
		return "◐"
	default:
		return "○"
	}
}

func markerStyle(t Theme, s progress.Status) lipgloss.Style {
	switch s {
	case progress.Solved:
		return t.Accent
	case progress.Attempted:
		return t.Warn
	case progress.Revealed:
		return t.Dim
	default:
		return t.Faint
	}
}
