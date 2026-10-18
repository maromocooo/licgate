package report

import (
	"encoding/json"
	"io"
	"sort"
	"time"

	"github.com/licgate/licgate/internal/license"
	"github.com/licgate/licgate/internal/model"
	"github.com/licgate/licgate/internal/policy"
)

type cdxLicense struct {
	License *struct {
		ID   string `json:"id,omitempty"`
		Name string `json:"name,omitempty"`
	} `json:"license,omitempty"`
	Expression string `json:"expression,omitempty"`
}

type cdxProp struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type cdxComp struct {
	BomRef     string       `json:"bom-ref"`
	Type       string       `json:"type"`
	Name       string       `json:"name"`
	Version    string       `json:"version,omitempty"`
	PURL       string       `json:"purl,omitempty"`
	Scope      string       `json:"scope,omitempty"`
	Licenses   []cdxLicense `json:"licenses,omitempty"`
	Copyright  string       `json:"copyright,omitempty"`
	Properties []cdxProp    `json:"properties,omitempty"`
}

// WriteCycloneDX writes a CycloneDX 1.5 JSON SBOM with resolved licenses
// and licgate's verdicts as properties.
func WriteCycloneDX(w io.Writer, r *policy.Result, m Meta) error {
	g := r.Graph
	type dep struct {
		Ref       string   `json:"ref"`
		DependsOn []string `json:"dependsOn,omitempty"`
	}
	doc := struct {
		BomFormat   string `json:"bomFormat"`
		SpecVersion string `json:"specVersion"`
		Version     int    `json:"version"`
		Metadata    struct {
			Timestamp string `json:"timestamp"`
			Tools     struct {
				Components []cdxComp `json:"components"`
			} `json:"tools"`
			Component *cdxComp `json:"component,omitempty"`
		} `json:"metadata"`
		Components   []cdxComp `json:"components"`
		Dependencies []dep     `json:"dependencies"`
	}{BomFormat: "CycloneDX", SpecVersion: "1.5", Version: 1}
	doc.Metadata.Timestamp = m.GeneratedAt.UTC().Format(time.RFC3339)
	doc.Metadata.Tools.Components = []cdxComp{{BomRef: "licgate", Type: "application", Name: "licgate", Version: m.Version}}
	if len(g.Roots) > 0 {
		root := g.Roots[0]
		doc.Metadata.Component = &cdxComp{BomRef: root, Type: "application", Name: g.Label(root)}
		if !r.Outbound.Proprietary {
			doc.Metadata.Component.Licenses = licensesFor(r.Outbound.Expr)
		}
	}
	level := map[string]*policy.Finding{}
	for _, f := range r.Findings {
		level[f.Component.ID] = f
	}
	for _, c := range g.Sorted() {
		cc := cdxComp{BomRef: c.ID, Type: "library", Name: c.Name, Version: c.Version, PURL: c.PURL}
		switch c.Scope {
		case model.ScopeRuntime:
			cc.Scope = "required"
		case model.ScopeOptional:
			cc.Scope = "optional"
		case model.ScopeDev:
			cc.Scope = "excluded"
		}
		if c.License != "" {
			cc.Licenses = licensesFor(c.License)
		}
		if len(c.Copyrights) > 0 {
			cc.Copyright = c.Copyrights[0]
		}
		if f := level[c.ID]; f != nil {
			cc.Properties = append(cc.Properties, cdxProp{"licgate:result", f.Level.String()})
		}
		if c.LicenseSource != "" {
			cc.Properties = append(cc.Properties, cdxProp{"licgate:license_source", c.LicenseSource})
		}
		doc.Components = append(doc.Components, cc)
	}
	var parents []string
	for p := range g.Edges {
		parents = append(parents, p)
	}
	sort.Strings(parents)
	for _, p := range parents {
		kids := append([]string(nil), g.Edges[p]...)
		sort.Strings(kids)
		doc.Dependencies = append(doc.Dependencies, dep{Ref: p, DependsOn: kids})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

func licensesFor(expr string) []cdxLicense {
	e, err := license.Parse(expr)
	if err != nil {
		return []cdxLicense{{License: &struct {
			ID   string `json:"id,omitempty"`
			Name string `json:"name,omitempty"`
		}{Name: expr}}}
	}
	if l, ok := e.(license.Leaf); ok && l.Exception == "" {
		lic := &struct {
			ID   string `json:"id,omitempty"`
			Name string `json:"name,omitempty"`
		}{}
		if len(l.ID) > 11 && l.ID[:11] == "LicenseRef-" {
			lic.Name = license.DisplayName(l.ID)
		} else {
			lic.ID = l.ID
		}
		return []cdxLicense{{License: lic}}
	}
	return []cdxLicense{{Expression: e.String()}}
}
