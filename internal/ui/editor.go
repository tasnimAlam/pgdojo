package ui

import (
	"fmt"
	"os/exec"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// editor wraps a textarea. While focused it shows the plain textarea; when
// not focused it shows the same text with SQL highlighting.
type editor struct {
	t    Theme
	ta   textarea.Model
	w, h int
}

func newEditor(t Theme) editor {
	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = true
	ta.CharLimit = 0
	ta.MaxHeight = 0
	ta.Placeholder = "-- write your SQL here, then press ctrl+r"
	st := ta.Styles()
	for _, s := range []*textarea.StyleState{&st.Focused, &st.Blurred} {
		s.Base = lipgloss.NewStyle()
		s.Text = t.Text
		s.LineNumber = t.LineNo
		s.CursorLineNumber = t.Dim
		s.CursorLine = lipgloss.NewStyle()
		s.Placeholder = t.Faint
		s.EndOfBuffer = t.Faint
	}
	ta.SetStyles(st)
	return editor{t: t, ta: ta}
}

func (e *editor) SetSize(w, h int) {
	e.w, e.h = w, h
	e.ta.SetWidth(w)
	e.ta.SetHeight(h)
}

func (e *editor) Value() string     { return e.ta.Value() }
func (e *editor) SetValue(s string) { e.ta.SetValue(s) }
func (e *editor) Focused() bool     { return e.ta.Focused() }
func (e *editor) Focus() tea.Cmd    { return e.ta.Focus() }
func (e *editor) Blur()             { e.ta.Blur() }
func (e *editor) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	e.ta, cmd = e.ta.Update(msg)
	return cmd
}

// View shows the textarea while typing; otherwise highlighted SQL, with a
// block cursor when cursor is set (NORMAL mode in the query pane).
func (e *editor) View(cursor bool) string {
	if e.ta.Focused() {
		return e.ta.View()
	}
	if e.ta.Value() == "" {
		if cursor {
			return e.t.Cursor.Render(" ") + " " + e.t.Faint.Render(e.ta.Placeholder)
		}
		return e.t.Faint.Render(e.ta.Placeholder)
	}
	src := strings.Split(e.ta.Value(), "\n")
	row, col := e.ta.Line(), e.ta.Column()
	numW := len(fmt.Sprint(len(src)))
	start := 0
	if cursor && row >= e.h { // keep the cursor line on screen
		start = row - e.h + 1
	}
	out := make([]string, 0, e.h)
	for i := start; i < len(src) && len(out) < e.h; i++ {
		line := highlightSQL(e.t, src[i])
		if cursor && i == row {
			r := []rune(src[i])
			col = min(col, len(r))
			under := " "
			if col < len(r) {
				under = string(r[col])
			}
			line = highlightSQL(e.t, string(r[:col])) + e.t.Cursor.Render(under)
			if col < len(r) {
				line += highlightSQL(e.t, string(r[col+1:]))
			}
		}
		out = append(out, e.t.LineNo.Render(fmt.Sprintf(" %*d ", numW, i+1))+line)
	}
	return strings.Join(out, "\n")
}

// WordForward moves the cursor to the start of the next word, like vim's w.
func (e *editor) WordForward() { e.moveWord(nextWordStart) }

// WordBackward moves the cursor to the start of the previous word, like
// vim's b.
func (e *editor) WordBackward() { e.moveWord(prevWordStart) }

func (e *editor) moveWord(motion func([]rune, int) int) {
	text := []rune(e.ta.Value())
	pos := 0 // rune offset of the cursor in the whole text
	for i, l := range strings.Split(string(text), "\n") {
		if i == e.ta.Line() {
			pos += min(e.ta.Column(), len([]rune(l)))
			break
		}
		pos += len([]rune(l)) + 1
	}
	pos = motion(text, pos)
	row := strings.Count(string(text[:pos]), "\n")
	col := pos - lastNewline(text[:pos]) - 1
	e.ta.MoveToBegin()
	for i := 0; i < row; i++ {
		e.ta.CursorDown()
	}
	e.ta.SetCursorColumn(col)
}

func lastNewline(r []rune) int {
	for i := len(r) - 1; i >= 0; i-- {
		if r[i] == '\n' {
			return i
		}
	}
	return -1
}

// charClass groups runes the way vim's w and b see them.
func charClass(r rune) int {
	switch {
	case r == ' ' || r == '\t' || r == '\n':
		return 0
	case r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
		return 1
	default:
		return 2 // punctuation
	}
}

// nextWordStart is vim's w: skip the rest of the current word, then
// whitespace. It stays on the last rune at the end of the text.
func nextWordStart(t []rune, i int) int {
	if len(t) == 0 {
		return 0
	}
	if i < len(t) {
		if c := charClass(t[i]); c != 0 {
			for i < len(t) && charClass(t[i]) == c {
				i++
			}
		}
	}
	for i < len(t) && charClass(t[i]) == 0 {
		i++
	}
	return min(i, len(t)-1)
}

// prevWordStart is vim's b: skip whitespace backwards, then go to the
// start of that word.
func prevWordStart(t []rune, i int) int {
	i = min(i, len(t))
	for i > 0 && charClass(t[i-1]) == 0 {
		i--
	}
	if i == 0 {
		return 0
	}
	c := charClass(t[i-1])
	for i > 0 && charClass(t[i-1]) == c {
		i--
	}
	return i
}

// highlightSQL colors SQL with the theme's token styles.
func highlightSQL(t Theme, src string) string {
	lexer := lexers.Get("postgresql")
	if lexer == nil {
		return t.Text.Render(src)
	}
	it, err := chroma.Coalesce(lexer).Tokenise(nil, src)
	if err != nil {
		return t.Text.Render(src)
	}
	var b strings.Builder
	for tok := it(); tok != chroma.EOF; tok = it() {
		st := t.Text
		switch {
		case tok.Type.InCategory(chroma.Keyword), tok.Type.InSubCategory(chroma.NameBuiltin):
			st = t.SQLKeyword
		case tok.Type.InSubCategory(chroma.LiteralString):
			st = t.SQLString
		case tok.Type.InCategory(chroma.Comment):
			st = t.SQLComment
		case tok.Type.InSubCategory(chroma.LiteralNumber):
			st = t.SQLNumber
		}
		// Style each line separately so newlines stay unstyled.
		parts := strings.Split(tok.Value, "\n")
		for i, p := range parts {
			if i > 0 {
				b.WriteByte('\n')
			}
			if p != "" {
				b.WriteString(st.Render(p))
			}
		}
	}
	return b.String()
}

// externalEditor runs the configured editor on path. command may include
// arguments, e.g. "emacsclient -t".
func externalEditor(command, path string, done func(error) tea.Msg) tea.Cmd {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		fields = []string{"vi"}
	}
	c := exec.Command(fields[0], append(fields[1:], path)...)
	return tea.ExecProcess(c, func(err error) tea.Msg { return done(err) })
}
