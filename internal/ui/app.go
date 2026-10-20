// Package ui is the Bubble Tea interface.
package ui

import (
	"context"
	"fmt"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tasnimAlam/pgdojo/internal/db"
	"github.com/tasnimAlam/pgdojo/internal/drafts"
	"github.com/tasnimAlam/pgdojo/internal/exercise"
	"github.com/tasnimAlam/pgdojo/internal/progress"
)

// Checker is what the UI needs from the database. *db.Runner implements
// it; tests use a fake.
type Checker interface {
	Check(ctx context.Context, ex exercise.Exercise, sql string) (db.Check, error)
	Run(ctx context.Context, sql string) (db.Result, error)
	Ping(ctx context.Context) error
}

type connState int

const (
	connUnknown connState = iota
	connUp
	connDown
)

const pingInterval = 30 * time.Second

// Deps are shared by every screen. Only the root model's Update mutates
// the plain fields.
type Deps struct {
	Catalog  *exercise.Catalog
	Checker  Checker
	Progress *progress.Store
	Drafts   *drafts.Store
	DBLabel  string // "host:port/db" for the header
	Editor   string // external editor command, e.g. "nvim" or "emacsclient -t"
	Theme    Theme
	Keys     KeyMap
	Now      func() time.Time // nil means time.Now; tests pin it

	conn    connState
	columns map[string]columnsMsg // expected column names, cached per session
}

// Screen is one page of the app.
type Screen interface {
	Init() tea.Cmd
	Update(tea.Msg) (Screen, tea.Cmd)
	View() string
	SetSize(w, h int)
}

// canceller is implemented by screens with work ctrl+c should cancel
// before it quits the app. Cancel reports whether it cancelled something.
type canceller interface{ Cancel() bool }

// leaver is implemented by screens that must save state before they are
// closed or the app quits.
type leaver interface{ Leave() tea.Cmd }

// capturer is implemented by screens that sometimes need every key, e.g.
// while typing a filter. Global single-letter keys are skipped then.
type capturer interface{ Capturing() bool }

// Model is the root Bubble Tea model: a stack of screens plus the help
// overlay.
type Model struct {
	deps        *Deps
	stack       []Screen
	width       int
	height      int
	help        bool
	confirmQuit bool // "Quit pgdojo? y/n" is showing
}

func New(d *Deps) Model {
	d.columns = map[string]columnsMsg{}
	if d.Now == nil {
		d.Now = time.Now
	}
	return Model{deps: d, stack: []Screen{newCategories(d)}, width: 80, height: 24}
}

func (m Model) Init() tea.Cmd { return tea.Batch(m.top().Init(), m.ping(true)) }

func (m Model) top() Screen { return m.stack[len(m.stack)-1] }

func (m Model) ping(loop bool) tea.Cmd {
	c := m.deps.Checker
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return pingMsg{err: c.Ping(ctx), loop: loop}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		for _, s := range m.stack {
			s.SetSize(m.width, m.height)
		}
		return m, nil

	case tea.KeyPressMsg:
		k := m.deps.Keys
		capturing := false
		if c, ok := m.top().(capturer); ok {
			capturing = c.Capturing()
		}
		switch {
		case m.confirmQuit:
			switch msg.String() {
			case "y", "Y":
				m.confirmQuit = false
				return m, m.quit()
			case "n", "N", "esc", "q":
				m.confirmQuit = false
			}
			return m, nil
		case msg.String() == "ctrl+c":
			if c, ok := m.top().(canceller); ok && c.Cancel() {
				return m, nil
			}
			return m, m.quit()
		case m.help:
			if key.Matches(msg, k.Help, k.Back, k.Quit) {
				m.help = false
			}
			return m, nil
		case capturing:
		case key.Matches(msg, k.Help):
			m.help = true
			return m, nil
		case key.Matches(msg, k.Quit):
			m.confirmQuit = true
			return m, nil
		}

	case pingMsg:
		was := m.deps.conn
		m.deps.conn = connUp
		if msg.err != nil {
			m.deps.conn = connDown
		} else if was == connDown {
			// Back online: retry column lookups that failed while offline.
			for k, v := range m.deps.columns {
				if v.err != nil {
					delete(m.deps.columns, k)
				}
			}
		}
		if !msg.loop {
			return m, nil
		}
		return m, tea.Tick(pingInterval, func(time.Time) tea.Msg { return pingTickMsg{} })

	case pingTickMsg:
		return m, m.ping(true)

	case pingNowMsg:
		return m, m.ping(false)

	case columnsMsg:
		m.deps.columns[msg.key] = msg

	case openCategoryMsg:
		return m.push(newExerciseList(m.deps, msg.index))

	case openExerciseMsg:
		ex, ok := m.deps.Catalog.Find(msg.key)
		if !ok {
			return m, nil
		}
		if msg.replace && len(m.stack) > 1 {
			m.stack = m.stack[:len(m.stack)-1]
		}
		return m.push(newExerciseView(m.deps, ex))

	case backMsg:
		if len(m.stack) > 1 {
			m.stack = m.stack[:len(m.stack)-1]
		}
		return m, nil
	}

	s, cmd := m.top().Update(msg)
	m.stack[len(m.stack)-1] = s
	return m, cmd
}

// quit lets the current screen save its state, then exits.
func (m Model) quit() tea.Cmd {
	if l, ok := m.top().(leaver); ok {
		return tea.Sequence(l.Leave(), tea.Quit)
	}
	return tea.Quit
}

func (m Model) push(s Screen) (tea.Model, tea.Cmd) {
	s.SetSize(m.width, m.height)
	m.stack = append(m.stack, s)
	return m, s.Init()
}

// Smallest terminal the layouts are designed for.
const minWidth, minHeight = 60, 16

func (m Model) View() tea.View {
	var content string
	if m.width < minWidth || m.height < minHeight {
		t := m.deps.Theme
		msg := t.Warn.Render("Terminal too small") + "\n" + t.Dim.Render(fmt.Sprintf(
			"pgdojo needs at least %d×%d, this window is %d×%d.\nEnlarge it or reduce the font size.", minWidth, minHeight, m.width, m.height))
		v := tea.NewView(lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, msg))
		v.AltScreen = true
		return v
	}
	content = m.top().View()
	if m.help {
		content = m.helpOverlay(content)
	}
	if m.confirmQuit {
		t := m.deps.Theme
		body := t.Text.Render("Quit pgdojo?") + "  " + t.HelpKey.Render("y") + t.Dim.Render(" yes · ") +
			t.HelpKey.Render("n") + t.Dim.Render(" no")
		modal := t.PaneColored("Quit", body, lipgloss.Width(body)+4, 3, t.P.Warn, t.Warn.Bold(true))
		content = t.Overlay(content, modal, m.width, m.height)
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// fetchColumns returns a command that looks up an exercise's expected
// column names, or nil if they are cached.
func (d *Deps) fetchColumns(ex exercise.Exercise) tea.Cmd {
	if _, ok := d.columns[ex.Key()]; ok {
		return nil
	}
	// Mark as pending so the cursor moving back and forth doesn't re-query.
	d.columns[ex.Key()] = columnsMsg{key: ex.Key()}
	sql := ex.Answer
	if ex.Mutates {
		sql = ex.Verify
	}
	c := d.Checker
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		res, err := c.Run(ctx, sql)
		return columnsMsg{key: ex.Key(), cols: res.Columns, err: err}
	}
}

// layout splits the terminal into header (1), body, footer (2).
func bodyHeight(h int) int { return max(h-3, 4) }

func joinScreen(header, body, footer string) string {
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}
