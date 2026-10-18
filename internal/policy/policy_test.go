package policy

import (
	"testing"

	"github.com/maromocooo/licgate/internal/model"
)

func graphWith(comps ...*model.Component) *model.Graph {
	g := model.NewGraph()
	g.AddRoot("root", "app")
	for _, c := range comps {
		if c.ID == "" {
			c.ID = "pkg:" + c.Ecosystem + "/" + c.Name + "@1.0.0"
		}
		if c.Version == "" {
			c.Version = "1.0.0"
		}
		if c.Scope == "" {
			c.Scope = model.ScopeRuntime
		}
		g.Components[c.ID] = c
		g.AddEdge("root", c.ID)
	}
	return g
}

func comp(name, lic, eco string) *model.Component {
	return &model.Component{Name: name, License: lic, Ecosystem: eco}
}

func levelOf(t *testing.T, r *Result, name string) Level {
	t.Helper()
	for _, f := range r.Findings {
		if f.Component.Name == name {
			return f.Level
		}
	}
	t.Fatalf("no finding for %s", name)
	return 0
}

func TestMatrix(t *testing.T) {
	cases := []struct {
		out  string
		dist Distribution
		link Linking
		dep  string
		eco  string
		want Level
	}{
		// permissive outbound, binary
		{"MIT", DistBinary, LinkAuto, "MIT", "npm", LevelNotice},
		{"MIT", DistBinary, LinkAuto, "0BSD", "npm", LevelOK},
		{"MIT", DistBinary, LinkAuto, "Apache-2.0", "npm", LevelNotice},
		{"MIT", DistBinary, LinkAuto, "GPL-3.0-only", "npm", LevelBlock},
		{"MIT", DistBinary, LinkAuto, "AGPL-3.0-or-later", "golang", LevelBlock},
		{"MIT", DistBinary, LinkAuto, "LGPL-2.1-only", "pypi", LevelNotice},   // dynamic
		{"MIT", DistBinary, LinkAuto, "LGPL-2.1-only", "golang", LevelReview}, // static
		{"MIT", DistBinary, LinkDynamic, "LGPL-3.0-or-later", "golang", LevelNotice},
		{"MIT", DistBinary, LinkAuto, "MPL-2.0", "golang", LevelNotice},
		{"MIT", DistBinary, LinkAuto, "GPL-2.0-only WITH Classpath-exception-2.0", "maven", LevelNotice},
		{"MIT", DistBinary, LinkAuto, "SSPL-1.0", "npm", LevelBlock},
		{"proprietary", DistBinary, LinkAuto, "BUSL-1.1", "golang", LevelReview},
		{"MIT", DistBinary, LinkAuto, "LicenseRef-licgate-proprietary", "npm", LevelBlock},
		{"MIT", DistBinary, LinkAuto, "LicenseRef-something-custom", "npm", LevelReview},
		{"MIT", DistBinary, LinkAuto, "", "npm", LevelReview},
		// OR picks the best alternative, AND the worst
		{"MIT", DistBinary, LinkAuto, "GPL-3.0-only OR MIT", "npm", LevelNotice},
		{"MIT", DistBinary, LinkAuto, "GPL-3.0-only AND MIT", "npm", LevelBlock},
		// library publishing
		{"MIT", DistLibrary, LinkAuto, "GPL-3.0-only", "npm", LevelReview},
		{"MIT", DistLibrary, LinkAuto, "Apache-2.0", "npm", LevelOK},
		{"GPL-2.0-only", DistLibrary, LinkAuto, "Apache-2.0", "pypi", LevelBlock},
		// SaaS
		{"proprietary", DistSaaS, LinkAuto, "GPL-3.0-only", "golang", LevelOK},
		{"proprietary", DistSaaS, LinkAuto, "GPL-3.0-only", "npm", LevelReview}, // may be served to browsers
		{"proprietary", DistSaaS, LinkAuto, "EUPL-1.2", "golang", LevelReview},
		{"proprietary", DistSaaS, LinkAuto, "AGPL-3.0-only", "npm", LevelBlock},
		{"MIT", DistSaaS, LinkAuto, "AGPL-3.0-only", "npm", LevelReview},
		{"AGPL-3.0-or-later", DistSaaS, LinkAuto, "AGPL-3.0-only", "npm", LevelNotice},
		// internal
		{"proprietary", DistInternal, LinkAuto, "GPL-3.0-only", "golang", LevelOK},
		// GPL outbound
		{"GPL-3.0-or-later", DistBinary, LinkAuto, "Apache-2.0", "golang", LevelNotice},
		{"GPL-2.0-only", DistBinary, LinkAuto, "Apache-2.0", "golang", LevelBlock},
		{"GPL-2.0-only", DistBinary, LinkAuto, "Apache-2.0 WITH LLVM-exception", "cargo", LevelNotice},
		{"GPL-2.0-only", DistBinary, LinkAuto, "GPL-3.0-only", "golang", LevelBlock},
		{"GPL-2.0-or-later", DistBinary, LinkAuto, "GPL-3.0-only", "golang", LevelNotice},
		{"GPL-3.0-only", DistBinary, LinkAuto, "GPL-2.0-only", "golang", LevelBlock},
		{"GPL-3.0-only", DistBinary, LinkAuto, "GPL-2.0-or-later", "golang", LevelNotice},
		{"GPL-3.0-only", DistBinary, LinkAuto, "AGPL-3.0-only", "golang", LevelNotice},
		{"GPL-2.0-only", DistBinary, LinkAuto, "AGPL-3.0-only", "golang", LevelBlock},
		{"GPL-3.0-only", DistBinary, LinkAuto, "EPL-2.0", "maven", LevelBlock},
		{"GPL-3.0-only", DistBinary, LinkAuto, "MPL-2.0", "golang", LevelNotice},
		{"GPL-3.0-only", DistBinary, LinkAuto, "BSD-4-Clause", "golang", LevelBlock},
		{"GPL-3.0-only", DistBinary, LinkAuto, "CDDL-1.0", "maven", LevelBlock},
		{"GPL-3.0-only", DistBinary, LinkAuto, "Elastic-2.0", "npm", LevelBlock},
		// dual-licensed outbound: every alternative must work
		{"MIT OR Apache-2.0", DistBinary, LinkAuto, "GPL-3.0-only", "cargo", LevelBlock},
		{"MIT OR GPL-2.0-only", DistBinary, LinkAuto, "Apache-2.0", "cargo", LevelBlock},
		// review regressions
		{"MIT OR GPL-2.0-only", DistBinary, LinkAuto, "Apache-2.0 OR GPL-2.0-only", "cargo", LevelNotice},
		{"LGPL-2.1-only", DistBinary, LinkAuto, "Apache-2.0", "pypi", LevelReview},
		{"LGPL-3.0-or-later", DistBinary, LinkAuto, "Apache-2.0", "pypi", LevelNotice},
		{"GPL-3.0-only", DistBinary, LinkAuto, "OFL-1.1", "npm", LevelNotice},
		{"GPL-3.0-only", DistBinary, LinkAuto, "LicenseRef-licgate-other-permissive", "npm", LevelReview},
		{"proprietary", DistBinary, LinkAuto, "GPL-2.0-only WITH Font-exception-2.0", "npm", LevelBlock},
		{"MIT", DistBinary, LinkAuto, "BSD-3-Clause-No-Military-License", "golang", LevelReview},
		{"MIT", DistBinary, LinkAuto, "MIT AND LicenseRef-licgate-field-of-use-restriction", "golang", LevelReview},
		// weak outbound behaves like permissive
		{"MPL-2.0", DistBinary, LinkAuto, "GPL-3.0-only", "npm", LevelBlock},
		{"MPL-2.0", DistBinary, LinkAuto, "MIT", "npm", LevelNotice},
	}
	for _, c := range cases {
		g := graphWith(comp("dep", c.dep, c.eco))
		r := Evaluate(g, Settings{License: c.out, Distribution: c.dist, Linking: c.link})
		if got := levelOf(t, r, "dep"); got != c.want {
			f := r.Findings[0]
			t.Errorf("out=%s dist=%s link=%s dep=%q eco=%s: got %s want %s (%v)", c.out, c.dist, c.link, c.dep, c.eco, got, c.want, f.Reasons)
		}
	}
}

