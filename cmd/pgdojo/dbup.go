package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tasnimAlam/pgdojo/internal/config"
)

// composeFile finds db/compose.yaml in the working directory (a source
// checkout) or in the data directory (a release install).
func composeFile(dirs ...string) (string, error) {
	for _, d := range dirs {
		f := filepath.Join(d, "compose.yaml")
		if _, err := os.Stat(f); err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(d, "clubdata.sql")); err != nil {
			return "", fmt.Errorf("%s is missing; download https://pgexercises.com/dbfiles/clubdata.sql there", filepath.Join(d, "clubdata.sql"))
		}
		return f, nil
	}
	return "", fmt.Errorf("no compose.yaml in %s; see the README's database setup", strings.Join(dirs, " or "))
}

// composeCmd mirrors scripts/compose.sh: podman first, then docker.
func composeCmd() ([]string, error) {
	if _, err := exec.LookPath("podman"); err == nil {
		if _, err := exec.LookPath("podman-compose"); err == nil {
			return []string{"podman-compose"}, nil
		}
		return []string{"podman", "compose"}, nil
	}
	if _, err := exec.LookPath("docker"); err == nil {
		return []string{"docker", "compose"}, nil
	}
	return nil, errors.New("neither podman nor docker found; install one: sudo pacman -S podman podman-compose")
}

// startDB runs "compose up -d" for the exercises database, like make db-up.
func startDB(ctx context.Context) error {
	file, err := composeFile("db", filepath.Join(config.DataDir(), "db"))
	if err != nil {
		return err
	}
	c, err := composeCmd()
	if err != nil {
		return err
	}
	out, err := exec.CommandContext(ctx, c[0], append(c[1:], "-f", file, "up", "-d")...).CombinedOutput()
	if err != nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		return fmt.Errorf("%s failed: %s", strings.Join(c, " "), lines[len(lines)-1])
	}
	return nil
}
