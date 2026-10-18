package collect

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/licgate/licgate/internal/i18n"
	"github.com/licgate/licgate/internal/model"
)

func detectPython(dir string) bool {
	for _, f := range []string{"pyproject.toml", "requirements.txt", "setup.py", "setup.cfg", "uv.lock", "poetry.lock", "Pipfile.lock"} {
		if exists(filepath.Join(dir, f)) {
			return true
		}
	}
	return false
}

var reqNameRe = regexp.MustCompile(`^\s*([A-Za-z0-9][A-Za-z0-9._-]*)`)

// reqName extracts the distribution name from a PEP 508 requirement and
// reports whether it only applies to an extra.
func reqName(req string) (string, bool) {
	name := ""
	if m := reqNameRe.FindStringSubmatch(req); m != nil {
		name = pep503(m[1])
	}
	extraOnly := false
	if i := strings.Index(req, ";"); i >= 0 {
		marker := req[i+1:]
		if regexp.MustCompile(`\bextra\s*==`).MatchString(marker) {
			extraOnly = true
		}
	}
	return name, extraOnly
}

type pyRootDeps struct {
	runtime []string
	dev     []string
	name    string
	found   bool
}

func readPyRootDeps(dir string) pyRootDeps {
	var r pyRootDeps
	add := func(dst *[]string, reqs ...string) {
		for _, q := range reqs {
			if n, _ := reqName(q); n != "" && n != "python" {
				*dst = appendUniq(*dst, n)
			}
		}
	}
	if b, err := os.ReadFile(filepath.Join(dir, "pyproject.toml")); err == nil {
		var pp struct {
			Project struct {
				Name                 string              `toml:"name"`
				Dependencies         []string            `toml:"dependencies"`
				OptionalDependencies map[string][]string `toml:"optional-dependencies"`
			} `toml:"project"`
			DependencyGroups map[string][]any `toml:"dependency-groups"`
			Tool             struct {
				Poetry struct {
					Name            string                    `toml:"name"`
					Dependencies    map[string]any            `toml:"dependencies"`
					DevDependencies map[string]any            `toml:"dev-dependencies"`
					Group           map[string]map[string]any `toml:"group"`
				} `toml:"poetry"`
				UV struct {
					DevDependencies []string `toml:"dev-dependencies"`
				} `toml:"uv"`
			} `toml:"tool"`
		}
		if _, err := toml.Decode(string(b), &pp); err == nil {
			r.found = true
			r.name = pp.Project.Name
			if r.name == "" {
				r.name = pp.Tool.Poetry.Name
			}
			add(&r.runtime, pp.Project.Dependencies...)
			// Extras are opt-in for users, but they are shipped features:
			// treat them as runtime.
			for _, reqs := range pp.Project.OptionalDependencies {
				add(&r.runtime, reqs...)
			}
			for _, items := range pp.DependencyGroups {
				for _, it := range items {
					if s, ok := it.(string); ok {
						add(&r.dev, s)
					}
				}
			}
			for n := range pp.Tool.Poetry.Dependencies {
				add(&r.runtime, n)
			}
			for n := range pp.Tool.Poetry.DevDependencies {
				add(&r.dev, n)
			}
			for gname, grp := range pp.Tool.Poetry.Group {
				deps, _ := grp["dependencies"].(map[string]any)
				for n := range deps {
					if gname == "main" {
						add(&r.runtime, n)
					} else {
						add(&r.dev, n)
					}
				}
			}
			add(&r.dev, pp.Tool.UV.DevDependencies...)
		}
	}
	readReq := func(name string, dst *[]string) {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			return
		}
		defer f.Close()
		r.found = true
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
				continue
			}
			add(dst, line)
		}
	}
	readReq("requirements.txt", &r.runtime)
	readReq("requirements.in", &r.runtime)
	for _, n := range []string{"requirements-dev.txt", "dev-requirements.txt", "requirements_dev.txt", "requirements-test.txt", "test-requirements.txt", filepath.Join("requirements", "dev.txt"), filepath.Join("requirements", "test.txt")} {
		readReq(n, &r.dev)
	}
	return r
}

