package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"

	"github.com/tasnimAlam/pgex/internal/db"
	"github.com/tasnimAlam/pgex/internal/exercise"
	"github.com/tasnimAlam/pgex/internal/progress"
)

type focusArea int

const (
	focusEditor focusArea = iota
	focusResult
	focusLeft
	focusCount
)

type confirmKind int

const (
	confirmNone confirmKind = iota
	confirmReveal
	confirmClear
)

// overlayKind is a pane shown as a modal in the narrow layout, where the
// left column doesn't fit.
type overlayKind int

const (
	overlayNone overlayKind = iota
	overlaySchema
	overlayHint
)

// narrowWidth is the width below which panes stack vertically.
const narrowWidth = 110

// resultMode is what the result pane is showing.
type resultMode int

const (
	resNone   resultMode = iota
	resCheck             // a completed check (pass, fail or user error)
	resError             // a run that could not complete
	resAnswer            // the revealed reference answer
)

// exerciseView is screens 3 and 4: question, editor, and result.
type exerciseView struct {
	d    *Deps
	ex   exercise.Exercise
	cat  *exercise.Category
	num  int // 1-based position in the category
	w, h int

	editor     editor
	focus      focusArea
	insert     bool // editor focused and taking keys (INSERT mode)
	hintsShown int
	confirm    confirmKind

	mode     resultMode
	prevMode resultMode // restored when the answer view closes
	chk      db.Check
	errTitle string // banner for resError
	errText  string // explanation with a next step
	view     viewMode
	rowOff   int
	colOff   int

	running bool
	cancel  context.CancelFunc
	seq     int
	spin    spinner.Model

	overlay overlayKind     // narrow layout: schema or hint shown on top
	disc    *viewport.Model // discussion overlay when non-nil
	note    string          // transient footer message
}

func newExerciseView(d *Deps, ex exercise.Exercise) *exerciseView {
	s := &exerciseView{d: d, ex: ex, editor: newEditor(d.Theme)}
	for ci := range d.Catalog.Categories {
		c := &d.Catalog.Categories[ci]
		for i, e := range c.Exercises {
			if e.Key() == ex.Key() {
				s.cat, s.num = c, i+1
			}
		}
	}
	s.spin = spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(d.Theme.Accent))
	s.hintsShown = min(d.Progress.Get(ex.Key()).HintsUsed, len(ex.Hints))
	return s
}

func (s *exerciseView) Init() tea.Cmd {
	p, drafts, ex := s.d.Progress, s.d.Drafts, s.ex
	load := func() tea.Msg {
		text, err := drafts.Load(ex.Key())
		return draftLoadedMsg{text: text, err: err}
	}
	setLast := func() tea.Msg {
		if err := p.SetLast(ex.Category, ex.ID); err != nil {
			return noteMsg{"Could not save progress: " + err.Error()}
		}
		return nil
	}
	s.insert = true
	return tea.Batch(load, setLast, s.editor.Focus())
}

// Capturing is true when keys must reach the editor or a prompt.
func (s *exerciseView) Capturing() bool {
	return s.inserting() || s.confirm != confirmNone
}

// Cancel stops a running query; ctrl+c then doesn't quit.
func (s *exerciseView) Cancel() bool {
	if s.running && s.cancel != nil {
		s.cancel()
		return true
	}
	return false
}

// Leave saves the draft and stops any running query.
func (s *exerciseView) Leave() tea.Cmd {
	s.Cancel()
	return s.saveDraft()
}

func (s *exerciseView) saveDraft() tea.Cmd {
	drafts, k, sql := s.d.Drafts, s.ex.Key(), s.editor.Value()
	return func() tea.Msg {
		if err := drafts.Save(k, sql); err != nil {
			return noteMsg{"Could not save the draft: " + err.Error()}
		}
		return nil
	}
}

// record runs a progress update off the UI goroutine.
func (s *exerciseView) record(f func(*progress.Store) error) tea.Cmd {
	p := s.d.Progress
	return func() tea.Msg {
		if err := f(p); err != nil {
			return noteMsg{"Could not save progress: " + err.Error()}
		}
		return nil
	}
}

