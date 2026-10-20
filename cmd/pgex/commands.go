package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/tasnimAlam/pgdojo/internal/db"
	"github.com/tasnimAlam/pgdojo/internal/exercise"
	"github.com/tasnimAlam/pgdojo/internal/progress"
)

func checkCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "check <category>/<exercise> <file.sql>",
		Short: "Run a SQL file against an exercise and print pass/fail with a diff (exit 0 on pass)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := a.catalog()
			if err != nil {
				return err
			}
			ex, ok := cat.Find(args[0])
			if !ok {
				return fmt.Errorf("no exercise %q; run 'pgex list' to see exercise keys", args[0])
			}
			sql, err := os.ReadFile(args[1])
			if err != nil {
				return fmt.Errorf("could not read %s: %w", args[1], err)
			}
			r, dsn, err := a.runner(cmd.Context())
			if err != nil {
				return err
			}
			chk, err := r.Check(cmd.Context(), ex, string(sql))
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return errors.New("cancelled")
				}
				if pingErr := r.Ping(cmd.Context()); pingErr != nil {
					return connectErr(dsn, pingErr)
				}
				return err
			}
			out := cmd.OutOrStdout()
			printCheck(out, ex, string(sql), chk)

			if store, err := a.progress(); err == nil {
				if err := store.RecordAttempt(ex.Key(), chk.Passed()); err != nil {
					fmt.Fprintln(cmd.ErrOrStderr(), "warning: could not save progress:", err)
				}
			} else {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning:", err)
			}
			if !chk.Passed() {
				return errQuiet
			}
			return nil
		},
	}
}

func printCheck(w io.Writer, ex exercise.Exercise, sql string, chk db.Check) {
	if e := chk.UserErr; e != nil {
		fmt.Fprintln(w, "✗ ERROR:", e.Message)
		if e.Detail != "" {
			fmt.Fprintln(w, "DETAIL:", e.Detail)
		}
		if e.Hint != "" {
			fmt.Fprintln(w, "HINT:", e.Hint)
		}
		if line, col, ok := e.Locate(sql); ok {
			fmt.Fprintf(w, "\n  %s\n  %s^\n", line, strings.Repeat(" ", col))
		}
		return
	}
	d := chk.Diff
	if d.Equal {
		fmt.Fprintf(w, "✓ Correct: all %d rows match the expected result.\n", len(chk.Want.Rows))
		for _, i := range d.NameMismatch {
			fmt.Fprintf(w, "  note: column %d is named %q, expected %q.\n", i+1, chk.Got.Columns[i], chk.Want.Columns[i])
		}
		return
	}
	fmt.Fprintln(w, "✗ Not quite:", d.Reason())
	fmt.Fprintf(w, "  expected %d rows, got %d · columns %d / %d", len(chk.Want.Rows), len(chk.Got.Rows), d.ColumnCount.Got, d.ColumnCount.Want)
	if ex.Ordered {
		fmt.Fprint(w, " · order matters")
	}
	fmt.Fprintln(w)

	if d.ColumnCount.Want != d.ColumnCount.Got {
		fmt.Fprintln(w, "\n  expected columns:", strings.Join(chk.Want.Columns, ", "))
		fmt.Fprintln(w, "  your columns:    ", strings.Join(chk.Got.Columns, ", "))
		return
	}
	const limit = 20
	var rows [][]string
	var gutter []string
	add := func(mark string, rs [][]db.Cell) {
		for _, r := range rs {
			rows = append(rows, cellStrings(r))
			gutter = append(gutter, mark)
		}
	}
	add("−", d.Missing)
	add("+", d.Extra)
	if len(rows) == 0 && d.FirstOrderErr >= 0 {
		fmt.Fprintf(w, "\n  row %d expected:\n", d.FirstOrderErr+1)
		add(" ", chk.Want.Rows[d.FirstOrderErr:d.FirstOrderErr+1])
		printTable(w, chk.Want.Columns, rows, gutter)
		rows, gutter = nil, nil
		fmt.Fprintf(w, "\n  row %d yours:\n", d.FirstOrderErr+1)
		add(" ", chk.Got.Rows[d.FirstOrderErr:d.FirstOrderErr+1])
		printTable(w, chk.Want.Columns, rows, gutter)
		return
	}
	fmt.Fprintln(w, "\n  − missing (expected, not in yours)   + extra (in yours, not expected)")
	more := len(rows) - limit
	if more > 0 {
		rows, gutter = rows[:limit], gutter[:limit]
	}
	printTable(w, chk.Want.Columns, rows, gutter)
	if more > 0 {
		fmt.Fprintf(w, "  … %d more\n", more)
	}
}

func cellStrings(r []db.Cell) []string {
	s := make([]string, len(r))
	for i, c := range r {
		s[i] = c.String()
	}
	return s
}

// printTable renders rows with content-sized columns capped at 30 cells.
func printTable(w io.Writer, cols []string, rows [][]string, gutter []string) {
	const maxWidth = 30
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = utf8.RuneCountInString(c)
	}
	for _, r := range rows {
		for i, v := range r {
			widths[i] = max(widths[i], utf8.RuneCountInString(v))
		}
	}
	line := func(mark string, vals []string) {
		var b strings.Builder
		b.WriteString("  " + mark + " ")
		for i, v := range vals {
			widths[i] = min(widths[i], maxWidth)
			if utf8.RuneCountInString(v) > widths[i] {
				v = string([]rune(v)[:widths[i]-1]) + "…"
			}
			b.WriteString(v + strings.Repeat(" ", widths[i]-utf8.RuneCountInString(v)) + "  ")
		}
		fmt.Fprintln(w, strings.TrimRight(b.String(), " "))
	}
	line(" ", cols)
	for i, r := range rows {
		line(gutter[i], r)
	}
}

