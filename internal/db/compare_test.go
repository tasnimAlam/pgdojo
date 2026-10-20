package db

import (
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func res(cols []string, rows ...[]Cell) Result { return Result{Columns: cols, Rows: rows} }
func row(vals ...string) []Cell {
	r := make([]Cell, len(vals))
	for i, v := range vals {
		if v == "<null>" {
			r[i] = Cell{Null: true}
		} else {
			r[i] = Cell{Text: v}
		}
	}
	return r
}

var ab = []string{"a", "b"}

func TestCompare(t *testing.T) {
	tests := []struct {
		name         string
		want, got    Result
		ordered      bool
		equal        bool
		missing      int
		extra        int
		orderErr     int
		nameMismatch []int
	}{
		{"identical", res(ab, row("1", "x")), res(ab, row("1", "x")), false, true, 0, 0, -1, nil},
		{"both empty", res(ab), res(ab), false, true, 0, 0, -1, nil},
		{"both empty ordered", res(ab), res(ab), true, true, 0, 0, -1, nil},
		{"got empty", res(ab, row("1", "x")), res(ab), false, false, 1, 0, -1, nil},
		{"want empty", res(ab), res(ab, row("1", "x")), false, false, 0, 1, -1, nil},
		{"unordered permutation", res(ab, row("1", "x"), row("2", "y")), res(ab, row("2", "y"), row("1", "x")), false, true, 0, 0, -1, nil},
		{"ordered permutation", res(ab, row("1", "x"), row("2", "y")), res(ab, row("2", "y"), row("1", "x")), true, false, 0, 0, 0, nil},
		{"ordered match", res(ab, row("1", "x"), row("2", "y")), res(ab, row("1", "x"), row("2", "y")), true, true, 0, 0, -1, nil},
		{"ordered wrong at row 2", res(ab, row("1", "x"), row("2", "y"), row("3", "z")), res(ab, row("1", "x"), row("3", "z"), row("2", "y")), true, false, 0, 0, 1, nil},
		{"ordered got shorter", res(ab, row("1", "x"), row("2", "y")), res(ab, row("1", "x")), true, false, 1, 0, 1, nil},
		{"duplicates missing", res(ab, row("1", "x"), row("1", "x")), res(ab, row("1", "x")), false, false, 1, 0, -1, nil},
		{"duplicates extra", res(ab, row("1", "x")), res(ab, row("1", "x"), row("1", "x")), false, false, 0, 1, -1, nil},
		{"duplicates match", res(ab, row("1", "x"), row("1", "x")), res(ab, row("1", "x"), row("1", "x")), false, true, 0, 0, -1, nil},
		{"null equals null", res(ab, row("1", "<null>")), res(ab, row("1", "<null>")), false, true, 0, 0, -1, nil},
		{"null vs empty string", res(ab, row("1", "<null>")), res(ab, row("1", "")), false, false, 1, 1, -1, nil},
		{"null vs glyph text", res(ab, row("1", "<null>")), res(ab, row("1", NullGlyph)), false, false, 1, 1, -1, nil},
		{"column names differ is warning only", res(ab, row("1", "x")), res([]string{"a", "c"}, row("1", "x")), false, true, 0, 0, -1, []int{1}},
		{"column count mismatch", res(ab, row("1", "x")), res([]string{"a"}, row("1")), false, false, 0, 0, -1, nil},
		{"wrong value", res(ab, row("1", "x")), res(ab, row("1", "y")), false, false, 1, 1, -1, nil},
		{"cell boundaries matter", res(ab, row("1", "23")), res(ab, row("12", "3")), false, false, 1, 1, -1, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Compare(tt.want, tt.got, tt.ordered)
			if d.Equal != tt.equal {
				t.Errorf("Equal = %v, want %v", d.Equal, tt.equal)
			}
			if len(d.Missing) != tt.missing || len(d.Extra) != tt.extra {
				t.Errorf("missing/extra = %d/%d, want %d/%d", len(d.Missing), len(d.Extra), tt.missing, tt.extra)
			}
			if d.FirstOrderErr != tt.orderErr {
				t.Errorf("FirstOrderErr = %d, want %d", d.FirstOrderErr, tt.orderErr)
			}
			if !reflect.DeepEqual(d.NameMismatch, tt.nameMismatch) {
				t.Errorf("NameMismatch = %v, want %v", d.NameMismatch, tt.nameMismatch)
			}
			if d.ColumnCount.Want != len(tt.want.Columns) || d.ColumnCount.Got != len(tt.got.Columns) {
				t.Errorf("ColumnCount = %+v", d.ColumnCount)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	ts := time.Date(2012, 9, 14, 8, 30, 0, 0, time.UTC)
	dhaka := time.FixedZone("BDT", 6*3600)
	num := func(i int64, exp int32) pgtype.Numeric {
		return pgtype.Numeric{Int: big.NewInt(i), Exp: exp, Valid: true}
	}
	tests := []struct {
		name string
		v    any
		oid  uint32
		want string
	}{
		{"timestamp", ts, pgtype.TimestampOID, "2012-09-14 08:30:00"},
		{"timestamp fraction", ts.Add(1500 * time.Microsecond), pgtype.TimestampOID, "2012-09-14 08:30:00.0015"},
		{"timestamptz to UTC", ts.In(dhaka), pgtype.TimestamptzOID, "2012-09-14 08:30:00 UTC"},
		{"date", ts, pgtype.DateOID, "2012-09-14"},
		{"numeric 5.50", num(550, -2), pgtype.NumericOID, "5.5"},
		{"numeric 5.00", num(500, -2), pgtype.NumericOID, "5"},
		{"numeric integer with zeros", num(1000, 0), pgtype.NumericOID, "1000"},
		{"numeric positive exp", num(12, 2), pgtype.NumericOID, "1200"},
		{"numeric small", num(5, -3), pgtype.NumericOID, "0.005"},
		{"numeric negative", num(-250, -2), pgtype.NumericOID, "-2.5"},
		{"numeric zero", num(0, -2), pgtype.NumericOID, "0"},
		{"float", 0.1, pgtype.Float8OID, "0.1"},
		{"float whole", 3.0, pgtype.Float8OID, "3"},
		{"bool", true, pgtype.BoolOID, "true"},
		{"int", int64(42), pgtype.Int8OID, "42"},
		{"interval days", pgtype.Interval{Days: 29, Valid: true}, pgtype.IntervalOID, "29 days"},
		{"interval one day", pgtype.Interval{Days: 1, Valid: true}, pgtype.IntervalOID, "1 day"},
		{"interval mixed", pgtype.Interval{Months: 14, Days: 3, Microseconds: (4*3600 + 5*60 + 6) * 1e6, Valid: true}, pgtype.IntervalOID, "1 year 2 mons 3 days 04:05:06"},
		{"interval zero", pgtype.Interval{Valid: true}, pgtype.IntervalOID, "00:00:00"},
		{"interval negative time", pgtype.Interval{Microseconds: -3600e6, Valid: true}, pgtype.IntervalOID, "-01:00:00"},
		{"time of day", pgtype.Time{Microseconds: (13*3600 + 5) * 1e6, Valid: true}, pgtype.TimeOID, "13:00:05"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalize(tt.v, tt.oid); got.Null || got.Text != tt.want {
				t.Errorf("normalize = %+v, want %q", got, tt.want)
			}
		})
	}
	if c := normalize(nil, pgtype.TextOID); !c.Null || c.String() != NullGlyph {
		t.Errorf("nil should be NULL, got %+v", c)
	}
}

func TestCheckSingleStatement(t *testing.T) {
	ok := []string{
		"select 1",
		"select 1;",
		"select 1 ;  \n",
		"select ';' as semi",
		"select 'it''s; fine'",
		`select 1 as "a;b"`,
		"select 1 -- trailing; comment",
		"select /* a; /* nested; */ still; */ 1",
		"select $$x;y$$",
		"select $tag$ a;b $$ c $tag$",
		`select E'a\';b'`,
		"select * from cd.facilities where name like'%;%'",
		"select $1",
		"with x as (select 1) select * from x;",
		"END_DATE",
	}
	for _, sql := range ok {
		if err := CheckSingleStatement(sql); err != nil {
			t.Errorf("%q rejected: %v", sql, err)
		}
	}
	bad := []string{
		"",
		"  ;  ",
		"-- only a comment",
		"select 1; select 2",
		"select 1; drop table cd.members;",
		"commit",
		"ROLLBACK;",
		"begin",
		"savepoint x",
		"/* c */ end",
	}
	for _, sql := range bad {
		if err := CheckSingleStatement(sql); err == nil {
			t.Errorf("%q accepted", sql)
		}
	}
}
