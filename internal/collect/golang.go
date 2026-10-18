package collect

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"

	"github.com/licgate/licgate/internal/i18n"
	"github.com/licgate/licgate/internal/model"
)

func detectGo(dir string) bool { return exists(filepath.Join(dir, "go.mod")) }

type goMod struct {
	path, version, dir string
	main               bool
	replaceDir         string // local replacement directory
}

func runCmd(dir string, timeout time.Duration, env []string, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 400 {
			msg = msg[:400] + "…"
		}
		return out, fmt.Errorf("%s %s: %v: %s", name, strings.Join(args, " "), err, msg)
	}
	return out, nil
}

const goListFmt = `{{with .Module}}{{.Path}}	{{.Version}}	{{.Dir}}	{{.Main}}{{with .Replace}}	{{.Path}}	{{.Version}}	{{.Dir}}{{end}}{{end}}`

func parseGoList(out []byte) map[string]*goMod {
	mods := map[string]*goMod{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		f := strings.Split(sc.Text(), "\t")
		if len(f) < 4 || f[0] == "" {
			continue
		}
		m := &goMod{path: f[0], version: f[1], dir: f[2], main: f[3] == "true"}
		if len(f) >= 7 {
			// replaced module: version/dir of the replacement
			if f[5] == "" { // local directory replacement
				m.replaceDir = f[6]
			} else {
				m.version = f[5]
			}
			if f[6] != "" {
				m.dir = f[6]
			}
		}
		mods[m.path] = m
	}
	return mods
}

func collectGo(dir string, o *Options) (*model.Graph, error) {
	if _, err := exec.LookPath("go"); err != nil {
		return collectGoModFile(dir)
	}
	// Default (readonly) module mode: never let licgate edit go.mod/go.sum.
	var env []string
	// Modules linked into non-test builds of every package.
	out, err := runCmd(dir, 5*time.Minute, env, "go", "list", "-e", "-deps", "-f", goListFmt, "./...")
	if err != nil && len(out) == 0 {
		o.Log("go list failed, falling back to go.mod: %v", err)
		g, ferr := collectGoModFile(dir)
		if g != nil {
			g.Warnings = append(g.Warnings, "go: `go list` failed; fell back to go.mod without scope information: "+err.Error())
		}
		return g, ferr
	}
	listErr := err
	linked := parseGoList(out)
	// Modules linked only on other platforms (e.g. golang.org/x/sys/windows
	// users, platform-specific drivers) ship in cross-compiled releases.
	for _, plat := range [][2]string{{"linux", "amd64"}, {"darwin", "arm64"}, {"windows", "amd64"}} {
		if plat[0] == runtime.GOOS && plat[1] == runtime.GOARCH {
			continue
		}
		pout, _ := runCmd(dir, 5*time.Minute, []string{"GOOS=" + plat[0], "GOARCH=" + plat[1], "CGO_ENABLED=1"}, "go", "list", "-e", "-deps", "-f", goListFmt, "./...")
		for p, m := range parseGoList(pout) {
			if _, ok := linked[p]; !ok {
				linked[p] = m
			}
		}
	}
	// Whole build list (includes test-only and unused modules).
	allOut, _ := runCmd(dir, 5*time.Minute, env, "go", "list", "-e", "-m", "-f", `{{.Path}}	{{.Version}}	{{.Dir}}	{{.Main}}{{with .Replace}}	{{.Path}}	{{.Version}}	{{.Dir}}{{end}}`, "all")
	all := parseGoList(allOut)
	for p, m := range linked {
		all[p] = m
	}

	g := model.NewGraph()
	g.Sources = append(g.Sources, "go:go list")
	if listErr != nil {
		g.Warnings = append(g.Warnings, "go: `go list` reported errors, results may be incomplete (run `go mod tidy`): "+listErr.Error())
	}
	idOf := map[string]string{}
	var mainPath string
	for _, m := range all {
		if m.main {
			mainPath = m.path
			id := "root:golang:" + m.path
			g.AddRoot(id, m.path)
			idOf[m.path] = id
		}
	}
	for p, m := range all {
		if m.main {
			continue
		}
		if m.replaceDir != "" && len(scanLicenseFiles(m.dir)) == 0 {
			// Local replacement without license files: treat as the user's
			// own code. (With license files it may be a vendored fork and
			// is evaluated like any other module.)
			id := "root:golang:" + p
			g.AddRoot(id, p+" (local)")
			idOf[p] = id
			continue
		}
		c := newComponent("golang", p, m.version)
		c.Dir = m.dir
		if _, ok := linked[p]; ok {
			c.Scope = model.ScopeRuntime
		} else {
			c.Scope = model.ScopeDev
		}
		g.Components[c.ID] = c
		idOf[p] = c.ID
	}
	// Dependency edges from `go mod graph`.
	if gout, err := runCmd(dir, 2*time.Minute, env, "go", "mod", "graph"); err == nil {
		sc := bufio.NewScanner(bytes.NewReader(gout))
		for sc.Scan() {
			a, b, ok := strings.Cut(sc.Text(), " ")
			if !ok {
				continue
			}
			ap, _, _ := strings.Cut(a, "@")
			bp, _, _ := strings.Cut(b, "@")
			if idOf[ap] != "" && idOf[bp] != "" {
				g.AddEdge(idOf[ap], idOf[bp])
			}
		}
	}
	if mainPath != "" {
		for _, ch := range g.Edges[idOf[mainPath]] {
			if c, ok := g.Components[ch]; ok {
				c.Direct = true
			}
		}
	}
	addGoStdlib(g)
	return g, nil
}

