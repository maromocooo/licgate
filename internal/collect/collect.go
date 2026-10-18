// Package collect builds a dependency graph for a project: natively for
// npm, Python, Go, Cargo and Composer, from CycloneDX/SPDX SBOMs, or via
// syft for everything else.
package collect

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/licgate/licgate/internal/i18n"
	"github.com/licgate/licgate/internal/license"
	"github.com/licgate/licgate/internal/model"
)

// Options controls collection.
type Options struct {
	Path      string
	SBOMs     []string
	Generator string // auto | native | syft
	Online    bool
	MaxDepth  int // how deep to look for nested projects (monorepos)
	Log       func(format string, args ...any)
}

type collector struct {
	name   string
	detect func(dir string) bool
	run    func(dir string, o *Options) (*model.Graph, error)
}

var collectors = []collector{
	{"npm", detectNPM, collectNPM},
	{"python", detectPython, collectPython},
	{"go", detectGo, collectGo},
	{"cargo", detectCargo, collectCargo},
	{"composer", detectComposer, collectComposer},
}

// nativeEcosystems are covered by built-in collectors; syft output for them
// is only used to fill gaps.
var nativeEcosystems = map[string]bool{"npm": true, "pypi": true, "golang": true, "cargo": true, "composer": true}

var skipDirs = map[string]bool{
	"node_modules": true, ".git": true, "vendor": true, ".venv": true, "venv": true, "target": true,
	"dist": true, "build": true, ".tox": true, "__pycache__": true, ".next": true, ".cache": true,
	".idea": true, ".vscode": true, "testdata": true, "fixtures": true, ".pnpm-store": true, ".yarn": true,
}

// Run collects the dependency graph.
func Run(o Options) (*model.Graph, error) {
	if o.Log == nil {
		o.Log = func(string, ...any) {}
	}
	if o.MaxDepth == 0 {
		o.MaxDepth = 3
	}
	abs, err := filepath.Abs(o.Path)
	if err != nil {
		return nil, err
	}
	o.Path = abs
	g := model.NewGraph()

	if len(o.SBOMs) > 0 {
		for _, p := range o.SBOMs {
			sg, err := IngestSBOMFile(p)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", p, err)
			}
			g.Merge(sg)
		}
	} else {
		if o.Generator != "syft" {
			dirs := projectDirs(abs, o.MaxDepth)
			for _, d := range dirs {
				for _, c := range collectors {
					if !c.detect(d) {
						continue
					}
					rel, _ := filepath.Rel(abs, d)
					o.Log("%s: %s", c.name, rel)
					sg, err := c.run(d, &o)
					if err != nil {
						g.Errors = append(g.Errors, fmt.Sprintf("%s (%s): %v", c.name, rel, err))
						continue
					}
					g.Merge(sg)
				}
			}
		}
		if o.Generator == "syft" || o.Generator == "auto" || o.Generator == "" {
			if path, err := exec.LookPath("syft"); err == nil {
				o.Log("syft: %s", path)
				sg, err := runSyft(path, abs)
				if err != nil && o.Generator == "syft" {
					g.Errors = append(g.Errors, "syft: "+err.Error())
				} else if err != nil {
					g.Warnings = append(g.Warnings, "syft: "+err.Error())
				} else {
					mergeSyft(g, sg, o.Generator == "syft")
				}
			} else if o.Generator == "syft" {
				return nil, fmt.Errorf("syft not found in PATH (install: https://github.com/anchore/syft)")
			}
		}
	}

	if o.Online {
		enrichOnline(g, o.Log)
	}
	for _, c := range g.Components {
		finalizeLicense(c)
	}
	if len(g.Components) == 0 && len(g.Warnings) == 0 {
		g.Warnings = append(g.Warnings, "no dependencies found (supported natively: npm, Python, Go, Cargo, Composer; others via syft or --sbom)")
	}
	return g, nil
}

