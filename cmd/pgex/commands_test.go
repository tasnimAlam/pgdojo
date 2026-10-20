package main

import (
	"strings"
	"testing"

	"github.com/tasnimAlam/pgex/internal/db"
	"github.com/tasnimAlam/pgex/internal/exercise"
)

func TestPrintCheck(t *testing.T) {
	cols := []string{"id"}
	want := db.Result{Columns: cols, Rows: [][]db.Cell{{{Text: "1"}}, {{Text: "2"}}}}
	short := db.Result{Columns: cols, Rows: [][]db.Cell{{{Text: "1"}}}}
	sql := "select\n  idd from t"

	tests := []struct {
		name string
		chk  db.Check
		want []string
	}{
		{"pass", db.Check{Want: want, Got: want, Diff: db.Compare(want, want, false)},
			[]string{"✓ Correct: all 2 rows match"}},
		{"fail", db.Check{Want: want, Got: short, Diff: db.Compare(want, short, false)},
			[]string{"✗ Not quite: 1 row missing.", "− 2"}},
		{"user error", db.Check{UserErr: &db.UserError{Message: "column \"idd\" does not exist", Hint: "try id", Position: 10}},
			[]string{"✗ ERROR: column \"idd\" does not exist", "HINT: try id", "  idd from t\n    ^"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			printCheck(&b, exercise.Exercise{}, sql, tt.chk)
			for _, s := range tt.want {
				if !strings.Contains(b.String(), s) {
					t.Errorf("output missing %q:\n%s", s, b.String())
				}
			}
		})
	}
}