func TestDevSkippedUnlessIncluded(t *testing.T) {
	d := comp("devtool", "GPL-3.0-only", "npm")
	d.Scope = model.ScopeDev
	g := graphWith(d)
	if l := levelOf(t, Evaluate(g, Settings{License: "MIT", Distribution: DistBinary}), "devtool"); l != LevelSkip {
		t.Errorf("dev dep: got %s", l)
	}
	if l := levelOf(t, Evaluate(g, Settings{License: "MIT", Distribution: DistBinary, IncludeDev: true}), "devtool"); l != LevelBlock {
		t.Errorf("dev dep with IncludeDev: got %s", l)
	}
}

func TestOverrideIgnoreDenyAllow(t *testing.T) {
	g := graphWith(comp("a", "", "npm"), comp("b", "GPL-3.0-only", "npm"), comp("c", "MIT", "npm"), comp("d", "LicenseRef-weird", "npm"))
	r := Evaluate(g, Settings{
		License: "MIT", Distribution: DistBinary,
		Overrides: []Override{{Package: "a", License: "ISC"}},
		Ignores:   []Ignore{{Package: "npm/b", Reason: "internal"}},
		Deny:      []string{"MIT"},
		Allow:     []string{"LicenseRef-weird"},
	})
	want := map[string]Level{"a": LevelNotice, "b": LevelSkip, "c": LevelBlock, "d": LevelOK}
	for n, w := range want {
		if got := levelOf(t, r, n); got != w {
			t.Errorf("%s: got %s want %s", n, got, w)
		}
	}
}