// mergeSyft merges syft results. Unless syftOnly, components of ecosystems
// we already collected natively are only used to fill in missing licenses.
func mergeSyft(g, sg *model.Graph, syftOnly bool) {
	have := map[string]bool{}
	for _, c := range g.Components {
		have[c.Ecosystem] = true
	}
	filtered := model.NewGraph()
	for _, r := range sg.Roots {
		filtered.AddRoot(r, sg.RootNames[r])
	}
	for id, c := range sg.Components {
		if !syftOnly && nativeEcosystems[c.Ecosystem] && have[c.Ecosystem] {
			if ex, ok := g.Components[id]; ok && ex.License == "" && c.License != "" {
				ex.License, ex.LicenseSource, ex.Declared = c.License, "syft", c.Declared
			}
			continue
		}
		filtered.Components[id] = c
	}
	for p, kids := range sg.Edges {
		for _, k := range kids {
			if _, ok := filtered.Components[k]; ok {
				filtered.AddEdge(p, k)
			}
		}
	}
	filtered.Sources = sg.Sources
	filtered.Warnings = sg.Warnings
	g.Merge(filtered)
}

func projectDirs(root string, maxDepth int) []string {
	var dirs []string
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		depth := 0
		if rel != "." {
			depth = strings.Count(rel, string(filepath.Separator)) + 1
			if skipDirs[d.Name()] || (strings.HasPrefix(d.Name(), ".") && d.Name() != ".") {
				return filepath.SkipDir
			}
		}
		if depth > maxDepth {
			return filepath.SkipDir
		}
		dirs = append(dirs, p)
		return nil
	})
	return dirs
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// ---- components -----------------------------------------------------------

func purlFor(eco, name, version string) string {
	typ := eco
	ns, n := "", name
	switch eco {
	case "npm":
		if strings.HasPrefix(name, "@") {
			if i := strings.Index(name, "/"); i > 0 {
				ns, n = "%40"+name[1:i], name[i+1:]
			}
		}
	case "pypi":
		n = pep503(name)
	case "golang", "composer", "maven":
		if i := strings.LastIndex(name, "/"); i > 0 {
			ns, n = name[:i], name[i+1:]
		}
	}
	var b strings.Builder
	b.WriteString("pkg:" + typ + "/")
	if ns != "" {
		b.WriteString(ns + "/")
	}
	b.WriteString(url.PathEscape(n))
	if version != "" {
		b.WriteString("@" + url.PathEscape(version))
	}
	return b.String()
}

func newComponent(eco, name, version string) *model.Component {
	p := purlFor(eco, name, version)
	return &model.Component{ID: p, PURL: p, Name: name, Version: version, Ecosystem: eco, Scope: model.ScopeUnknown}
}

var pep503Re = regexp.MustCompile(`[-_.]+`)

func pep503(name string) string { return strings.ToLower(pep503Re.ReplaceAllString(name, "-")) }

// declare records raw declarations and sets License if they normalise.
func declare(c *model.Component, source string, raws ...string) {
	var exprs []string
	for _, r := range raws {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		c.Declared = appendUniq(c.Declared, r)
		n := license.Normalize(r)
		if n.SeeFile != "" && c.Dir != "" {
			if b, err := os.ReadFile(filepath.Join(c.Dir, n.SeeFile)); err == nil {
				d := license.DetectText(string(b))
				if d.ID != "" && d.Confidence >= 0.8 {
					exprs = append(exprs, d.ID)
					continue
				}
			}
			continue
		}
		// Keep partly or wholly unrecognised declarations: an unknown part
		// ("MIT AND Acme-Noncommercial") may be exactly the restriction
		// that matters. Unknown leaves become REVIEW in the policy.
		if n.Expr != "" {
			exprs = append(exprs, n.Expr)
		}
	}
	if len(exprs) == 0 || c.License != "" {
		return
	}
	if len(exprs) == 1 {
		c.License = exprs[0]
	} else {
		// Several declarations (legacy "licenses" arrays) mean "any of".
		for i, e := range exprs {
			if strings.Contains(e, " ") {
				exprs[i] = "(" + e + ")"
			}
		}
		c.License = strings.Join(uniq(exprs), " OR ")
	}
	c.LicenseSource = source
}