func (s *exerciseView) layout() (lw, rw, bh, editorH int) {
	bh = bodyHeight(s.h)
	lw = min(42, s.w*2/5)
	rw = max(s.w-lw-1, 10)
	editorH = max(6, bh*2/5)
	return
}

func (s *exerciseView) narrow() bool { return s.w < narrowWidth }

// narrowHeights splits the body between question, editor and result.
func (s *exerciseView) narrowHeights() (qH, editorH, resultH int) {
	bh := bodyHeight(s.h)
	qH = min(len(s.questionLines(s.w-4))+2, bh/3)
	rest := bh - qH
	editorH = max(5, rest*2/5)
	return qH, editorH, rest - editorH
}

func (s *exerciseView) SetSize(w, h int) {
	s.w, s.h = w, h
	if s.narrow() {
		_, editorH, _ := s.narrowHeights()
		s.editor.SetSize(w-4, editorH-2)
	} else {
		_, rw, _, editorH := s.layout()
		s.editor.SetSize(rw-4, editorH-2)
		if s.overlay == overlayHint { // hints have their own pane when wide
			s.overlay = overlayNone
		}
	}
	bh := bodyHeight(h)
	if s.disc != nil {
		s.disc.SetWidth(w - 4)
		s.disc.SetHeight(bh - 2)
	}
}

// inserting reports INSERT mode: keys go to the editor.
func (s *exerciseView) inserting() bool { return s.focus == focusEditor && s.insert }

// setFocus moves focus; focusing the editor enters INSERT mode.
func (s *exerciseView) setFocus(f focusArea) tea.Cmd {
	s.focus = f
	if f == focusEditor {
		s.insert = true
		return s.editor.Focus()
	}
	s.insert = false
	s.editor.Blur()
	return nil
}

// normalMode leaves INSERT mode but keeps the query pane focused.
func (s *exerciseView) normalMode() {
	s.insert = false
	s.editor.Blur()
}

func (s *exerciseView) run() tea.Cmd {
	if s.running {
		return nil
	}
	s.seq++
	seq, ex, sql, c := s.seq, s.ex, s.editor.Value(), s.d.Checker
	ctx, cancel := context.WithCancel(context.Background())
	s.running, s.cancel, s.confirm, s.note = true, cancel, confirmNone, ""
	return tea.Batch(s.saveDraft(), s.spin.Tick, func() tea.Msg {
		chk, err := c.Check(ctx, ex, sql)
		return checkDoneMsg{seq: seq, chk: chk, err: err}
	})
}

func (s *exerciseView) finish(msg checkDoneMsg) tea.Cmd {
	s.running = false
	s.cancel()
	s.rowOff, s.colOff, s.view = 0, 0, viewYours
	switch {
	case errors.Is(msg.err, context.Canceled):
		s.mode, s.errTitle, s.errText = resError, "Query cancelled", "Press ctrl+r to run it again."
		return nil
	case errors.Is(msg.err, db.ErrExercise):
		s.mode, s.errTitle = resError, "This exercise's reference answer failed"
		s.errText = "Your query can't be checked. Run 'pgex validate' and report the exercise.\n\n" + msg.err.Error()
		return nil
	case msg.err != nil:
		s.mode, s.errTitle = resError, "Could not reach the database"
		s.errText = "Start it with 'make db-up', then run again. Check the setup with 'pgex doctor'."
		return func() tea.Msg { return pingNowMsg{} } // refresh the header status
	}
	s.mode, s.chk = resCheck, msg.chk
	k, passed := s.ex.Key(), msg.chk.Passed()
	return s.record(func(p *progress.Store) error { return p.RecordAttempt(k, passed) })
}

// neighbor returns the key of the exercise delta steps away across all
// categories in order.
func (s *exerciseView) neighbor(delta int) (string, bool) {
	var keys []string
	for _, c := range s.d.Catalog.Categories {
		for _, e := range c.Exercises {
			keys = append(keys, e.Key())
		}
	}
	for i, k := range keys {
		if k == s.ex.Key() && i+delta >= 0 && i+delta < len(keys) {
			return keys[i+delta], true
		}
	}
	return "", false
}

