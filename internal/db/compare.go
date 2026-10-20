package db

import "strings"

// Diff describes how a user's result differs from the expected one.
type Diff struct {
	Equal         bool
	ColumnCount   struct{ Want, Got int }
	NameMismatch  []int    // column indexes whose names differ (warning only)
	Missing       [][]Cell // in want, not in got (multiset)
	Extra         [][]Cell // in got, not in want (multiset)
	FirstOrderErr int      // ordered only: first row index that differs, -1 if none
}

// Compare checks got against want. Columns are matched by position; names
// only produce warnings. With ordered, rows must also match in sequence.
func Compare(want, got Result, ordered bool) Diff {
	d := Diff{FirstOrderErr: -1}
	d.ColumnCount.Want, d.ColumnCount.Got = len(want.Columns), len(got.Columns)
	for i := 0; i < len(want.Columns) && i < len(got.Columns); i++ {
		if want.Columns[i] != got.Columns[i] {
			d.NameMismatch = append(d.NameMismatch, i)
		}
	}
	if len(want.Columns) != len(got.Columns) {
		return d
	}

	// Multiset difference: count want rows, consume them with got rows.
	counts := map[string]int{}
	for _, r := range want.Rows {
		counts[rowKey(r)]++
	}
	for _, r := range got.Rows {
		k := rowKey(r)
		if counts[k] > 0 {
			counts[k]--
		} else {
			d.Extra = append(d.Extra, r)
		}
	}
	for _, r := range want.Rows {
		k := rowKey(r)
		if counts[k] > 0 {
			counts[k]--
			d.Missing = append(d.Missing, r)
		}
	}
	d.Equal = len(d.Missing) == 0 && len(d.Extra) == 0

	if ordered {
		n := min(len(want.Rows), len(got.Rows))
		for i := 0; i < n; i++ {
			if rowKey(want.Rows[i]) != rowKey(got.Rows[i]) {
				d.FirstOrderErr = i
				break
			}
		}
		if d.FirstOrderErr == -1 && len(want.Rows) != len(got.Rows) {
			d.FirstOrderErr = n
		}
		d.Equal = d.Equal && d.FirstOrderErr == -1
	}
	return d
}

// rowKey encodes a row unambiguously: NULL and the text "∅" or "" differ.
func rowKey(r []Cell) string {
	var b strings.Builder
	for _, c := range r {
		if c.Null {
			b.WriteString("N\x00")
			continue
		}
		b.WriteString("V")
		b.WriteString(c.Text)
		b.WriteByte(0)
	}
	return b.String()
}
