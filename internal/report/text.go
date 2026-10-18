package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/licgate/licgate/internal/i18n"
	"github.com/licgate/licgate/internal/policy"
)

// Palette shared with the TUI.
var (
	ColorBlock  = lipgloss.AdaptiveColor{Light: "#C62828", Dark: "#FF6B6B"}
	ColorReview = lipgloss.AdaptiveColor{Light: "#B26A00", Dark: "#FFC857"}
	ColorNotice = lipgloss.AdaptiveColor{Light: "#1565C0", Dark: "#7CB7FF"}
	ColorOK     = lipgloss.AdaptiveColor{Light: "#2E7D32", Dark: "#7EE081"}
	ColorSkip   = lipgloss.AdaptiveColor{Light: "#757575", Dark: "#8A8A8A"}
	ColorDim    = lipgloss.AdaptiveColor{Light: "#666666", Dark: "#9A9A9A"}
)

// LevelColor maps a level to its color.
func LevelColor(l policy.Level) lipgloss.AdaptiveColor {
	switch l {
	case policy.LevelBlock:
		return ColorBlock
	case policy.LevelReview:
		return ColorReview
	case policy.LevelNotice:
		return ColorNotice
	case policy.LevelOK:
		return ColorOK
	}
	return ColorSkip
}

// LevelBadge renders a fixed-width colored level label.
func LevelBadge(l policy.Level) string {
	return lipgloss.NewStyle().Bold(true).Foreground(LevelColor(l)).Width(6).Render(l.String())
}

// WriteText writes the terminal summary. all=false hides NOTICE/OK/SKIP.
func WriteText(w io.Writer, r *policy.Result, m Meta, all bool) {
	dim := lipgloss.NewStyle().Foreground(ColorDim)
	bold := lipgloss.NewStyle().Bold(true)
	p := func(format string, a ...any) { fmt.Fprintf(w, format, a...) }

	p("%s %s\n", bold.Render("licgate"), dim.Render(m.Project))
	for _, l := range settingsLines(r) {
		p("  %s %s\n", dim.Render(l[0]+":"), l[1])
	}
	p("\n  ")
	for _, lvl := range Levels {
		p("%s %-5d ", lipgloss.NewStyle().Foreground(LevelColor(lvl)).Bold(true).Render(lvl.String()), r.Counts[lvl.String()])
	}
	p("\n\n")
	for _, wn := range append(append([]string{}, r.Graph.Warnings...), r.Warnings...) {
		p("%s %s\n", lipgloss.NewStyle().Foreground(ColorReview).Render("!"), wn)
	}
	if len(r.Graph.Warnings)+len(r.Warnings) > 0 {
		p("\n")
	}
	hidden := 0
	for _, f := range sortedFindings(r) {
		if !all && f.Level < policy.LevelReview {
			hidden++
			continue
		}
		c := f.Component
		p("%s %s %s  %s\n", LevelBadge(f.Level), bold.Render(c.Key()), dim.Render("("+c.Ecosystem+")"), LicenseLabel(c))
		if len(f.Path) > 1 {
			p("       %s %s\n", dim.Render(i18n.T("rep.path")+":"), dim.Render(strings.Join(f.Path, " → ")))
		}
		for _, rs := range f.Reasons {
			p("       - %s\n", rs)
		}
		if all {
			for _, o := range f.Obligations {
				p("       ☐ %s\n", policy.ObligationText(o))
			}
		}
	}
	if hidden > 0 {
		p("%s\n", dim.Render(i18n.T("rep.more_hidden", hidden)))
	}
	if obs := ObligationSummary(r); len(obs) > 0 {
		p("\n%s\n", bold.Render(i18n.T("rep.ob_summary")))
		for _, o := range obs {
			p("  ☐ %s %s\n", policy.ObligationText(o.Key), dim.Render("("+i18n.T("rep.ob_count", o.Count)+")"))
		}
		p("  %s\n", dim.Render(i18n.T("rep.notice_hint")))
	}
	p("\n")
	status := lipgloss.NewStyle().Bold(true).Foreground(ColorOK).Render(i18n.T("rep.pass"))
	if m.Failed {
		status = lipgloss.NewStyle().Bold(true).Foreground(ColorBlock).Render(i18n.T("rep.fail"))
	}
	p("%s  %s\n", status, Verdict(r))
	p("%s\n", dim.Render(i18n.T("rep.disclaimer")))
}
