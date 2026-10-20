package ui

// Navigation messages. Screens emit these; the root model acts on them.
type (
	openCategoryMsg struct{ index int }
	openExerciseMsg struct{ key string }
	backMsg         struct{}
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
