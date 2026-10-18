package collect

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/licgate/licgate/internal/model"
)

func detectNPM(dir string) bool { return exists(filepath.Join(dir, "package.json")) }

type pkgJSON struct {
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	License              json.RawMessage   `json:"license"`
	Licenses             json.RawMessage   `json:"licenses"`
	Dependencies         map[string]string `json:"dependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	PeerDependencies     map[string]string `json:"peerDependencies"`
	Homepage             string            `json:"homepage"`
	Private              bool              `json:"private"`
}

// npmLicenses extracts license strings from package.json "license" (string
// or legacy {type}) and legacy "licenses" arrays.
func npmLicenses(lic, lics json.RawMessage) []string {
	var out []string
	parse := func(raw json.RawMessage) {
		if len(raw) == 0 {
			return
		}
		var s string
		if json.Unmarshal(raw, &s) == nil {
			out = append(out, s)
			return
		}
		var o struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(raw, &o) == nil && o.Type != "" {
			out = append(out, o.Type)
			return
		}
		var arr []json.RawMessage
		if json.Unmarshal(raw, &arr) == nil {
			for _, a := range arr {
				if json.Unmarshal(a, &s) == nil {
					out = append(out, s)
				} else if json.Unmarshal(a, &o) == nil && o.Type != "" {
					out = append(out, o.Type)
				}
			}
		}
	}
	parse(lic)
	parse(lics)
	return out
}

func readPkgJSON(p string) (*pkgJSON, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var pj pkgJSON
	if err := json.Unmarshal(b, &pj); err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	return &pj, nil
}

// npmEntry is one installed package location ("node_modules/a/node_modules/b").
type npmEntry struct {
	loc      string
	name     string
	version  string
	licenses []string
	deps     []string // dependency names (runtime + optional + peer)
	devDeps  []string // only used for roots
	dev      bool
	optional bool
	hasFlags bool // dev/optional flags are authoritative (lockfile v2+)
	link     bool
	homepage string
	isRoot   bool
}

func sortedKeys(maps ...map[string]string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range maps {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	sort.Strings(out)
	return out
}

func collectNPM(dir string, o *Options) (*model.Graph, error) {
	root, err := readPkgJSON(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil, err
	}
	var entries map[string]*npmEntry
	source := ""
	for _, lf := range []string{"npm-shrinkwrap.json", "package-lock.json"} {
		if exists(filepath.Join(dir, lf)) {
			entries, err = readNPMLock(filepath.Join(dir, lf))
			if err != nil {
				return nil, err
			}
			source = lf
			break
		}
	}
	if entries == nil {
		if !isDir(filepath.Join(dir, "node_modules")) {
			if exists(filepath.Join(dir, "yarn.lock")) || exists(filepath.Join(dir, "pnpm-lock.yaml")) || exists(filepath.Join(dir, "bun.lockb")) || exists(filepath.Join(dir, "bun.lock")) {
				return nil, fmt.Errorf("found a yarn/pnpm/bun lockfile but no node_modules: run your package manager's install first, or pass an SBOM with --sbom")
			}
			if len(root.Dependencies)+len(root.DevDependencies)+len(root.OptionalDependencies) == 0 {
				return model.NewGraph(), nil
			}
			return nil, fmt.Errorf("no package-lock.json or node_modules: run `npm install` first, or pass an SBOM with --sbom")
		}
		entries = walkNodeModules(dir)
		source = "node_modules"
	}
	// The root entry always comes from package.json.
	rootName := root.Name
	if rootName == "" {
		rootName = filepath.Base(dir)
	}
	entries[""] = &npmEntry{
		loc: "", name: rootName, version: root.Version, isRoot: true,
		deps:    sortedKeys(root.Dependencies, root.OptionalDependencies, root.PeerDependencies),
		devDeps: sortedKeys(root.DevDependencies),
	}

	g := model.NewGraph()
	g.Sources = append(g.Sources, "npm:"+source)
	rootID := "root:npm:" + filepath.ToSlash(dir)
	g.AddRoot(rootID, rootName)

	idOf := map[string]string{} // loc -> component ID (or root ID)
	idOf[""] = rootID
	for loc, e := range entries {
		if loc == "" {
			continue
		}
		if e.link {
			continue // resolved to its target below
		}
		if !strings.Contains(loc, "node_modules/") {
			// Workspace package (the user's own code).
			wid := "root:npm:" + filepath.ToSlash(filepath.Join(dir, loc))
			idOf[loc] = wid
			g.AddRoot(wid, e.name)
			continue
		}
		c := newComponent("npm", e.name, e.version)
		if ex, ok := g.Components[c.ID]; ok {
			idOf[loc] = ex.ID
			if e.hasFlags && !e.dev && ex.Scope == model.ScopeDev {
				ex.Scope = model.ScopeRuntime
			}
			continue
		}
		c.Dir = filepath.Join(dir, filepath.FromSlash(loc))
		if !isDir(c.Dir) {
			c.Dir = ""
		}
		c.Homepage = e.homepage
		lics := e.licenses
		if len(lics) == 0 && c.Dir != "" {
			if pj, err := readPkgJSON(filepath.Join(c.Dir, "package.json")); err == nil {
				lics = npmLicenses(pj.License, pj.Licenses)
				if c.Homepage == "" {
					c.Homepage = pj.Homepage
				}
			}
		}
		declare(c, source, lics...)
		if e.hasFlags {
			switch {
			case e.dev:
				c.Scope = model.ScopeDev
			case e.optional:
				c.Scope = model.ScopeOptional
			default:
				c.Scope = model.ScopeRuntime
			}
		}
		g.Components[c.ID] = c
		idOf[loc] = c.ID
	}
	for loc, e := range entries {
		if e.link {
			if t := linkTarget(entries, loc); t != "" {
				idOf[loc] = idOf[t]
			}
		}
	}
	for loc, e := range entries {
		if e.link {
			continue
		}
		from := idOf[loc]
		if from == "" {
			continue
		}
		for _, dn := range e.deps {
			if to := resolveNode(entries, loc, dn); to != "" {
				g.AddEdge(from, idOf[to])
			}
		}
		for _, dn := range e.devDeps {
			if to := resolveNode(entries, loc, dn); to != "" {
				g.AddEdge(from, idOf[to])
			}
		}
	}
	// Direct flags.
	for _, r := range g.Roots {
		for _, ch := range g.Edges[r] {
			if c, ok := g.Components[ch]; ok {
				c.Direct = true
			}
		}
	}
	// Without lockfile flags, derive scopes from reachability.
	if source == "node_modules" {
		var runtimeStarts, devStarts []string
		for loc, e := range entries {
			if !e.isRoot {
				continue
			}
			for _, dn := range e.deps {
				if to := resolveNode(entries, loc, dn); to != "" {
					runtimeStarts = append(runtimeStarts, idOf[to])
				}
			}
			for _, dn := range e.devDeps {
				if to := resolveNode(entries, loc, dn); to != "" {
					devStarts = append(devStarts, idOf[to])
				}
			}
		}
		rt := g.ReachableFrom(runtimeStarts)
		dv := g.ReachableFrom(devStarts)
		for id, c := range g.Components {
			switch {
			case rt[id]:
				c.Scope = model.ScopeRuntime
			case dv[id]:
				c.Scope = model.ScopeDev
			}
		}
	}
	return g, nil
}

// resolveNode implements Node's lookup: walk up from loc through every
// node_modules directory.
func resolveNode(entries map[string]*npmEntry, loc, name string) string {
	cur := loc
	for {
		var cand string
		if cur == "" {
			cand = "node_modules/" + name
		} else {
			cand = cur + "/node_modules/" + name
		}
		if e, ok := entries[cand]; ok {
			if e.link {
				return linkTarget(entries, cand)
			}
			return cand
		}
		if cur == "" {
			return ""
		}
		// Move to the parent package location.
		i := strings.LastIndex(cur, "/node_modules/")
		if i < 0 {
			cur = ""
			continue
		}
		cur = cur[:i]
	}
}

// link entries in lockfile v2 store their target in "resolved" which we
// keep in the version field.
func linkTarget(entries map[string]*npmEntry, loc string) string {
	e := entries[loc]
	t := path.Clean(e.version)
	if _, ok := entries[t]; ok {
		return t
	}
	return ""
}

type lockPkg struct {
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	Resolved             string            `json:"resolved"`
	License              json.RawMessage   `json:"license"`
	Dev                  bool              `json:"dev"`
	Optional             bool              `json:"optional"`
	DevOptional          bool              `json:"devOptional"`
	Link                 bool              `json:"link"`
	Dependencies         map[string]string `json:"dependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	PeerDependencies     map[string]string `json:"peerDependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
}

