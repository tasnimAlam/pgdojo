# pgex

PostgreSQL exercises in the terminal, in the spirit of pgexercises.com: pick an
exercise, write SQL, run it against a real local Postgres, and see whether the
result matches the expected one.

Status: milestone 1 (skeleton and exercise data). The TUI comes in later milestones.

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
