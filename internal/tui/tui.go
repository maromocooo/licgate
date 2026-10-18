// Package tui is the interactive terminal UI: a setup wizard and a
// browser for the results with dependency paths and export.
package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/licgate/licgate/internal/config"
	"github.com/licgate/licgate/internal/i18n"
	"github.com/licgate/licgate/internal/policy"
	"github.com/licgate/licgate/internal/report"
)

// Options configures Run.
type Options struct {
	Result      *policy.Result
	Meta        report.Meta
	FailOn      policy.Level
	Dir         string
	ConfigPath  string
	ConfigFound bool
	Reevaluate  func(policy.Settings) *policy.Result
	Write       func(format, path string, r *policy.Result, m report.Meta) error
}

type mode int

const (
	modeList mode = iota
	modeDetail
	modeSettings
	modeExport
	modeHelp
)

var filters = []policy.Level{-1, policy.LevelBlock, policy.LevelReview, policy.LevelNotice, policy.LevelOK, policy.LevelSkip}

type exportItem struct {
	format, file, label string
}

func exportItems() []exportItem {
	return []exportItem{
		{"html", "licgate-report.html", i18n.T("t.ex_html")},
		{"md", "licgate-report.md", i18n.T("t.ex_md")},
		{"json", "licgate-report.json", i18n.T("t.ex_json")},
		{"notice", "THIRD_PARTY_NOTICES.md", i18n.T("t.ex_notice")},
		{"cyclonedx", "bom.cdx.json", i18n.T("t.ex_sbom")},
		{"config", config.FileName, i18n.T("t.ex_config")},
	}
}

type model struct {
	o       Options
	res     *policy.Result
	meta    report.Meta
	mode    mode
	filter  int
	search  textinput.Model
	srch    bool
	visible []*policy.Finding
	cursor  int
	offset  int
	width   int
	height  int
	vp      viewport.Model
	wiz     wizard
	exCur   int
	status  string
	statusT time.Time
}

func newModel(o Options) model {
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.Placeholder = i18n.T("t.search_ph")
	ti.CharLimit = 80
	m := model{o: o, res: o.Result, meta: o.Meta, search: ti, vp: viewport.New(40, 10)}
	// Start on the most severe non-empty filter so problems are first.
	m.filter = 0
	m.refilter()
	return m
}

func (m *model) refilter() {
	q := strings.ToLower(strings.TrimSpace(m.search.Value()))
	lv := filters[m.filter]
	m.visible = m.visible[:0]
	for _, f := range m.res.Findings {
		if lv >= 0 && f.Level != lv {
			continue
		}
		if q != "" {
			c := f.Component
			hay := strings.ToLower(c.Name + " " + c.Version + " " + c.License + " " + c.Ecosystem)
			if !strings.Contains(hay, q) {
				continue
			}
		}
		m.visible = append(m.visible, f)
	}
	if m.cursor >= len(m.visible) {
		m.cursor = max(0, len(m.visible)-1)
	}
	m.clampOffset()
	m.updateDetail()
}

func (m *model) listHeight() int {
	h := m.height - 6 // header(2) + rule + search + footer + status
	if m.wide() {
		return max(3, h)
	}
	return max(3, h)
}

func (m *model) wide() bool { return m.width >= 110 }

func (m *model) clampOffset() {
	h := m.listHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *model) detailWidth() int {
	if m.wide() {
		return m.width - m.listWidth() - 3
	}
	return m.width - 2
}

func (m *model) listWidth() int {
	if m.wide() {
		return m.width * 52 / 100
	}
	return m.width
}

func (m *model) updateDetail() {
	m.vp.Width = max(20, m.detailWidth())
	if m.wide() {
		m.vp.Height = m.listHeight()
	} else {
		m.vp.Height = max(3, m.height-4)
	}
	if len(m.visible) == 0 {
		m.vp.SetContent("")
		return
	}
	m.vp.SetContent(renderDetail(m.visible[m.cursor], m.vp.Width))
	m.vp.GotoTop()
}

func (m model) Init() tea.Cmd { return nil }

