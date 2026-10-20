package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/tasnimAlam/pgex/internal/db"
	"github.com/tasnimAlam/pgex/internal/drafts"
	"github.com/tasnimAlam/pgex/internal/exercise"
	"github.com/tasnimAlam/pgex/internal/progress"
)

var fixedNow = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

// fakeChecker answers Run with fixed columns and Check with check.
type fakeChecker struct {
	offline bool
	check   func(ctx context.Context, sql string) (db.Check, error)
}

func (f fakeChecker) Check(ctx context.Context, ex exercise.Exercise, sql string) (db.Check, error) {
	if f.check != nil {
		return f.check(ctx, sql)
	}
	return db.Check{}, nil
}

func (f fakeChecker) Run(ctx context.Context, sql string) (db.Result, error) {
	if f.offline {
		return db.Result{}, errors.New("offline")
	}
	return db.Result{Columns: []string{"starttime", "name"}}, nil
}

func (f fakeChecker) Ping(ctx context.Context) error {
	if f.offline {
		return errors.New("offline")
	}
	return nil
}

const progressFixture = `{
  "version": 1,
  "last": {"category": "joins", "exercise": "self"},
  "exercises": {
    "basics/selectall": {"status": "solved", "attempts": 2, "hints_used": 1, "last_pass": true, "updated_at": "2026-10-18T12:00:00Z"},
    "basics/selectspecific": {"status": "solved", "attempts": 1, "hints_used": 0, "last_pass": true, "updated_at": "2026-10-18T12:00:00Z"},
    "joins/simplejoin": {"status": "attempted", "attempts": 3, "hints_used": 2, "updated_at": "2026-10-20T11:00:00Z"},
    "joins/self2": {"status": "revealed", "revealed": true, "attempts": 1, "hints_used": 2, "updated_at": "2026-10-19T12:00:00Z"}
  }
}`

func testDeps(t *testing.T) *Deps {
	t.Helper()
	cat, err := exercise.Load()
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home) // drafts path renders as ~/... in golden files
	path := filepath.Join(home, "progress.json")
	if err := os.WriteFile(path, []byte(progressFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := progress.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return &Deps{
		Catalog:  cat,
		Checker:  fakeChecker{},
		Progress: store,
		Drafts:   drafts.New(filepath.Join(home, ".local", "share", "pgex", "drafts")),
		DBLabel:  "localhost:5433/exercises",
		Theme:    NewTheme(""),
		Keys:     NewKeyMap(true),
		Now:      func() time.Time { return fixedNow },
	}
}

// drive feeds messages straight into the model and runs the commands they
// return, so navigation and data loading take effect without timing.
func drive(m tea.Model, msgs ...tea.Msg) tea.Model {
	for _, msg := range msgs {
		var cmd tea.Cmd
		m, cmd = m.Update(msg)
		m = runCmd(m, cmd)
	}
	return m
}

func runCmd(m tea.Model, cmd tea.Cmd) tea.Model {
	if cmd == nil {
		return m
	}
	// Ticks (ping interval, cursor blink) would block; drop slow commands.
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-ch:
	case <-time.After(50 * time.Millisecond):
		return m
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = runCmd(m, c)
		}
		return m
	}
	switch msg.(type) {
	case nil, pingTickMsg:
		return m
	}
	m, cmd = m.Update(msg)
	return runCmd(m, cmd)
}

func keyPress(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	}
	if c, ok := strings.CutPrefix(s, "ctrl+"); ok {
		return tea.KeyPressMsg{Code: []rune(c)[0], Mod: tea.ModCtrl}
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: s}
}

func keys(ss ...string) []tea.Msg {
	out := make([]tea.Msg, len(ss))
	for i, s := range ss {
		out[i] = keyPress(s)
	}
	return out
}

func typeText(s string) []tea.Msg {
	var out []tea.Msg
	for _, r := range s {
		out = append(out, keyPress(string(r)))
	}
	return out
}

