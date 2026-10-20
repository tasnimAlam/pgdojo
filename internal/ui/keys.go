package ui

import "charm.land/bubbles/v2/key"

// KeyMap is every key binding in the app. The help overlay is generated
// from these so it can't drift from the real bindings.
type KeyMap struct {
	// Navigation
	Up, Down, Open, Back, Filter, Continue, Next, Prev, Help, Quit, Reset key.Binding
	// Query
	Run, Cancel, Editor, Focus, FocusBack, Clear, Diff key.Binding
	// Learning
	Hint, Answer, Discussion, Schema key.Binding
}

func NewKeyMap(vim bool) KeyMap {
	up, down := []string{"up"}, []string{"down"}
	upHelp, downHelp := "↑", "↓"
	if vim {
		up, down = append(up, "k"), append(down, "j")
		upHelp, downHelp = "↑/k", "↓/j"
	}
	b := func(keys []string, k, desc string) key.Binding {
		return key.NewBinding(key.WithKeys(keys...), key.WithHelp(k, desc))
	}
	return KeyMap{
		Up:       b(up, upHelp, "move up"),
		Down:     b(down, downHelp, "move down"),
		Open:     b([]string{"enter"}, "enter", "open"),
		Back:     b([]string{"esc"}, "esc", "back / close"),
		Filter:   b([]string{"/"}, "/", "filter"),
		Continue: b([]string{"c"}, "c", "continue"),
		Next:     b([]string{"n"}, "n", "next exercise"),
		Prev:     b([]string{"p"}, "p", "previous exercise"),
		Help:     b([]string{"?"}, "?", "help"),
		Quit:     b([]string{"q"}, "q", "quit"),
		Reset:    b([]string{"R"}, "R", "reset progress"),

		Run:       b([]string{"ctrl+r", "f5"}, "ctrl+r/F5", "run query"),
		Cancel:    b([]string{"ctrl+c"}, "ctrl+c", "cancel / quit"),
		Editor:    b([]string{"e"}, "e", "open $EDITOR"),
		Focus:     b([]string{"tab"}, "tab", "next pane"),
		FocusBack: b([]string{"shift+tab"}, "shift+tab", "previous pane"),
		Clear:     b([]string{"ctrl+l"}, "ctrl+l", "clear editor"),
		Diff:      b([]string{"v"}, "v", "cycle diff"),

		Hint:       b([]string{"h"}, "h", "next hint"),
		Answer:     b([]string{"a"}, "a", "reveal answer"),
		Discussion: b([]string{"d"}, "d", "discussion"),
		Schema:     b([]string{"s"}, "s", "toggle schema"),
	}
}

// HelpGroups returns the help overlay columns.
func (k KeyMap) HelpGroups() []helpGroup {
	return []helpGroup{
		{"Navigation", items(k.Up, k.Down, k.Open, k.Back, k.Filter, k.Continue, k.Next, k.Prev, k.Reset, k.Help, k.Quit)},
		{"Query", items(k.Run, k.Cancel, k.Editor, k.Focus, k.FocusBack, k.Clear, k.Diff)},
		{"Learning", items(k.Hint, k.Answer, k.Discussion, k.Schema)},
	}
}

type helpItem struct{ key, desc string }

type helpGroup struct {
	title string
	items []helpItem
}

func items(bs ...key.Binding) []helpItem {
	out := make([]helpItem, len(bs))
	for i, b := range bs {
		out[i] = helpItem{b.Help().Key, b.Help().Desc}
	}
	return out
}

// short builds a footer item with a shorter description than the overlay.
func short(b key.Binding, desc string) helpItem { return helpItem{b.Help().Key, desc} }
