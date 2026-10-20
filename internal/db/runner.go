package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tasnimAlam/pgex/internal/exercise"
)

// Check is the outcome of running a user's query against an exercise.
type Check struct {
	Got, Want Result
	Diff      Diff
	// UserErr is set when the user's SQL failed (syntax error, timeout,
	// rejected input). Got and Diff are empty then; Want is still filled.
	UserErr *UserError
}

// Passed reports whether the user's result matched.
func (c Check) Passed() bool { return c.UserErr == nil && c.Diff.Equal }

// UserError is a problem with the user's SQL, shown in the result pane.
type UserError struct {
	Message  string
	Detail   string
	Hint     string
	Code     string // SQLSTATE, empty for errors detected before running
	Position int    // 1-based character offset into the SQL, 0 if unknown
}

func (e *UserError) Error() string { return e.Message }

// Runner executes queries in transactions that are always rolled back.
type Runner struct {
	pool *pgxpool.Pool
}

func NewRunner(pool *pgxpool.Pool) *Runner { return &Runner{pool: pool} }

// Ping checks that the database is reachable.
func (r *Runner) Ping(ctx context.Context) error { return r.pool.Ping(ctx) }

// Run executes a single query in a rolled-back transaction. Errors are
// returned as Go errors; use Check for user SQL.
func (r *Runner) Run(ctx context.Context, sql string) (Result, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	return query(ctx, tx, sql)
}

// Check runs userSQL and the exercise's reference answer in one transaction
// that is always rolled back, then compares the results.
func (r *Runner) Check(ctx context.Context, ex exercise.Exercise, userSQL string) (Check, error) {
	var c Check
	if uerr := CheckSingleStatement(userSQL); uerr != nil {
		c.UserErr = uerr
		return c, nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return c, err
	}
	// Rollback must run even after ctx is cancelled.
	defer tx.Rollback(context.WithoutCancel(ctx))

	if _, err := tx.Exec(ctx, "SAVEPOINT s"); err != nil {
		return c, err
	}
	// The savepoint lets the reference answer run even if the user's SQL
	// failed and aborted the transaction.
	if ex.Mutates {
		_, err = tx.Exec(ctx, userSQL)
		if err == nil {
			c.Got, err = query(ctx, tx, ex.Verify)
			if err != nil {
				return c, refErr(ex, "verify query", err)
			}
		}
	} else {
		c.Got, err = query(ctx, tx, userSQL)
	}
	if err != nil {
		if c.UserErr = asUserErr(ctx, tx, err); c.UserErr == nil {
			return c, err
		}
		c.Got = Result{}
	}
	if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT s"); err != nil {
		return c, err
	}

	if ex.Mutates {
		if _, err := tx.Exec(ctx, ex.Answer); err != nil {
			return c, refErr(ex, "reference answer", err)
		}
		if c.Want, err = query(ctx, tx, ex.Verify); err != nil {
			return c, refErr(ex, "verify query", err)
		}
	} else if c.Want, err = query(ctx, tx, ex.Answer); err != nil {
		return c, refErr(ex, "reference answer", err)
	}

	if c.UserErr == nil {
		c.Diff = Compare(c.Want, c.Got, ex.Ordered)
	}
	return c, nil
}

func query(ctx context.Context, tx pgx.Tx, sql string) (Result, error) {
	start := time.Now()
	rows, err := tx.Query(ctx, sql)
	if err != nil {
		return Result{}, err
	}
	return collect(rows, start)
}

func refErr(ex exercise.Exercise, what string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("exercise %s: %s failed: %w", ex.Key(), what, err)
}

// asUserErr classifies an error from the user's SQL. It returns nil when the
// error is not the user's fault (cancellation, lost connection).
func asUserErr(ctx context.Context, tx pgx.Tx, err error) *UserError {
	if ctx.Err() != nil || tx.Conn().IsClosed() {
		return nil
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return &UserError{
			Message:  pg.Message,
			Detail:   pg.Detail,
			Hint:     pg.Hint,
			Code:     pg.Code,
			Position: int(pg.Position),
		}
	}
	// e.g. pgx rejecting "$1" placeholders with no arguments.
	return &UserError{Message: err.Error()}
}