func (s *exerciseView) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return s, s.handleKey(msg)
	case checkDoneMsg:
		if msg.seq == s.seq && s.running {
			return s, s.finish(msg)
		}
		return s, nil
	case spinner.TickMsg:
		if !s.running {
			return s, nil
		}
		var cmd tea.Cmd
		s.spin, cmd = s.spin.Update(msg)
		return s, cmd
	case draftLoadedMsg:
		if msg.err != nil {
			s.note = "Could not read the draft: " + msg.err.Error()
		} else {
			s.editor.SetValue(msg.text)
		}
		return s, nil
	case editorDoneMsg:
		if msg.err != nil {
			s.note = "The editor exited with an error: " + msg.err.Error()
		}
		drafts, k := s.d.Drafts, s.ex.Key()
		return s, func() tea.Msg {
			text, err := drafts.Load(k)
			return draftLoadedMsg{text: text, err: err}
		}
	case noteMsg:
		s.note = msg.text
		return s, nil
	}
	if s.disc != nil {
		var cmd tea.Cmd
		*s.disc, cmd = s.disc.Update(msg)
		return s, cmd
	}
	return s, s.editor.Update(msg) // cursor blink and similar
}

func (s *exerciseView) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	k := s.d.Keys
	if s.disc != nil {
		if key.Matches(msg, k.Back) {
			s.disc = nil
			return nil
		}
		var cmd tea.Cmd
		*s.disc, cmd = s.disc.Update(msg)
		return cmd
	}

	if s.confirm != confirmNone {
		switch msg.String() {
		case "y":
			c := s.confirm
			s.confirm = confirmNone
			if c == confirmClear {
				s.editor.SetValue("")
				return nil
			}
			s.prevMode, s.mode = s.mode, resAnswer
			key := s.ex.Key()
			return s.record(func(p *progress.Store) error { return p.RecordReveal(key) })
		case "n", "esc":
			s.confirm = confirmNone
		}
		return nil
	}

	switch {
	case key.Matches(msg, k.Run):
		return s.run()
	case key.Matches(msg, k.Focus):
		return s.setFocus((s.focus + 1) % focusCount)
	case key.Matches(msg, k.FocusBack):
		return s.setFocus((s.focus + focusCount - 1) % focusCount)
	}

	if s.inserting() {
		switch {
		case key.Matches(msg, k.Back):
			s.normalMode()
			return nil
		case key.Matches(msg, k.Clear):
			if strings.TrimSpace(s.editor.Value()) != "" {
				s.confirm = confirmClear
			}
			return nil
		}
		return s.editor.Update(msg)
	}

	switch {
	case key.Matches(msg, k.Back) && s.overlay != overlayNone:
		s.overlay = overlayNone
		return nil
	case key.Matches(msg, k.Back) && s.mode == resAnswer:
		s.mode = s.prevMode
		return nil
	case key.Matches(msg, k.Back) && s.focus == focusEditor:
		return nil // already in NORMAL mode; like vim, esc does nothing more
	case key.Matches(msg, k.Back), msg.String() == "backspace":
		return tea.Batch(s.Leave(), func() tea.Msg { return backMsg{} })
	case msg.String() == "enter" || msg.String() == "i":
		return s.setFocus(focusEditor)
	case msg.String() == "y" && s.mode == resAnswer:
		s.editor.SetValue(strings.TrimSpace(s.ex.Answer))
		s.mode = s.prevMode
		return s.setFocus(focusEditor)
	case key.Matches(msg, k.Hint):
		if s.narrow() {
			s.overlay = overlayHint
		}
		if s.hintsShown < len(s.ex.Hints) {
			s.hintsShown++
			key, n := s.ex.Key(), s.hintsShown
			return s.record(func(p *progress.Store) error { return p.RecordHint(key, n) })
		}
	case key.Matches(msg, k.Answer):
		s.confirm = confirmReveal
	case key.Matches(msg, k.Discussion):
		s.openDiscussion()
	case key.Matches(msg, k.Schema):
		if s.overlay == overlaySchema {
			s.overlay = overlayNone
		} else {
			s.overlay = overlaySchema
		}
	case key.Matches(msg, k.Diff):
		if s.mode == resCheck && s.chk.UserErr == nil {
			s.view = (s.view + 1) % 3
			s.rowOff = 0
		}
	case key.Matches(msg, k.Next), key.Matches(msg, k.Prev):
		delta := 1
		if key.Matches(msg, k.Prev) {
			delta = -1
		}
		if next, ok := s.neighbor(delta); ok {
			return tea.Batch(s.Leave(), func() tea.Msg { return openExerciseMsg{key: next, replace: true} })
		}
		s.note = "No more exercises in that direction."
	case key.Matches(msg, k.Editor):
		path := s.d.Drafts.Path(s.ex.Key())
		return tea.Sequence(s.saveDraft(), externalEditor(s.d.Editor, path, func(err error) tea.Msg { return editorDoneMsg{err} }))
	case s.focus == focusResult && key.Matches(msg, k.Top):
		s.rowOff = 0
	case s.focus == focusResult && key.Matches(msg, k.Bottom):
		s.rowOff = s.lastRow()
	case s.focus == focusResult && key.Matches(msg, k.PageDown):
		s.rowOff = min(s.rowOff+s.h/4, s.lastRow())
	case s.focus == focusResult && key.Matches(msg, k.PageUp):
		s.rowOff = max(s.rowOff-s.h/4, 0)
	case s.focus == focusResult && key.Matches(msg, k.Down):
		s.rowOff = min(s.rowOff+1, s.lastRow())
	case s.focus == focusResult && key.Matches(msg, k.Up):
		s.rowOff = max(s.rowOff-1, 0)
	case s.focus == focusResult && msg.String() == "right":
		s.colOff++
	case s.focus == focusResult && msg.String() == "left":
		s.colOff = max(s.colOff-1, 0)
	}
	return nil
}