func findSitePackages(dir string) []string {
	var cands []string
	if v := os.Getenv("VIRTUAL_ENV"); v != "" {
		if rel, err := filepath.Rel(dir, v); err == nil && !strings.HasPrefix(rel, "..") {
			cands = append(cands, v)
		}
	}
	for _, n := range []string{".venv", "venv", "env", ".env"} {
		cands = append(cands, filepath.Join(dir, n))
	}
	var out []string
	for _, c := range cands {
		if !isDir(c) {
			continue
		}
		m, _ := filepath.Glob(filepath.Join(c, "lib", "python*", "site-packages"))
		m2, _ := filepath.Glob(filepath.Join(c, "Lib", "site-packages"))
		out = append(out, m...)
		out = append(out, m2...)
		if len(out) > 0 {
			break
		}
	}
	return out
}

type pyDist struct {
	name, version string
	licExpr       string
	licText       string
	classifiers   []string
	requires      []string
	licenseFiles  []string
	homepage      string
	dir           string // dist-info dir
}

func parseMetadata(p string) (*pyDist, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	d := &pyDist{dir: filepath.Dir(p)}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	var lastKey string
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			break // body starts
		}
		if (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) && lastKey == "License" {
			d.licText += "\n" + strings.TrimSpace(line)
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		lastKey = k
		switch k {
		case "Name":
			d.name = v
		case "Version":
			d.version = v
		case "License-Expression":
			d.licExpr = v
		case "License":
			d.licText = v
		case "Classifier":
			d.classifiers = append(d.classifiers, v)
		case "Requires-Dist":
			d.requires = append(d.requires, v)
		case "License-File":
			d.licenseFiles = append(d.licenseFiles, v)
		case "Home-page":
			d.homepage = v
		case "Project-URL":
			if d.homepage == "" {
				if _, u, ok := strings.Cut(v, ","); ok {
					d.homepage = strings.TrimSpace(u)
				}
			}
		}
	}
	return d, nil
}

func collectPython(dir string, o *Options) (*model.Graph, error) {
	roots := readPyRootDeps(dir)
	sites := findSitePackages(dir)
	if len(sites) > 0 {
		return collectPythonEnv(dir, sites, roots)
	}
	if exists(filepath.Join(dir, "uv.lock")) {
		return collectUVLock(dir, roots, o)
	}
	if exists(filepath.Join(dir, "poetry.lock")) {
		return collectPoetryLock(dir, roots, o)
	}
	if exists(filepath.Join(dir, "requirements.txt")) {
		return collectRequirementsPins(dir, o)
	}
	if !roots.found || len(roots.runtime)+len(roots.dev) == 0 {
		return model.NewGraph(), nil
	}
	return nil, fmt.Errorf("no virtualenv (.venv/venv) or lockfile found: create the environment (e.g. `uv sync`) or use --online / --sbom")
}

func pyRootID(dir string) string { return "root:pypi:" + filepath.ToSlash(dir) }

func collectPythonEnv(dir string, sites []string, roots pyRootDeps) (*model.Graph, error) {
	g := model.NewGraph()
	g.Sources = append(g.Sources, "python:site-packages")
	rootName := roots.name
	if rootName == "" {
		rootName = filepath.Base(dir)
	}
	rootID := pyRootID(dir)
	g.AddRoot(rootID, rootName)
	byName := map[string]*model.Component{}
	reqs := map[string][]string{}
	for _, sp := range sites {
		infos, _ := filepath.Glob(filepath.Join(sp, "*.dist-info", "METADATA"))
		eggs, _ := filepath.Glob(filepath.Join(sp, "*.egg-info", "PKG-INFO"))
		for _, mp := range append(infos, eggs...) {
			d, err := parseMetadata(mp)
			if err != nil || d.name == "" {
				continue
			}
			key := pep503(d.name)
			if key == pep503(rootName) {
				continue // the project itself installed in editable mode
			}
			if _, dup := byName[key]; dup {
				continue
			}
			c := newComponent("pypi", d.name, d.version)
			c.Dir = d.dir
			c.Homepage = d.homepage
			pythonLicense(c, d)
			byName[key] = c
			g.Components[c.ID] = c
			for _, r := range d.requires {
				if n, extraOnly := reqName(r); n != "" && !extraOnly {
					reqs[key] = appendUniq(reqs[key], n)
				}
			}
		}
	}
	for from, tos := range reqs {
		for _, t := range tos {
			if tc, ok := byName[t]; ok {
				g.AddEdge(byName[from].ID, tc.ID)
			}
		}
	}
	var rt, dv []string
	for _, n := range roots.runtime {
		if c, ok := byName[n]; ok {
			g.AddEdge(rootID, c.ID)
			c.Direct = true
			rt = append(rt, c.ID)
		}
	}
	for _, n := range roots.dev {
		if c, ok := byName[n]; ok {
			g.AddEdge(rootID, c.ID)
			c.Direct = true
			dv = append(dv, c.ID)
		}
	}
	applyReachScopes(g, rt, dv, roots.found && len(rt)+len(dv) > 0)
	return g, nil
}

