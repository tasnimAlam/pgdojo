package ui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/tasnimAlam/pgex/internal/exercise"
)

// exercisePlaceholder stands in for the exercise view until milestone 5.
// It shows the question and records the exercise as the last one opened.
type exercisePlaceholder struct {
	d    *Deps
	ex   exercise.Exercise
	w, h int
}

func newExercisePlaceholder(d *Deps, ex exercise.Exercise) *exercisePlaceholder {
	return &exercisePlaceholder{d: d, ex: ex}
}

func (s *exercisePlaceholder) Init() tea.Cmd {
	p, cat, id := s.d.Progress, s.ex.Category, s.ex.ID
	return func() tea.Msg {
		_ = p.SetLast(cat, id) // best effort; failure only loses "continue"
		return nil
	}
}

func (s *exercisePlaceholder) SetSize(w, h int) { s.w, s.h = w, h }

func (s *exercisePlaceholder) Update(msg tea.Msg) (Screen, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok && key.Matches(k, s.d.Keys.Back) {
		return s, func() tea.Msg { return backMsg{} }
	}
	return s, nil
}

func (s *exercisePlaceholder) View() string {
	t := s.d.Theme
	catTitle := s.ex.Category
	for _, c := range s.d.Catalog.Categories {
		if c.ID == s.ex.Category {
			catTitle = c.Title
		}
	}
	lines := wrap(s.ex.Title, s.w-4, t.Title)
	lines = append(lines, "")
	lines = append(lines, wrap(s.ex.Question, s.w-4, t.Text)...)
	lines = append(lines, "", t.Dim.Render("The editor and runner arrive in the next milestone. Meanwhile use: pgex check "+s.ex.Key()+" file.sql"))
	body := t.Pane("Question", strings.Join(lines, "\n"), s.w, bodyHeight(s.h), true)
	header := t.Header([]string{"Categories", catTitle, s.ex.Title}, "", s.w)
	k := s.d.Keys
	return joinScreen(header, body, t.Footer([]helpItem{short(k.Back, "back"), short(k.Help, "help"), short(k.Quit, "quit")}, "", s.w))
}