func start(t *testing.T, w, h int) tea.Model {
	m := New(testDeps(t))
	return drive(m, tea.WindowSizeMsg{Width: w, Height: h}, pingMsg{})
}

func screenText(m tea.Model) string { return ansi.Strip(m.(Model).View().Content) }

func TestGoldenScreens(t *testing.T) {
	cases := map[string][]tea.Msg{
		"home":          nil,
		"exercise_list": keys("down", "enter"),
		"filter":        append(keys("down", "enter", "/"), typeText("mem")...),
		"help":          keys("?"),
	}
	for name, msgs := range cases {
		t.Run(name, func(t *testing.T) {
			m := drive(start(t, 120, 36), msgs...)
			out := screenText(m)
			assertSize(t, out, 120, 36)
			golden.RequireEqual(t, out)
		})
	}
}

// Every screen must render at 80×24 without panicking or overflowing.
func TestSmallTerminal(t *testing.T) {
	for _, msgs := range [][]tea.Msg{nil, keys("enter"), keys("?"), keys("enter", "enter")} {
		m := drive(start(t, 80, 24), msgs...)
		assertSize(t, screenText(m), 80, 24)
	}
}

func assertSize(t *testing.T, out string, w, h int) {
	t.Helper()
	lines := strings.Split(out, "\n")
	if len(lines) != h {
		t.Errorf("view has %d lines, want %d", len(lines), h)
	}
	for i, l := range lines {
		if n := ansi.StringWidth(l); n > w {
			t.Errorf("line %d is %d cells wide, want <= %d: %q", i, n, w, l)
		}
	}
}

func TestNavigation(t *testing.T) {
	m := start(t, 120, 36)
	top := func() Screen { return m.(Model).top() }

	m = drive(m, keys("down", "enter")...)
	el, ok := top().(*exerciseList)
	if !ok || el.cat.ID != "joins" {
		t.Fatalf("enter should open Joins, got %T", top())
	}
	m = drive(m, keys("down", "enter")...)
	if p, ok := top().(*exerciseView); !ok || p.ex.Key() != "joins/simplejoin2" {
		t.Fatalf("enter should open the second exercise, got %T", top())
	}
	if last, _ := m.(Model).deps.Progress.Last(); last.Exercise != "simplejoin2" {
		t.Errorf("opening an exercise should record it as last, got %+v", last)
	}
	m = drive(m, keys("esc", "esc")...) // leave INSERT mode, then go back
	if _, ok := top().(*exerciseList); !ok {
		t.Fatalf("esc should go back to the list, got %T", top())
	}
	m = drive(m, keys("backspace")...)
	if _, ok := top().(*categories); !ok {
		t.Fatalf("backspace should go back home, got %T", top())
	}
	m = drive(m, keys("c")...)
	if p, ok := top().(*exerciseView); !ok || p.ex.Key() != "joins/simplejoin2" {
		t.Fatalf("c should continue the last exercise, got %T", top())
	}
}

func TestFilter(t *testing.T) {
	m := drive(start(t, 120, 36), keys("down", "enter", "/")...)
	m = drive(m, typeText("q")...)
	if m.(Model).help || len(m.(Model).stack) != 2 {
		t.Fatal("typing in the filter must not trigger global keys")
	}
	m = drive(m, keyPress("backspace"))
	m = drive(m, typeText("no joins")...)
	el := m.(Model).top().(*exerciseList)
	if n := len(el.list.VisibleItems()); n != 1 {
		t.Fatalf("filter 'no joins' should match 1 exercise, got %d", n)
	}
	if !strings.Contains(screenText(m), "/ no joins▌") || !strings.Contains(screenText(m), "1 matches") {
		t.Errorf("filter line missing:\n%s", screenText(m))
	}
	m = drive(m, keys("enter", "enter")...)
	if p, ok := m.(Model).top().(*exerciseView); !ok || p.ex.ID != "sub" {
		t.Fatalf("enter on filtered item should open it, got %T", m.(Model).top())
	}
	m = drive(m, keys("esc", "esc", "esc")...) // INSERT mode, back, clear filter
	if _, ok := m.(Model).top().(*exerciseList); !ok {
		t.Fatal("esc on a filtered list should clear the filter, not go back")
	}
	if el := m.(Model).top().(*exerciseList); el.list.IsFiltered() {
		t.Error("filter should be cleared")
	}
}

