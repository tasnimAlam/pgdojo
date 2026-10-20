// Package config loads pgdojo settings from the config file, environment and
// command-line flags, and resolves XDG paths.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	"github.com/BurntSushi/toml"

	"github.com/tasnimAlam/pgdojo/internal/db"
)

// Config holds user settings. Zero values mean "use the default".
type Config struct {
	DSN     string `toml:"dsn"`
	Editor  string `toml:"editor"`
	Accent  string `toml:"accent"`
	VimKeys *bool  `toml:"vim_keys"`
}

// Flags are command-line overrides; empty strings are ignored.
type Flags struct {
	ConfigPath string
	DSN        string
	Accent     string
}

// Load reads the config file and applies overrides. DSN precedence is
// --dsn flag > PGDOJO_DSN env > config file > default. A missing config file is
// fine unless its path was given explicitly.
func Load(f Flags) (Config, error) {
	path := f.ConfigPath
	if path == "" {
		path = DefaultPath()
	}
	var c Config
	if _, err := toml.DecodeFile(path, &c); err != nil {
		if !errors.Is(err, fs.ErrNotExist) || f.ConfigPath != "" {
			return c, fmt.Errorf("could not read config file %s: %w", path, err)
		}
	}
	if env := os.Getenv("PGDOJO_DSN"); env != "" {
		c.DSN = env
	}
	if f.DSN != "" {
		c.DSN = f.DSN
	}
	if c.DSN == "" {
		c.DSN = db.DefaultDSN
	}
	if f.Accent != "" {
		c.Accent = f.Accent
	}
	if c.Accent != "" && !hexColor.MatchString(c.Accent) {
		return c, fmt.Errorf("accent %q is not a hex color; use the form #5ccfb8 (in %s or --accent)", c.Accent, path)
	}
	return c, nil
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// EditorCommand is the editor from the config file, then $EDITOR, then vi.
func (c Config) EditorCommand() string {
	if c.Editor != "" {
		return c.Editor
	}
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	return "vi"
}

// UseVimKeys reports whether j/k move in lists (default true).
func (c Config) UseVimKeys() bool { return c.VimKeys == nil || *c.VimKeys }

// DefaultPath is $XDG_CONFIG_HOME/pgdojo/config.toml.
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "pgdojo", "config.toml")
}

// DataDir is $XDG_DATA_HOME/pgdojo, defaulting to ~/.local/share/pgdojo.
func DataDir() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "pgdojo")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".local", "share", "pgdojo")
}

// ProgressPath is where progress.json lives.
func ProgressPath() string { return filepath.Join(DataDir(), "progress.json") }

// DraftsDir is the root directory for per-exercise drafts.
func DraftsDir() string { return filepath.Join(DataDir(), "drafts") }