type lockV1Dep struct {
	Version      string                `json:"version"`
	Dev          bool                  `json:"dev"`
	Optional     bool                  `json:"optional"`
	Requires     map[string]string     `json:"requires"`
	Dependencies map[string]*lockV1Dep `json:"dependencies"`
}

func readNPMLock(p string) (map[string]*npmEntry, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var lock struct {
		LockfileVersion int                   `json:"lockfileVersion"`
		Packages        map[string]*lockPkg   `json:"packages"`
		Dependencies    map[string]*lockV1Dep `json:"dependencies"`
	}
	if err := json.Unmarshal(b, &lock); err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	entries := map[string]*npmEntry{}
	if len(lock.Packages) > 0 {
		for loc, lp := range lock.Packages {
			if loc == "" {
				continue
			}
			name := lp.Name
			if i := strings.LastIndex(loc, "node_modules/"); i >= 0 {
				if name == "" || lp.Link {
					name = loc[i+len("node_modules/"):]
				}
			} else if name == "" {
				name = path.Base(loc)
			}
			e := &npmEntry{
				loc: loc, name: name, version: lp.Version,
				licenses: npmLicenses(lp.License, nil),
				deps:     sortedKeys(lp.Dependencies, lp.OptionalDependencies, lp.PeerDependencies),
				// devOptional = in the dev tree AND an optional dep of a
				// non-dev package, so it can ship: treat as optional.
				dev: lp.Dev, optional: lp.Optional || lp.DevOptional, hasFlags: true, link: lp.Link,
			}
			if lp.Link {
				e.version = lp.Resolved // target location
			}
			if !strings.Contains(loc, "node_modules/") {
				// workspace package source: its devDependencies are roots too
				e.devDeps = sortedKeys(lp.DevDependencies)
				e.isRoot = true
			}
			entries[loc] = e
		}
		return entries, nil
	}
	// lockfile v1: nested "dependencies".
	var walk func(prefix string, deps map[string]*lockV1Dep)
	walk = func(prefix string, deps map[string]*lockV1Dep) {
		for name, d := range deps {
			loc := prefix + "node_modules/" + name
			entries[loc] = &npmEntry{
				loc: loc, name: name, version: d.Version, deps: sortedKeys(d.Requires),
				dev: d.Dev, optional: d.Optional, hasFlags: true,
			}
			if len(d.Dependencies) > 0 {
				walk(loc+"/", d.Dependencies)
			}
		}
	}
	walk("", lock.Dependencies)
	return entries, nil
}

