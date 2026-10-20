package ui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
)

type column struct {
	name, typ string
	key       string // "PK", "FK" or ""
	ref       string // referenced table for FKs, e.g. "members"
}

// cdSchema describes the cd tables. Rows are ordered so foreign keys line
// up with the columns they reference in the diagram.
var cdSchema = map[string][]column{
	"cd.members": {
		{"memid", "integer", "PK", ""}, {"surname", "varchar", "", ""}, {"firstname", "varchar", "", ""},
		{"address", "varchar", "", ""}, {"zipcode", "integer", "", ""}, {"telephone", "varchar", "", ""},
		{"recommendedby", "integer", "FK", "members"}, {"joindate", "timestamp", "", ""},
	},
	"cd.bookings": {
		{"bookid", "integer", "PK", ""}, {"memid", "integer", "FK", "members"}, {"facid", "integer", "FK", "facilities"},
		{"starttime", "timestamp", "", ""}, {"slots", "integer", "", ""},
	},
	"cd.facilities": {
		{"facid", "integer", "PK", ""}, {"name", "varchar", "", ""}, {"membercost", "numeric", "", ""},
		{"guestcost", "numeric", "", ""}, {"initialoutlay", "numeric", "", ""}, {"monthlymaintenance", "numeric", "", ""},
	},
}

var shortType = map[string]string{"integer": "int", "varchar": "text", "numeric": "num", "timestamp": "ts"}

const (
	gapW  = 6 // connector width between boxes
	loopW = 2 // self-reference loop left of members
)

// diagramWidth is the diagram's width with or without the type column.
func diagramWidth(types bool) int {
	return loopW + 3*boxWidth(types) + 2*gapW
}

// boxWidth fits "monthlymaintenance", an optional 4-cell type and "PK".
func boxWidth(types bool) int {
	if types {
		return 30
	}
	return 25
}

// schemaView renders the schema overlay body for width w: a diagram when
// it fits, otherwise a list. used tables are highlighted.
func schemaView(t Theme, used []string, w int) []string {
	var lines []string
	switch {
	case w >= diagramWidth(true):
		lines = schemaDiagram(t, used, true)
	case w >= diagramWidth(false):
		lines = schemaDiagram(t, used, false)
	default:
		lines = schemaList(t, used, w)
	}
	lines = append(lines, "")
	notes := []string{
		"bookings.memid → members.memid: who made the booking (memid 0 is the guest account)",
		"bookings.facid → facilities.facid: what was booked",
		"members.recommendedby → members.memid: who recommended them, NULL if nobody",
		"slots are half-hour periods; costs are per slot",
	}
	for _, n := range notes {
		lines = append(lines, wrap("• "+n, w, t.Dim)...)
	}
	return append(lines, "", t.Dim.Render("s or esc closes"))
}

func schemaDiagram(t Theme, used []string, types bool) []string {
	boxW := boxWidth(types)
	box := func(table string, padTop int) string {
		var rows []string
		for _, c := range cdSchema[table] {
			name := t.Text.Render(fmt.Sprintf("%-18s", c.name))
			key := "  "
			switch c.key {
			case "PK":
				name, key = t.Accent.Render(fmt.Sprintf("%-18s", c.name)), t.Accent.Render("PK")
			case "FK":
				name, key = t.Warn.Render(fmt.Sprintf("%-18s", c.name)), t.Warn.Render("FK")
			}
			typ := ""
			if types {
				typ = t.Dim.Render(fmt.Sprintf("%-4s", shortType[c.typ])) + " "
			}
			rows = append(rows, name+" "+typ+key)
		}
		// Every table gets the accent border; the exercise's tables get a bold title.
		title := t.Accent
		if slices.Contains(used, table) {
			title = t.Accent.Bold(true)
		}
		b := t.PaneColored(strings.TrimPrefix(table, "cd."), strings.Join(rows, "\n"), boxW, len(rows)+2, t.P.Accent, title)
		return strings.Repeat(strings.Repeat(" ", boxW)+"\n", padTop) + b
	}
	// connector draws text on one line of an otherwise blank column.
	connector := func(w, line int, text string) string {
		col := make([]string, line+1)
		for i := range col {
			col[i] = strings.Repeat(" ", w)
		}
		col[line] = t.Warn.Render(text)
		return strings.Join(col, "\n")
	}

	// members is shifted down 1 and facilities 2 so that members.memid sits on
	// line 2 next to bookings.memid, and facilities.facid on line 3 next to
	// bookings.facid. recommendedby is members row 6, i.e. line 8.
	loop := []string{"  ", "  ", t.Warn.Render("╭►")}
	for i := 3; i < 8; i++ {
		loop = append(loop, t.Warn.Render("│ "))
	}
	loop = append(loop, t.Warn.Render("╰─"))

	d := lipgloss.JoinHorizontal(lipgloss.Top,
		strings.Join(loop, "\n"),
		box("cd.members", 1),
		connector(gapW, 2, "◄"+strings.Repeat("─", gapW-1)),
		box("cd.bookings", 0),
		connector(gapW, 3, strings.Repeat("─", gapW-1)+"►"),
		box("cd.facilities", 2),
	)
	return append(strings.Split(d, "\n"), "",
		t.Accent.Render("PK")+t.Dim.Render(" primary key  ")+t.Warn.Render("FK")+t.Dim.Render(" foreign key, arrow points to the referenced column"))
}

// schemaList is the narrow-terminal fallback: one wrapped line per table.
func schemaList(t Theme, used []string, w int) []string {
	var lines []string
	for _, table := range []string{"cd.members", "cd.bookings", "cd.facilities"} {
		title := t.Dim.Bold(true).Render(table)
		if slices.Contains(used, table) {
			title = t.Accent.Bold(true).Render(table)
		}
		var cols []string
		for _, c := range cdSchema[table] {
			switch c.key {
			case "PK":
				cols = append(cols, t.Accent.Render(c.name+" PK"))
			case "FK":
				cols = append(cols, t.Warn.Render(c.name+" → "+c.ref))
			default:
				cols = append(cols, t.Text.Render(c.name))
			}
		}
		// Wrap on the separators so styled column names are never split.
		line := title + t.Dim.Render(":")
		for i, c := range cols {
			sep := " "
			if i > 0 {
				sep = t.Dim.Render(", ")
			}
			if lipgloss.Width(line+sep+c) > w {
				lines = append(lines, line+strings.TrimSpace(sep))
				line, sep = "  ", ""
			}
			line += sep + c
		}
		lines = append(lines, line)
	}
	return lines
}
