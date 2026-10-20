package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tasnimAlam/pgdojo/internal/db"
)

func TestLoadPrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("dsn = \"file\"\naccent = \"#ff0000\"\nvim_keys = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PGEX_DSN", "")
	c, err := Load(Flags{ConfigPath: path})
	if err != nil || c.DSN != "file" || c.Accent != "#ff0000" || c.UseVimKeys() {
		t.Fatalf("file only: %+v, %v", c, err)
	}

	t.Setenv("PGEX_DSN", "env")
	if c, _ = Load(Flags{ConfigPath: path}); c.DSN != "env" {
		t.Errorf("env should beat file, got %q", c.DSN)
	}
	if c, _ = Load(Flags{ConfigPath: path, DSN: "flag", Accent: "#00ff00"}); c.DSN != "flag" || c.Accent != "#00ff00" {
		t.Errorf("flags should win, got %+v", c)
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PGEX_DSN", "")
	c, err := Load(Flags{})
	if err != nil || c.DSN != db.DefaultDSN || !c.UseVimKeys() {
		t.Fatalf("defaults: %+v, %v", c, err)
	}
	if _, err := Load(Flags{ConfigPath: "/nonexistent/pgex.toml"}); err == nil {
		t.Error("explicit missing config path should fail")
	}
	if _, err := Load(Flags{Accent: "teal"}); err == nil {
		t.Error("invalid accent should fail")
	}
}
