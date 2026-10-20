// Command pgdojo is a terminal trainer for PostgreSQL exercises.
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/spf13/cobra"

	"github.com/tasnimAlam/pgdojo/internal/config"
	"github.com/tasnimAlam/pgdojo/internal/db"
	"github.com/tasnimAlam/pgdojo/internal/drafts"
	"github.com/tasnimAlam/pgdojo/internal/exercise"
	"github.com/tasnimAlam/pgdojo/internal/progress"
	"github.com/tasnimAlam/pgdojo/internal/ui"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

// buildVersion is version, or the module version for go install builds,
// which don't get the release ldflags.
func buildVersion() string {
	if version == "dev" {
		if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			return strings.TrimPrefix(bi.Main.Version, "v")
		}
	}
	return version
}

// errQuiet makes main exit 1 without printing; the command already
// reported the problem.
var errQuiet = errors.New("")

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	err := rootCmd().ExecuteContext(ctx)
	if err != nil {
		if !errors.Is(err, errQuiet) {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	var flags config.Flags
	root := &cobra.Command{
		Use:           "pgdojo",
		Short:         "PostgreSQL exercises in your terminal",
		Version:       buildVersion(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&flags.DSN, "dsn", "", "Postgres connection string (overrides PGDOJO_DSN and the config file)")
	root.PersistentFlags().StringVar(&flags.ConfigPath, "config", "", "config file (default "+config.DefaultPath()+")")
	root.PersistentFlags().StringVar(&flags.Accent, "accent", "", "accent color as hex, e.g. #5ccfb8")

	app := &app{flags: &flags}
	root.RunE = func(cmd *cobra.Command, args []string) error { return app.tui(cmd.Context()) }
	root.AddCommand(doctorCmd(app), checkCmd(app), listCmd(app), validateCmd(app))
	return root
}

// app lazily builds the dependencies a command needs.
type app struct {
	flags *config.Flags
	cfg   config.Config
}

func (a *app) config() (config.Config, error) {
	if a.cfg.DSN == "" {
		c, err := config.Load(*a.flags)
		if err != nil {
			return c, err
		}
		a.cfg = c
	}
	return a.cfg, nil
}

func (a *app) catalog() (*exercise.Catalog, error) {
	cat, err := exercise.Load()
	if err != nil {
		return nil, fmt.Errorf("the built-in exercise data is invalid; please report this:\n%w", err)
	}
	return cat, nil
}

func (a *app) runner(ctx context.Context) (*db.Runner, string, error) {
	cfg, err := a.config()
	if err != nil {
		return nil, "", err
	}
	pool, err := db.NewPool(ctx, cfg.DSN)
	if err != nil {
		return nil, "", err
	}
	return db.NewRunner(pool), cfg.DSN, nil
}

func (a *app) progress() (*progress.Store, error) {
	return progress.Open(config.ProgressPath())
}

// tui launches the interactive interface.
func (a *app) tui(ctx context.Context) error {
	cfg, err := a.config()
	if err != nil {
		return err
	}
	cat, err := a.catalog()
	if err != nil {
		return err
	}
	store, err := a.progress()
	if err != nil {
		return err
	}
	r, dsn, err := a.runner(ctx)
	if err != nil {
		return err
	}
	m := ui.New(&ui.Deps{
		Catalog:  cat,
		Checker:  r,
		Progress: store,
		Drafts:   drafts.New(config.DraftsDir()),
		DBLabel:  db.Describe(dsn),
		Editor:   cfg.EditorCommand(),
		Theme:    ui.NewTheme(cfg.Accent),
		Keys:     ui.NewKeyMap(cfg.UseVimKeys()),
	})
	_, err = tea.NewProgram(m, tea.WithContext(ctx)).Run()
	if errors.Is(err, tea.ErrInterrupted) || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// connectErr turns a ping failure into a sentence with a next step.
func connectErr(dsn string, err error) error {
	return fmt.Errorf("could not connect to Postgres at %s (%s).\nStart the database with 'make db-up', or point pgdojo elsewhere with --dsn or PGDOJO_DSN", db.Describe(dsn), rootCause(err))
}

// rootCause shortens pgx's multi-line connect errors to the part that
// matters, e.g. "connection refused" or an authentication failure.
func rootCause(err error) string {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return pg.Message
	}
	var op *net.OpError
	if errors.As(err, &op) {
		return op.Err.Error()
	}
	return err.Error()
}