// walkNodeModules indexes an installed node_modules tree (npm, yarn
// classic, pnpm) without a lockfile.
func walkNodeModules(dir string) map[string]*npmEntry {
	entries := map[string]*npmEntry{}
	visited := map[string]bool{}
	var walkNM func(nmRel string)
	addPkg := func(rel string) {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		real, err := filepath.EvalSymlinks(full)
		if err != nil {
			return
		}
		pj, err := readPkgJSON(filepath.Join(full, "package.json"))
		if err != nil {
			return
		}
		name := pj.Name
		if name == "" {
			name = rel[strings.LastIndex(rel, "node_modules/")+len("node_modules/"):]
		}
		entries[rel] = &npmEntry{
			loc: rel, name: name, version: pj.Version,
			licenses: npmLicenses(pj.License, pj.Licenses),
			deps:     sortedKeys(pj.Dependencies, pj.OptionalDependencies, pj.PeerDependencies),
			homepage: pj.Homepage,
		}
		if visited[real] {
			return
		}
		visited[real] = true
		if isDir(filepath.Join(full, "node_modules")) {
			walkNM(rel + "/node_modules")
		}
	}
	walkNM = func(nmRel string) {
		ents, err := os.ReadDir(filepath.Join(dir, filepath.FromSlash(nmRel)))
		if err != nil {
			return
		}
		for _, e := range ents {
			n := e.Name()
			switch {
			case n == ".pnpm":
				inner, _ := os.ReadDir(filepath.Join(dir, filepath.FromSlash(nmRel), ".pnpm"))
				for _, ie := range inner {
					if ie.Name() == "node_modules" || strings.HasPrefix(ie.Name(), ".") {
						continue
					}
					sub := nmRel + "/.pnpm/" + ie.Name() + "/node_modules"
					if isDir(filepath.Join(dir, filepath.FromSlash(sub))) {
						walkNM(sub)
					}
				}
			case strings.HasPrefix(n, "."):
				continue
			case strings.HasPrefix(n, "@"):
				scoped, _ := os.ReadDir(filepath.Join(dir, filepath.FromSlash(nmRel), n))
				for _, se := range scoped {
					addPkg(nmRel + "/" + n + "/" + se.Name())
				}
			default:
				addPkg(nmRel + "/" + n)
			}
		}
	}
	walkNM("node_modules")
	return entries
}
