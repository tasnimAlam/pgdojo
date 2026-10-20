# pgex

PostgreSQL exercises in the terminal, in the spirit of pgexercises.com: pick an
exercise, write SQL, run it against a real local Postgres, and see whether the
result matches the expected one.

_Screenshots coming soon._
<!-- Add docs/screenshot.png (home and exercise screens at 120×36) and link it here. -->

## Install

**Arch Linux (AUR):**

```fish
paru -S pgex        # or: yay -S pgex
```

The package installs the binary, shell completions for fish, bash and zsh,
and the database setup files in `/usr/share/pgex/db/`.

**Release archive** (linux amd64 / arm64) from the GitHub releases page:

```fish
tar -xzf pgex_*_linux_amd64.tar.gz
install -Dm755 pgex ~/.local/bin/pgex
```

**From source** (Go 1.27+):

```fish
go install github.com/tasnimAlam/pgex/cmd/pgex@latest
```

## Database setup

pgex runs your SQL against a local Postgres 17 container loaded with the
pgexercises country club dataset. You need podman (with podman-compose) or
docker with the compose plugin.

From a clone of this repository:

```fish
sudo pacman -S --needed podman podman-compose
curl -o db/clubdata.sql https://pgexercises.com/dbfiles/clubdata.sql
make db-up
```

After installing the AUR package or a release archive, copy the setup files
somewhere writable first (archives contain the same `db/` folder):

```fish
mkdir -p ~/.local/share/pgex/db
cp /usr/share/pgex/db/* ~/.local/share/pgex/db/
cd ~/.local/share/pgex/db
curl -o clubdata.sql https://pgexercises.com/dbfiles/clubdata.sql
podman-compose up -d     # or: docker compose up -d
```

Then check everything with `pgex doctor` and start with `pgex`.

## Fish setup

Add to `~/.config/fish/config.fish` as needed:

```fish
# Only needed if your database is not the default localhost:5433 container.
set -gx PGEX_DSN postgres://pgex_player:pgex@localhost:5433/exercises

# Editor used by `e` in an exercise (falls back to $EDITOR, then vi).
set -gx EDITOR nvim      # or: set -gx EDITOR "emacsclient -t"

abbr -a px pgex
```

Completions are installed by the AUR package. Otherwise generate them once:

```fish
pgex completion fish > ~/.config/fish/completions/pgex.fish
```

## Development

Requires Go (latest stable). `make build`, `make test`, `make lint`,
`make run`. Releases are built with GoReleaser from a `v*` tag:

```fish
git tag v0.1.0
goreleaser release --clean
```

The AUR files live in `packaging/aur/`. After tagging, run `updpkgsums` and
`makepkg --printsrcinfo > .SRCINFO` there before pushing to the AUR.

## Commands

Run `pgex` with no arguments for the interactive interface. Press `?` for
keyboard shortcuts, `/` to filter a list, `c` on the home screen to continue
the last exercise. With `vim_keys = true` (the default) lists also take
`j`/`k` to move, `l` to open, `h` to go back, `g`/`G` for top and bottom, and
`ctrl+u`/`ctrl+d` to page; `g`/`G`/`ctrl+u`/`ctrl+d` also scroll a focused
result table.

An exercise opens in NORMAL mode with the query focused. Press `i` to type
(INSERT mode) and `esc` to return to NORMAL; only `i` enters INSERT, so
`tab` back into the query keeps NORMAL. `ctrl+r` runs the query in either
mode. In NORMAL mode single keys work: `backspace` back to the list, `h` hint, `a`
reveal answer, `v` cycle yours / expected / diff, `d` discussion, `s` schema
window (tables, keys and how they relate),
`n`/`p` next/previous, `e` edit in `$EDITOR`.
`ctrl+c` cancels a running query. Below 110 columns the panes stack
vertically and `h` opens hints as an overlay.

On the home screen `R` resets all progress after you type `reset` (drafts are
kept).

## Config

`~/.config/pgex/config.toml`:

```toml
dsn = "postgres://pgex_player:pgex@localhost:5433/exercises"
editor = "nvim"          # overrides $EDITOR; may include args, e.g. "emacsclient -t"
accent = "#5ccfb8"       # hex color, also settable with --accent
vim_keys = true          # h/j/k/l, g/G, ctrl+u/d in lists
```

```fish
pgex list                                   # categories and exercises with your progress
pgex check joins/simplejoin q.sql             # run a file, print pass/fail and a diff; exit 0 on pass
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

The 71 exercises are adapted from [pgexercises](https://github.com/AlisdairO/pgexercises)
by Alisdair Owens and are licensed under
[CC BY-SA 3.0](https://creativecommons.org/licenses/by-sa/3.0/) (see
`internal/exercise/data/LICENSE-exercises`). They were converted from the
original HTML question files to YAML and Markdown, and modifying exercises
gained `verify` queries. The rest of pgex is separate from that licence.

Exercise data lives in `internal/exercise/data/*.yaml`, one file per category,
and is embedded in the binary. The loader validates ids, required fields, and
`verify` queries for modifying exercises; `pgex validate` runs every reference
answer against the database.

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
