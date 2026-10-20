// Command pgex is a terminal trainer for PostgreSQL exercises.
package main

import (
	"fmt"
	"os"

	"github.com/tasnimAlam/pgex/internal/exercise"
)

func main() {
	// Milestone 1 placeholder: load and summarize the catalog. The cobra CLI
	// and TUI replace this in later milestones.
	cat, err := exercise.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, c := range cat.Categories {
		fmt.Printf("%-28s %d exercises\n", c.Title, len(c.Exercises))
	}
}