var statusMarker = map[progress.Status]string{
	progress.Solved:    "✓",
	progress.Attempted: "●",
	progress.New:       "○",
	progress.Revealed:  "◐",
}

func listCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Print categories and exercises with your progress",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := a.catalog()
			if err != nil {
				return err
			}
			store, err := a.progress()
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			for ci, c := range cat.Categories {
				solved := 0
				for _, ex := range c.Exercises {
					if store.Get(ex.Key()).Status == progress.Solved {
						solved++
					}
				}
				if ci > 0 {
					fmt.Fprintln(w)
				}
				fmt.Fprintf(w, "%s  (%d/%d solved)\n", c.Title, solved, len(c.Exercises))
				for i, ex := range c.Exercises {
					fmt.Fprintf(w, "  %s %2d. %-45s %s\n", statusMarker[store.Get(ex.Key()).Status], i+1, ex.Title, ex.Key())
				}
			}
			fmt.Fprintln(w, "\n✓ solved  ● attempted  ○ new  ◐ revealed")
			return nil
		},
	}
}

func validateCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Load all exercises and run every reference answer against the database",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			cat, err := a.catalog()
			if err != nil {
				return err
			}
			for _, warn := range cat.Warnings {
				fmt.Fprintln(w, "warning:", warn)
			}
			r, dsn, err := a.runner(cmd.Context())
			if err != nil {
				return err
			}
			if err := r.Ping(cmd.Context()); err != nil {
				return connectErr(dsn, err)
			}
			total, failed := 0, 0
			for _, c := range cat.Categories {
				for _, ex := range c.Exercises {
					total++
					chk, err := r.Check(cmd.Context(), ex, ex.Answer)
					switch {
					case err != nil:
						failed++
						fmt.Fprintf(w, "✗ %s: %v\n", ex.Key(), err)
					case !chk.Passed():
						failed++
						reason := chk.Diff.Reason()
						if chk.UserErr != nil {
							reason = chk.UserErr.Message
						}
						fmt.Fprintf(w, "✗ %s: answer does not pass its own check: %s\n", ex.Key(), reason)
					default:
						fmt.Fprintf(w, "✓ %s (%d rows)\n", ex.Key(), len(chk.Want.Rows))
					}
				}
			}
			fmt.Fprintf(w, "\n%d of %d exercises valid.\n", total-failed, total)
			if failed > 0 {
				return errQuiet
			}
			return nil
		},
	}
}

func doctorCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check the DSN, connectivity, schema cd, role permissions and $EDITOR",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			ok := true
			report := func(pass bool, msg, fix string) {
				if pass {
					fmt.Fprintln(w, "✓", msg)
					return
				}
				ok = false
				fmt.Fprintln(w, "✗", msg)
				if fix != "" {
					fmt.Fprintln(w, "   fix:", fix)
				}
			}

			cfg, err := a.config()
			if err != nil {
				report(false, err.Error(), "fix or remove the config file")
				return errQuiet
			}
			editor := cfg.EditorCommand()
			_, lookErr := exec.LookPath(strings.Fields(editor + " _")[0])
			report(lookErr == nil, "editor: "+editor, "set $EDITOR (fish: set -Ux EDITOR nvim) or 'editor' in the config file")

			ctx := cmd.Context()
			r, dsn, err := a.runner(ctx)
			if err != nil {
				report(false, "DSN: "+err.Error(), "check --dsn, PGEX_DSN or 'dsn' in the config file")
				return errQuiet
			}
			report(true, "DSN: "+db.Describe(dsn), "")
			if err := r.Ping(ctx); err != nil {
				report(false, "connect: "+rootCause(err), "start the database with 'make db-up' and check the user and password in the DSN")
				return errQuiet
			}
			report(true, "connected", "")

			res, err := r.Run(ctx, `select current_user,
				has_schema_privilege('cd', 'USAGE'),
				has_schema_privilege('cd', 'CREATE'),
				(select count(*) from information_schema.tables where table_schema = 'cd')`)
			if err != nil {
				report(false, "schema cd: "+err.Error(), "load db/clubdata.sql: 'make db-reset'")
				return errQuiet
			}
			v := res.Rows[0]
			report(v[1].Text == "true", "schema cd is usable by "+v[0].Text, "run db/roles.sql or 'make db-reset'")
			report(v[3].Text == "3", "schema cd has "+v[3].Text+" tables (want 3)", "reload data with 'make db-reset'")
			report(v[2].Text == "false", "role cannot create objects in cd", "connect as pgex_player, not a superuser")

			for _, t := range []string{"members", "facilities", "bookings"} {
				res, err := r.Run(ctx, fmt.Sprintf(`select
					has_table_privilege('cd.%[1]s', 'SELECT') and has_table_privilege('cd.%[1]s', 'INSERT')
					and has_table_privilege('cd.%[1]s', 'UPDATE') and has_table_privilege('cd.%[1]s', 'DELETE'),
					(select count(*) from cd.%[1]s)`, t))
				if err != nil {
					report(false, "cd."+t+": "+err.Error(), "reload data with 'make db-reset'")
					continue
				}
				report(res.Rows[0][0].Text == "true", fmt.Sprintf("cd.%s: read/write, %s rows", t, res.Rows[0][1].Text), "run db/roles.sql or 'make db-reset'")
			}
			if !ok {
				return errQuiet
			}
			fmt.Fprintln(w, "\nAll checks passed.")
			return nil
		},
	}
}
