package collect

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/licgate/licgate/internal/i18n"
	"github.com/licgate/licgate/internal/model"
)

func detectCargo(dir string) bool {
	return exists(filepath.Join(dir, "Cargo.toml")) && (exists(filepath.Join(dir, "Cargo.lock")) || !insideCargoWorkspace(dir))
}

// insideCargoWorkspace: member crates of a workspace are handled from the
// workspace root (which has the Cargo.lock).
func insideCargoWorkspace(dir string) bool {
	for p := filepath.Dir(dir); p != filepath.Dir(p); p = filepath.Dir(p) {
		if exists(filepath.Join(p, "Cargo.lock")) && exists(filepath.Join(p, "Cargo.toml")) {
			return true
		}
	}
	return false
}

type cargoMeta struct {
	Packages []struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		Version      string `json:"version"`
		License      string `json:"license"`
		LicenseFile  string `json:"license_file"`
		ManifestPath string `json:"manifest_path"`
		Source       string `json:"source"`
		Repository   string `json:"repository"`
		Homepage     string `json:"homepage"`
		Links        string `json:"links"`
		Targets      []struct {
			Kind []string `json:"kind"`
		} `json:"targets"`
	} `json:"packages"`
	WorkspaceMembers []string `json:"workspace_members"`
	Resolve          *struct {
		Nodes []struct {
			ID   string `json:"id"`
			Deps []struct {
				Pkg      string `json:"pkg"`
				DepKinds []struct {
					Kind *string `json:"kind"`
				} `json:"dep_kinds"`
			} `json:"deps"`
		} `json:"nodes"`
	} `json:"resolve"`
}

func collectCargo(dir string, o *Options) (*model.Graph, error) {
	if _, err := exec.LookPath("cargo"); err == nil {
		out, err := runCmd(dir, 5*time.Minute, nil, "cargo", "metadata", "--format-version", "1", "--all-features", "--offline")
		if err != nil {
			out, err = runCmd(dir, 5*time.Minute, nil, "cargo", "metadata", "--format-version", "1", "--all-features")
		}
		if err == nil {
			var m cargoMeta
			if jerr := json.Unmarshal(out, &m); jerr == nil && m.Resolve != nil {
				return cargoFromMetadata(&m)
			}
		} else {
			o.Log("cargo metadata failed: %v", err)
		}
	}
	return collectCargoLock(dir)
}

func cargoFromMetadata(m *cargoMeta) (*model.Graph, error) {
	g := model.NewGraph()
	g.Sources = append(g.Sources, "cargo:metadata")
	members := map[string]bool{}
	for _, w := range m.WorkspaceMembers {
		members[w] = true
	}
	idOf := map[string]string{}
	procMacro := map[string]bool{}
	hasLinks := map[string]bool{} // component/root ID -> crate has `links` (a -sys crate)
	nameOf := map[string]string{}
	for _, p := range m.Packages {
		if members[p.ID] || p.Source == "" && members[p.ID] {
			rid := "root:cargo:" + p.ID
			g.AddRoot(rid, p.Name)
			idOf[p.ID] = rid
			continue
		}
		if p.Source == "" && p.License == "" && p.LicenseFile == "" && len(scanLicenseFiles(filepath.Dir(p.ManifestPath))) == 0 {
			// Local path dependency without any license info: treat as the
			// user's own code. (With license info it is evaluated below —
			// it may be a vendored third-party fork.)
			rid := "root:cargo:" + p.ID
			g.AddRoot(rid, p.Name+" (path)")
			idOf[p.ID] = rid
			continue
		}
		c := newComponent("cargo", p.Name, p.Version)
		c.Dir = filepath.Dir(p.ManifestPath)
		c.Homepage = p.Repository
		if c.Homepage == "" {
			c.Homepage = p.Homepage
		}
		declare(c, "Cargo.toml", p.License)
		if c.License == "" && p.LicenseFile != "" {
			if b, err := os.ReadFile(filepath.Join(c.Dir, p.LicenseFile)); err == nil {
				c.LicenseFiles = append(c.LicenseFiles, licenseFileFromText(p.LicenseFile, string(b)))
			}
		}
		for _, t := range p.Targets {
			for _, k := range t.Kind {
				if k == "proc-macro" {
					procMacro[c.ID] = true
				}
			}
		}
		g.Components[c.ID] = c
		idOf[p.ID] = c.ID
	}
	for _, p := range m.Packages {
		if id := idOf[p.ID]; id != "" {
			hasLinks[id] = p.Links != ""
			nameOf[id] = p.Name
		}
	}
	// Edges; remember which are "normal" (linked) edges.
	normal := map[string][]string{}
	for _, n := range m.Resolve.Nodes {
		from := idOf[n.ID]
		for _, d := range n.Deps {
			to := idOf[d.Pkg]
			if from == "" || to == "" {
				continue
			}
			g.AddEdge(from, to)
			for _, k := range d.DepKinds {
				// Normal dependencies are linked. A -sys crate's build
				// dependency on a "*-src" crate compiles bundled C code
				// (openssl-src, lua-src…) into the binary as well.
				if k.Kind == nil || (*k.Kind == "build" && hasLinks[from] && strings.HasSuffix(nameOf[to], "-src")) {
					normal[from] = append(normal[from], to)
					break
				}
			}
		}
	}
	// Runtime = reachable from workspace members through normal edges,
	// not passing through proc-macro crates (their deps run at build time).
	runtime := map[string]bool{}
	var stack []string
	for _, r := range g.Roots {
		stack = append(stack, r)
	}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if runtime[n] {
			continue
		}
		runtime[n] = true
		if procMacro[n] {
			continue
		}
		stack = append(stack, normal[n]...)
	}
	for id, c := range g.Components {
		switch {
		case procMacro[id]:
			c.Scope = model.ScopeDev
			c.LicenseNotes = append(c.LicenseNotes, i18n.T("c.proc_macro"))
		case runtime[id]:
			c.Scope = model.ScopeRuntime
		default:
			c.Scope = model.ScopeDev
		}
	}
	markDirect(g)
	return g, nil
}