func appendUniq(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

func uniq(s []string) []string {
	var out []string
	for _, v := range s {
		out = appendUniq(out, v)
	}
	return out
}

var licenseFileRe = regexp.MustCompile(`(?i)^(licen[cs]e|copying|unlicense|notice|copyright|legal|licen[cs]e[-_.].+|copying[-_.].+|notice[-_.].+|mit-license|apache-license)(\.(md|txt|rst|markdown|html))?$`)

// scanLicenseFiles reads license/notice files directly inside dir (and in a
// "licenses" or "LICENSES" subdirectory).
func scanLicenseFiles(dir string) []model.LicenseFile {
	var out []model.LicenseFile
	add := func(d string, rel string) {
		ents, err := os.ReadDir(d)
		if err != nil {
			return
		}
		for _, e := range ents {
			if e.IsDir() || !licenseFileRe.MatchString(e.Name()) {
				continue
			}
			full := filepath.Join(d, e.Name())
			st, err := os.Stat(full)
			if err != nil || st.Size() > 512*1024 {
				continue
			}
			b, err := os.ReadFile(full)
			if err != nil {
				continue
			}
			lf := model.LicenseFile{Path: filepath.ToSlash(filepath.Join(rel, e.Name())), Text: string(b)}
			lf.IsNotice = strings.HasPrefix(strings.ToLower(e.Name()), "notice")
			if !lf.IsNotice {
				det := license.DetectText(lf.Text)
				lf.DetectedID, lf.Confidence = det.ID, det.Confidence
			}
			out = append(out, lf)
		}
	}
	add(dir, "")
	for _, sub := range []string{"licenses", "LICENSES", "license"} {
		if isDir(filepath.Join(dir, sub)) {
			add(filepath.Join(dir, sub), sub)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

var dualNameRe = regexp.MustCompile(`(?i)^licen[cs]e[-_.](mit|apache|apache-2\.0|apache2|bsd|isc|unlicense|zlib|mpl|boost|bsl|cc0)`)

// licenseFromFiles derives an expression from detected license files.
func licenseFromFiles(files []model.LicenseFile) (expr string, ambiguousVersion bool) {
	var ids []string
	allDualNamed := true
	for _, f := range files {
		if f.IsNotice || f.DetectedID == "" || f.Confidence < 0.8 {
			continue
		}
		ids = appendUniq(ids, f.DetectedID)
		if !dualNameRe.MatchString(filepath.Base(f.Path)) {
			allDualNamed = false
		}
		if strings.HasSuffix(f.DetectedID, "-only") && strings.Contains(f.DetectedID, "GPL") {
			if d := license.DetectText(f.Text); d.VersionAmbiguous {
				ambiguousVersion = true
			}
		}
	}
	switch len(ids) {
	case 0:
		return "", false
	case 1:
		return ids[0], ambiguousVersion
	}
	op := " AND "
	if allDualNamed {
		op = " OR "
	}
	for i, id := range ids {
		if strings.Contains(id, " ") {
			ids[i] = "(" + id + ")"
		}
	}
	return strings.Join(ids, op), ambiguousVersion
}

// finalizeLicense fills License from license files when metadata had none,
// and records disagreements between metadata and files.
func finalizeLicense(c *model.Component) {
	if len(c.LicenseFiles) == 0 && c.Dir != "" {
		c.LicenseFiles = scanLicenseFiles(c.Dir)
	}
	for _, f := range c.LicenseFiles {
		if f.IsNotice {
			continue
		}
		for _, cr := range license.ExtractCopyrights(f.Text) {
			c.Copyrights = appendUniq(c.Copyrights, cr)
		}
	}
	fileExpr, amb := licenseFromFiles(c.LicenseFiles)
	if c.License == "" && len(c.LicenseFiles) == 0 && c.Dir != "" {
		if hint := readmeLicense(c.Dir); hint != "" {
			c.LicenseNotes = appendUniq(c.LicenseNotes, i18n.T("c.readme_says", hint))
		}
	}
	if fileExpr == "" {
		for _, f := range c.LicenseFiles {
			if !f.IsNotice && f.DetectedID != "" && f.Confidence < 0.8 {
				c.LicenseNotes = appendUniq(c.LicenseNotes, i18n.T("c.low_confidence", f.Path, license.DisplayName(f.DetectedID)))
			}
		}
	}
	// Metadata only said "GPL"/"BSD"/"LGPL" but the license file is precise.
	if c.License != "" && fileExpr != "" && !strings.Contains(fileExpr, " ") {
		if mi, ok := license.Lookup(c.License); ok && mi.Ambiguous {
			if fi, ok := license.Lookup(fileExpr); ok && fi.Category == mi.Category && !fi.Ambiguous {
				c.License, c.LicenseSource = fileExpr, "license-file"
				if amb {
					c.LicenseNotes = append(c.LicenseNotes, noteGPLVersion())
				}
				return
			}
		}
	}
	if c.License == "" {
		if fileExpr != "" {
			c.License = fileExpr
			c.LicenseSource = "license-file"
			if amb {
				c.LicenseNotes = append(c.LicenseNotes, noteGPLVersion())
			}
		}
		return
	}
	// Metadata says X but a bundled license file adds something stricter
	// (another file is GPL, a Commons Clause is appended, …).
	if fileExpr != "" && fileStricter(fileExpr, c.License) {
		c.FileLicense = fileExpr
	}
	if fileExpr != "" && hasUnknownLeaf(c.License) && !hasUnknownLeaf(fileExpr) {
		c.LicenseNotes = appendUniq(c.LicenseNotes, i18n.T("c.file_suggests", fileExpr))
	}
}

func leafRank(id string) int {
	info, ok := license.Lookup(id)
	if !ok {
		return 6
	}
	switch info.Category {
	case license.CatPublicDomain:
		return 0
	case license.CatPermissive:
		return 1
	case license.CatWeakCopyleft:
		return 2
	case license.CatStrongCopyleft:
		return 3
	case license.CatNetworkCopyleft:
		return 4
	}
	return 5
}

func leafFamilies(expr string) []string {
	e, err := license.Parse(expr)
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range license.Leaves(e) {
		out = append(out, l.ID)
	}
	return out
}

func hasUnknownLeaf(expr string) bool {
	for _, id := range leafFamilies(expr) {
		if !license.IsKnown(id) {
			return true
		}
	}
	return false
}

// fileStricter: does the file expression contain a license that the
// metadata does not mention and that is stricter than everything the
// metadata mentions?
func fileStricter(fileExpr, meta string) bool {
	metaIDs := leafFamilies(meta)
	maxMeta := -1
	for _, id := range metaIDs {
		if r := leafRank(id); r > maxMeta {
			maxMeta = r
		}
	}
	for _, id := range leafFamilies(fileExpr) {
		known := false
		for _, m := range metaIDs {
			if sameFamily(id, m) {
				known = true
			}
		}
		if !known && leafRank(id) > maxMeta {
			return true
		}
	}
	return false
}

func sameFamily(a, b string) bool {
	trim := func(s string) string {
		s = strings.TrimSuffix(s, "-only")
		s = strings.TrimSuffix(s, "-or-later")
		return s
	}
	return trim(a) == trim(b)
}

func fromClassifiers(cls []string) string { return license.FromClassifiers(cls).Expr }

func licenseFileFromText(p, text string) model.LicenseFile {
	lf := model.LicenseFile{Path: filepath.ToSlash(p), Text: text}
	lf.IsNotice = strings.HasPrefix(strings.ToLower(filepath.Base(p)), "notice")
	if !lf.IsNotice {
		d := license.DetectText(text)
		lf.DetectedID, lf.Confidence = d.ID, d.Confidence
	}
	return lf
}

var readmeLicenseRe = regexp.MustCompile(`(?is)#+\s*licen[cs]e\s*\n+(.{1,200})`)

// readmeLicense returns what a README's "License" section says, if it is
// a short recognisable declaration. Used only as a hint: a README is not a
// license grant, so the package still needs a human check.
func readmeLicense(dir string) string {
	for _, n := range []string{"README.md", "README", "README.markdown", "readme.md", "README.rst"} {
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			continue
		}
		m := readmeLicenseRe.FindSubmatch(b)
		if m == nil {
			continue
		}
		first := strings.TrimSpace(strings.SplitN(strings.TrimSpace(string(m[1])), "\n", 2)[0])
		if norm := license.Normalize(first); norm.OK() {
			return norm.Expr
		}
		if d := license.DetectText(string(m[1])); d.ID != "" {
			return d.ID
		}
	}
	return ""
}
