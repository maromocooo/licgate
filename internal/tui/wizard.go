package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/maromocooo/licgate/internal/i18n"
	"github.com/maromocooo/licgate/internal/policy"
	"github.com/maromocooo/licgate/internal/report"
)

type choice struct {
	value, label, desc string
}

func licenseChoices() []choice {
	return []choice{
		{"MIT", "MIT", i18n.T("w.mit")},
		{"Apache-2.0", "Apache-2.0", i18n.T("w.apache")},
		{"MIT OR Apache-2.0", "MIT OR Apache-2.0", i18n.T("w.dual")},
		{"BSD-3-Clause", "BSD-3-Clause", i18n.T("w.bsd")},
		{"MPL-2.0", "MPL-2.0", i18n.T("w.mpl")},
		{"LGPL-3.0-or-later", "LGPL-3.0-or-later", i18n.T("w.lgpl")},
		{"GPL-3.0-or-later", "GPL-3.0-or-later", i18n.T("w.gpl")},
		{"AGPL-3.0-or-later", "AGPL-3.0-or-later", i18n.T("w.agpl")},
		{"proprietary", i18n.T("w.proprietary_label"), i18n.T("w.proprietary_desc")},
		{"", i18n.T("w.other"), i18n.T("w.other_desc")},
	}
}

func distChoices() []choice {
	return []choice{
		{"binary", "binary", i18n.T("w.d_binary")},
		{"library", "library", i18n.T("w.d_library")},
		{"saas", "saas", i18n.T("w.d_saas")},
		{"internal", "internal", i18n.T("w.d_internal")},
	}
}

func linkChoices() []choice {
	return []choice{
		{"auto", "auto", i18n.T("w.l_auto")},
		{"static", "static", i18n.T("w.l_static")},
		{"dynamic", "dynamic", i18n.T("w.l_dynamic")},
	}
}

func devChoices() []choice {
	return []choice{
		{"no", i18n.T("w.dev_no"), i18n.T("w.dev_no_desc")},
		{"yes", i18n.T("w.dev_yes"), i18n.T("w.dev_yes_desc")},
	}
}

// wizard asks for the release settings step by step.
type wizard struct {
	step     int
	cursor   int
	settings policy.Settings
	input    textinput.Model
	typing   bool
	done     bool
	canceled bool
	width    int
	errMsg   string
}

func newWizard(s policy.Settings) wizard {
	ti := textinput.New()
	ti.Placeholder = "e.g. EUPL-1.2, ISC, Apache-2.0 OR MIT"
	ti.CharLimit = 120
	w := wizard{settings: s, input: ti}
	if w.settings.Distribution == "" {
		w.settings.Distribution = policy.DistBinary
	}
	if w.settings.Linking == "" {
		w.settings.Linking = policy.LinkAuto
	}
	w.cursor = w.indexOf(w.current(), w.currentValue())
	return w
}

func (w wizard) current() []choice {
	switch w.step {
	case 0:
		return licenseChoices()
	case 1:
		return distChoices()
	case 2:
		return linkChoices()
	}
	return devChoices()
}

func (w wizard) currentValue() string {
	switch w.step {
	case 0:
		return w.settings.License
	case 1:
		return string(w.settings.Distribution)
	case 2:
		return string(w.settings.Linking)
	}
	if w.settings.IncludeDev {
		return "yes"
	}
	return "no"
}

func (w wizard) indexOf(cs []choice, v string) int {
	for i, c := range cs {
		if c.value != "" && strings.EqualFold(c.value, v) {
			return i
		}
	}
	return 0
}

func (w wizard) title() string {
	return []string{i18n.T("w.q_license"), i18n.T("w.q_dist"), i18n.T("w.q_link"), i18n.T("w.q_dev")}[w.step]
}

