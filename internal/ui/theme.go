package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Palette holds the color tokens from the design. A light palette can be
// added later by providing another constructor.
type Palette struct {
	Bg, Fg, FgBright, Dim, Faint, LineNo, Border     color.Color
	Accent, SelBg, Warn, Fail, FailBg, FailRow, Pass color.Color
	PassBg, Keyword                                  color.Color
}

// DarkPalette is the default dark-first palette. accent overrides the
// accent color when it is a valid hex string.
func DarkPalette(accent string) Palette {
	p := Palette{
		Bg:       lipgloss.Color("#111318"),
		Fg:       lipgloss.Color("#d4d8e0"),
		FgBright: lipgloss.Color("#ffffff"),
		Dim:      lipgloss.Color("#8b93a4"),
		Faint:    lipgloss.Color("#3a404c"),
		LineNo:   lipgloss.Color("#4a5160"),
		Border:   lipgloss.Color("#2c313c"),
		Accent:   lipgloss.Color("#5ccfb8"),
		SelBg:    lipgloss.Color("#1b2a28"),
		Warn:     lipgloss.Color("#e8b04a"),
		Fail:     lipgloss.Color("#f07a5a"),
		FailBg:   lipgloss.Color("#3a2320"),
		FailRow:  lipgloss.Color("#f4a28c"),
		PassBg:   lipgloss.Color("#17322d"),
		Keyword:  lipgloss.Color("#7aa2f7"),
	}
	if isHexColor(accent) {
		p.Accent = lipgloss.Color(accent)
	}
	p.Pass = p.Accent
	return p
}

func isHexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	return strings.Trim(strings.ToLower(s[1:]), "0123456789abcdef") == ""
}

// Theme is every style the UI uses. No other file defines colors.
type Theme struct {
	P Palette

	Badge, Crumb, CrumbCurrent, Text, Bright, Title, Dim, Faint lipgloss.Style
	Accent, Warn, Fail, Key, HelpKey, Chip, SelRow, SelBar      lipgloss.Style
	Underline                                                   lipgloss.Style

	// Exercise view
	PassBanner, FailBanner, WarnBanner, MissingRow, ExtraGutter, TableHead lipgloss.Style
	LineNo, SQLKeyword, SQLString, SQLComment, SQLNumber                   lipgloss.Style
}

func NewTheme(accent string) Theme {
	p := DarkPalette(accent)
	s := lipgloss.NewStyle
	return Theme{
		P:            p,
		Badge:        s().Background(p.Accent).Foreground(p.Bg).Bold(true),
		Crumb:        s().Foreground(p.Dim),
		CrumbCurrent: s().Foreground(p.Fg).Bold(true),
		Text:         s().Foreground(p.Fg),
		Bright:       s().Foreground(p.FgBright),
		Title:        s().Foreground(p.FgBright).Bold(true),
		Dim:          s().Foreground(p.Dim),
		Faint:        s().Foreground(p.Faint),
		Accent:       s().Foreground(p.Accent),
		Warn:         s().Foreground(p.Warn),
		Fail:         s().Foreground(p.Fail),
		Key:          s().Foreground(p.Fg),
		HelpKey:      s().Foreground(p.Warn),
		Chip:         s().Foreground(p.Fg).Background(p.Border),
		SelRow:       s().Foreground(p.FgBright).Background(p.SelBg),
		SelBar:       s().Foreground(p.Accent).Background(p.SelBg),
		Underline:    s().Underline(true),

		PassBanner:  s().Foreground(p.Accent).Background(p.PassBg).Bold(true),
		FailBanner:  s().Foreground(p.Fail).Background(p.FailBg).Bold(true),
		WarnBanner:  s().Foreground(p.Warn).Background(p.FailBg).Bold(true),
		MissingRow:  s().Foreground(p.FailRow).Background(p.FailBg),
		ExtraGutter: s().Foreground(p.Warn),
		TableHead:   s().Foreground(p.Dim).Bold(true),
		LineNo:      s().Foreground(p.LineNo),
		SQLKeyword:  s().Foreground(p.Keyword),
		SQLString:   s().Foreground(p.Warn),
		SQLComment:  s().Foreground(p.Dim).Italic(true),
		SQLNumber:   s().Foreground(p.Accent),
	}
}

