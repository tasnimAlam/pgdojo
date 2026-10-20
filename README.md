# pgex

PostgreSQL exercises in the terminal, in the spirit of pgexercises.com: pick an
exercise, write SQL, run it against a real local Postgres, and see whether the
result matches the expected one.

Status: milestone 5 (exercise view). Run `pgex`, open an exercise, write SQL and press `ctrl+r`.

## Requirements

- Go (latest stable)
- podman (with podman-compose) or docker with the compose plugin

## Quick start (fish)

```fish
sudo pacman -S --needed go podman podman-compose
# Download the country club dataset from pgexercises.com and save it as db/clubdata.sql
make db-up
make run
```

## Commands

Run `pgex` with no arguments for the interactive interface. Press `?` for
keyboard shortcuts, `/` to filter a list, `c` on the home screen to continue
the last exercise.

In an exercise the editor starts in INSERT mode. `ctrl+r` (or `F5`) runs the
query, `esc` switches to NORMAL mode where single keys work: `h` hint, `a`
reveal answer, `v` cycle yours / expected / diff, `d` discussion, `s` schema,
`n`/`p` next/previous, `e` edit in `$EDITOR`, `enter` back to editing.
`ctrl+c` cancels a running query.

```fish
pgex list                                   # categories and exercises with your progress
pgex check joins/member-booking-starttimes q.sql   # run a file, print pass/fail and a diff; exit 0 on pass
pgex doctor                                 # check DSN, connection, schema cd, role permissions, $EDITOR
pgex validate                               # run every reference answer against the database
```

Global flags: `--dsn`, `--config`, `--accent`. The DSN is taken from `--dsn`,
then `PGEX_DSN`, then the config file, then the default
`postgres://pgex_player:pgex@localhost:5433/exercises`.

```fish
set -x PGEX_DSN postgres://pgex_player:pgex@localhost:5433/exercises
```

## Files

| What | Path |
|---|---|
| Config | `~/.config/pgex/config.toml` (keys: `dsn`, `editor`, `accent`, `vim_keys`) |
| Progress | `~/.local/share/pgex/progress.json` |
| Drafts | `~/.local/share/pgex/drafts/<category>/<exercise>.sql` |

`XDG_CONFIG_HOME` and `XDG_DATA_HOME` are respected. `pgex check` records an
attempt in the progress file.

## Make targets

| Target | What it does |
|---|---|
| `build` | Build `bin/pgex` |
| `test` | `go test -race ./...` |
| `lint` | `go vet` and a gofmt check |
| `run` | Build and run |
| `db-up` | Start Postgres 17 on `localhost:5433` (fails clearly if `db/clubdata.sql` is missing) |
| `db-down` | Stop the container |
| `db-reset` | Drop the volume and start fresh |

The app connects as `pgex_player`, a role that can only read and modify rows in
schema `cd` (see `db/roles.sql`).

## Exercises

Exercise data lives in `internal/exercise/data/*.yaml`, one file per category,
and is embedded in the binary. Add exercises by editing YAML; the loader
validates ids, required fields, and `verify` queries for mutating exercises.

## How checking works

Your query and the reference answer run in one transaction that is always
rolled back, so nothing you run changes the data. Mutating exercises run your
statement, read the result through the exercise's `verify` query, roll back to
a savepoint, then do the same with the reference answer. Results are compared
by column position; row order matters only when the exercise says so.

Only one statement is allowed per run, and transaction commands (`BEGIN`,
`COMMIT`, ...) are rejected. Queries time out after 5 seconds.

## Tests

`make test` runs unit tests and, when docker or podman is available and
`db/clubdata.sql` exists, integration tests in a throwaway Postgres container
(via testcontainers). Every reference answer is checked against itself there.
Use `go test -short ./...` to skip the container.

UI screens are checked against golden files in `internal/ui/testdata` rendered
at 120×36. After an intended visual change, regenerate them with:

```fish
go test ./internal/ui -run TestGoldenScreens -update
```
