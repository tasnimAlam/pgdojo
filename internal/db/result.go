package db

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Result is a query result with every value normalized to text.
type Result struct {
	Columns  []string
	Rows     [][]Cell
	Duration time.Duration
}

// Cell is one normalized value. Text is used for both display and comparison.
type Cell struct {
	Text string
	Null bool
}

// NullGlyph is how NULL cells are displayed.
const NullGlyph = "∅"

func (c Cell) String() string {
	if c.Null {
		return NullGlyph
	}
	return c.Text
}

// collect reads all rows and normalizes them.
func collect(rows pgx.Rows, start time.Time) (Result, error) {
	defer rows.Close()
	fds := rows.FieldDescriptions()
	res := Result{Columns: make([]string, len(fds)), Rows: [][]Cell{}}
	for i, fd := range fds {
		res.Columns[i] = fd.Name
	}
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return Result{}, err
		}
		row := make([]Cell, len(vals))
		for i, v := range vals {
			row[i] = normalize(v, fds[i].DataTypeOID)
		}
		res.Rows = append(res.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return Result{}, err
	}
	res.Duration = time.Since(start)
	return res, nil
}

// Fractional seconds are kept so values that differ by microseconds don't
// compare equal; with no fraction this is exactly "2006-01-02 15:04:05".
const tsLayout = "2006-01-02 15:04:05.999999"

func normalize(v any, oid uint32) Cell {
	if v == nil {
		return Cell{Null: true}
	}
	var s string
	switch x := v.(type) {
	case time.Time:
		switch oid {
		case pgtype.DateOID:
			s = x.Format("2006-01-02")
		case pgtype.TimestamptzOID:
			s = x.UTC().Format(tsLayout) + " UTC"
		default:
			s = x.Format(tsLayout)
		}
	case pgtype.Numeric:
		s = numericText(x)
	case float64:
		s = strconv.FormatFloat(x, 'f', -1, 64)
	case float32:
		s = strconv.FormatFloat(float64(x), 'f', -1, 32)
	case pgtype.Interval:
		s = intervalText(x)
	case pgtype.Time:
		s = clockText(x.Microseconds)
	case bool:
		s = strconv.FormatBool(x)
	default:
		s = fmt.Sprint(x)
	}
	return Cell{Text: s}
}

// numericText returns the canonical decimal form with trailing zeros trimmed.
func numericText(n pgtype.Numeric) string {
	v, err := n.Value()
	if err != nil || v == nil {
		return fmt.Sprint(n)
	}
	s := v.(string)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	if s == "-0" {
		s = "0"
	}
	return s
}

// intervalText mimics Postgres' default (postgres style) interval output,
// e.g. "1 year 2 mons 3 days 04:05:06".
func intervalText(iv pgtype.Interval) string {
	var parts []string
	unit := func(n int64, name string) {
		if n == 0 {
			return
		}
		if n != 1 {
			name += "s"
		}
		parts = append(parts, fmt.Sprintf("%d %s", n, name))
	}
	unit(int64(iv.Months/12), "year")
	unit(int64(iv.Months%12), "mon")
	unit(int64(iv.Days), "day")
	if iv.Microseconds != 0 || len(parts) == 0 {
		parts = append(parts, clockText(iv.Microseconds))
	}
	return strings.Join(parts, " ")
}

// clockText formats microseconds as [-]HH:MM:SS[.ffffff].
func clockText(us int64) string {
	sign := ""
	if us < 0 {
		sign, us = "-", -us
	}
	sec := us / 1e6
	s := fmt.Sprintf("%s%02d:%02d:%02d", sign, sec/3600, sec/60%60, sec%60)
	if frac := us % 1e6; frac != 0 {
		s += strings.TrimRight(fmt.Sprintf(".%06d", frac), "0")
	}
	return s
}
