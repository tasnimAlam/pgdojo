package ui

import (
	"fmt"
	"os/exec"
	"strings"

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

func (e *editor) View() string {
	if e.ta.Focused() {
		return e.ta.View()
	}
	if strings.TrimSpace(e.ta.Value()) == "" {
		return e.t.Faint.Render(e.ta.Placeholder)
	}
	lines := strings.Split(highlightSQL(e.t, e.ta.Value()), "\n")
	numW := len(fmt.Sprint(len(lines)))
	out := make([]string, 0, min(len(lines), e.h))
	for i, l := range lines {
		if i == e.h {
			break
		}
		out = append(out, e.t.LineNo.Render(fmt.Sprintf(" %*d ", numW, i+1))+l)
	}
	return strings.Join(out, "\n")
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