// addGoStdlib records the Go standard library/runtime, which is compiled
// into every binary and is BSD-3-Clause licensed.
func addGoStdlib(g *model.Graph) {
	ver := strings.TrimPrefix(runtime.Version(), "go")
	if out, err := runCmd(".", 30*time.Second, nil, "go", "env", "GOVERSION"); err == nil {
		ver = strings.TrimPrefix(strings.TrimSpace(string(out)), "go")
	}
	c := newComponent("golang", "stdlib", ver)
	c.License, c.LicenseSource = "BSD-3-Clause", "builtin"
	c.Scope = model.ScopeRuntime
	c.Homepage = "https://go.dev/LICENSE"
	c.Copyrights = []string{"Copyright 2009 The Go Authors."}
	c.LicenseNotes = []string{i18n.T("c.go_stdlib")}
	if out, err := runCmd(".", 30*time.Second, nil, "go", "env", "GOROOT"); err == nil {
		root := strings.TrimSpace(string(out))
		if b, err := os.ReadFile(filepath.Join(root, "LICENSE")); err == nil {
			c.LicenseFiles = []model.LicenseFile{licenseFileFromText("LICENSE", string(b))}
		}
	}
	g.Components[c.ID] = c
	for _, r := range g.Roots {
		g.AddEdge(r, c.ID)
	}
}

// collectGoModFile is the no-toolchain fallback: read go.mod and look for
// sources in the module cache.
func collectGoModFile(dir string) (*model.Graph, error) {
	b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return nil, err
	}
	g := model.NewGraph()
	g.Sources = append(g.Sources, "go:go.mod")
	modCache := os.Getenv("GOMODCACHE")
	if modCache == "" {
		gp := os.Getenv("GOPATH")
		if gp == "" {
			home, _ := os.UserHomeDir()
			gp = filepath.Join(home, "go")
		}
		modCache = filepath.Join(strings.Split(gp, string(os.PathListSeparator))[0], "pkg", "mod")
	}
	var mainPath string
	inReq := false
	sc := bufio.NewScanner(bytes.NewReader(b))
	var reqs [][3]string // path, version, indirect
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if i := strings.Index(line, "//"); i >= 0 && !strings.Contains(line[i:], "indirect") {
			line = strings.TrimSpace(line[:i])
		}
		switch {
		case strings.HasPrefix(line, "module "):
			mainPath = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "module ")), `"`)
		case line == "require (":
			inReq = true
		case inReq && line == ")":
			inReq = false
		case strings.HasPrefix(line, "require "):
			f := strings.Fields(strings.TrimPrefix(line, "require "))
			if len(f) >= 2 {
				reqs = append(reqs, [3]string{f[0], f[1], fmt.Sprint(strings.Contains(line, "indirect"))})
			}
		case inReq && line != "":
			f := strings.Fields(line)
			if len(f) >= 2 {
				reqs = append(reqs, [3]string{f[0], f[1], fmt.Sprint(strings.Contains(line, "indirect"))})
			}
		}
	}
	rootID := "root:golang:" + mainPath
	g.AddRoot(rootID, mainPath)
	for _, r := range reqs {
		c := newComponent("golang", r[0], r[1])
		c.Direct = r[2] == "false"
		if d := filepath.Join(modCache, escapeModPath(r[0])+"@"+r[1]); isDir(d) {
			c.Dir = d
		}
		g.Components[c.ID] = c
		g.AddEdge(rootID, c.ID)
	}
	addGoStdlib(g)
	g.Warnings = append(g.Warnings, "go: toolchain not available; scopes are unknown (test-only modules are not separated)")
	return g, nil
}

// escapeModPath applies the module cache's case encoding (!lower for upper).
func escapeModPath(p string) string {
	var b strings.Builder
	for _, r := range p {
		if unicode.IsUpper(r) {
			b.WriteByte('!')
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