func (m *model) setStatus(s string) {
	m.status = s
	m.statusT = time.Now()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.wiz.width = msg.Width
		m.clampOffset()
		m.updateDetail()
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		switch m.mode {
		case modeSettings:
			var cmd tea.Cmd
			m.wiz, cmd = m.wiz.Update(msg)
			if m.wiz.done {
				m.mode = modeList
				if !m.wiz.canceled {
					m.apply(m.wiz.settings)
				}
			}
			return m, cmd
		case modeExport:
			return m.updateExport(msg)
		case modeHelp:
			m.mode = modeList
			return m, nil
		case modeDetail:
			switch msg.String() {
			case "esc", "enter", "q", "left", "h", "backspace":
				m.mode = modeList
				return m, nil
			}
			var cmd tea.Cmd
			m.vp, cmd = m.vp.Update(msg)
			return m, cmd
		}
		if m.srch {
			switch msg.String() {
			case "enter", "esc":
				m.srch = false
				m.search.Blur()
				if msg.String() == "esc" {
					m.search.SetValue("")
					m.refilter()
				}
				return m, nil
			}
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(msg)
			m.cursor = 0
			m.refilter()
			return m, cmd
		}
		switch msg.String() {
		case "q", "esc":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.clampOffset()
				m.updateDetail()
			}
		case "down", "j":
			if m.cursor < len(m.visible)-1 {
				m.cursor++
				m.clampOffset()
				m.updateDetail()
			}
		case "pgup", "ctrl+b":
			m.cursor = max(0, m.cursor-m.listHeight())
			m.clampOffset()
			m.updateDetail()
		case "pgdown", "ctrl+f", " ":
			m.cursor = min(max(0, len(m.visible)-1), m.cursor+m.listHeight())
			m.clampOffset()
			m.updateDetail()
		case "g", "home":
			m.cursor = 0
			m.clampOffset()
			m.updateDetail()
		case "G", "end":
			m.cursor = max(0, len(m.visible)-1)
			m.clampOffset()
			m.updateDetail()
		case "tab", "right", "l":
			m.filter = (m.filter + 1) % len(filters)
			m.cursor = 0
			m.refilter()
		case "shift+tab", "left", "h":
			m.filter = (m.filter + len(filters) - 1) % len(filters)
			m.cursor = 0
			m.refilter()
		case "1", "2", "3", "4", "5", "0":
			m.filter = int(msg.String()[0] - '0')
			m.cursor = 0
			m.refilter()
		case "/":
			m.srch = true
			m.search.Focus()
			return m, textinput.Blink
		case "enter":
			if !m.wide() && len(m.visible) > 0 {
				m.mode = modeDetail
			}
		case "J", "ctrl+d":
			m.vp.HalfPageDown()
		case "K", "ctrl+u":
			m.vp.HalfPageUp()
		case "s":
			m.wiz = newWizard(m.res.Settings)
			m.wiz.width = m.width
			m.mode = modeSettings
		case "e", "w":
			m.mode = modeExport
			m.exCur = 0
		case "?":
			m.mode = modeHelp
		}
	}
	return m, nil
}

func (m *model) apply(s policy.Settings) {
	if m.o.Reevaluate == nil {
		return
	}
	m.res = m.o.Reevaluate(s)
	m.meta.Failed = m.res.Worst() >= m.o.FailOn
	m.cursor, m.offset = 0, 0
	m.refilter()
	m.setStatus(i18n.T("t.reevaluated", m.res.Outbound.Expr, string(s.Distribution)))
}

func (m model) updateExport(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := exportItems()
	switch msg.String() {
	case "esc", "q", "left", "h":
		m.mode = modeList
	case "up", "k":
		if m.exCur > 0 {
			m.exCur--
		}
	case "down", "j":
		if m.exCur < len(items)-1 {
			m.exCur++
		}
	case "enter", "right", "l":
		it := items[m.exCur]
		p := filepath.Join(m.o.Dir, it.file)
		var err error
		if it.format == "config" {
			p = m.o.ConfigPath
			err = writeConfig(p, m.res.Settings, m.meta.FailOn)
		} else {
			err = m.o.Write(it.format, p, m.res, m.meta)
		}
		if err != nil {
			m.setStatus("✗ " + err.Error())
		} else {
			m.setStatus("✓ " + i18n.T("t.wrote", p))
		}
		m.mode = modeList
	}
	return m, nil
}

