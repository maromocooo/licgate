package collect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maromocooo/licgate/internal/model"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func byName(g *model.Graph, name string) *model.Component {
	for _, c := range g.Components {
		if c.Name == name {
			return c
		}
	}
	return nil
}

const mitBody = "Permission is hereby granted, free of charge, to any person obtaining a copy of this software. The above copyright notice and this permission notice shall be included in all copies."

func TestNPMLockV3(t *testing.T) {
	d := t.TempDir()
	write(t, d, "package.json", `{"name":"app","version":"1.0.0","dependencies":{"a":"^1"},"devDependencies":{"jest":"^29"},"workspaces":["packages/*"]}`)
	write(t, d, "package-lock.json", `{
  "lockfileVersion": 3,
  "packages": {
    "": {"name":"app","version":"1.0.0"},
    "node_modules/a": {"version":"1.0.0","license":"MIT","dependencies":{"b":"^2"}},
    "node_modules/b": {"version":"2.0.0","license":"(MIT OR Apache-2.0)"},
    "node_modules/a/node_modules/b": {"version":"1.0.0","license":"GPL-3.0"},
    "node_modules/jest": {"version":"29.0.0","license":"MIT","dev":true},
    "node_modules/@scope/lib": {"version":"3.0.0","dev":true},
    "node_modules/wsp": {"resolved":"packages/wsp","link":true},
    "packages/wsp": {"name":"wsp","version":"0.0.1","dependencies":{"b":"^2"}}
  }
}`)
	write(t, d, "node_modules/@scope/lib/package.json", `{"name":"@scope/lib","version":"3.0.0","licenses":[{"type":"BSD-3-Clause"}]}`)
	write(t, d, "node_modules/a/LICENSE", "Copyright (c) 2020 A Author\n\n"+mitBody)
	g, err := collectNPM(d, &Options{Log: func(string, ...any) {}})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range g.Components {
		finalizeLicense(c)
	}
	nested := g.Components["pkg:npm/b@1.0.0"]
	if nested == nil || nested.License != "GPL-3.0-only" || nested.Scope != model.ScopeRuntime {
		t.Fatalf("nested b: %+v", nested)
	}
	if p := g.PathTo(nested.ID); len(p) != 3 || g.Label(p[1]) != "a@1.0.0" {
		t.Errorf("path to nested b: %v", p)
	}
	if b2 := g.Components["pkg:npm/b@2.0.0"]; b2 == nil || b2.License != "MIT OR Apache-2.0" {
		t.Errorf("b@2: %+v", b2)
	}
	if j := byName(g, "jest"); j.Scope != model.ScopeDev {
		t.Errorf("jest scope %s", j.Scope)
	}
	if s := g.Components["pkg:npm/%40scope/lib@3.0.0"]; s == nil || s.License != "BSD-3-Clause" {
		t.Errorf("scoped legacy licenses: %+v", s)
	}
	if a := byName(g, "a"); len(a.Copyrights) != 1 || !a.Direct {
		t.Errorf("a: copyrights %v direct %v", a.Copyrights, a.Direct)
	}
	if byName(g, "wsp") != nil {
		t.Errorf("workspace package must be a root, not a component")
	}
	if len(g.Roots) != 2 {
		t.Errorf("roots: %v", g.Roots)
	}
}