// Pane draws a rounded box of exactly w×h cells with the title in the top
// border and 1 cell of horizontal padding. Body lines are truncated and the
// body is padded or clipped to fit.
func (t Theme) Pane(title, body string, w, h int, focused bool) string {
	border, titleStyle := t.P.Border, t.Dim
	if focused {
		border, titleStyle = t.P.Accent, t.Accent.Bold(true)
	}
	return t.PaneColored(title, body, w, h, border, titleStyle)
}

// PaneColored is Pane with an explicit border color and title style.
func (t Theme) PaneColored(title, body string, w, h int, border color.Color, titleStyle lipgloss.Style) string {
	if w < 4 || h < 2 {
		return ""
	}
	b := lipgloss.NewStyle().Foreground(border)
	inner := w - 4

	var top string
	if title != "" {
		title = ansi.Truncate(title, max(w-6, 0), "…")
		top = b.Render("╭ ") + titleStyle.Render(title) + b.Render(" "+strings.Repeat("─", max(w-4-lipgloss.Width(title), 0))+"╮")
	} else {
		top = b.Render("╭" + strings.Repeat("─", w-2) + "╮")
	}

	lines := strings.Split(body, "\n")
	out := make([]string, 0, h)
	out = append(out, top)
	side := b.Render("│")
	for i := 0; i < h-2; i++ {
		var l string
		if i < len(lines) {
			l = fit(lines[i], inner)
		} else {
			l = strings.Repeat(" ", inner)
		}
		out = append(out, side+" "+l+" "+side)
	}
	out = append(out, b.Render("╰"+strings.Repeat("─", w-2)+"╯"))
	return strings.Join(out, "\n")
}

// fit truncates or right-pads a styled line to exactly w cells.
func fit(s string, w int) string {
	if lipgloss.Width(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	return s + strings.Repeat(" ", max(w-lipgloss.Width(s), 0))
}

// spread puts left and right on one line of width w, right-aligned.
func spread(left, right string, w int) string {
	rw := lipgloss.Width(right)
	if rw >= w {
		return fit(left, w)
	}
	// Keep the right side; truncate the left side if both don't fit.
	if lipgloss.Width(left)+1+rw > w {
		left = ansi.Truncate(left, w-rw-1, "…")
	}
	gap := w - lipgloss.Width(left) - rw
	return left + strings.Repeat(" ", gap) + right
}

// Header renders the top row: badge, breadcrumb (last part bold), and
// right-aligned context info.
func (t Theme) Header(crumbs []string, right string, w int) string {
	left := t.Badge.Render(" pgex ") + " "
	for i, c := range crumbs {
		if i > 0 {
			left += t.Crumb.Render(" › ")
		}
		if i == len(crumbs)-1 && len(crumbs) > 1 {
			left += t.CrumbCurrent.Render(c)
		} else {
			left += t.Crumb.Render(c)
		}
	}
	return spread(left, right+" ", w)
}

// Footer renders a separator line and a help line built from bindings,
// with a short status on the right.
func (t Theme) Footer(help []helpItem, status string, w int) string {
	parts := make([]string, 0, len(help))
	for _, h := range help {
		parts = append(parts, t.Key.Render(h.key)+" "+t.Dim.Render(h.desc))
	}
	line := " " + strings.Join(parts, t.Dim.Render(" · "))
	return t.Faint.Foreground(t.P.Border).Render(strings.Repeat("─", w)) + "\n" + spread(line, status+" ", w)
}

// ProgressBar draws a 10-cell bar with the solved fraction in accent.
func (t Theme) ProgressBar(done, total int) string {
	const cells = 10
	n := 0
	if total > 0 {
		n = done * cells / total
	}
	return t.Accent.Render(strings.Repeat("█", n)) + t.Faint.Render(strings.Repeat("█", cells-n))
}

// Dimmed re-renders a whole view in the faint color for use behind overlays.
func (t Theme) Dimmed(view string) string {
	lines := strings.Split(ansi.Strip(view), "\n")
	for i, l := range lines {
		lines[i] = t.Faint.Render(l)
	}
	return strings.Join(lines, "\n")
}

// Banner renders a full-width status line on a colored background.
func (t Theme) Banner(st lipgloss.Style, text string, w int) string {
	return st.Width(w).Render(ansi.Truncate(" "+text, w, "…"))
}

// Overlay centers modal over a dimmed copy of under, which is w×h cells.
func (t Theme) Overlay(under, modal string, w, h int) string {
	x := max((w-lipgloss.Width(modal))/2, 0)
	y := max((h-lipgloss.Height(modal))/2, 0)
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(t.Dimmed(under)),
		lipgloss.NewLayer(modal).X(x).Y(y).Z(1),
	).Render()
}
