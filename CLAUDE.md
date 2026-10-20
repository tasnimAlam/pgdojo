# pgex: PostgreSQL exercises in the terminal

Implementation spec for Claude Code. Build a Go TUI that works like pgexercises.com: the user picks an exercise, writes SQL, runs it against a real local Postgres, and sees whether the result matches the expected one.

Read this whole file before writing code. Work **one milestone at a time** (section 12). At the end of each milestone, run the tests, commit, and stop so I can review before you continue.

---

## 1. Environment

- OS: Arch Linux, Hyprland, terminal emulator with truecolor. My shell is **fish**, so any shell snippets you give me for my own use must be fish-compatible (`set -x VAR value`, not `export VAR=value`). Scripts inside the repo (Makefile, `scripts/*.sh`) can use bash with a shebang.
- Go: latest stable from `pacman -S go`. Pin all module versions in `go.mod`.
- Postgres: run in a container (`podman` or `docker`; support both via `compose.yaml`). Don't assume a host Postgres install.
- Editor: `$EDITOR` is set (it may be `emacsclient -t` or `nvim`). Opening an external editor must work with terminal editors.

## 2. Product summary

| Concept | Meaning |
|---|---|
| Category | A group of exercises (Basics, Joins and Subqueries, Modifying Data, Aggregation, Working with Timestamps, String Operations, Recursive Queries) |
| Exercise | A question, a hint list, a reference answer query, and comparison options |
| Attempt | Running the user's SQL and comparing it with the reference answer's result |
| Progress | Per exercise: `new`, `attempted`, `solved`, or `revealed` (answer shown before solving) |

Dataset: the "country club" schema `cd` with tables `cd.members`, `cd.facilities`, `cd.bookings`.

## 3. Stack

| Concern | Library |
|---|---|
| TUI | `github.com/charmbracelet/bubbletea` (latest stable major; check its README for the current import path and API before writing code) |
| Widgets | `github.com/charmbracelet/bubbles` (list, textarea, table, viewport, help, key, spinner, textinput) |
| Styling | `github.com/charmbracelet/lipgloss` |
| Markdown | `github.com/charmbracelet/glamour` (discussion text) |
| SQL highlighting | `github.com/alecthomas/chroma/v2` |
| Postgres | `github.com/jackc/pgx/v5` with `pgxpool` |
| CLI | `github.com/spf13/cobra` |
| YAML | `gopkg.in/yaml.v3` |
| Config | TOML via `github.com/BurntSushi/toml` |
| Tests | stdlib `testing`, `github.com/charmbracelet/x/exp/teatest` for UI, `github.com/testcontainers/testcontainers-go/modules/postgres` for DB integration tests |

No other dependencies without asking me first.

## 4. Repository layout

```
pgex/
├── cmd/pgex/main.go
├── internal/
│   ├── config/config.go          # load/merge config file + flags + env
│   ├── exercise/
│   │   ├── model.go              # Category, Exercise structs
│   │   ├── loader.go             # go:embed data/*.yaml, validate
│   │   ├── loader_test.go
│   │   └── data/
│   │       ├── 01-basics.yaml
│   │       ├── 02-joins.yaml
│   │       └── ...
│   ├── db/
│   │   ├── pool.go               # pgxpool, AfterConnect: statement_timeout, search_path
│   │   ├── runner.go             # Run, Check (transactions, savepoints)
│   │   ├── result.go             # Result, value normalization
│   │   ├── compare.go            # Compare -> Diff
│   │   ├── compare_test.go
│   │   └── runner_test.go        # integration, testcontainers
│   ├── progress/
│   │   ├── store.go              # JSON file, atomic writes
│   │   └── store_test.go
│   ├── drafts/drafts.go          # per-exercise saved SQL
│   └── ui/
│       ├── app.go                # root model, screen router, global keys
│       ├── keys.go               # all key.Binding definitions
│       ├── theme.go              # lipgloss styles from section 8
│       ├── msgs.go               # shared tea.Msg types
│       ├── categories.go         # screen 1
│       ├── exercises.go          # screen 2
│       ├── exerciseview.go       # screens 3 and 4
│       ├── editor.go             # textarea wrapper + $EDITOR via tea.ExecProcess
│       ├── resultview.go         # result table + diff rendering
│       ├── help.go               # screen 5 overlay
│       └── *_test.go
├── db/
│   ├── compose.yaml
│   └── clubdata.sql              # I will provide this file (see 6.1)
├── scripts/
├── Makefile
├── .goreleaser.yaml              # milestone 7
└── README.md
```

