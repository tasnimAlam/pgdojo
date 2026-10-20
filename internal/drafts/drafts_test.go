package drafts

import (
	"path/filepath"
	"testing"
)

func TestSaveLoad(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	if got, err := s.Load("joins/x"); err != nil || got != "" {
		t.Fatalf("missing draft = %q, %v", got, err)
	}
	if err := s.Save("joins/x", "select 1"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Load("joins/x"); got != "select 1" {
		t.Errorf("Load = %q", got)
	}
	if want := filepath.Join(dir, "joins", "x.sql"); s.Path("joins/x") != want {
		t.Errorf("Path = %q, want %q", s.Path("joins/x"), want)
	}
}
