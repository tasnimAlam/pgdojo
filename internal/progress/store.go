// Package progress records per-exercise status in a JSON file.
package progress

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Status string

const (
	New       Status = "new"
	Attempted Status = "attempted"
	Solved    Status = "solved"
	Revealed  Status = "revealed"
)

// Entry is the progress of one exercise.
type Entry struct {
	Status    Status `json:"status"`
	Attempts  int    `json:"attempts"`
	HintsUsed int    `json:"hints_used"`
	// Revealed stays true after a revealed exercise is later solved, so
	// stats can tell honest solves apart.
	Revealed  bool      `json:"revealed,omitempty"`
	LastPass  bool      `json:"last_pass,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Last is the most recently opened exercise.
type Last struct {
	Category string `json:"category"`
	Exercise string `json:"exercise"`
}

type file struct {
	Version   int              `json:"version"`
	Last      *Last            `json:"last,omitempty"`
	Exercises map[string]Entry `json:"exercises"`
}

// Store is safe for concurrent use. Every mutation is written to disk
// atomically before it returns.
type Store struct {
	path string
	now  func() time.Time

	mu   sync.Mutex
	data file
}

// Open loads the progress file; a missing file is an empty store.
func Open(path string) (*Store, error) {
	s := &Store{path: path, now: time.Now, data: file{Version: 1, Exercises: map[string]Entry{}}}
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &s.data); err != nil {
		return nil, fmt.Errorf("progress file %s is corrupt (%v); move it away to start fresh", path, err)
	}
	if s.data.Exercises == nil {
		s.data.Exercises = map[string]Entry{}
	}
	return s, nil
}

// Path is the progress file location.
func (s *Store) Path() string { return s.path }

// Get returns the entry for key ("category/exercise"), Status New if unseen.
func (s *Store) Get(key string) Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.get(key)
}

func (s *Store) get(key string) Entry {
	e, ok := s.data.Exercises[key]
	if !ok || e.Status == "" {
		e.Status = New
	}
	return e
}

// Last returns the most recently opened exercise, if any.
func (s *Store) Last() (Last, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Last == nil {
		return Last{}, false
	}
	return *s.data.Last, true
}

// SetLast records the most recently opened exercise.
func (s *Store) SetLast(category, exercise string) error {
	return s.update(func() { s.data.Last = &Last{Category: category, Exercise: exercise} })
}

// RecordAttempt counts a run. Passing makes the exercise solved; solved
// never downgrades.
func (s *Store) RecordAttempt(key string, passed bool) error {
	return s.modify(key, func(e *Entry) {
		e.Attempts++
		e.LastPass = passed
		switch {
		case passed:
			e.Status = Solved
		case e.Status == New:
			e.Status = Attempted
		}
	})
}

// RecordHint notes that hints 1..shown have been seen.
func (s *Store) RecordHint(key string, shown int) error {
	return s.modify(key, func(e *Entry) { e.HintsUsed = max(e.HintsUsed, shown) })
}

// RecordReveal marks the answer as revealed unless already solved.
func (s *Store) RecordReveal(key string) error {
	return s.modify(key, func(e *Entry) {
		if e.Status != Solved {
			e.Status = Revealed
			e.Revealed = true
		}
	})
}

// Reset forgets all progress.
func (s *Store) Reset() error {
	return s.update(func() { s.data = file{Version: 1, Exercises: map[string]Entry{}} })
}

func (s *Store) modify(key string, f func(*Entry)) error {
	return s.update(func() {
		e := s.get(key)
		f(&e)
		e.UpdatedAt = s.now().UTC()
		s.data.Exercises[key] = e
	})
}

func (s *Store) update(f func()) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f()
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(s.path, raw)
}

// writeAtomic writes to a temp file in the same directory and renames it
// over path, so a crash never leaves a half-written file.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".progress-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