func (w wizard) Update(msg tea.Msg) (wizard, tea.Cmd) {
	if w.typing {
		if k, ok := msg.(tea.KeyMsg); ok {
			switch k.String() {
			case "enter":
				v := strings.TrimSpace(w.input.Value())
				if v == "" {
					return w, nil
				}
				if out := policy.ParseOutbound(v); len(out.Unknown) > 0 {
					w.errMsg = i18n.T("w.unknown_license", v)
				} else {
					w.errMsg = ""
				}
				w.settings.License = v
				w.typing = false
				w.input.Blur()
				return w.next(), nil
			case "esc":
				w.typing = false
				w.input.Blur()
				return w, nil
			}
		}
		var cmd tea.Cmd
		w.input, cmd = w.input.Update(msg)
		return w, cmd
	}
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return w, nil
	}
	cs := w.current()
	switch k.String() {
	case "up", "k":
		if w.cursor > 0 {
			w.cursor--
		}
	case "down", "j":
		if w.cursor < len(cs)-1 {
			w.cursor++
		}
	case "esc", "left", "h", "backspace":
		if w.step == 0 {
			w.canceled = true
			w.done = true
			return w, nil
		}
		w.step--
		w.cursor = w.indexOf(w.current(), w.currentValue())
	case "ctrl+c", "q":
		w.canceled = true
		w.done = true
	case "enter", "right", "l", " ":
		c := cs[w.cursor]
		switch w.step {
		case 0:
			if c.value == "" {
				w.typing = true
				w.input.SetValue("")
				w.input.Focus()
				return w, textinput.Blink
			}
			w.settings.License = c.value
		case 1:
			w.settings.Distribution = policy.Distribution(c.value)
		case 2:
			w.settings.Linking = policy.Linking(c.value)
		case 3:
			w.settings.IncludeDev = c.value == "yes"
		}
		return w.next(), nil
	}
	return w, nil
}

func (w wizard) next() wizard {
	if w.step == 3 {
		w.done = true
		return w
	}
	w.step++
	w.cursor = w.indexOf(w.current(), w.currentValue())
	return w
}

func (w wizard) View() string {
	var b strings.Builder
	title := lipgloss.NewStyle().Bold(true)
	dim := lipgloss.NewStyle().Foreground(report.ColorDim)
	sel := lipgloss.NewStyle().Bold(true).Foreground(report.ColorNotice)
	b.WriteString(dim.Render("licgate · "+i18n.T("w.setup")+" ") + dim.Render(strings.Repeat("●", w.step+1)+strings.Repeat("○", 3-w.step)) + "\n\n")
	b.WriteString(title.Render(w.title()) + "\n\n")
	if w.typing {
		b.WriteString("  " + w.input.View() + "\n\n")
		b.WriteString(dim.Render("  enter " + i18n.T("w.k_confirm") + " · esc " + i18n.T("w.k_back")))
		return b.String()
	}
	for i, c := range w.current() {
		cursor := "  "
		label := c.label
		if i == w.cursor {
			cursor = sel.Render("› ")
			label = sel.Render(label)
		}
		b.WriteString(cursor + label + "\n")
		if c.desc != "" {
			b.WriteString("    " + dim.Render(wrap(c.desc, max(20, w.width-6), "    ")) + "\n")
		}
	}
	if w.errMsg != "" {
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(report.ColorReview).Render(w.errMsg) + "\n")
	}
	b.WriteString("\n" + dim.Render("↑↓ "+i18n.T("w.k_choose")+" · enter "+i18n.T("w.k_confirm")+" · esc "+i18n.T("w.k_back")+" · q "+i18n.T("w.k_quit")))
	return b.String()
}

// wizardProgram wraps the wizard for standalone use before scanning.
type wizardProgram struct{ w wizard }

func (p wizardProgram) Init() tea.Cmd { return nil }

func (p wizardProgram) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		p.w.width = ws.Width
		return p, nil
	}
	var cmd tea.Cmd
	p.w, cmd = p.w.Update(msg)
	if p.w.done {
		return p, tea.Quit
	}
	return p, cmd
}

func (p wizardProgram) View() string {
	if p.w.done {
		return ""
	}
	return lipgloss.NewStyle().Padding(1, 2).Render(p.w.View())
}

// AskSettings runs the setup wizard. ok=false means the user cancelled.
func AskSettings(s policy.Settings) (policy.Settings, bool, error) {
	m, err := tea.NewProgram(wizardProgram{w: newWizard(s)}).Run()
	if err != nil {
		return s, false, err
	}
	w := m.(wizardProgram).w
	if w.canceled {
		return s, false, nil
	}
	return w.settings, true, nil
}