// applyReachScopes sets scopes from reachability. When the root's
// dependencies are known, packages reachable from neither set are tools
// that happen to be installed (pip, setuptools…) and are treated as dev.
func applyReachScopes(g *model.Graph, rt, dv []string, rootsKnown bool) {
	rtSet := g.ReachableFrom(rt)
	dvSet := g.ReachableFrom(dv)
	for id, c := range g.Components {
		switch {
		case rtSet[id]:
			c.Scope = model.ScopeRuntime
		case dvSet[id]:
			c.Scope = model.ScopeDev
		case rootsKnown:
			c.Scope = model.ScopeDev
			c.LicenseNotes = append(c.LicenseNotes, i18n.T("c.unreachable"))
		}
	}
}

func pythonLicense(c *model.Component, d *pyDist) {
	if d.licExpr != "" {
		declare(c, "metadata", d.licExpr)
	}
	if c.License == "" && d.licText != "" {
		declare(c, "metadata", d.licText)
	}
	if c.License == "" {
		if n := fromClassifiers(d.classifiers); n != "" {
			c.License, c.LicenseSource = n, "classifier"
			for _, cl := range d.classifiers {
				if strings.HasPrefix(cl, "License ::") {
					c.Declared = appendUniq(c.Declared, cl)
				}
			}
		}
	}
	// License files: PEP 639 puts them under .dist-info/licenses/, older
	// wheels directly in .dist-info/.
	var files []model.LicenseFile
	files = append(files, scanLicenseFiles(d.dir)...)
	for _, lf := range d.licenseFiles {
		for _, base := range []string{filepath.Join(d.dir, "licenses"), d.dir} {
			p := filepath.Join(base, filepath.FromSlash(lf))
			if b, err := os.ReadFile(p); err == nil {
				already := false
				for _, f := range files {
					if filepath.Base(f.Path) == filepath.Base(lf) {
						already = true
					}
				}
				if !already {
					files = append(files, licenseFileFromText(lf, string(b)))
				}
				break
			}
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	c.LicenseFiles = files
}

// ---- lockfiles without an environment ---------------------------------------

func collectUVLock(dir string, roots pyRootDeps, o *Options) (*model.Graph, error) {
	var lock struct {
		Package []struct {
			Name         string         `toml:"name"`
			Version      string         `toml:"version"`
			Source       map[string]any `toml:"source"`
			Dependencies []struct {
				Name string `toml:"name"`
			} `toml:"dependencies"`
			OptionalDependencies map[string][]struct {
				Name string `toml:"name"`
			} `toml:"optional-dependencies"`
			DevDependencies map[string][]struct {
				Name string `toml:"name"`
			} `toml:"dev-dependencies"`
		} `toml:"package"`
	}
	if _, err := toml.DecodeFile(filepath.Join(dir, "uv.lock"), &lock); err != nil {
		return nil, fmt.Errorf("uv.lock: %w", err)
	}
	g := model.NewGraph()
	g.Sources = append(g.Sources, "python:uv.lock")
	byName := map[string]string{}
	local := map[string]bool{}
	for _, p := range lock.Package {
		key := pep503(p.Name)
		if _, ok := p.Source["editable"]; ok {
			local[key] = true
		}
		if _, ok := p.Source["virtual"]; ok {
			local[key] = true
		}
		if local[key] {
			id := "root:pypi:" + filepath.ToSlash(dir) + ":" + key
			g.AddRoot(id, p.Name)
			byName[key] = id
			continue
		}
		c := newComponent("pypi", p.Name, p.Version)
		g.Components[c.ID] = c
		byName[key] = c.ID
	}
	var rt, dv []string
	for _, p := range lock.Package {
		from := byName[pep503(p.Name)]
		for _, d := range p.Dependencies {
			to := byName[pep503(d.Name)]
			g.AddEdge(from, to)
			if local[pep503(p.Name)] {
				rt = append(rt, to)
			}
		}
		for _, ds := range p.OptionalDependencies {
			for _, d := range ds {
				to := byName[pep503(d.Name)]
				g.AddEdge(from, to)
				if local[pep503(p.Name)] {
					rt = append(rt, to)
				}
			}
		}
		for _, ds := range p.DevDependencies {
			for _, d := range ds {
				to := byName[pep503(d.Name)]
				g.AddEdge(from, to)
				if local[pep503(p.Name)] {
					dv = append(dv, to)
				}
			}
		}
	}
	markDirect(g)
	applyReachScopes(g, rt, dv, len(rt)+len(dv) > 0)
	if !o.Online {
		g.Warnings = append(g.Warnings, "python (uv.lock): no virtualenv found, licenses are unknown without --online (or run `uv sync` first)")
	}
	return g, nil
}

func collectPoetryLock(dir string, roots pyRootDeps, o *Options) (*model.Graph, error) {
	var lock struct {
		Package []struct {
			Name         string         `toml:"name"`
			Version      string         `toml:"version"`
			Category     string         `toml:"category"`
			Dependencies map[string]any `toml:"dependencies"`
		} `toml:"package"`
	}
	if _, err := toml.DecodeFile(filepath.Join(dir, "poetry.lock"), &lock); err != nil {
		return nil, fmt.Errorf("poetry.lock: %w", err)
	}
	g := model.NewGraph()
	g.Sources = append(g.Sources, "python:poetry.lock")
	rootID := pyRootID(dir)
	name := roots.name
	if name == "" {
		name = filepath.Base(dir)
	}
	g.AddRoot(rootID, name)
	byName := map[string]string{}
	for _, p := range lock.Package {
		c := newComponent("pypi", p.Name, p.Version)
		if p.Category == "dev" {
			c.Scope = model.ScopeDev
		}
		g.Components[c.ID] = c
		byName[pep503(p.Name)] = c.ID
	}
	for _, p := range lock.Package {
		for d := range p.Dependencies {
			g.AddEdge(byName[pep503(p.Name)], byName[pep503(d)])
		}
	}
	var rt, dv []string
	for _, n := range roots.runtime {
		if id, ok := byName[n]; ok {
			g.AddEdge(rootID, id)
			rt = append(rt, id)
		}
	}
	for _, n := range roots.dev {
		if id, ok := byName[n]; ok {
			g.AddEdge(rootID, id)
			dv = append(dv, id)
		}
	}
	markDirect(g)
	applyReachScopes(g, rt, dv, len(rt)+len(dv) > 0)
	if !o.Online {
		g.Warnings = append(g.Warnings, "python (poetry.lock): no virtualenv found, licenses are unknown without --online (or run `poetry install` first)")
	}
	return g, nil
}

var pinRe = regexp.MustCompile(`^\s*([A-Za-z0-9][A-Za-z0-9._-]*)(\[[^\]]*\])?\s*==\s*([^\s;#\\]+)`)

func collectRequirementsPins(dir string, o *Options) (*model.Graph, error) {
	f, err := os.Open(filepath.Join(dir, "requirements.txt"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	g := model.NewGraph()
	g.Sources = append(g.Sources, "python:requirements.txt")
	rootID := pyRootID(dir)
	g.AddRoot(rootID, filepath.Base(dir))
	sc := bufio.NewScanner(f)
	unpinned := 0
	for sc.Scan() {
		line := sc.Text()
		if m := pinRe.FindStringSubmatch(line); m != nil {
			c := newComponent("pypi", m[1], m[3])
			c.Scope = model.ScopeRuntime
			g.Components[c.ID] = c
			g.AddEdge(rootID, c.ID)
			c.Direct = true
		} else if n, _ := reqName(line); n != "" && !strings.HasPrefix(strings.TrimSpace(line), "#") && !strings.HasPrefix(strings.TrimSpace(line), "-") {
			unpinned++
		}
	}
	if unpinned > 0 {
		g.Warnings = append(g.Warnings, fmt.Sprintf("python (requirements.txt): %d unpinned requirement(s) skipped and transitive dependencies are unknown — create a virtualenv for a complete picture", unpinned))
	}
	if !o.Online {
		g.Warnings = append(g.Warnings, "python (requirements.txt): no virtualenv found, licenses are unknown without --online")
	}
	return g, nil
}

func markDirect(g *model.Graph) {
	for _, r := range g.Roots {
		for _, ch := range g.Edges[r] {
			if c, ok := g.Components[ch]; ok {
				c.Direct = true
			}
		}
	}
}
