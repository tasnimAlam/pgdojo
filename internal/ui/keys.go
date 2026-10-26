package ui

import "charm.land/bubbles/v2/key"

// KeyMap is every key binding in the app. The help overlay is generated
// from these so it can't drift from the real bindings.
type KeyMap struct {
	Vim bool // vim-style keys (h/j/k/l, g/G, ctrl+d/u) are enabled

	// Navigation
	Up, Down, Open, Back, ListBack, Filter, Continue, Next, Prev, Help, Quit, Reset, StartDB key.Binding
	Top, Bottom, PageUp, PageDown                                                            key.Binding
	// Query
	Run, Cancel, Editor, Focus, FocusBack, Clear, Diff key.Binding
	// Learning
	Hint, Answer, Discussion, Schema key.Binding
}

func NewKeyMap(vim bool) KeyMap {
	up, down := []string{"up"}, []string{"down"}
	upHelp, downHelp := "↑", "↓"
	open, openHelp := []string{"enter"}, "enter"
	// h goes back only on list screens; in an exercise h is "hint".
	listBack := key.NewBinding(key.WithDisabled())
	top, bottom := []string{"home"}, []string{"end"}
	pgUp, pgDown := []string{"pgup"}, []string{"pgdown"}
	topHelp, bottomHelp, pgUpHelp, pgDownHelp := "home", "end", "pgup", "pgdown"
	if vim {
		top, bottom = append(top, "g"), append(bottom, "G")
		pgUp, pgDown = append(pgUp, "ctrl+u"), append(pgDown, "ctrl+d")
		topHelp, bottomHelp, pgUpHelp, pgDownHelp = "g", "G", "ctrl+u", "ctrl+d"
		up, down = append(up, "k"), append(down, "j")
		upHelp, downHelp = "↑/k", "↓/j"
		open, openHelp = append(open, "l"), "enter/l"
		listBack = key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "back (lists)"))
	}
	b := func(keys []string, k, desc string) key.Binding {
		return key.NewBinding(key.WithKeys(keys...), key.WithHelp(k, desc))
	}
	return KeyMap{
		Vim:      vim,
		Top:      b(top, topHelp, "top"),
		Bottom:   b(bottom, bottomHelp, "bottom"),
		PageUp:   b(pgUp, pgUpHelp, "page up"),
		PageDown: b(pgDown, pgDownHelp, "page down"),
		Up:       b(up, upHelp, "move up"),
		Down:     b(down, downHelp, "move down"),
		Open:     b(open, openHelp, "open"),
		Back:     b([]string{"esc"}, "esc", "back / close"),
		ListBack: listBack,
		Filter:   b([]string{"/"}, "/", "filter"),
		Continue: b([]string{"c"}, "c", "continue"),
		Next:     b([]string{"n"}, "n", "next exercise"),
		Prev:     b([]string{"p"}, "p", "previous exercise"),
		Help:     b([]string{"?"}, "?", "help"),
		Quit:     b([]string{"q"}, "q", "quit (asks y/n)"),
		Reset:    b([]string{"X"}, "X", "reset progress"),
		StartDB:  b([]string{"R"}, "R", "start database"),

		Run:       b([]string{"ctrl+r"}, "ctrl+r", "run query"),
		Cancel:    b([]string{"ctrl+c"}, "ctrl+c", "cancel run"),
		Editor:    b([]string{"e"}, "e", "open $EDITOR"),
		Focus:     b([]string{"tab"}, "tab", "next pane"),
		FocusBack: b([]string{"shift+tab"}, "shift+tab", "previous pane"),
		Clear:     b([]string{"ctrl+l"}, "ctrl+l", "clear editor"),
		Diff:      b([]string{"v"}, "v", "cycle diff"),

		Hint:       b([]string{"H", "shift+h"}, "H", "next hint"),
		Answer:     b([]string{"a"}, "a", "reveal answer"),
		Discussion: b([]string{"D", "shift+d"}, "D", "discussion"),
		Schema:     b([]string{"s"}, "s", "toggle schema"),
	}
}

// HelpGroups returns the help overlay columns.
func (k KeyMap) HelpGroups() []helpGroup {
	return []helpGroup{
		{"Navigation", append(append(items(k.Up, k.Down), k.jumpHelp()...),
			items(k.Open, k.Back, k.ListBack, k.Filter, k.Continue, k.Next, k.Prev, k.StartDB, k.Reset, k.Help, k.Quit)...)},
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
	var out []helpItem
	for _, b := range bs {
		if b.Enabled() {
			out = append(out, helpItem{b.Help().Key, b.Help().Desc})
		}
	}
	return out
}

// jumpHelp pairs top/bottom and page up/down so the help overlay fits 80×24.
func (k KeyMap) jumpHelp() []helpItem {
	page := "pgup/pgdown"
	if k.Vim {
		page = "ctrl+u/d"
	}
	return []helpItem{
		{k.Top.Help().Key + "/" + k.Bottom.Help().Key, "top / bottom"},
		{page, "page up / down"},
	}
}

// moveHelp is the footer item for moving in lists.
func (k KeyMap) moveHelp() helpItem {
	if k.Vim {
		return helpItem{"j/k", "move"}
	}
	return helpItem{"↑/↓", "move"}
}

// short builds a footer item with a shorter description than the overlay.
func short(b key.Binding, desc string) helpItem { return helpItem{b.Help().Key, desc} }