// lastRow is the highest useful scroll offset for the result table.
func (s *exerciseView) lastRow() int {
	rows, _ := tableRows(s.chk, s.view)
	return max(len(rows)-1, 0)
}

func (s *exerciseView) openDiscussion() {
	text := strings.TrimSpace(s.ex.Discussion)
	if text == "" {
		text = "_No discussion for this exercise yet._"
	}
	md := "# " + s.ex.Title + "\n\n" + text + "\n"
	out := md
	if r, err := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"), glamour.WithWordWrap(max(s.w-8, 20))); err == nil {
		if rendered, err := r.Render(md); err == nil {
			out = rendered
		}
	}
	vp := viewport.New(viewport.WithWidth(s.w-4), viewport.WithHeight(bodyHeight(s.h)-2))
	vp.SetContent(out)
	s.disc = &vp
}

func (s *exerciseView) View() string {
	t := s.d.Theme
	header := t.Header([]string{"Categories", s.cat.Title, fmt.Sprintf("%d. %s", s.num, s.ex.Title)},
		t.Dim.Render(fmt.Sprintf("%d / %d", s.num, len(s.cat.Exercises))), s.w)
	if s.disc != nil {
		body := t.Pane("Discussion", s.disc.View(), s.w, bodyHeight(s.h), true)
		k := s.d.Keys
		return joinScreen(header, body, t.Footer([]helpItem{{"↑/↓", "scroll"}, short(k.Back, "close")}, "", s.w))
	}

	var body string
	if s.narrow() {
		qH, editorH, resultH := s.narrowHeights()
		body = lipgloss.JoinVertical(lipgloss.Left,
			t.Pane("Question", strings.Join(s.questionLines(s.w-4), "\n"), s.w, qH, s.focus == focusLeft),
			s.editorPane(s.w, editorH), s.resultPane(s.w, resultH))
	} else {
		lw, rw, bh, editorH := s.layout()
		left := s.leftColumn(lw, bh)
		right := lipgloss.JoinVertical(lipgloss.Left, s.editorPane(rw, editorH), s.resultPane(rw, bh-editorH))
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
	}
	out := joinScreen(header, body, t.Footer(s.helpItems(), s.statusBadge(), s.w))
	if s.overlay != overlayNone {
		out = t.Overlay(out, s.overlayPane(), s.w, s.h)
	}
	return out
}