## 5. Exercise data

### 5.1 YAML format

One file per category. Files are embedded with `//go:embed data/*.yaml` and sorted by filename.

```yaml
id: joins
title: Joins and Subqueries
description: Combine rows from several tables with joins and subqueries.
exercises:
  - id: member-booking-starttimes        # unique within category, kebab-case
    title: Start times of a member's bookings
    question: |
      Produce a list of the start times for bookings made by the member
      named David Farrell.
    tables: [cd.bookings, cd.members]     # shown in the preview pane
    ordered: false                        # true => row order must match
    mutates: false                        # true => INSERT/UPDATE/DELETE exercise
    verify: ""                            # required when mutates: true
    hints:
      - Join bookings to members on memid.
      - Filter on both firstname and surname.
    answer: |
      select bks.starttime
        from cd.bookings bks
        join cd.members mems on mems.memid = bks.memid
       where mems.firstname = 'David' and mems.surname = 'Farrell';
    discussion: |
      Markdown text explaining the solution.
```

### 5.2 Validation (loader returns an error listing every problem)

- `id` unique per category; category ids unique.
- `question`, `answer` non-empty; at least one hint.
- `mutates: true` requires a non-empty `verify` query.
- `ordered: true` should have `order by` in `answer` (warn, don't fail).

### 5.3 Content

- **Write original question text yourself.** Do not copy question, hint, or discussion text from pgexercises.com. Questions only need to make sense against the `cd` schema.
- Milestone 1 ships 3 exercises per category (21 total), covering a spread of difficulty. More can be added later by editing YAML only.
- At least one exercise in Modifying Data must use `mutates: true` with `verify`.

## 6. Database layer

### 6.1 Provisioning

`db/compose.yaml`:

```yaml
services:
  pg:
    image: docker.io/library/postgres:17
    environment:
      POSTGRES_USER: pgex
      POSTGRES_PASSWORD: pgex
      POSTGRES_DB: exercises
    ports: ["5433:5432"]
    volumes:
      - ./clubdata.sql:/docker-entrypoint-initdb.d/01-clubdata.sql:ro
      - ./roles.sql:/docker-entrypoint-initdb.d/02-roles.sql:ro
```

- I will download `clubdata.sql` from pgexercises.com myself and place it in `db/`. If it's missing, the Makefile target `db-up` must fail with a clear message, not a container error.
- `db/roles.sql`: creates role `pgex_player` with login, `USAGE` on schema `cd`, `SELECT, INSERT, UPDATE, DELETE` on all tables in `cd`, and nothing else. The app connects as this role by default.

### 6.2 Connection

- Default DSN: `postgres://pgex_player:pgex@localhost:5433/exercises`.
- Override order: `--dsn` flag > `PGEX_DSN` env > config file.
- `AfterConnect`: `SET statement_timeout = '5s'`, `SET search_path = cd, public`.
- Pool max 4 connections.

### 6.3 Running a check

`func (r *Runner) Check(ctx context.Context, ex exercise.Exercise, userSQL string) (Check, error)`

All work happens in **one transaction that is always rolled back**:

```
BEGIN
  if !mutates:
    got  := query(userSQL)
    want := query(ex.Answer)
  if mutates:
    SAVEPOINT s
    exec(userSQL);    got  := query(ex.Verify)
    ROLLBACK TO SAVEPOINT s
    exec(ex.Answer);  want := query(ex.Verify)
ROLLBACK
```

Rules:

- Reject multiple statements in `userSQL` (pgx simple protocol would allow them). Detect with a cheap check (strip comments and string literals, count `;` not at the end) and return a friendly error.
- A user error (syntax error, timeout) is not a Go error from `Check`; return `Check{UserErr: pgErr}` so the UI can show it in the result pane with the Postgres message, `DETAIL`, `HINT`, and error position if present.
- Errors in the reference answer *are* Go errors (bad exercise data).
- Context cancellation (user pressed `ctrl+c` during a run) must cancel the query.

### 6.4 Result and normalization

```go
type Result struct {
    Columns  []string
    Rows     [][]Cell
    Duration time.Duration
}
type Cell struct {
    Text string // display and comparison form
    Null bool
}
```

Normalize with `rows.Values()`:

| Postgres type | Text form |
|---|---|
| NULL | `Null: true`, displayed `∅` |
| timestamp | `2006-01-02 15:04:05` |
| timestamptz | convert to UTC, same layout + ` UTC` |
| date | `2006-01-02` |
| numeric | canonical decimal string, trailing zeros trimmed (`5.50` → `5.5`, `5.00` → `5`) |
| float | `strconv.FormatFloat(v, 'f', -1, 64)` |
| interval | Postgres text output |
| bool | `true` / `false` |
| everything else | `fmt.Sprint` |

### 6.5 Comparison

`func Compare(want, got Result, ordered bool) Diff`

```go
type Diff struct {
    Equal         bool
    ColumnCount   struct{ Want, Got int }
    NameMismatch  []int     // column indexes whose names differ (warning only)
    Missing       [][]Cell  // in want, not in got (multiset)
    Extra         [][]Cell  // in got, not in want (multiset)
    FirstOrderErr int       // ordered only: first row index that differs, -1 if none
}
```

- Columns are compared **positionally**; different names are a warning, not a failure.
- Different column counts: not equal; skip row diffing.
- Unordered: multiset comparison (duplicates count).
- Ordered: equal only if rows match in sequence; still fill Missing/Extra for display.
- Unit tests must cover: NULLs, duplicates, numeric trailing zeros, timestamps, order sensitivity, column count mismatch, empty results.

## 7. Persistence

Follow XDG:

| What | Path |
|---|---|
| Config | `$XDG_CONFIG_HOME/pgex/config.toml` (default `~/.config/pgex/config.toml`) |
| Progress | `$XDG_DATA_HOME/pgex/progress.json` |
| Drafts | `$XDG_DATA_HOME/pgex/drafts/<category>/<exercise>.sql` |

- Progress JSON: `{ "version": 1, "last": {"category": "...", "exercise": "..."}, "exercises": { "joins/member-booking-starttimes": {"status": "solved", "attempts": 3, "hints_used": 1, "updated_at": "..."} } }`.
- Write atomically (temp file + rename).
- Status transitions: `new → attempted` on first run; `→ solved` on first pass; `→ revealed` if the answer is revealed before solving. `solved` never downgrades. `revealed` can become `solved` only by passing later, and then it stays `revealed` in a separate `revealed: true` flag so stats are honest.
- Drafts are saved on every run and when leaving the exercise view.

Config keys: `dsn`, `editor` (overrides `$EDITOR`), `accent` (hex), `vim_keys` (bool, default true for `j/k`).

## 8. Visual design

Mockups exist; this section is the source of truth. Use lipgloss `AdaptiveColor` only where noted; the design is dark-first.

### 8.1 Colors

| Token | Hex | Use |
|---|---|---|
| `bg` | `#111318` | do not paint the terminal background; only use for text on accent |
| `fg` | `#d4d8e0` | body text |
| `fgBright` | `#ffffff` | selected item text, titles |
| `dim` | `#8b93a4` | labels, secondary text, help line |
| `faint` | `#3a404c` | empty progress segments, "new" marker |
| `lineNo` | `#4a5160` | editor line numbers |
| `border` | `#2c313c` | unfocused pane borders, separators |
| `accent` | `#5ccfb8` | focused pane border, app badge, pass, solved, progress fill (configurable) |
| `selBg` | `#1b2a28` | selected list row background |
| `warn` | `#e8b04a` | attempted marker, hint pane border, string literals, highlighted keys |
| `fail` | `#f07a5a` | failure banner text, missing-row count |
| `failBg` | `#3a2320` | failure banner and missing-row background |
| `failRow` | `#f4a28c` | missing-row text |
| `passBg` | `#17322d` | success banner background |
| `keyword` | `#7aa2f7` | SQL keywords |

### 8.2 Common chrome (every screen)

- **Header row:** left: badge ` pgex ` (accent background, `bg` text, bold), then a breadcrumb in `dim` with the current item in bold `fg`. Right: context info (connection status on home, `N / M solved` on lists, `i / n` position in the exercise view).
- **Panes:** `lipgloss.RoundedBorder()`, 1 cell horizontal padding. Focused pane: accent border and accent title. Unfocused: `border` color and `dim` title.
- **Footer:** a top border line in `border`, then a help line: keys in `fg`, descriptions in `dim`, separated by ` · `. Right side: a short status (percentage on home, `solved`/`attempted` badge in the exercise view).
- **Status markers:** `✓` solved (accent), `●` attempted (warn), `○` new (faint), `◐` revealed (dim).

### 8.3 Screen 1: Categories (home)

```
 pgex  PostgreSQL exercises in your terminal           db localhost:5433/exercises ● connected
╭ Categories ───────────────────────────────╮ ╭ Basics ──────────────────────────────╮
│ 7 categories · 32 of 83 solved            │ │ Retrieve rows from a single table... │
│▌▸ Basics                ██████████  10/10 │ │ solved 10/10     hints used 2        │
│   Joins and Subqueries  ████░░░░░░   4/11 │ │ answers 0 rev.   last run 2 days ago │
│   Modifying Data        ░░░░░░░░░░    0/8 │ │ Tables used: [cd.facilities] ...     │
│   ...                                     │ ╰──────────────────────────────────────╯
│                                           │ ╭ Continue where you left off ─────────╮
│                                           │ │ → Joins › 5. Members who used ...    │
╰───────────────────────────────────────────╯ ╰──────────────────────────────────────╯
 ↑/↓ move · enter open · c continue · / filter · ? help · q quit                    39%
```

- Left pane ~45% width (min 50 cols); right column is the rest, split into a detail pane (grows) and a "continue" pane (fixed height). Hide the continue pane if there is no `last`.
- Progress bar: 10 cells, `█` accent for the solved fraction, `█` faint for the rest.
- Connection indicator: `● connected` in accent, `● offline` in fail. Ping the DB on startup and every 30s via `tea.Tick`. If offline, the app still works for browsing; running a query shows a clear error with the fix (`make db-up`).

### 8.4 Screen 2: Exercise list

- Breadcrumb `Categories › <Category>`; right side `N / M solved`.
- Left pane (~540px ≈ 45%): `bubbles/list` with a custom delegate: marker, number, title. Filter with `/` shows `/ query▌` at the top and `N matches` on the right; matched substrings underlined.
- Right pane: preview: title (bold, bright), question text, expected column chips (run the reference answer once, cache column names per exercise for the session), "order matters yes/no", tables, attempt count and last outcome, and the draft file path in the bottom area.
- Legend at the bottom of the list pane: `✓ solved ● attempted ○ new`.

### 8.5 Screens 3 and 4: Exercise view

Layout (wide, ≥ 110 cols):

```
header
┌ left column, 420px ≈ 40 cols ┐  ┌ right column, rest ──────────────────────┐
│ Question                      │  │ Query (editor)  [mode · run key · $EDITOR] │
│                               │  │  1 select ...                            │
├───────────────────────────────┤  ├──────────────────────────────────────────┤
│ Hint N of M  (only if shown)  │  │ Result  [rows · duration] or [diff view]  │
├───────────────────────────────┤  │ banner: ✓ Correct / ✗ Not quite / error   │
│ Schema (s toggles) OR         │  │ table                                    │
│ Diff summary (after a fail)   │  │ footer hint text                         │
└───────────────────────────────┘  └──────────────────────────────────────────┘
footer
```

- Narrow terminals (< 110 cols): stack vertically: question, editor, result. Schema and hint become toggled overlays.
- **Editor:** `bubbles/textarea` with line numbers in `lineNo`. Show chroma-highlighted SQL when the editor is *not* focused; show the plain textarea while editing (acceptable trade-off; don't try to highlight inside the textarea). Pane title shows mode (`INSERT` when focused, `NORMAL` otherwise) and `attempt N`.
- **Run (`ctrl+r` / `F5`):** show a spinner in the result pane title; run `Check` as a `tea.Cmd`; on completion:
  - Pass: `passBg` banner `✓ Correct: all N rows match the expected result`, the table, and `Press n for the next exercise or d to read the discussion.`
  - Fail: `failBg` banner explaining the main reason in plain words (column count mismatch / N rows missing / N extra rows / right rows, wrong order at row K). Left column's lower pane switches to **Diff summary** (expected rows, got rows, `− missing`, `+ extra`, columns `x / y ✓/✗`).
  - User SQL error: `failBg` banner with the Postgres message; below it `DETAIL` / `HINT`; and, if a position is given, the offending line from the query with a `^` under the column.
- **Diff view (`v`):** cycles `yours → expected → diff`. Diff mode merges both: rows present in both in normal color with a blank gutter; missing rows with `−` gutter, `failBg` background, `failRow` text; extra rows with `+` gutter in `warn`. Show at most what fits; last line `… N more` in dim.
- **Result table:** `bubbles/table` or a custom renderer; column widths from content, capped at 30 cells with `…` truncation; horizontal scroll with `←/→` when the result pane is focused; NULL shown as `∅` in dim.
- **Hint (`h`):** reveals the next hint in a `warn`-bordered pane titled `Hint i of n`, with `h for the next hint · a reveals the answer`. Count hints used in progress.
- **Answer (`a`):** confirmation prompt (`Reveal the answer? This marks the exercise as revealed. y/n`). Then show the answer, syntax highlighted, in the result area, with `y` to copy it into the editor.
- **Discussion (`d`):** full-screen glamour-rendered viewport; `esc` closes.
- **`e`:** write the editor content to the draft file, run `$EDITOR <file>` via `tea.ExecProcess`, reload the file into the textarea when it returns.
- **`n` / `p`:** next/previous exercise in the category; at the end, go to the next category's first exercise.
- Footer right side shows the exercise status badge.

### 8.6 Screen 5: Help overlay

- `?` from any screen. Center a modal (~70% width) with an accent rounded border over the current screen; everything behind it rendered in `faint` (re-render the underlying view with a dimmed style, or render blank panes with faint borders; either is fine).
- Title `Keyboard shortcuts` (accent, bold), right: `esc or ? to close`.
- Three columns: **Navigation**, **Query**, **Learning**; keys in `warn`. Generate the content from the `key.Binding` help text in `keys.go` so it can't drift.
- Bottom note: `Revealing the answer asks for confirmation and marks the exercise as revealed, not solved.`

## 9. Key bindings

| Key | Where | Action |
|---|---|---|
| `↑/↓`, `j/k` | lists | move |
| `enter` | lists | open |
| `esc` | anywhere | back / close overlay / clear filter |
| `backspace` | exercise list (when filter empty) | back |
| `/` | lists | filter |
| `c` | home | continue last exercise |
| `ctrl+r`, `F5` | exercise | run |
| `ctrl+c` | exercise, while running | cancel the running query |
| `ctrl+c` | otherwise | quit |
| `e` | exercise (editor not focused) | open `$EDITOR` |
| `tab` / `shift+tab` | exercise | cycle focus: editor → result → left column |
| `v` | exercise | cycle diff view |
| `s` | exercise | toggle schema |
| `h` | exercise | next hint |
| `a` | exercise | reveal answer (confirm) |
| `d` | exercise | discussion |
| `n` / `p` | exercise | next / previous |
| `ctrl+l` | editor | clear editor (confirm if non-empty) |
| `R` | home | reset all progress (confirm by typing `reset`) |
| `?` | anywhere except editor insert | help overlay |
| `q` | anywhere except editor | quit |

When the editor is focused, single-letter bindings go to the textarea; only `ctrl+…`, `F5`, `esc` (unfocus editor), and `tab` are handled by the view.

## 10. Architecture rules

- Root model in `ui/app.go` holds: current screen enum, a stack for back navigation, window size, shared deps (`*db.Runner`, `*progress.Store`, `*drafts.Store`, catalog, theme, keys), and the help overlay flag.
- Each screen is its own type with `Init`, `Update(msg) (Screen, tea.Cmd)`, `View() string`, `SetSize(w, h int)`.
- Navigation via messages: `openCategoryMsg`, `openExerciseMsg`, `backMsg`. Screens never reach into each other.
- **No blocking I/O inside `Update`.** DB, file writes of progress, and pings are `tea.Cmd`s returning messages.
- `internal/exercise`, `internal/db`, `internal/progress`, `internal/drafts` must not import Bubble Tea or lipgloss.
- Handle `tea.WindowSizeMsg` in the root and propagate. Every screen must render without panics at 80×24 and look right at 120×36 and above.
- All styles live in `theme.go`; no inline colors elsewhere.
- Errors shown to the user are plain sentences with a next step, never a raw Go error alone.

## 11. CLI

```
pgex                          # launch TUI
pgex doctor                   # check DSN, connectivity, schema cd, role permissions, $EDITOR
pgex check <cat>/<ex> <file>  # non-interactive: run file against exercise, print pass/fail + diff, exit 0/1
pgex list                     # print categories/exercises with status
pgex validate                 # load all YAML, run every reference answer against the DB, report failures
```

Global flags: `--dsn`, `--config`, `--accent`.

## 12. Milestones

Each milestone: implement, `go vet ./...`, `go test ./...`, update README, commit with a conventional message, then **stop and summarize** what was done, what was decided, and anything I need to do.

1. **Skeleton + data.** `go mod init`, layout, Makefile (`build`, `test`, `lint`, `db-up`, `db-down`, `db-reset`, `run`), `compose.yaml`, `roles.sql`, exercise model, loader with validation, 21 original exercises, loader tests.
   *Done when:* `make test` passes; `make db-up` fails clearly without `clubdata.sql`.
2. **DB layer.** pool, runner with transaction/savepoint logic, multi-statement rejection, normalization, comparator, unit tests for compare, integration tests with testcontainers (skip with a message if no container runtime).
   *Done when:* all reference answers pass their own comparison in a test.
3. **CLI.** cobra commands `doctor`, `check`, `list`, `validate`; progress and drafts stores with tests.
   *Done when:* `pgex check joins/member-booking-starttimes q.sql` prints a readable pass/fail with diff.
4. **TUI navigation.** root model, theme, keys, home screen, exercise list with filter and preview, help overlay, resize handling.
   *Done when:* screens 1, 2, 5 match section 8; teatest covers navigation and filter.
5. **Exercise view.** editor, run/cancel, result table, pass/fail/error banners, diff view, hints, answer reveal, discussion, next/prev, drafts, progress updates.
   *Done when:* screens 3 and 4 match section 8; teatest covers pass, fail, user error, and reveal flows using a fake runner interface.
6. **Polish.** `$EDITOR` integration, narrow layout, connection ping, config file, accent override, `R` reset, empty and error states everywhere.
7. **Release.** `.goreleaser.yaml` (linux amd64/arm64), `PKGBUILD` for AUR in `packaging/aur/`, README with install, fish-shell setup snippet, screenshots placeholder.

## 13. Testing requirements

- Comparator: table-driven, ≥ 15 cases.
- Runner: the `UI` depends on an interface (`type Checker interface { Check(ctx, ex, sql) (db.Check, error) }`) so UI tests use a fake.
- A test that loads every exercise and runs its reference answer against the test DB, asserting `Compare(want, want)` is equal and (for mutating exercises) that the DB is unchanged after `Check`.
- teatest golden files for each screen at 120×36; update them with `-update`.
- `go test -race ./...` in `make test`.

## 14. Out of scope (for now)

- Remote multiplayer, accounts, or syncing.
- Writing to any schema other than `cd`.
- In-editor SQL autocompletion (maybe later).
- Light theme (structure `theme.go` so it can be added).

## 15. Quick start (for me, fish shell)

```fish
sudo pacman -S --needed go podman podman-compose
cd ~/code/pgex
# put clubdata.sql in db/
make db-up
make run
```
