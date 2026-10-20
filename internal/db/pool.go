// Package db runs user and reference queries against Postgres and compares
// their results.
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DefaultDSN points at the container from db/compose.yaml.
const DefaultDSN = "postgres://pgex_player:pgex@localhost:5433/exercises"

// NewPool opens a small pool whose connections have a statement timeout and
// search_path = cd, public. It does not connect eagerly.
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid DSN %q: %w", dsn, err)
	}
	cfg.MaxConns = 4
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET statement_timeout = '5s'; SET search_path = cd, public")
		return err
	}
	return pgxpool.NewWithConfig(ctx, cfg)
}

// Describe returns "host:port/database" for display, without credentials.
func Describe(dsn string) string {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return "invalid DSN"
	}
	return fmt.Sprintf("%s:%d/%s", cfg.Host, cfg.Port, cfg.Database)
}
