package collect

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/maromocooo/licgate/internal/model"
)

func detectComposer(dir string) bool { return exists(filepath.Join(dir, "composer.lock")) }

type composerPkg struct {
	Name       string            `json:"name"`
	Version    string            `json:"version"`
	License    []string          `json:"license"`
	Require    map[string]string `json:"require"`
	Homepage   string            `json:"homepage"`
	RequireDev map[string]string `json:"require-dev"`
}

func collectComposer(dir string, o *Options) (*model.Graph, error) {
	b, err := os.ReadFile(filepath.Join(dir, "composer.lock"))
	if err != nil {
		return nil, err
	}
	var lock struct {
		Packages    []composerPkg `json:"packages"`
		PackagesDev []composerPkg `json:"packages-dev"`
	}
	if err := json.Unmarshal(b, &lock); err != nil {
		return nil, fmt.Errorf("composer.lock: %w", err)
	}
	var root composerPkg
	if rb, err := os.ReadFile(filepath.Join(dir, "composer.json")); err == nil {
		_ = json.Unmarshal(rb, &root)
	}
	g := model.NewGraph()
	g.Sources = append(g.Sources, "composer:composer.lock")
	name := root.Name
	if name == "" {
		name = filepath.Base(dir)
	}
	rootID := "root:composer:" + filepath.ToSlash(dir)
	g.AddRoot(rootID, name)
	byName := map[string]string{}
	add := func(pkgs []composerPkg, scope model.Scope) {
		for _, p := range pkgs {
			c := newComponent("composer", p.Name, strings.TrimPrefix(p.Version, "v"))
			c.Scope = scope
			c.Homepage = p.Homepage
			if d := filepath.Join(dir, "vendor", filepath.FromSlash(p.Name)); isDir(d) {
				c.Dir = d
			}
			// composer "license" arrays mean "any of these".
			declare(c, "composer.lock", p.License...)
			g.Components[c.ID] = c
			byName[strings.ToLower(p.Name)] = c.ID
		}
	}
	add(lock.Packages, model.ScopeRuntime)
	add(lock.PackagesDev, model.ScopeDev)
	for _, set := range [][]composerPkg{lock.Packages, lock.PackagesDev} {
		for _, p := range set {
			for d := range p.Require {
				g.AddEdge(byName[strings.ToLower(p.Name)], byName[strings.ToLower(d)])
			}
		}
	}
	for d := range root.Require {
		g.AddEdge(rootID, byName[strings.ToLower(d)])
	}
	for d := range root.RequireDev {
		g.AddEdge(rootID, byName[strings.ToLower(d)])
	}
	markDirect(g)
	return g, nil
}
