package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	lm "github.com/maromocooo/licgate/internal/model"
	"github.com/maromocooo/licgate/internal/policy"
	"github.com/maromocooo/licgate/internal/report"
)

func sampleGraph() *lm.Graph {
	g := lm.NewGraph()
	g.AddRoot("root", "app")
	add := func(name, lic string) {
		c := &lm.Component{ID: "pkg:npm/" + name + "@1.0.0", Name: name, Version: "1.0.0", Ecosystem: "npm", License: lic, Scope: lm.ScopeRuntime}
		g.Components[c.ID] = c
		g.AddEdge("root", c.ID)
	}
	add("left-pad", "MIT")
	add("gpl-thing", "GPL-3.0-only")
	add("mystery", "")
	return g
}

func key(s string) tea.KeyMsg {
	switch s {
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestBrowserFlow(t *testing.T) {
	g := sampleGraph()
	s := policy.Settings{License: "MIT", Distribution: policy.DistBinary, Linking: policy.LinkAuto}
	res := policy.Evaluate(g, s)
	var m tea.Model = newModel(Options{
		Result: res, Meta: report.Meta{Project: "app", GeneratedAt: time.Now(), Failed: true}, FailOn: policy.LevelBlock,
		Dir: t.TempDir(), ConfigPath: t.TempDir() + "/.licgate.toml",
		Reevaluate: func(s policy.Settings) *policy.Result { return policy.Evaluate(g, s) },
		Write:      func(string, string, *policy.Result, report.Meta) error { return nil },
	})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	v := m.View()
	for _, want := range []string{"gpl-thing", "BLOCK", "left-pad", "GPL-3.0-only"} {
		if !strings.Contains(v, want) {
			t.Errorf("wide view missing %q", want)
		}
	}
	// filter to OK only (tab cycles ALL -> BLOCK -> REVIEW -> NOTICE -> OK)
	for i := 0; i < 4; i++ {
		m, _ = m.Update(key("tab"))
	}
	if v := m.View(); strings.Contains(v, "gpl-thing") {
		t.Errorf("OK filter still shows gpl-thing")
	}
	m, _ = m.Update(key("0"))
	// search
	m, _ = m.Update(key("/"))
	for _, r := range "myst" {
		m, _ = m.Update(key(string(r)))
	}
	m, _ = m.Update(key("enter"))
	if v := m.View(); !strings.Contains(v, "mystery") || strings.Contains(v, "left-pad") {
		t.Errorf("search did not filter")
	}
	// change settings via wizard: SaaS makes GPL fine
	m, _ = m.Update(key("s"))
	m, _ = m.Update(key("enter")) // keep license (MIT)
	m, _ = m.Update(key("down"))
	m, _ = m.Update(key("down"))
	m, _ = m.Update(key("enter")) // saas
	m, _ = m.Update(key("enter")) // linking auto
	m, _ = m.Update(key("enter")) // dev: no
	mm := m.(model)
	if mm.res.Settings.Distribution != policy.DistSaaS {
		t.Fatalf("wizard did not apply: %+v", mm.res.Settings)
	}
	if mm.res.Counts["BLOCK"] != 0 {
		t.Errorf("SaaS should have no BLOCK, got %v", mm.res.Counts)
	}
	// narrow layout + detail view
	m, _ = m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m, _ = m.Update(key("esc")) // clear search? esc quits in list mode; ensure it returns a quit cmd
}

func TestNarrowDetail(t *testing.T) {
	g := sampleGraph()
	res := policy.Evaluate(g, policy.Settings{License: "MIT", Distribution: policy.DistBinary})
	var m tea.Model = newModel(Options{Result: res, Meta: report.Meta{Project: "app"}, Dir: t.TempDir()})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m, _ = m.Update(key("enter"))
	v := m.View()
	if !strings.Contains(v, "GPL") || !strings.Contains(v, "app") {
		t.Errorf("detail view: %s", v)
	}
	for _, line := range strings.Split(v, "\n") {
		if w := len([]rune(stripANSI(line))); w > 60 {
			t.Errorf("line wider than terminal (%d): %q", w, stripANSI(line))
		}
	}
}

func TestWizardOtherLicense(t *testing.T) {
	w := newWizard(policy.Settings{})
	for i := 0; i < len(licenseChoices())-1; i++ {
		w, _ = w.Update(key("down"))
	}
	w, _ = w.Update(key("enter"))
	if !w.typing {
		t.Fatal("expected text input")
	}
	for _, r := range "EUPL-1.2" {
		w, _ = w.Update(key(string(r)))
	}
	w, _ = w.Update(key("enter"))
	if w.settings.License != "EUPL-1.2" || w.step != 1 {
		t.Errorf("got %+v step %d", w.settings, w.step)
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			in = true
		case in && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'):
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return b.String()
}