// collectCargoLock is the fallback without cargo: Cargo.lock + sources in
// ~/.cargo/registry/src.
func collectCargoLock(dir string) (*model.Graph, error) {
	var lock struct {
		Package []struct {
			Name         string   `toml:"name"`
			Version      string   `toml:"version"`
			Source       string   `toml:"source"`
			Dependencies []string `toml:"dependencies"`
		} `toml:"package"`
	}
	if _, err := toml.DecodeFile(filepath.Join(dir, "Cargo.lock"), &lock); err != nil {
		return nil, fmt.Errorf("Cargo.lock: %w (run `cargo generate-lockfile`)", err)
	}
	g := model.NewGraph()
	g.Sources = append(g.Sources, "cargo:Cargo.lock")
	cargoHome := os.Getenv("CARGO_HOME")
	if cargoHome == "" {
		home, _ := os.UserHomeDir()
		cargoHome = filepath.Join(home, ".cargo")
	}
	srcDirs, _ := filepath.Glob(filepath.Join(cargoHome, "registry", "src", "*"))
	idOf := map[string]string{} // "name version" and "name" -> id
	for _, p := range lock.Package {
		var id string
		if p.Source == "" {
			id = "root:cargo:" + p.Name + "@" + p.Version
			g.AddRoot(id, p.Name)
		} else {
			c := newComponent("cargo", p.Name, p.Version)
			for _, sd := range srcDirs {
				d := filepath.Join(sd, p.Name+"-"+p.Version)
				if isDir(d) {
					c.Dir = d
					var man struct {
						Package struct {
							License    any    `toml:"license"`
							Repository string `toml:"repository"`
						} `toml:"package"`
					}
					if _, err := toml.DecodeFile(filepath.Join(d, "Cargo.toml"), &man); err == nil {
						if s, ok := man.Package.License.(string); ok {
							declare(c, "Cargo.toml", s)
						}
						c.Homepage = man.Package.Repository
					}
					break
				}
			}
			g.Components[c.ID] = c
			id = c.ID
		}
		idOf[p.Name+" "+p.Version] = id
		if _, dup := idOf[p.Name]; !dup {
			idOf[p.Name] = id
		} else {
			idOf[p.Name] = "" // ambiguous bare name
		}
	}
	for _, p := range lock.Package {
		from := idOf[p.Name+" "+p.Version]
		for _, d := range p.Dependencies {
			f := strings.Fields(d) // "name", "name version", "name version (source)"
			var to string
			if len(f) >= 2 {
				to = idOf[f[0]+" "+f[1]]
			} else if len(f) == 1 {
				to = idOf[f[0]]
			}
			g.AddEdge(from, to)
		}
	}
	markDirect(g)
	g.Warnings = append(g.Warnings, "cargo: `cargo metadata` unavailable; dev/build-only crates are not separated")
	return g, nil
}