// overlayPane renders the schema window, or the hint window in the narrow
// layout.
func (s *exerciseView) overlayPane() string {
	t := s.d.Theme
	if s.overlay == overlaySchema {
		w := min(s.w-2, diagramWidth(true)+4)
		lines := schemaView(t, s.ex.Tables, w-4)
		return t.PaneColored("Schema", strings.Join(lines, "\n"), w, min(len(lines)+2, s.h-2), t.P.Accent, t.Accent.Bold(true))
	}
	w := min(s.w-4, 64)
	inner := w - 4
	title, lines := "Hints", []string{t.Dim.Render("This exercise has no hints.")}
	if s.hintsShown > 0 {
		title = fmt.Sprintf("Hint %d of %d", s.hintsShown, len(s.ex.Hints))
		lines = wrap(s.ex.Hints[s.hintsShown-1], inner, t.Text)
		lines = append(lines, "")
		lines = append(lines, wrap("h for the next hint · a reveals the answer · esc closes", inner, t.Dim)...)
	}
	return t.PaneColored(title, strings.Join(lines, "\n"), w, min(len(lines)+2, s.h-2), t.P.Warn, t.Warn.Bold(true))
}

func (s *exerciseView) helpItems() []helpItem {
	k := s.d.Keys
	switch {
	case s.confirm != confirmNone:
		return []helpItem{{"y", "yes"}, {"n", "no"}}
	case s.inserting():
		return []helpItem{short(k.Run, "run"), {"esc", "normal mode"}, short(k.Focus, "next pane"), short(k.Clear, "clear")}
	case s.mode == resAnswer:
		return []helpItem{{"y", "copy to editor"}, short(k.Back, "close answer"), short(k.Run, "run")}
	}
	return []helpItem{short(k.Run, "run"), {"enter", "edit"}, short(k.Hint, "hint"), short(k.Answer, "answer"),
		short(k.Diff, "view"), short(k.Discussion, "discussion"), {"n/p", "next/prev"}, short(k.Editor, "$EDITOR"), {"backspace", "back"}}
}

func (s *exerciseView) statusBadge() string {
	t := s.d.Theme
	if s.note != "" {
		return t.Fail.Render(ansiTrunc(s.note, s.w/2))
	}
	st := s.d.Progress.Get(s.ex.Key()).Status
	return markerStyle(t, st).Render(markerGlyph(st) + " " + string(st))
}

func ansiTrunc(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	return string([]rune(s)[:max(w-1, 0)]) + "…"
}

// questionLines is the question text plus table and order notes.
func (s *exerciseView) questionLines(inner int) []string {
	t := s.d.Theme
	q := wrap(s.ex.Question, inner, t.Text)
	if len(s.ex.Tables) > 0 {
		q = append(q, "", t.Dim.Render("tables: ")+t.Text.Render(strings.Join(s.ex.Tables, ", ")))
	}
	if s.ex.Ordered {
		q = append(q, t.Dim.Render("row order matters"))
	}
	return q
}