// writeConfig writes .licgate.toml, keeping a backup of an existing file
// because the template drops custom overrides.
func writeConfig(p string, s policy.Settings, failOn string) error {
	if b, err := os.ReadFile(p); err == nil {
		if cur, err := config.Load(p); err == nil && cur != nil && (len(cur.Override) > 0 || len(cur.Ignore) > 0 || len(cur.Allow) > 0 || len(cur.Deny) > 0) {
			return fmt.Errorf("%s", i18n.T("t.config_has_custom", p))
		}
		_ = os.WriteFile(p+".bak", b, 0o644)
	}
	c := config.Config{License: s.License, Distribution: string(s.Distribution), Linking: string(s.Linking), IncludeDev: s.IncludeDev, FailOn: failOn}
	if c.FailOn == "" {
		c.FailOn = "block"
	}
	return os.WriteFile(p, config.Template(c), 0o644)
}

// ---- view ---------------------------------------------------------------------

var (
	dimS  = lipgloss.NewStyle().Foreground(report.ColorDim)
	boldS = lipgloss.NewStyle().Bold(true)
	selS  = lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{Light: "#E8E6E0", Dark: "#2E2E2B"})
)

func (m model) View() string {
	if m.width == 0 {
		return ""
	}
	switch m.mode {
	case modeSettings:
		return lipgloss.NewStyle().Padding(1, 2).Render(m.wiz.View())
	case modeHelp:
		return lipgloss.NewStyle().Padding(1, 2).Render(helpText())
	case modeDetail:
		head := m.header()
		return head + "\n" + m.vp.View() + "\n" + dimS.Render(" ↑↓ "+i18n.T("t.k_scroll")+" · esc "+i18n.T("w.k_back"))
	}
	var b strings.Builder
	b.WriteString(m.header() + "\n")
	// filter tabs + search
	var tabs []string
	for i, lv := range filters {
		name := i18n.T("t.all")
		n := len(m.res.Findings)
		if lv >= 0 {
			name = lv.String()
			n = m.res.Counts[lv.String()]
		}
		label := fmt.Sprintf("%s %d", name, n)
		st := lipgloss.NewStyle().Padding(0, 1)
		if lv >= 0 {
			st = st.Foreground(report.LevelColor(lv))
		}
		if i == m.filter {
			st = st.Bold(true).Reverse(true)
		}
		tabs = append(tabs, st.Render(label))
	}
	line := strings.Join(tabs, " ")
	if m.srch || m.search.Value() != "" {
		line += "  " + m.search.View()
	}
	b.WriteString(ansi.Truncate(line, m.width, "…") + "\n")

	list := m.renderList()
	if m.wide() {
		sep := dimS.Render(strings.Repeat("│\n", m.listHeight()-1) + "│")
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, list, " "+sep+" ", m.vp.View()))
	} else {
		b.WriteString(list)
	}
	b.WriteString("\n")
	if m.mode == modeExport {
		return m.exportOverlay(b.String())
	}
	b.WriteString(m.footer())
	return b.String()
}

func (m model) header() string {
	s := m.res.Settings
	status := lipgloss.NewStyle().Bold(true).Padding(0, 1).Foreground(lipgloss.Color("#ffffff")).Background(lipgloss.Color("#2E7D32")).Render(i18n.T("rep.pass"))
	if m.meta.Failed {
		status = lipgloss.NewStyle().Bold(true).Padding(0, 1).Foreground(lipgloss.Color("#ffffff")).Background(lipgloss.Color("#C62828")).Render(i18n.T("rep.fail"))
	}
	out := m.res.Outbound.Expr
	left := boldS.Render("licgate") + " " + m.meta.Project
	right := fmt.Sprintf("%s · %s · %s", out, s.Distribution, s.Linking)
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 1
	l1 := left + strings.Repeat(" ", max(1, gap)) + dimS.Render(right)
	l2 := status + " " + report.Verdict(m.res)
	return ansi.Truncate(l1, m.width, "…") + "\n" + ansi.Truncate(l2, m.width, "…")
}