func TestGPLVersionConflict(t *testing.T) {
	g := graphWith(comp("v2only", "GPL-2.0-only", "golang"), comp("apache", "Apache-2.0", "golang"))
	r := Evaluate(g, Settings{License: "GPL-2.0-or-later", Distribution: DistBinary})
	if l := levelOf(t, r, "v2only"); l != LevelBlock {
		t.Errorf("v2only: got %s", l)
	}
	if len(r.Warnings) == 0 {
		t.Errorf("expected a conflict warning")
	}
}

func TestFileLicenseMismatch(t *testing.T) {
	c := comp("liar", "MIT", "npm")
	c.FileLicense = "GPL-3.0-only"
	r := Evaluate(graphWith(c), Settings{License: "MIT", Distribution: DistBinary})
	if l := levelOf(t, r, "liar"); l != LevelBlock {
		t.Errorf("got %s", l)
	}
}

func TestParseOutbound(t *testing.T) {
	if o := ParseOutbound("proprietary"); !o.Proprietary {
		t.Error("proprietary")
	}
	if o := ParseOutbound("Apache 2.0"); o.Expr != "Apache-2.0" || len(o.Unknown) > 0 {
		t.Errorf("got %+v", o)
	}
	if o := ParseOutbound("WeirdLicense-9"); len(o.Unknown) == 0 {
		t.Errorf("expected unknown: %+v", o)
	}
}

func TestDenyFamily(t *testing.T) {
	g := graphWith(comp("x", "GPL-3.0-or-later", "golang"))
	r := Evaluate(g, Settings{License: "GPL-3.0-or-later", Distribution: DistBinary, Deny: []string{"GPL-3.0"}})
	if l := levelOf(t, r, "x"); l != LevelBlock {
		t.Errorf("deny GPL-3.0 should match GPL-3.0-or-later, got %s", l)
	}
}

func TestOverrideDoesNotMutate(t *testing.T) {
	c := comp("x", "", "npm")
	c.FileLicense = "GPL-3.0-only"
	g := graphWith(c)
	r := Evaluate(g, Settings{License: "MIT", Distribution: DistBinary, Overrides: []Override{{Package: "x", License: "MIT"}}})
	if l := levelOf(t, r, "x"); l != LevelNotice {
		t.Errorf("override should win, got %s", l)
	}
	if c.License != "" || c.FileLicense == "" {
		t.Errorf("graph was mutated: %+v", c)
	}
}
