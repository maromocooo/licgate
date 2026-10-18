// Package model holds the data types shared by collectors, the policy engine
// and the renderers.
package model

import (
	"sort"
	"strings"
)

// Scope says whether a component ends up in what you ship.
type Scope string

const (
	ScopeRuntime  Scope = "runtime"
	ScopeDev      Scope = "dev"     // dev/test/build-only
	ScopeUnknown  Scope = "unknown" // could not tell; treated like runtime
	ScopeOptional Scope = "optional"
)

// LicenseFile is a license/notice file found inside a package.
type LicenseFile struct {
	Path       string  `json:"path"`
	DetectedID string  `json:"detected_id,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
	Text       string  `json:"-"`
	IsNotice   bool    `json:"is_notice,omitempty"`
}

// Component is one third-party package.
type Component struct {
	ID        string `json:"id"` // stable key: purl without qualifiers, or eco/name@version
	Name      string `json:"name"`
	Version   string `json:"version"`
	Ecosystem string `json:"ecosystem"`
	PURL      string `json:"purl,omitempty"`

	// Declared is the raw license declaration(s) as found.
	Declared []string `json:"declared,omitempty"`
	// License is the normalised SPDX expression ("" = unknown).
	License string `json:"license"`
	// LicenseSource tells where License came from (lockfile, metadata,
	// license-file, registry, sbom, jev, override …).
	LicenseSource string `json:"license_source,omitempty"`
	// LicenseNotes carries caveats from normalisation/detection.
	LicenseNotes []string `json:"license_notes,omitempty"`
	// FileLicense is set when the package's own license files clearly
	// disagree with its metadata (e.g. metadata "MIT", LICENSE is GPL).
	FileLicense string `json:"file_license,omitempty"`

	LicenseFiles []LicenseFile `json:"license_files,omitempty"`
	Copyrights   []string      `json:"copyrights,omitempty"`

	Scope  Scope `json:"scope"`
	Direct bool  `json:"direct"`

	// Dir is where the package lives on disk, if known.
	Dir      string `json:"-"`
	Homepage string `json:"homepage,omitempty"`
}

// Key returns the display key name@version.
func (c *Component) Key() string {
	if c.Version == "" {
		return c.Name
	}
	return c.Name + "@" + c.Version
}

// Graph is the set of components plus dependency edges.
type Graph struct {
	// Roots are the IDs of the project's own packages (not evaluated).
	Roots      []string              `json:"roots"`
	RootNames  map[string]string     `json:"-"`
	Components map[string]*Component `json:"-"`
	// Edges: parent ID -> child IDs. Roots are parents too.
	Edges map[string][]string `json:"-"`
	// Sources lists what produced the data (collector names, SBOM files).
	Sources  []string `json:"sources"`
	Warnings []string `json:"warnings,omitempty"`
}

// NewGraph returns an empty graph.
func NewGraph() *Graph {
	return &Graph{Components: map[string]*Component{}, Edges: map[string][]string{}, RootNames: map[string]string{}}
}

// AddEdge records parent -> child (deduplicated).
func (g *Graph) AddEdge(parent, child string) {
	if parent == "" || child == "" || parent == child {
		return
	}
	for _, c := range g.Edges[parent] {
		if c == child {
			return
		}
	}
	g.Edges[parent] = append(g.Edges[parent], child)
}

// AddRoot registers a project root.
func (g *Graph) AddRoot(id, name string) {
	for _, r := range g.Roots {
		if r == id {
			return
		}
	}
	g.Roots = append(g.Roots, id)
	g.RootNames[id] = name
}

// Sorted returns components sorted by ecosystem, name, version.
func (g *Graph) Sorted() []*Component {
	out := make([]*Component, 0, len(g.Components))
	for _, c := range g.Components {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Ecosystem != b.Ecosystem {
			return a.Ecosystem < b.Ecosystem
		}
		if !strings.EqualFold(a.Name, b.Name) {
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		}
		return a.Version < b.Version
	})
	return out
}

// PathTo returns the shortest dependency path from any root to id, as a
// list of IDs starting with the root. nil if unreachable.
func (g *Graph) PathTo(id string) []string {
	type item struct {
		id   string
		prev int
	}
	var queue []item
	seen := map[string]bool{}
	for _, r := range g.Roots {
		queue = append(queue, item{r, -1})
		seen[r] = true
	}
	for i := 0; i < len(queue); i++ {
		cur := queue[i]
		if cur.id == id {
			var path []string
			for j := i; j >= 0; j = queue[j].prev {
				path = append([]string{queue[j].id}, path...)
			}
			return path
		}
		for _, ch := range g.Edges[cur.id] {
			if !seen[ch] {
				seen[ch] = true
				queue = append(queue, item{ch, i})
			}
		}
	}
	return nil
}

// Label returns a printable label for an ID (root name or name@version).
func (g *Graph) Label(id string) string {
	if n, ok := g.RootNames[id]; ok {
		return n
	}
	if c, ok := g.Components[id]; ok {
		return c.Key()
	}
	return id
}

// Merge adds other into g. Components with the same ID are merged: missing
// fields are filled in from other.
func (g *Graph) Merge(other *Graph) {
	for _, r := range other.Roots {
		g.AddRoot(r, other.RootNames[r])
	}
	for id, c := range other.Components {
		if ex, ok := g.Components[id]; ok {
			if ex.License == "" && c.License != "" {
				ex.License, ex.LicenseSource, ex.Declared = c.License, c.LicenseSource, c.Declared
			}
			if len(ex.LicenseFiles) == 0 {
				ex.LicenseFiles = c.LicenseFiles
			}
			if len(ex.Copyrights) == 0 {
				ex.Copyrights = c.Copyrights
			}
			if ex.Scope == ScopeUnknown && c.Scope != ScopeUnknown {
				ex.Scope = c.Scope
			}
			ex.Direct = ex.Direct || c.Direct
			if ex.Dir == "" {
				ex.Dir = c.Dir
			}
			continue
		}
		g.Components[id] = c
	}
	for p, kids := range other.Edges {
		for _, k := range kids {
			g.AddEdge(p, k)
		}
	}
	g.Sources = append(g.Sources, other.Sources...)
	g.Warnings = append(g.Warnings, other.Warnings...)
}

// ComputeScopes marks components reachable from roots through runtime
// edges as runtime, given a predicate for dev-only edges. Collectors that
// know scopes directly do not need this.
func (g *Graph) ReachableFrom(starts []string) map[string]bool {
	seen := map[string]bool{}
	stack := append([]string(nil), starts...)
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[n] {
			continue
		}
		seen[n] = true
		stack = append(stack, g.Edges[n]...)
	}
	return seen
}