func (m model) renderList() string {
	w := m.listWidth()
	h := m.listHeight()
	var lines []string
	if len(m.visible) == 0 {
		lines = append(lines, dimS.Render("  "+i18n.T("t.empty")))
	}
	for i := m.offset; i < len(m.visible) && i < m.offset+h; i++ {
		f := m.visible[i]
		c := f.Component
		badge := lipgloss.NewStyle().Foreground(report.LevelColor(f.Level)).Bold(true).Width(7).Render(f.Level.String())
		lic := report.LicenseLabel(c)
		licW := min(28, max(10, w/3))
		nameW := w - 7 - licW - 3
		name := ansi.Truncate(c.Name+" "+dimS.Render(c.Version), max(8, nameW), "…")
		name = name + strings.Repeat(" ", max(0, nameW-lipgloss.Width(name)))
		row := badge + name + " " + ansi.Truncate(lic, licW, "…")
		row = row + strings.Repeat(" ", max(0, w-lipgloss.Width(row)))
		if i == m.cursor {
			row = selS.Render(row)
		}
		lines = append(lines, row)
	}
	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", w))
	}
	return strings.Join(lines, "\n")
}

func (m model) footer() string {
	if m.status != "" && time.Since(m.statusT) < 8*time.Second {
		return ansi.Truncate(m.status, m.width, "…")
	}
	keys := "↑↓ " + i18n.T("t.k_move") + " · tab " + i18n.T("t.k_filter") + " · / " + i18n.T("t.k_search")
	if !m.wide() {
		keys += " · enter " + i18n.T("t.k_detail")
	}
	keys += " · s " + i18n.T("t.k_settings") + " · e " + i18n.T("t.k_export") + " · ? · q " + i18n.T("w.k_quit")
	return dimS.Render(ansi.Truncate(keys, m.width, "…"))
}