func TestHelpOverlayAndOffline(t *testing.T) {
	m := drive(start(t, 120, 36), keys("?")...)
	out := screenText(m)
	for _, want := range []string{"Keyboard shortcuts", "Navigation", "Query", "Learning", "ctrl+r", "reveal answer"} {
		if !strings.Contains(out, want) {
			t.Errorf("help overlay missing %q", want)
		}
	}
	m = drive(m, keys("esc")...)
	if m.(Model).help {
		t.Error("esc should close help")
	}

	d := testDeps(t)
	d.Checker = fakeChecker{offline: true}
	m = New(d)
	m = drive(m, tea.WindowSizeMsg{Width: 120, Height: 36}, pingMsg{err: errors.New("down")})
	m = drive(m, keys("enter")...)
	out = screenText(m)
	if !strings.Contains(out, "unavailable (database offline)") {
		t.Errorf("preview should explain offline columns:\n%s", out)
	}
	m = drive(m, keys("esc")...)
	if !strings.Contains(screenText(m), "● offline") {
		t.Error("home header should show offline")
	}
}

// TestProgram runs the real Bubble Tea program loop via teatest.
func TestProgram(t *testing.T) {
	tm := teatest.NewTestModel(t, New(testDeps(t)), teatest.WithInitialTermSize(120, 36))
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool { return strings.Contains(string(b), "connected") },
		teatest.WithDuration(3*time.Second))
	tm.Send(keyPress("down"))
	tm.Send(keyPress("enter"))
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool { return strings.Contains(string(b), "Expected columns") },
		teatest.WithDuration(3*time.Second))
	tm.Send(keyPress("q"))
	fm := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second)).(Model)
	if el, ok := fm.top().(*exerciseList); !ok || el.cat.ID != "joins" {
		t.Errorf("final screen = %T", fm.top())
	}
}

func TestVimKeys(t *testing.T) {
	m := start(t, 120, 36)
	top := func() Screen { return m.(Model).top() }

	m = drive(m, keys("j", "l")...)
	if el, ok := top().(*exerciseList); !ok || el.cat.ID != "joins" {
		t.Fatalf("j then l should open Joins, got %T", top())
	}
	m = drive(m, keys("G")...)
	if el := top().(*exerciseList); el.list.Index() != len(el.cat.Exercises)-1 {
		t.Errorf("G should go to the last exercise, at %d", el.list.Index())
	}
	m = drive(m, keys("g")...)
	if el := top().(*exerciseList); el.list.Index() != 0 {
		t.Errorf("g should go to the first exercise, at %d", el.list.Index())
	}
	m = drive(m, keys("h")...)
	if _, ok := top().(*categories); !ok {
		t.Fatalf("h should go back home, got %T", top())
	}
	m = drive(m, keys("h")...) // no-op at home
	if _, ok := top().(*categories); !ok || len(m.(Model).stack) != 1 {
		t.Fatal("h at home should do nothing")
	}

	// In an exercise, h is still "hint", not back.
	m = drive(m, keys("l", "l", "esc", "h")...)
	if v, ok := top().(*exerciseView); !ok || v.hintsShown == 0 {
		t.Errorf("h in an exercise should show a hint, got %T", top())
	}

	// With vim keys off, l and h do nothing in lists.
	d := testDeps(t)
	d.Keys = NewKeyMap(false)
	m = drive(New(d), tea.WindowSizeMsg{Width: 120, Height: 36}, pingMsg{})
	m = drive(m, keys("l")...)
	if _, ok := m.(Model).top().(*categories); !ok {
		t.Error("l must not open with vim keys off")
	}
}
