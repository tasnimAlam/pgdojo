package ui

import "github.com/tasnimAlam/pgex/internal/db"

// Navigation messages. Screens emit these; the root model acts on them.
type (
	openCategoryMsg struct{ index int }
	// replace swaps the current screen instead of pushing (next/previous).
	openExerciseMsg struct {
		key     string
		replace bool
	}
	backMsg struct{}
)

// pingMsg reports the result of a connectivity check.
type pingMsg struct{ err error }

// pingTickMsg schedules the next ping.
type pingTickMsg struct{}

// columnsMsg carries the expected column names for an exercise.
type columnsMsg struct {
	key  string
	cols []string
	err  error
}

// checkDoneMsg is the result of running the user's query. seq ignores
// results of runs that were superseded.
type checkDoneMsg struct {
	seq int
	chk db.Check
	err error
}

// draftLoadedMsg carries a draft read from disk.
type draftLoadedMsg struct {
	text string
	err  error
}

// editorDoneMsg is sent when the external editor exits.
type editorDoneMsg struct{ err error }

// noteMsg shows a short message in the footer, e.g. a failed save.
type noteMsg struct{ text string }
