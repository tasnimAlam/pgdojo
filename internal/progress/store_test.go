package progress

import (
	"os"
	"path/filepath"
	"testing"
)

func open(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sub", "progress.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return s, path
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestTransitions(t *testing.T) {
	const k = "joins/x"
	tests := []struct {
		name     string
		steps    func(s *Store)
		want     Status
		revealed bool
		attempts int
	}{
		{"unseen", func(s *Store) {}, New, false, 0},
		{"failed run", func(s *Store) { s.RecordAttempt(k, false) }, Attempted, false, 1},
		{"first run passes", func(s *Store) { s.RecordAttempt(k, true) }, Solved, false, 1},
		{"solved never downgrades on fail", func(s *Store) {
			s.RecordAttempt(k, true)
			s.RecordAttempt(k, false)
		}, Solved, false, 2},
		{"solved ignores reveal", func(s *Store) {
			s.RecordAttempt(k, true)
			s.RecordReveal(k)
		}, Solved, false, 1},
		{"reveal before solving", func(s *Store) {
			s.RecordAttempt(k, false)
			s.RecordReveal(k)
		}, Revealed, true, 1},
		{"revealed stays revealed on fail", func(s *Store) {
			s.RecordReveal(k)
			s.RecordAttempt(k, false)
		}, Revealed, true, 1},
		{"revealed then solved keeps flag", func(s *Store) {
			s.RecordReveal(k)
			s.RecordAttempt(k, true)
		}, Solved, true, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := open(t)
			tt.steps(s)
			e := s.Get(k)
			if e.Status != tt.want || e.Revealed != tt.revealed || e.Attempts != tt.attempts {
				t.Errorf("got %+v, want status=%s revealed=%v attempts=%d", e, tt.want, tt.revealed, tt.attempts)
			}
		})
	}
}

func TestPersistence(t *testing.T) {
	s, path := open(t)
	must(t, s.RecordAttempt("a/b", true))
	must(t, s.RecordHint("a/b", 2))
	must(t, s.RecordHint("a/b", 1))
	must(t, s.SetLast("a", "b"))

	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	e := s2.Get("a/b")
	if e.Status != Solved || e.HintsUsed != 2 || e.UpdatedAt.IsZero() {
		t.Errorf("reloaded entry = %+v", e)
	}
	if last, ok := s2.Last(); !ok || last != (Last{"a", "b"}) {
		t.Errorf("Last = %+v, %v", last, ok)
	}

	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("temp files left behind: %v", entries)
	}

	must(t, s2.Reset())
	if s2.Get("a/b").Status != New {
		t.Error("Reset did not clear progress")
	}
	if _, ok := s2.Last(); ok {
		t.Error("Reset did not clear last")
	}
}

func TestCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "progress.json")
	must(t, os.WriteFile(path, []byte("{nope"), 0o644))
	if _, err := Open(path); err == nil {
		t.Fatal("want error for corrupt file")
	}
}
