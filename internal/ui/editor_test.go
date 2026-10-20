package ui

import (
	"strings"
	"testing"
)

func TestVimWordMotions(t *testing.T) {
	text := []rune("select name, membercost\nfrom cd.facilities  where x>=1")
	at := func(i int) string { return string(text[i:min(i+4, len(text))]) }

	// w from the start visits each word and punctuation run, like vim.
	var got []string
	for i, n := 0, 0; n < 11; n++ {
		i = nextWordStart(text, i)
		got = append(got, at(i))
	}
	want := []string{"name", ", me", "memb", "from", "cd.f", ".fac", "faci", "wher", "x>=1", ">=1", "1"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("w stops:\n got %q\nwant %q", got, want)
	}

	// b walks back over the same stops.
	got = nil
	for i, n := len(text)-1, 0; n < 10; n++ {
		i = prevWordStart(text, i)
		got = append(got, at(i))
	}
	want = []string{">=1", "x>=1", "wher", "faci", ".fac", "cd.f", "from", "memb", ", me", "name"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("b stops:\n got %q\nwant %q", got, want)
	}

	if nextWordStart(text, len(text)-1) != len(text)-1 || prevWordStart(text, 0) != 0 {
		t.Error("motions must stop at the text edges")
	}
	if nextWordStart(nil, 0) != 0 || prevWordStart(nil, 0) != 0 {
		t.Error("empty text")
	}
}

func TestEditorWordKeys(t *testing.T) {
	m := openExercise(t, passCheck()) // INSERT mode
	m = drive(m, typeText("select name")...)
	m = drive(m, keys("enter")...) // newline in the textarea
	m = drive(m, typeText("from cd.facilities")...)
	m = drive(m, keys("esc")...)
	ed := &view(m).editor

	m = drive(m, keys("b")...)
	if ed.ta.Line() != 1 || ed.ta.Column() != 8 {
		t.Errorf("b from the end should land on facilities (1,8), got (%d,%d)", ed.ta.Line(), ed.ta.Column())
	}
	m = drive(m, keys("b", "b", "b")...) // . cd from
	if ed.ta.Line() != 1 || ed.ta.Column() != 0 {
		t.Errorf("b x3 should reach 'from' (1,0), got (%d,%d)", ed.ta.Line(), ed.ta.Column())
	}
	m = drive(m, keys("b")...)
	if ed.ta.Line() != 0 || ed.ta.Column() != 7 {
		t.Errorf("b should cross the line break to 'name' (0,7), got (%d,%d)", ed.ta.Line(), ed.ta.Column())
	}
	m = drive(m, keys("w")...)
	if ed.ta.Line() != 1 || ed.ta.Column() != 0 {
		t.Errorf("w should cross the line break to 'from' (1,0), got (%d,%d)", ed.ta.Line(), ed.ta.Column())
	}
	if out := screenText(m); !strings.Contains(out, "Query · NORMAL") {
		t.Error("w/b must stay in NORMAL mode")
	}

	// i inserts at the cursor position.
	m = drive(m, keys("i")...)
	m = drive(m, typeText("X")...)
	if v := view(m).editor.Value(); v != "select name\nXfrom cd.facilities" {
		t.Errorf("insert after w landed wrong: %q", v)
	}
}