func (s *exerciseView) leftColumn(w, h int) string {
	t := s.d.Theme
	inner := w - 4
	q := s.questionLines(inner)

	var hint []string
	if s.hintsShown > 0 {
		hint = wrap(s.ex.Hints[s.hintsShown-1], inner, t.Text)
		hint = append(hint, "")
		hint = append(hint, wrap("h for the next hint · a reveals the answer", inner, t.Dim)...)
	}

	var lowerTitle string
	var lower []string
	if s.mode == resCheck && s.chk.UserErr == nil && !s.chk.Passed() {
		lowerTitle, lower = "Diff summary", strings.Split(diffSummary(t, s.chk, s.ex.Ordered), "\n")
	}

	hintH := 0
	if hint != nil {
		hintH = min(len(hint)+2, h/3)
	}
	qH := min(len(q)+2, h-hintH)
	lowerH := h - qH - hintH
	if lower == nil || lowerH < 4 {
		qH, lowerH = h-hintH, 0
	}

	parts := []string{t.Pane("Question", strings.Join(q, "\n"), w, qH, s.focus == focusLeft)}
	if hintH > 0 {
		title := fmt.Sprintf("Hint %d of %d", s.hintsShown, len(s.ex.Hints))
		parts = append(parts, t.PaneColored(title, strings.Join(hint, "\n"), w, hintH, t.P.Warn, t.Warn.Bold(true)))
	}
	if lowerH > 0 {
		parts = append(parts, t.Pane(lowerTitle, strings.Join(lower, "\n"), w, lowerH, false))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (s *exerciseView) editorPane(w, h int) string {
	mode := "NORMAL"
	if s.inserting() {
		mode = "INSERT"
	}
	title := fmt.Sprintf("Query · %s · attempt %d", mode, s.d.Progress.Get(s.ex.Key()).Attempts+1)
	return s.d.Theme.Pane(title, s.editor.View(), w, h, s.focus == focusEditor)
}

func (s *exerciseView) resultPane(w, h int) string {
	t := s.d.Theme
	inner, innerH := w-4, h-2
	title := "Result"
	var lines []string

	if s.confirm != confirmNone {
		prompt := "Reveal the answer? This marks the exercise as revealed. y/n"
		if s.confirm == confirmClear {
			prompt = "Clear the editor? y/n"
		}
		lines = append(lines, t.Banner(t.WarnBanner, prompt, inner), "")
	}

	switch {
	case s.running:
		title += " · " + s.spin.View() + " running… (ctrl+c cancels)"
	case s.mode == resNone:
		lines = append(lines, t.Dim.Render("Press ctrl+r to run your query against the database."))
	case s.mode == resError:
		lines = append(lines, t.Banner(t.FailBanner, "✗ "+s.errTitle, inner), "")
		lines = append(lines, wrap(s.errText, inner, t.Text)...)
	case s.mode == resAnswer:
		title = "Answer"
		lines = append(lines, strings.Split(highlightSQL(t, strings.TrimSpace(s.ex.Answer)), "\n")...)
		lines = append(lines, "", t.Dim.Render("y copies it into the editor · esc returns"))
	case s.chk.UserErr != nil:
		lines = append(lines, s.userErrLines(inner)...)
	default:
		title += fmt.Sprintf(" · %s · %s", plural(len(s.chk.Got.Rows), "row"), s.chk.Got.Duration.Round(10e3))
		var foot []string
		if s.chk.Passed() {
			lines = append(lines, t.Banner(t.PassBanner, fmt.Sprintf("✓ Correct: all %s match the expected result", plural(len(s.chk.Want.Rows), "row")), inner), "")
			foot = []string{"", t.Dim.Render("Press n for the next exercise or d to read the discussion.")}
		} else {
			title += " · " + s.view.String()
			lines = append(lines, t.Banner(t.FailBanner, "✗ Not quite: "+s.chk.Diff.Reason(), inner), "")
		}
		cols := s.chk.Got.Columns
		if s.view != viewYours {
			cols = s.chk.Want.Columns
		}
		rows, marks := tableRows(s.chk, s.view)
		tableH := max(innerH-len(lines)-len(foot), 2)
		lines = append(lines, strings.Split(renderTable(t, cols, rows, marks, inner, tableH, s.rowOff, s.colOff), "\n")...)
		lines = append(lines, foot...)
	}
	return t.Pane(title, strings.Join(lines, "\n"), w, h, s.focus == focusResult)
}

func (s *exerciseView) userErrLines(w int) []string {
	t := s.d.Theme
	e := s.chk.UserErr
	lines := []string{t.Banner(t.FailBanner, "✗ ERROR: "+e.Message, w)}
	if lipgloss.Width(e.Message)+10 > w {
		lines = append(lines, wrap(e.Message, w, t.Text)...)
	}
	if e.Detail != "" {
		lines = append(lines, wrap("DETAIL: "+e.Detail, w, t.Text)...)
	}
	if e.Hint != "" {
		lines = append(lines, wrap("HINT: "+e.Hint, w, t.Text)...)
	}
	if line, col, ok := e.Locate(s.editor.Value()); ok {
		lines = append(lines, "", highlightSQL(t, line), strings.Repeat(" ", col)+t.Fail.Render("^"))
	}
	return lines
}
