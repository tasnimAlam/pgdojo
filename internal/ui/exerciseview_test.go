package ui

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/tasnimAlam/pgex/internal/db"
	"github.com/tasnimAlam/pgex/internal/progress"
)

func cells(vals ...string) []db.Cell {
	r := make([]db.Cell, len(vals))
	for i, v := range vals {
		if v == "<null>" {
			r[i] = db.Cell{Null: true}
		} else {
			r[i] = db.Cell{Text: v}
		}
	}
	return r
}

var (
	wantRes = db.Result{Columns: []string{"name", "membercost"}, Rows: [][]db.Cell{
		cells("Tennis Court 1", "5"), cells("Tennis Court 2", "5"), cells("Massage Room 1", "9.9"),
	}}
	gotFail = db.Result{Columns: []string{"name", "membercost"}, Rows: [][]db.Cell{
		cells("Tennis Court 1", "5"), cells("Massage Room 1", "9.9"), cells("Squash Court", "<null>"),
	}}
)

func passCheck() db.Check {
	return db.Check{Want: wantRes, Got: wantRes, Diff: db.Compare(wantRes, wantRes, false)}
}

func failCheck() db.Check {
	return db.Check{Want: wantRes, Got: gotFail, Diff: db.Compare(wantRes, gotFail, false)}
}

func userErrCheck() db.Check {
	return db.Check{Want: wantRes, UserErr: &db.UserError{
		Message: `column "nme" does not exist`, Hint: `Perhaps you meant to reference the column "facilities.name".`,
		Code: "42703", Position: 15,
	}}
}

// openExercise starts the app with a fake check result and opens
// basics/selectall in the exercise view.
func openExercise(t *testing.T, chk db.Check) tea.Model {
	d := testDeps(t)
	d.Checker = fakeChecker{check: func(context.Context, string) (db.Check, error) { return chk, nil }}
	m := drive(New(d), tea.WindowSizeMsg{Width: 120, Height: 36}, pingMsg{})
	return drive(m, keys("enter", "enter", "i")...) // open, then INSERT mode
}

func view(m tea.Model) *exerciseView { return m.(Model).top().(*exerciseView) }