// txControl are statements that would end or escape the check transaction.
var txControl = map[string]bool{
	"begin": true, "start": true, "commit": true, "end": true, "rollback": true,
	"abort": true, "savepoint": true, "release": true, "prepare": true,
}

// CheckSingleStatement rejects empty input, more than one statement, and
// transaction control. It ignores semicolons inside comments, string
// literals, quoted identifiers and dollar-quoted strings.
func CheckSingleStatement(sql string) *UserError {
	code := strings.TrimSpace(stripLiterals(sql))
	code = strings.TrimSpace(strings.TrimRight(code, "; \t\r\n"))
	if code == "" {
		return &UserError{Message: "The query is empty. Write a SQL statement, then run it."}
	}
	if strings.Contains(code, ";") {
		return &UserError{Message: "Only one SQL statement can be run at a time. Remove everything after the first semicolon."}
	}
	words := strings.FieldsFunc(code, func(r rune) bool { return r > 127 || !isIdent(byte(r)) })
	if len(words) > 0 && txControl[strings.ToLower(words[0])] {
		return &UserError{Message: "Transaction commands like " + strings.ToUpper(words[0]) + " are not allowed: pgex runs every query in its own transaction and rolls it back."}
	}
	return nil
}

// stripLiterals replaces comments and quoted text with spaces so only SQL
// tokens remain.
func stripLiterals(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		switch {
		case strings.HasPrefix(s[i:], "--"):
			for i < len(s) && s[i] != '\n' {
				i++
			}
			b.WriteByte(' ')
		case strings.HasPrefix(s[i:], "/*"):
			// Postgres block comments nest.
			depth := 0
			for i < len(s) {
				if strings.HasPrefix(s[i:], "/*") {
					depth++
					i += 2
				} else if strings.HasPrefix(s[i:], "*/") {
					depth--
					i += 2
					if depth == 0 {
						break
					}
				} else {
					i++
				}
			}
			b.WriteByte(' ')
		case s[i] == '\'' || s[i] == '"':
			// E'...' strings allow backslash escapes.
			escapes := s[i] == '\'' && i > 0 && (s[i-1] == 'E' || s[i-1] == 'e') &&
				(i == 1 || !isIdent(s[i-2]))
			i = skipQuoted(s, i, s[i], escapes)
			b.WriteByte(' ')
		case s[i] == '$':
			if tag, ok := dollarTag(s[i:]); ok {
				end := strings.Index(s[i+len(tag):], tag)
				if end < 0 {
					i = len(s)
				} else {
					i += len(tag) + end + len(tag)
				}
				b.WriteByte(' ')
			} else {
				b.WriteByte(s[i])
				i++
			}
		default:
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String()
}

// skipQuoted returns the index after the closing quote. A doubled quote is
// an escaped quote.
func skipQuoted(s string, i int, q byte, backslash bool) int {
	for i++; i < len(s); i++ {
		switch {
		case backslash && s[i] == '\\':
			i++
		case s[i] == q && i+1 < len(s) && s[i+1] == q:
			i++
		case s[i] == q:
			return i + 1
		}
	}
	return len(s)
}

func isIdent(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// dollarTag returns "$tag$" if s starts with a dollar-quote opener.
func dollarTag(s string) (string, bool) {
	for j := 1; j < len(s); j++ {
		c := s[j]
		switch {
		case c == '$':
			return s[:j+1], true
		case c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || j > 1 && c >= '0' && c <= '9':
		default:
			return "", false
		}
	}
	return "", false
}

// Locate converts Position into the offending line of sql and a 0-based
// rune column within it. ok is false when there is no position.
func (e *UserError) Locate(sql string) (line string, col int, ok bool) {
	if e.Position <= 0 {
		return "", 0, false
	}
	runes := []rune(sql)
	pos := min(e.Position-1, len(runes))
	start := pos
	for start > 0 && runes[start-1] != '\n' {
		start--
	}
	end := pos
	for end < len(runes) && runes[end] != '\n' {
		end++
	}
	return string(runes[start:end]), pos - start, true
}
