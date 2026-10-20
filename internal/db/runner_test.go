package db

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/tasnimAlam/pgdojo/internal/exercise"
)

var (
	dbOnce   sync.Once
	dbRunner *Runner
	dbErr    error
	dbCtr    *postgres.PostgresContainer
)

func TestMain(m *testing.M) {
	code := m.Run()
	if dbCtr != nil {
		_ = testcontainers.TerminateContainer(dbCtr)
	}
	os.Exit(code)
}

// testRunner starts one Postgres container for the whole package, loaded
// with db/clubdata.sql and db/roles.sql, and connects as pgex_player.
func testRunner(t *testing.T) *Runner {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	clubdata, _ := filepath.Abs("../../db/clubdata.sql")
	roles, _ := filepath.Abs("../../db/roles.sql")
	if _, err := os.Stat(clubdata); err != nil {
		t.Skip("skipping integration test: db/clubdata.sql is missing (download it from pgexercises.com)")
	}
	testcontainers.SkipIfProviderIsNotHealthy(t)

	dbOnce.Do(func() {
		ctx := context.Background()
		dbCtr, dbErr = postgres.Run(ctx, "docker.io/library/postgres:17",
			postgres.WithDatabase("postgres"),
			postgres.WithUsername("pgex"),
			postgres.WithPassword("pgex"),
			postgres.WithOrderedInitScripts(clubdata, roles),
			postgres.BasicWaitStrategies(),
		)
		if dbErr != nil {
			return
		}
		host, err := dbCtr.Host(ctx)
		if err != nil {
			dbErr = err
			return
		}
		port, err := dbCtr.MappedPort(ctx, "5432/tcp")
		if err != nil {
			dbErr = err
			return
		}
		dsn := fmt.Sprintf("postgres://pgex_player:pgex@%s:%s/exercises", host, port.Port())
		pool, err := NewPool(ctx, dsn)
		if err != nil {
			dbErr = err
			return
		}
		dbRunner = NewRunner(pool)
	})
	if dbErr != nil {
		t.Fatalf("start test database: %v", dbErr)
	}
	return dbRunner
}

// TestReferenceAnswers runs every exercise's answer through Check and
// expects it to pass against itself without changing the database.
func TestReferenceAnswers(t *testing.T) {
	r := testRunner(t)
	cat, err := exercise.Load()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, c := range cat.Categories {
		for _, ex := range c.Exercises {
			t.Run(ex.Key(), func(t *testing.T) {
				var before Result
				if ex.Mutates {
					if before, err = r.Run(ctx, ex.Verify); err != nil {
						t.Fatal(err)
					}
				}
				chk, err := r.Check(ctx, ex, ex.Answer)
				if err != nil {
					t.Fatal(err)
				}
				if chk.UserErr != nil {
					t.Fatalf("answer failed as user SQL: %v", chk.UserErr)
				}
				if !chk.Passed() {
					t.Fatalf("answer does not match itself: %+v", chk.Diff)
				}
				if len(chk.Want.Rows) == 0 && !ex.Mutates {
					t.Error("reference answer returns no rows")
				}
				if !Compare(chk.Want, chk.Want, ex.Ordered).Equal {
					t.Error("Compare(want, want) not equal")
				}
				if ex.Mutates {
					after, err := r.Run(ctx, ex.Verify)
					if err != nil {
						t.Fatal(err)
					}
					if !Compare(before, after, true).Equal {
						t.Error("database changed after Check")
					}
					if Compare(before, chk.Want, true).Equal {
						t.Error("reference answer did not change the verify result")
					}
				}
			})
		}
	}
}

func TestCheckOutcomes(t *testing.T) {
	r := testRunner(t)
	ctx := context.Background()
	ex := exercise.Exercise{
		ID: "t", Category: "test", Ordered: true,
		Answer: "select facid, name from cd.facilities order by facid",
	}

	t.Run("wrong order", func(t *testing.T) {
		chk, err := r.Check(ctx, ex, "select facid, name from facilities order by facid desc")
		if err != nil {
			t.Fatal(err)
		}
		if chk.Passed() || chk.Diff.FirstOrderErr != 0 || len(chk.Diff.Missing) != 0 {
			t.Errorf("want order error at row 0, got %+v", chk.Diff)
		}
	})

	t.Run("syntax error has position", func(t *testing.T) {
		chk, err := r.Check(ctx, ex, "select facid, nme from cd.facilities")
		if err != nil {
			t.Fatal(err)
		}
		if chk.UserErr == nil || chk.UserErr.Position == 0 || chk.UserErr.Code != "42703" {
			t.Fatalf("want undefined column error with position, got %+v", chk.UserErr)
		}
		if len(chk.Want.Rows) == 0 {
			t.Error("Want should still be filled after a user error")
		}
	})

	t.Run("multiple statements rejected", func(t *testing.T) {
		chk, err := r.Check(ctx, ex, "select 1; delete from cd.bookings")
		if err != nil || chk.UserErr == nil {
			t.Fatalf("want user error, got %v / %v", chk.UserErr, err)
		}
	})

	t.Run("timeout is a user error", func(t *testing.T) {
		chk, err := r.Check(ctx, ex, "select pg_sleep(10)")
		if err != nil {
			t.Fatal(err)
		}
		if chk.UserErr == nil || chk.UserErr.Code != "57014" {
			t.Fatalf("want statement timeout, got %+v", chk.UserErr)
		}
	})

	t.Run("cancel stops the query", func(t *testing.T) {
		cctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		defer cancel()
		start := time.Now()
		_, err := r.Check(cctx, ex, "select pg_sleep(4)")
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("want context error, got %v", err)
		}
		if time.Since(start) > 2*time.Second {
			t.Error("query was not cancelled promptly")
		}
	})

	t.Run("player cannot create tables", func(t *testing.T) {
		mut := exercise.Exercise{ID: "m", Category: "test", Mutates: true,
			Answer: "select 1", Verify: "select 1"}
		chk, err := r.Check(ctx, mut, "create table cd.evil(x int)")
		if err != nil {
			t.Fatal(err)
		}
		if chk.UserErr == nil || chk.UserErr.Code != "42501" {
			t.Fatalf("want permission denied, got %+v", chk.UserErr)
		}
	})
}
