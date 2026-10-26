package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComposeFile(t *testing.T) {
	empty, noData, full := t.TempDir(), t.TempDir(), t.TempDir()
	for _, f := range []string{filepath.Join(noData, "compose.yaml"), filepath.Join(full, "compose.yaml"), filepath.Join(full, "clubdata.sql")} {
		if err := os.WriteFile(f, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if f, err := composeFile(empty, full); err != nil || f != filepath.Join(full, "compose.yaml") {
		t.Errorf("want the second dir's compose.yaml, got %q, %v", f, err)
	}
	if _, err := composeFile(noData, full); err == nil || !strings.Contains(err.Error(), "clubdata.sql") {
		t.Errorf("want missing clubdata.sql error, got %v", err)
	}
	if _, err := composeFile(empty); err == nil {
		t.Error("want an error without compose.yaml")
	}
}
