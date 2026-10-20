package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/tasnimAlam/pgex/internal/db"
	"github.com/tasnimAlam/pgex/internal/progress"
)

func TestNarrowLayout(t *testing.T) {
	d := testDeps(t)
	d.Checker = fakeChecker{check: func(context.Context, string) (db.Check, error) { return failCheck(), nil }}
	m := drive(New(d), tea.WindowSizeMsg{Width: 100, Height: 30}, pingMsg{})
	m = drive(m, keys("enter", "enter", "ctrl+r", "esc")...)
	out := screenText(m)
	assertSize(t, out, 100, 30)
	if strings.Contains(out, "Schema") || strings.Contains(out, "Diff summary") {
		t.Errorf("narrow layout should not show the left column panes:\n%s", out)
	}
	if !strings.Contains(out, "✗ Not quite") {
		t.Errorf("result missing:\n%s", out)
	}
	golden.RequireEqual(t, out)

	m = drive(m, keys("s")...)
	if out := screenText(m); !strings.Contains(out, "s or esc closes") || !strings.Contains(out, "monthlymaintenance") {
		t.Errorf("s should open the schema overlay:\n%s", out)
	}
	m = drive(m, keys("s", "h")...)
	if out := screenText(m); !strings.Contains(out, "Hint 1 of 1") {
		t.Errorf("h should open the hint overlay with the next hint:\n%s", out)
	}
	m = drive(m, keys("esc")...)
	if v := view(m); v.overlay != overlayNone || len(m.(Model).stack) != 3 {
		t.Error("esc should close the overlay, not leave the exercise")
	}

	// Widening the terminal switches back to the two-column layout.
	m = drive(m, tea.WindowSizeMsg{Width: 120, Height: 36})
	if out := screenText(m); !strings.Contains(out, "Diff summary") {
		t.Errorf("wide layout should show the diff summary:\n%s", out)
	}
}

func TestTooSmall(t *testing.T) {
	m := drive(New(testDeps(t)), tea.WindowSizeMsg{Width: 50, Height: 12})
	if out := screenText(m); !strings.Contains(out, "Terminal too small") || !strings.Contains(out, "50×12") {
		t.Errorf("want too-small message, got:\n%s", out)
	}
}

func TestReset(t *testing.T) {
	m := start(t, 120, 36)
	d := m.(Model).deps

	m = drive(m, keys("R")...)
	m = drive(m, typeText("nope")...)
	m = drive(m, keys("enter")...)
	if d.Progress.Get("basics/selectall").Status != progress.Solved {
		t.Fatal("wrong confirmation word must not reset")
	}
	if !strings.Contains(screenText(m), "Reset cancelled") {
		t.Error("cancel note missing")
	}

	m = drive(m, keys("R")...)
	if out := screenText(m); !strings.Contains(out, "Reset all progress") {
		t.Fatalf("R should open the reset prompt:\n%s", out)
	}
	m = drive(m, typeText("reset")...) // q, ? etc. must not trigger while typing
	if m.(Model).help {
		t.Fatal("typing in the reset prompt triggered a global key")
	}
	m = drive(m, keys("enter")...)
	if d.Progress.Get("basics/selectall").Status != progress.New {
		t.Error("progress not reset")
	}
	if !strings.Contains(screenText(m), "All progress was reset.") {
		t.Error("reset note missing")
	}
}

func TestRunOfflinePingsNow(t *testing.T) {
	d := testDeps(t)
	d.Checker = fakeChecker{offline: true, check: func(context.Context, string) (db.Check, error) {
		return db.Check{}, errors.New("dial tcp: connection refused")
	}}
	m := drive(New(d), tea.WindowSizeMsg{Width: 120, Height: 36}, pingMsg{})
	m = drive(m, keys("enter", "enter", "ctrl+r")...)
	out := screenText(m)
	if !strings.Contains(out, "Could not reach the database") || !strings.Contains(out, "make db-up") {
		t.Errorf("want offline banner with fix:\n%s", out)
	}
	if d.conn != connDown {
		t.Error("a failed run should refresh the connection status")
	}
}