func TestNodeModulesWalkPnpm(t *testing.T) {
	d := t.TempDir()
	write(t, d, "package.json", `{"name":"app","dependencies":{"x":"1"},"devDependencies":{"t":"1"}}`)
	write(t, d, "pnpm-lock.yaml", "lockfileVersion: '9.0'\n")
	write(t, d, "node_modules/.pnpm/x@1.0.0/node_modules/x/package.json", `{"name":"x","version":"1.0.0","license":"MIT","dependencies":{"y":"1"}}`)
	write(t, d, "node_modules/.pnpm/y@1.0.0/node_modules/y/package.json", `{"name":"y","version":"1.0.0","license":"ISC"}`)
	write(t, d, "node_modules/.pnpm/t@1.0.0/node_modules/t/package.json", `{"name":"t","version":"1.0.0","license":"MIT"}`)
	// pnpm links: root node_modules/x -> .pnpm/x…, and x's sibling y
	must(t, os.Symlink(filepath.Join(d, "node_modules/.pnpm/x@1.0.0/node_modules/x"), filepath.Join(d, "node_modules/x")))
	must(t, os.Symlink(filepath.Join(d, "node_modules/.pnpm/t@1.0.0/node_modules/t"), filepath.Join(d, "node_modules/t")))
	must(t, os.Symlink(filepath.Join(d, "node_modules/.pnpm/y@1.0.0/node_modules/y"), filepath.Join(d, "node_modules/.pnpm/x@1.0.0/node_modules/y")))
	g, err := collectNPM(d, &Options{Log: func(string, ...any) {}})
	if err != nil {
		t.Fatal(err)
	}
	if y := byName(g, "y"); y == nil || y.Scope != model.ScopeRuntime {
		t.Fatalf("y should be runtime via x: %+v", y)
	}
	if tt := byName(g, "t"); tt == nil || tt.Scope != model.ScopeDev {
		t.Errorf("t should be dev: %+v", tt)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestPythonEnv(t *testing.T) {
	d := t.TempDir()
	write(t, d, "pyproject.toml", "[project]\nname='app'\ndependencies=['Req-Lib>=1']\n[dependency-groups]\ndev=['pytest']\n")
	sp := ".venv/lib/python3.12/site-packages/"
	write(t, d, sp+"req_lib-1.0.dist-info/METADATA", "Metadata-Version: 2.4\nName: Req_Lib\nVersion: 1.0\nLicense-Expression: Apache-2.0\nRequires-Dist: helper>=1\nRequires-Dist: extra-only; extra == \"fast\"\n\nbody\n")
	write(t, d, sp+"helper-2.0.dist-info/METADATA", "Metadata-Version: 2.1\nName: helper\nVersion: 2.0\nLicense: UNKNOWN\nClassifier: License :: OSI Approved :: GNU Lesser General Public License v3 (LGPLv3)\n\n")
	write(t, d, sp+"extra_only-1.0.dist-info/METADATA", "Name: extra-only\nVersion: 1.0\nLicense: MIT\n\n")
	write(t, d, sp+"pytest-8.0.dist-info/METADATA", "Name: pytest\nVersion: 8.0\nLicense: MIT\n\n")
	write(t, d, sp+"pip-24.0.dist-info/METADATA", "Name: pip\nVersion: 24.0\nLicense: MIT\n\n")
	write(t, d, sp+"lic_file-1.0.dist-info/METADATA", "Name: lic-file\nVersion: 1.0\nLicense-File: LICENSE\n\n")
	write(t, d, sp+"lic_file-1.0.dist-info/licenses/LICENSE", mitBody)
	g, err := collectPython(d, &Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range g.Components {
		finalizeLicense(c)
	}
	check := func(name, lic string, scope model.Scope) {
		t.Helper()
		c := byName(g, name)
		if c == nil {
			t.Fatalf("%s missing", name)
		}
		if c.License != lic || c.Scope != scope {
			t.Errorf("%s: license %q scope %s, want %q %s", name, c.License, c.Scope, lic, scope)
		}
	}
	check("Req_Lib", "Apache-2.0", model.ScopeRuntime)
	check("helper", "LGPL-3.0-only", model.ScopeRuntime)
	check("extra-only", "MIT", model.ScopeUnknown) // only via an extra nobody requested
	check("pytest", "MIT", model.ScopeDev)
	check("pip", "MIT", model.ScopeDev)
	check("lic-file", "MIT", model.ScopeUnknown)

	// Requesting the extra on the root requirement makes it runtime, and
	// requirements files included with -r are followed.
	write(t, d, "pyproject.toml", "[project]\nname='app'\n")
	write(t, d, "requirements.txt", "-r requirements/prod.txt\n")
	write(t, d, "requirements/prod.txt", "req-lib[fast]==1.0  # pinned\n")
	write(t, d, "requirements-dev.txt", "pytest\n")
	g, err = collectPython(d, &Options{})
	if err != nil {
		t.Fatal(err)
	}
	check("extra-only", "MIT", model.ScopeRuntime)
	check("helper", "LGPL-3.0-only", model.ScopeRuntime)
	check("pytest", "MIT", model.ScopeDev)
}

func TestCycloneDXIngest(t *testing.T) {
	doc := `{"bomFormat":"CycloneDX","specVersion":"1.5",
 "metadata":{"component":{"bom-ref":"app","name":"app"}},
 "components":[
  {"bom-ref":"a","name":"a","version":"1.0","purl":"pkg:npm/a@1.0","licenses":[{"license":{"id":"MIT"}}],"scope":"required"},
  {"bom-ref":"g","name":"guava","group":"com.google.guava","version":"33.0","purl":"pkg:maven/com.google.guava/guava@33.0","licenses":[{"expression":"Apache-2.0"}]},
  {"bom-ref":"x","name":"x","version":"2","purl":"pkg:pypi/x@2","licenses":[{"license":{"name":"BSD License"}},{"license":{"id":"GPL-2.0-only"}}],"scope":"excluded"}
 ],
 "dependencies":[{"ref":"app","dependsOn":["a","g"]},{"ref":"a","dependsOn":["x"]}]}`
	g, err := IngestSBOM([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if c := g.Components["pkg:maven/com.google.guava/guava@33.0"]; c == nil || c.Name != "com.google.guava:guava" || c.License != "Apache-2.0" {
		t.Errorf("guava: %+v", c)
	}
	x := g.Components["pkg:pypi/x@2"]
	if x == nil || x.License != "LicenseRef-licgate-BSD-unspecified AND GPL-2.0-only" || x.Scope != model.ScopeDev {
		t.Errorf("x: %+v", x)
	}
	if p := g.PathTo(x.ID); len(p) != 3 {
		t.Errorf("path: %v", p)
	}
}

func TestSPDXIngest(t *testing.T) {
	doc := `{"spdxVersion":"SPDX-2.3","SPDXID":"SPDXRef-DOCUMENT","documentDescribes":["SPDXRef-app"],
 "packages":[
  {"SPDXID":"SPDXRef-app","name":"app","versionInfo":"1"},
  {"SPDXID":"SPDXRef-a","name":"a","versionInfo":"1.0","licenseConcluded":"NOASSERTION","licenseDeclared":"MIT OR LicenseRef-custom",
   "externalRefs":[{"referenceCategory":"PACKAGE-MANAGER","referenceType":"purl","referenceLocator":"pkg:npm/a@1.0"}]},
  {"SPDXID":"SPDXRef-b","name":"b","versionInfo":"2","licenseConcluded":"LicenseRef-b",
   "externalRefs":[{"referenceType":"purl","referenceLocator":"pkg:golang/github.com/x/b@v2"}]}
 ],
 "hasExtractedLicensingInfos":[{"licenseId":"LicenseRef-b","extractedText":"` + mitBody + `"}],
 "relationships":[
  {"spdxElementId":"SPDXRef-app","relationshipType":"DEPENDS_ON","relatedSpdxElement":"SPDXRef-a"},
  {"spdxElementId":"SPDXRef-b","relationshipType":"DEV_DEPENDENCY_OF","relatedSpdxElement":"SPDXRef-app"}
 ]}`
	g, err := IngestSBOM([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if a := g.Components["pkg:npm/a@1.0"]; a == nil || a.License != "MIT OR LicenseRef-custom" {
		t.Errorf("a: %+v", a)
	}
	b := g.Components["pkg:golang/github.com/x/b@v2"]
	if b == nil || b.License != "MIT" || b.Scope != model.ScopeDev || b.Name != "github.com/x/b" {
		t.Errorf("b: %+v", b)
	}
}

func TestComposer(t *testing.T) {
	d := t.TempDir()
	write(t, d, "composer.json", `{"name":"me/app","require":{"monolog/monolog":"^3"},"require-dev":{"phpunit/phpunit":"^10"}}`)
	write(t, d, "composer.lock", `{"packages":[{"name":"monolog/monolog","version":"3.5.0","license":["MIT"],"require":{"psr/log":"^3"}},{"name":"psr/log","version":"3.0.0","license":["MIT"]}],
"packages-dev":[{"name":"phpunit/phpunit","version":"10.0.0","license":["BSD-3-Clause"]}]}`)
	g, err := collectComposer(d, &Options{})
	if err != nil {
		t.Fatal(err)
	}
	if c := byName(g, "phpunit/phpunit"); c == nil || c.Scope != model.ScopeDev {
		t.Errorf("phpunit: %+v", c)
	}
	if c := byName(g, "psr/log"); c == nil || c.License != "MIT" || g.PathTo(c.ID) == nil {
		t.Errorf("psr/log: %+v", c)
	}
}

func TestLicenseFilesDualAndMismatch(t *testing.T) {
	d := t.TempDir()
	write(t, d, "LICENSE-MIT", mitBody)
	write(t, d, "LICENSE-APACHE", "Apache License\nVersion 2.0, January 2004\nhttp://www.apache.org/licenses/")
	write(t, d, "NOTICE", "Some notice")
	c := &model.Component{Name: "dual", Dir: d}
	finalizeLicense(c)
	if c.License != "Apache-2.0 OR MIT" || c.LicenseSource != "license-file" {
		t.Errorf("dual: %q from %s", c.License, c.LicenseSource)
	}
	d2 := t.TempDir()
	write(t, d2, "COPYING", "GNU GENERAL PUBLIC LICENSE\nVersion 3, 29 June 2007\n"+strings.Repeat("body text ", 500))
	c2 := &model.Component{Name: "mismatch", Dir: d2, License: "MIT"}
	finalizeLicense(c2)
	if c2.FileLicense != "GPL-3.0-only" {
		t.Errorf("mismatch: %+v", c2)
	}
}

func TestPURL(t *testing.T) {
	if got := purlFor("npm", "@types/node", "20.0.0"); got != "pkg:npm/%40types/node@20.0.0" {
		t.Errorf("got %s", got)
	}
	p, ok := parsePURL("pkg:npm/%40types/node@20.0.0?arch=x")
	if !ok || p.namespace != "@types" || p.name != "node" || p.version != "20.0.0" {
		t.Errorf("parse: %+v", p)
	}
	c := componentFromPURL("pkg:npm/%40types/node@20.0.0", "", "")
	if c.ID != "pkg:npm/%40types/node@20.0.0" || c.Name != "@types/node" {
		t.Errorf("component: %+v", c)
	}
}

func TestReviewRegressions(t *testing.T) {
	// H4: partly unknown declaration is kept.
	c := &model.Component{Name: "a"}
	declare(c, "x", "MIT AND Acme-Noncommercial")
	if !hasUnknownLeaf(c.License) {
		t.Errorf("unknown part dropped: %q", c.License)
	}
	// H5: a second, stricter license file.
	d := t.TempDir()
	write(t, d, "LICENSE", mitBody)
	write(t, d, "COPYING", "GNU GENERAL PUBLIC LICENSE\nVersion 3, 29 June 2007\n"+strings.Repeat("x ", 3000))
	c2 := &model.Component{Name: "b", Dir: d, License: "MIT"}
	finalizeLicense(c2)
	if c2.FileLicense == "" {
		t.Errorf("stricter COPYING ignored")
	}
	// H8: merging keeps the wider scope.
	g1, g2 := model.NewGraph(), model.NewGraph()
	g1.Components["x"] = &model.Component{ID: "x", Scope: model.ScopeDev}
	g2.Components["x"] = &model.Component{ID: "x", Scope: model.ScopeRuntime}
	g1.Merge(g2)
	if g1.Components["x"].Scope != model.ScopeRuntime {
		t.Errorf("merge scope: %s", g1.Components["x"].Scope)
	}
	// H9: SPDX dev relation does not hide a runtime path.
	doc := `{"spdxVersion":"SPDX-2.3","documentDescribes":["SPDXRef-root"],"packages":[
 {"SPDXID":"SPDXRef-root","name":"root"},
 {"SPDXID":"SPDXRef-a","name":"a","versionInfo":"1","licenseConcluded":"MIT","externalRefs":[{"referenceType":"purl","referenceLocator":"pkg:npm/a@1"}]},
 {"SPDXID":"SPDXRef-x","name":"x","versionInfo":"1","licenseConcluded":"GPL-3.0-only","externalRefs":[{"referenceType":"purl","referenceLocator":"pkg:npm/x@1"}]}],
 "relationships":[
 {"spdxElementId":"SPDXRef-x","relationshipType":"DEV_DEPENDENCY_OF","relatedSpdxElement":"SPDXRef-root"},
 {"spdxElementId":"SPDXRef-root","relationshipType":"DEPENDS_ON","relatedSpdxElement":"SPDXRef-a"},
 {"spdxElementId":"SPDXRef-a","relationshipType":"DEPENDS_ON","relatedSpdxElement":"SPDXRef-x"}]}`
	g, err := IngestSBOM([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if x := g.Components["pkg:npm/x@1"]; x.Scope != model.ScopeRuntime {
		t.Errorf("spdx x scope %s", x.Scope)
	}
	// H7: devOptional can ship.
	d2 := t.TempDir()
	write(t, d2, "package.json", `{"name":"app","dependencies":{"a":"1"}}`)
	write(t, d2, "package-lock.json", `{"lockfileVersion":3,"packages":{"":{},"node_modules/a":{"version":"1","license":"MIT","optionalDependencies":{"o":"1"}},"node_modules/o":{"version":"1","license":"GPL-3.0","devOptional":true,"optional":false}}}`)
	ng, err := collectNPM(d2, &Options{Log: func(string, ...any) {}})
	if err != nil {
		t.Fatal(err)
	}
	if o := byName(ng, "o"); o.Scope == model.ScopeDev {
		t.Errorf("devOptional treated as dev")
	}
}