func TestExercisePass(t *testing.T) {
	m := openExercise(t, passCheck())
	m = drive(m, typeText("select name, membercost from facilities")...)
	m = drive(m, keys("ctrl+r")...)

	out := screenText(m)
	for _, want := range []string{"✓ Correct: all 3 rows match", "Tennis Court 2", "Press n for the next exercise"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	d := m.(Model).deps
	if e := d.Progress.Get("basics/selectall"); e.Status != progress.Solved || e.Attempts != 3 {
		t.Errorf("progress = %+v, want solved with 3 attempts", e)
	}
	draft, _ := os.ReadFile(d.Drafts.Path("basics/selectall"))
	if string(draft) != "select name, membercost from facilities" {
		t.Errorf("draft not saved on run: %q", draft)
	}
	golden.RequireEqual(t, out)

	m = drive(m, keys("esc", "n")...)
	if v := view(m); v.ex.Key() != "basics/selectspecific" || len(m.(Model).stack) != 3 {
		t.Errorf("n should replace the view with the next exercise, got %s (stack %d)", v.ex.Key(), len(m.(Model).stack))
	}
}

func TestExerciseFail(t *testing.T) {
	m := openExercise(t, failCheck())
	m = drive(m, keys("ctrl+r")...)
	out := screenText(m)
	for _, want := range []string{"✗ Not quite: 1 row missing and 1 extra row.", "Diff summary", "expected rows  3", "· yours"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	if s := m.(Model).deps.Progress.Get("basics/selectall").Status; s != progress.Solved {
		t.Errorf("a failed run must not downgrade solved, got %s", s)
	}

	m = drive(m, keys("esc", "v", "v")...)
	out = screenText(m)
	if !strings.Contains(out, "· diff") || !strings.Contains(out, "− Tennis Court 2") || !strings.Contains(out, "+ Squash Court    ∅") {
		t.Errorf("diff view wrong:\n%s", out)
	}
	golden.RequireEqual(t, out)
}

func TestExerciseUserError(t *testing.T) {
	m := openExercise(t, userErrCheck())
	m = drive(m, typeText("select facid, nme from facilities")...)
	m = drive(m, keys("ctrl+r")...)
	out := screenText(m)
	for _, want := range []string{`✗ ERROR: column "nme" does not exist`, "HINT: Perhaps you meant", "select facid, nme from facilities\n                 │ │               ^"} {
		if !strings.Contains(out, strings.Split(want, "\n")[0]) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	golden.RequireEqual(t, out)
}

func TestExerciseRevealAndHints(t *testing.T) {
	d := testDeps(t)
	m := drive(New(d), tea.WindowSizeMsg{Width: 120, Height: 36}, pingMsg{})
	m = drive(m, keys("down", "down", "enter", "enter", "esc")...) // modifying/insert, normal mode

	m = drive(m, keys("H", "H")...) // the second H has no more hints to show
	if out := screenText(m); !strings.Contains(out, "Hint 1 of 1") || !strings.Contains(out, "can be used to insert data") {
		t.Errorf("hint pane wrong:\n%s", out)
	}
	m = drive(m, keys("a")...)
	if !strings.Contains(screenText(m), "Reveal the answer? This marks the exercise as revealed. y/n") {
		t.Fatal("a should ask for confirmation")
	}
	m = drive(m, keys("n")...)
	if d.Progress.Get("modifying/insert").Status != progress.New {
		t.Fatal("declining must not reveal")
	}
	m = drive(m, keys("a", "y")...)
	out := screenText(m)
	if !strings.Contains(out, "insert into cd.facilities") {
		t.Errorf("answer not shown:\n%s", out)
	}
	e := d.Progress.Get("modifying/insert")
	if e.Status != progress.Revealed || e.HintsUsed != 1 {
		t.Errorf("progress = %+v, want revealed with 1 hint", e)
	}
	golden.RequireEqual(t, out)

	m = drive(m, keys("y")...)
	v := view(m)
	if !strings.HasPrefix(v.editor.Value(), "insert into cd.facilities") || v.focus != focusEditor || v.inserting() {
		t.Errorf("y should copy the answer into the editor and focus it in NORMAL mode, got %q", v.editor.Value())
	}
}

func TestExerciseDiscussionAndClear(t *testing.T) {
	m := openExercise(t, passCheck())
	m = drive(m, typeText("select 1")...)
	m = drive(m, keys("ctrl+l")...)
	if !strings.Contains(screenText(m), "Clear the editor? y/n") {
		t.Fatal("ctrl+l on a non-empty editor should confirm")
	}
	m = drive(m, keys("y")...)
	if v := view(m).editor.Value(); v != "" {
		t.Errorf("editor not cleared: %q", v)
	}
	m = drive(m, keys("esc", "d")...)
	if out := screenText(m); !strings.Contains(out, "Discussion") || !strings.Contains(out, "scroll") {
		t.Errorf("discussion not shown:\n%s", out)
	}
	m = drive(m, keys("esc")...)
	if view(m).disc != nil {
		t.Error("esc should close the discussion")
	}
}

// TestExerciseCancel uses the real program loop: ctrl+c during a run
// cancels the query instead of quitting.
func TestExerciseCancel(t *testing.T) {
	d := testDeps(t)
	d.Checker = fakeChecker{check: func(ctx context.Context, _ string) (db.Check, error) {
		<-ctx.Done()
		return db.Check{}, ctx.Err()
	}}
	tm := teatest.NewTestModel(t, New(d), teatest.WithInitialTermSize(120, 36))
	wait := func(s string) {
		t.Helper()
		teatest.WaitFor(t, tm.Output(), func(b []byte) bool { return strings.Contains(string(b), s) },
			teatest.WithDuration(3*time.Second))
	}
	wait("connected")
	tm.Send(keyPress("enter"))
	wait("Preview")
	tm.Send(keyPress("enter"))
	wait("NORMAL")
	tm.Send(keyPress("ctrl+r"))
	wait("running")
	tm.Send(keyPress("ctrl+c"))
	wait("Query cancelled")
	tm.Send(keyPress("esc"))
	tm.Send(keyPress("q"))
	tm.Send(keyPress("y"))
	fm := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second)).(Model)
	if v, ok := fm.top().(*exerciseView); !ok || v.running {
		t.Errorf("want stopped exercise view, got %T", fm.top())
	}
}

func TestF5DoesNotRun(t *testing.T) {
	m := openExercise(t, passCheck())
	m = drive(m, tea.KeyPressMsg{Code: tea.KeyF5})
	if v := view(m); v.running || v.mode != resNone {
		t.Error("F5 must not run the query")
	}
}

func TestEscInQuery(t *testing.T) {
	m := openExercise(t, passCheck())
	m = drive(m, keys("esc")...)
	v := view(m)
	if v.inserting() || v.focus != focusEditor {
		t.Fatalf("esc should switch to NORMAL mode and keep the query focused (insert=%v focus=%v)", v.insert, v.focus)
	}
	if !strings.Contains(screenText(m), "Query · NORMAL") {
		t.Error("query title should say NORMAL")
	}
	m = drive(m, keys("esc", "esc")...)
	if _, ok := m.(Model).top().(*exerciseView); !ok {
		t.Fatal("esc in NORMAL mode must not leave the exercise")
	}
	m = drive(m, keys("i")...)
	if !view(m).inserting() {
		t.Error("i should return to INSERT mode")
	}
	m = drive(m, keys("esc", "backspace")...)
	if _, ok := m.(Model).top().(*exerciseList); !ok {
		t.Errorf("backspace in NORMAL mode should go back, got %T", m.(Model).top())
	}

	// From the result pane, esc still goes back.
	m = drive(m, keys("enter", "tab", "esc")...)
	if _, ok := m.(Model).top().(*exerciseList); !ok {
		t.Errorf("esc with the result pane focused should go back, got %T", m.(Model).top())
	}
}

func TestOnlyIEntersInsert(t *testing.T) {
	m := drive(start(t, 120, 36), keys("enter", "enter")...)
	if v := view(m); v.inserting() || v.focus != focusEditor {
		t.Fatal("an exercise should open in NORMAL mode with the query focused")
	}
	m = drive(m, keys("tab", "tab", "tab")...) // result, question, back to query
	if v := view(m); v.focus != focusEditor || v.inserting() {
		t.Errorf("tab back to the query should stay in NORMAL mode (focus=%v insert=%v)", v.focus, v.insert)
	}
	m = drive(m, keys("enter")...)
	if view(m).inserting() {
		t.Error("enter must not enter INSERT mode")
	}
	m = drive(m, keys("i")...)
	m = drive(m, typeText("select 1")...)
	if v := view(m); !v.inserting() || v.editor.Value() != "select 1" {
		t.Errorf("i should enter INSERT mode and accept typing, got %q", v.editor.Value())
	}
	m = drive(m, keys("tab", "shift+tab")...)
	if v := view(m); v.focus != focusEditor || v.inserting() {
		t.Error("leaving and returning with tab should land in NORMAL mode")
	}
}
