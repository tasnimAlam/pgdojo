package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/tasnimAlam/pgdojo/internal/db"
)

// viewMode is what the result pane shows after a run; v cycles it.
type viewMode int

const (
	viewYours viewMode = iota
	viewExpected
	viewDiff
)

func (v viewMode) String() string { return [...]string{"yours", "expected", "diff"}[v] }

type gutter int

const (
	gutterNone gutter = iota
	gutterSame
	gutterMissing
	gutterExtra
)

const maxColWidth = 30

// tableRows returns the rows (and gutter marks) to display for a check in
// the given mode. Diff mode lists expected rows, marking those missing from
// the user's result, then the user's extra rows.
func tableRows(chk db.Check, mode viewMode) ([][]db.Cell, []gutter) {
	switch mode {
	case viewExpected:
		return chk.Want.Rows, nil
	case viewDiff:
		missing := map[string]int{}
		for _, r := range chk.Diff.Missing {
			missing[rowText(r)]++
		}
		var rows [][]db.Cell
		var marks []gutter
		for _, r := range chk.Want.Rows {
			m := gutterSame
			if k := rowText(r); missing[k] > 0 {
				missing[k]--
				m = gutterMissing
			}
			rows, marks = append(rows, r), append(marks, m)
		}
		for _, r := range chk.Diff.Extra {
			rows, marks = append(rows, r), append(marks, gutterExtra)
		}
		return rows, marks
	default:
		return chk.Got.Rows, nil
	}
}

func rowText(r []db.Cell) string {
	var b strings.Builder
	for _, c := range r {
		if c.Null {
			b.WriteString("\x01")
		} else {
			b.WriteString(c.Text)
		}
		b.WriteByte(0)
	}
	return b.String()
}

// renderTable draws columns and rows into at most h lines of width w,
// starting at row rowOff and column colOff. marks may be nil.
func renderTable(t Theme, cols []string, rows [][]db.Cell, marks []gutter, w, h, rowOff, colOff int) string {
	if len(cols) == 0 {
		return t.Dim.Render("The statement returned no columns.")
	}
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = min(lipgloss.Width(c), maxColWidth)
	}
	for _, r := range rows {
		for i, c := range r {
			if i < len(widths) {
				widths[i] = min(max(widths[i], lipgloss.Width(c.String())), maxColWidth)
			}
		}
	}
	colOff = min(max(colOff, 0), len(cols)-1)

	cell := func(s string, wd int) string {
		if lipgloss.Width(s) > wd {
			s = ansi.Truncate(s, wd, "…")
		}
		return s + strings.Repeat(" ", wd-lipgloss.Width(s))
	}
	line := func(vals []string, styles []lipgloss.Style, g gutter, rowStyle *lipgloss.Style) string {
		var b strings.Builder
		if marks != nil {
			switch g {
			case gutterMissing:
				b.WriteString(t.MissingRow.Render("− "))
			case gutterExtra:
				b.WriteString(t.ExtraGutter.Render("+ "))
			default:
				b.WriteString("  ")
			}
		}
		for i := colOff; i < len(vals); i++ {
			st := styles[i]
			if rowStyle != nil {
				st = *rowStyle
			}
			b.WriteString(st.Render(cell(vals[i], widths[i])))
			if i < len(vals)-1 {
				sep := " "
				if rowStyle != nil {
					sep = rowStyle.Render(" ")
				}
				b.WriteString(sep + sep)
			}
		}
		return ansi.Truncate(b.String(), w, "…")
	}

	headStyles := make([]lipgloss.Style, len(cols))
	for i := range headStyles {
		headStyles[i] = t.TableHead
	}
	out := []string{line(cols, headStyles, gutterNone, nil)}

	rowOff = min(max(rowOff, 0), max(len(rows)-1, 0))
	if len(rows) == 0 {
		return strings.Join(append(out, t.Dim.Render("(no rows)")), "\n")
	}
	avail := max(h-1, 1) // lines left under the header
	n := len(rows) - rowOff
	if n > avail {
		n = avail - 1 // leave room for "… N more"
	}
	for i := rowOff; i < rowOff+n; i++ {
		vals := make([]string, len(cols))
		styles := make([]lipgloss.Style, len(cols))
		for j := range cols {
			if j < len(rows[i]) && rows[i][j].Null {
				vals[j], styles[j] = db.NullGlyph, t.Dim
			} else if j < len(rows[i]) {
				vals[j], styles[j] = rows[i][j].Text, t.Text
			}
		}
		var g gutter
		var rs *lipgloss.Style
		if marks != nil {
			g = marks[i]
			if g == gutterMissing {
				rs = &t.MissingRow
			}
		}
		out = append(out, line(vals, styles, g, rs))
	}
	if more := len(rows) - rowOff - n; more > 0 {
		out = append(out, t.Dim.Render(fmt.Sprintf("… %d more", more)))
	}
	return strings.Join(out, "\n")
}

// diffSummary is the left column's lower pane after a failed run.
func diffSummary(t Theme, chk db.Check, ordered bool) string {
	d := chk.Diff
	lv := func(label, val string) string { return t.labelValue(label, val, 15) }
	cols := t.Accent.Render("✓")
	if d.ColumnCount.Want != d.ColumnCount.Got {
		cols = t.Fail.Render("✗")
	}
	lines := []string{
		lv("expected rows", t.Text.Render(fmt.Sprint(len(chk.Want.Rows)))),
		lv("your rows", t.Text.Render(fmt.Sprint(len(chk.Got.Rows)))),
		lv("− missing", t.Fail.Render(fmt.Sprint(len(d.Missing)))),
		lv("+ extra", t.Warn.Render(fmt.Sprint(len(d.Extra)))),
		lv("columns", t.Text.Render(fmt.Sprintf("%d / %d ", d.ColumnCount.Got, d.ColumnCount.Want))+cols),
	}
	if ordered && d.FirstOrderErr >= 0 && len(d.Missing) == 0 && len(d.Extra) == 0 {
		lines = append(lines, lv("order", t.Fail.Render(fmt.Sprintf("differs at row %d", d.FirstOrderErr+1))))
	}
	for _, i := range d.NameMismatch {
		if i < len(chk.Got.Columns) && i < len(chk.Want.Columns) {
			lines = append(lines, t.Dim.Render(fmt.Sprintf("note: column %d is %q, expected %q", i+1, chk.Got.Columns[i], chk.Want.Columns[i])))
		}
	}
	lines = append(lines, "", t.Dim.Render("v cycles yours / expected / diff"))
	return strings.Join(lines, "\n")
}
