// Package drafts saves the SQL a user is working on, one file per exercise.
package drafts

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Store keeps drafts at <dir>/<category>/<exercise>.sql.
type Store struct {
	dir string
}

func New(dir string) *Store { return &Store{dir: dir} }

// Path returns the draft file for a "category/exercise" key.
func (s *Store) Path(key string) string {
	return filepath.Join(s.dir, filepath.FromSlash(key)+".sql")
}

// Load returns the saved draft, or "" if there is none.
func (s *Store) Load(key string) (string, error) {
	b, err := os.ReadFile(s.Path(key))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return string(b), err
}

// Save writes the draft, creating directories as needed.
func (s *Store) Save(key, sql string) error {
	p := s.Path(key)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(sql), 0o644)
}