func (m model) exportOverlay(base string) string {
	var b strings.Builder
	b.WriteString(boldS.Render(i18n.T("t.export_title")) + "\n\n")
	for i, it := range exportItems() {
		p := it.file
		if it.format == "config" {
			p = filepath.Base(m.o.ConfigPath)
		}
		pad := max(1, 24-lipgloss.Width(it.label))
		line := it.label + strings.Repeat(" ", pad) + dimS.Render(p)
		if i == m.exCur {
			line = lipgloss.NewStyle().Bold(true).Foreground(report.ColorNotice).Render("› ") + line
		} else {
			line = "  " + line
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n" + dimS.Render(i18n.T("t.export_dir", m.o.Dir)) + "\n")
	b.WriteString(dimS.Render("enter " + i18n.T("w.k_confirm") + " · esc " + i18n.T("w.k_back")))
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).Render(b.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func renderDetail(f *policy.Finding, width int) string {
	c := f.Component
	var b strings.Builder
	sec := func(t string) { b.WriteString("\n" + dimS.Render(t) + "\n") }
	b.WriteString(lipgloss.NewStyle().Foreground(report.LevelColor(f.Level)).Bold(true).Render(f.Level.String()) + "  " + boldS.Render(c.Key()) + dimS.Render(" ("+c.Ecosystem+")") + "\n")
	b.WriteString(report.LicenseLabel(c))
	if f.Chosen != "" {
		b.WriteString(dimS.Render("  → " + f.Chosen))
	}
	b.WriteString("\n")
	scope := string(c.Scope)
	if c.Direct {
		scope += " · " + i18n.T("rep.direct")
	} else {
		scope += " · " + i18n.T("rep.transitive")
	}
	b.WriteString(dimS.Render(scope) + "\n")
	if len(f.Reasons) > 0 {
		sec(i18n.T("rep.reasons"))
		for _, r := range f.Reasons {
			b.WriteString("• " + wrap(r, width-2, "  ") + "\n")
		}
	}
	if len(f.Obligations) > 0 {
		sec(i18n.T("rep.obligations"))
		for _, o := range f.Obligations {
			b.WriteString("☐ " + wrap(policy.ObligationText(o), width-2, "  ") + "\n")
		}
	}
	if len(f.Path) > 0 {
		sec(i18n.T("rep.path"))
		for i, p := range f.Path {
			prefix := strings.Repeat("  ", i)
			if i > 0 {
				prefix = strings.Repeat("  ", i-1) + "└ "
			}
			b.WriteString(ansi.Truncate(prefix+p, width, "…") + "\n")
		}
	}
	if len(c.Declared) > 0 || c.LicenseSource != "" {
		sec(i18n.T("rep.declared"))
		b.WriteString(wrap(strings.Join(c.Declared, " | "), width, "") + "\n")
		if c.LicenseSource != "" {
			b.WriteString(dimS.Render(i18n.T("rep.source")+": "+c.LicenseSource) + "\n")
		}
	}
	if len(c.LicenseFiles) > 0 {
		sec(i18n.T("rep.files"))
		for _, lf := range c.LicenseFiles {
			s := lf.Path
			if lf.DetectedID != "" {
				s += fmt.Sprintf(" → %s (%.0f%%)", lf.DetectedID, lf.Confidence*100)
			} else if lf.IsNotice {
				s += " (NOTICE)"
			}
			b.WriteString(ansi.Truncate(s, width, "…") + "\n")
		}
	}
	if len(c.Copyrights) > 0 {
		sec("Copyright")
		for i, cr := range c.Copyrights {
			if i == 3 {
				b.WriteString(dimS.Render(fmt.Sprintf("… +%d", len(c.Copyrights)-3)) + "\n")
				break
			}
			b.WriteString(ansi.Truncate(cr, width, "…") + "\n")
		}
	}
	if c.Homepage != "" {
		sec("URL")
		b.WriteString(ansi.Truncate(c.Homepage, width, "…") + "\n")
	}
	return b.String()
}

// wrap word-wraps s to width using display width (CJK aware).
func wrap(s string, width int, indent string) string {
	if width < 10 {
		width = 10
	}
	var w string
	if hasWide(s) {
		// CJK text has no spaces to break at: hard-wrap by display width.
		w = ansi.Hardwrap(s, width, false)
	} else {
		w = lipgloss.NewStyle().Width(width).Render(s)
	}
	lines := strings.Split(w, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
		if i > 0 {
			lines[i] = indent + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

func hasWide(s string) bool {
	for _, r := range s {
		if r >= 0x2E80 {
			return true
		}
	}
	return false
}

func helpText() string {
	rows := [][2]string{
		{"↑↓ / j k", i18n.T("t.k_move")},
		{"tab / ← →  0-5", i18n.T("t.k_filter")},
		{"/", i18n.T("t.k_search")},
		{"enter", i18n.T("t.k_detail")},
		{"J K", i18n.T("t.k_scroll")},
		{"s", i18n.T("t.k_settings")},
		{"e", i18n.T("t.k_export")},
		{"q", i18n.T("w.k_quit")},
	}
	var b strings.Builder
	b.WriteString(boldS.Render("licgate") + "\n\n")
	for _, r := range rows {
		b.WriteString(fmt.Sprintf("  %-16s %s\n", r[0], r[1]))
	}
	b.WriteString("\n" + dimS.Render(i18n.T("t.levels_help")) + "\n\n" + dimS.Render(i18n.T("rep.disclaimer")))
	return b.String()
}

// Run starts the interactive browser and returns the process exit code.
func Run(o Options) (int, error) {
	m, err := tea.NewProgram(newModel(o), tea.WithAltScreen()).Run()
	if err != nil {
		return 2, err
	}
	fm := m.(model)
	// Leave a summary in the scrollback.
	fmt.Println(boldS.Render("licgate") + " " + fm.meta.Project + " — " + fm.res.Outbound.Expr + " / " + string(fm.res.Settings.Distribution))
	for _, lv := range report.Levels {
		fmt.Printf("%s %d  ", lipgloss.NewStyle().Foreground(report.LevelColor(lv)).Bold(true).Render(lv.String()), fm.res.Counts[lv.String()])
	}
	fmt.Println()
	fmt.Println(report.Verdict(fm.res))
	if fm.meta.Failed {
		return 1, nil
	}
	return 0, nil
}
