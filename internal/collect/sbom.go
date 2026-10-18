package collect

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/licgate/licgate/internal/license"
	"github.com/licgate/licgate/internal/model"
)

// IngestSBOMFile reads a CycloneDX or SPDX JSON document.
func IngestSBOMFile(p string) (*model.Graph, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	g, err := IngestSBOM(b)
	if err != nil {
		return nil, err
	}
	g.Sources = append(g.Sources, "sbom:"+p)
	return g, nil
}

// IngestSBOM parses CycloneDX (1.2–1.6) or SPDX 2.x JSON.
func IngestSBOM(b []byte) (*model.Graph, error) {
	var probe struct {
		BomFormat   string `json:"bomFormat"`
		SPDXVersion string `json:"spdxVersion"`
		Context     any    `json:"@context"`
	}
	if err := json.Unmarshal(b, &probe); err != nil {
		if strings.HasPrefix(strings.TrimSpace(string(b)), "<") {
			return nil, fmt.Errorf("XML SBOMs are not supported; export CycloneDX or SPDX as JSON")
		}
		return nil, fmt.Errorf("not a JSON SBOM: %w", err)
	}
	switch {
	case strings.EqualFold(probe.BomFormat, "CycloneDX"):
		return ingestCycloneDX(b)
	case strings.HasPrefix(probe.SPDXVersion, "SPDX-2"):
		return ingestSPDX(b)
	case probe.Context != nil:
		return nil, fmt.Errorf("SPDX 3 JSON-LD is not supported yet; export SPDX 2.3 JSON or CycloneDX JSON")
	}
	return nil, fmt.Errorf("unrecognised SBOM format (expected CycloneDX or SPDX 2.x JSON)")
}

// ---- purl -------------------------------------------------------------------

type purl struct {
	typ, namespace, name, version string
}

func parsePURL(s string) (purl, bool) {
	if !strings.HasPrefix(s, "pkg:") {
		return purl{}, false
	}
	s = strings.TrimPrefix(s, "pkg:")
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	var p purl
	if i := strings.LastIndex(s, "@"); i >= 0 && i > strings.Index(s, "/") {
		p.version, _ = url.PathUnescape(s[i+1:])
		s = s[:i]
	}
	parts := strings.Split(strings.Trim(s, "/"), "/")
	if len(parts) < 2 {
		return purl{}, false
	}
	p.typ = strings.ToLower(parts[0])
	p.name, _ = url.PathUnescape(parts[len(parts)-1])
	if len(parts) > 2 {
		var ns []string
		for _, x := range parts[1 : len(parts)-1] {
			u, _ := url.PathUnescape(x)
			ns = append(ns, u)
		}
		p.namespace = strings.Join(ns, "/")
	}
	return p, true
}

// componentFromPURL creates a component whose ID matches what the native
// collectors would produce, so data from SBOMs and collectors merges.
func componentFromPURL(ps, fallbackName, fallbackVersion string) *model.Component {
	p, ok := parsePURL(ps)
	if !ok {
		name := fallbackName
		c := &model.Component{Name: name, Version: fallbackVersion, Ecosystem: "generic", Scope: model.ScopeUnknown}
		c.ID = "pkg:generic/" + url.PathEscape(name)
		if fallbackVersion != "" {
			c.ID += "@" + url.PathEscape(fallbackVersion)
		}
		c.PURL = c.ID
		return c
	}
	version := p.version
	if version == "" {
		version = fallbackVersion
	}
	var name string
	switch p.typ {
	case "maven":
		name = p.namespace + ":" + p.name
		if p.namespace == "" {
			name = p.name
		}
	default:
		if p.namespace != "" {
			name = p.namespace + "/" + p.name
		} else {
			name = p.name
		}
	}
	c := newComponent(p.typ, name, version)
	if p.typ == "maven" {
		c.ID = "pkg:maven/" + p.namespace + "/" + p.name + "@" + version
		c.PURL = c.ID
	}
	return c
}

// ---- CycloneDX ---------------------------------------------------------------

type cdxLicenseChoice struct {
	License *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Text *struct {
			Content string `json:"content"`
		} `json:"text"`
	} `json:"license"`
	Expression string `json:"expression"`
}

type cdxComponent struct {
	BomRef     string             `json:"bom-ref"`
	Type       string             `json:"type"`
	Group      string             `json:"group"`
	Name       string             `json:"name"`
	Version    string             `json:"version"`
	PURL       string             `json:"purl"`
	Scope      string             `json:"scope"`
	Licenses   []cdxLicenseChoice `json:"licenses"`
	Copyright  string             `json:"copyright"`
	Components []cdxComponent     `json:"components"`
	ExtRefs    []struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	} `json:"externalReferences"`
}

func ingestCycloneDX(b []byte) (*model.Graph, error) {
	var doc struct {
		Metadata struct {
			Component *cdxComponent `json:"component"`
		} `json:"metadata"`
		Components   []cdxComponent `json:"components"`
		Dependencies []struct {
			Ref       string   `json:"ref"`
			DependsOn []string `json:"dependsOn"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	g := model.NewGraph()
	refToID := map[string]string{}
	rootRef := ""
	if mc := doc.Metadata.Component; mc != nil {
		rootRef = mc.BomRef
		id := "root:sbom:" + mc.BomRef
		if mc.BomRef == "" {
			id = "root:sbom:" + mc.Name
		}
		g.AddRoot(id, firstNonEmpty(mc.Name, "project"))
		refToID[mc.BomRef] = id
	}
	var walk func(cs []cdxComponent)
	walk = func(cs []cdxComponent) {
		for i := range cs {
			cc := &cs[i]
			if cc.Type == "file" || cc.Type == "operating-system" && cc.PURL == "" {
				continue
			}
			name := cc.Name
			if cc.Group != "" && cc.PURL == "" {
				name = cc.Group + "/" + cc.Name
			}
			c := componentFromPURL(cc.PURL, name, cc.Version)
			if ex, ok := g.Components[c.ID]; ok {
				refToID[cc.BomRef] = ex.ID
				if len(cc.Components) > 0 {
					walk(cc.Components)
				}
				continue
			}
			switch cc.Scope {
			case "required":
				c.Scope = model.ScopeRuntime
			case "optional":
				c.Scope = model.ScopeOptional
			case "excluded":
				c.Scope = model.ScopeDev
			}
			var exprs []string
			for _, l := range cc.Licenses {
				switch {
				case l.Expression != "":
					c.Declared = appendUniq(c.Declared, l.Expression)
					exprs = append(exprs, l.Expression)
				case l.License != nil && l.License.ID != "":
					c.Declared = appendUniq(c.Declared, l.License.ID)
					exprs = append(exprs, l.License.ID)
				case l.License != nil && l.License.Name != "":
					c.Declared = appendUniq(c.Declared, l.License.Name)
					exprs = append(exprs, l.License.Name)
				}
				if l.License != nil && l.License.Text != nil && l.License.Text.Content != "" {
					c.LicenseFiles = append(c.LicenseFiles, licenseFileFromText("sbom:license-text", l.License.Text.Content))
				}
			}
			setFromExprs(c, exprs, "sbom")
			if cc.Copyright != "" {
				c.Copyrights = append(c.Copyrights, cc.Copyright)
			}
			for _, r := range cc.ExtRefs {
				if r.Type == "website" || r.Type == "vcs" {
					c.Homepage = r.URL
					break
				}
			}
			g.Components[c.ID] = c
			refToID[cc.BomRef] = c.ID
			if len(cc.Components) > 0 {
				walk(cc.Components)
			}
		}
	}
	walk(doc.Components)
	incoming := map[string]bool{}
	for _, d := range doc.Dependencies {
		from := refToID[d.Ref]
		for _, to := range d.DependsOn {
			if t := refToID[to]; t != "" && from != "" {
				g.AddEdge(from, t)
				incoming[t] = true
			}
		}
	}
	if len(g.Roots) == 0 {
		g.AddRoot("root:sbom", "project")
	}
	// Components with no parent hang off the root so paths always exist.
	root := g.Roots[0]
	if rootRef == "" || len(doc.Dependencies) == 0 {
		for id := range g.Components {
			if !incoming[id] {
				g.AddEdge(root, id)
			}
		}
	}
	markDirect(g)
	return g, nil
}

// setFromExprs normalises several license entries. Multiple separate
// entries are treated as all applying (AND), the conservative reading.
func setFromExprs(c *model.Component, exprs []string, source string) {
	var norm []string
	for _, e := range exprs {
		n := license.Normalize(e)
		if n.Expr == "" {
			continue
		}
		if strings.Contains(n.Expr, " ") {
			norm = appendUniq(norm, "("+n.Expr+")")
		} else {
			norm = appendUniq(norm, n.Expr)
		}
	}
	if len(norm) == 0 {
		return
	}
	expr := strings.Join(norm, " AND ")
	if len(norm) == 1 {
		expr = strings.TrimSuffix(strings.TrimPrefix(norm[0], "("), ")")
	}
	c.License, c.LicenseSource = expr, source
}

// ---- SPDX 2.x ------------------------------------------------------------------

var licenseRefRe = regexp.MustCompile(`(?:DocumentRef-[A-Za-z0-9.\-]+:)?LicenseRef-[A-Za-z0-9.\-]+`)

func ingestSPDX(b []byte) (*model.Graph, error) {
	var doc struct {
		DocumentDescribes []string `json:"documentDescribes"`
		Packages          []struct {
			SPDXID           string `json:"SPDXID"`
			Name             string `json:"name"`
			VersionInfo      string `json:"versionInfo"`
			LicenseConcluded string `json:"licenseConcluded"`
			LicenseDeclared  string `json:"licenseDeclared"`
			CopyrightText    string `json:"copyrightText"`
			Homepage         string `json:"homepage"`
			ExternalRefs     []struct {
				ReferenceType    string `json:"referenceType"`
				ReferenceLocator string `json:"referenceLocator"`
			} `json:"externalRefs"`
		} `json:"packages"`
		Relationships []struct {
			Element string `json:"spdxElementId"`
			Type    string `json:"relationshipType"`
			Related string `json:"relatedSpdxElement"`
		} `json:"relationships"`
		Extracted []struct {
			LicenseID     string `json:"licenseId"`
			ExtractedText string `json:"extractedText"`
			Name          string `json:"name"`
		} `json:"hasExtractedLicensingInfos"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	// Map LicenseRef-* to known ids when their text is recognisable.
	refMap := map[string]string{}
	for _, x := range doc.Extracted {
		d := license.DetectText(x.ExtractedText)
		if d.ID != "" && d.Confidence >= 0.8 {
			refMap[x.LicenseID] = d.ID
		} else if n := license.Normalize(x.Name); n.OK() {
			refMap[x.LicenseID] = n.Expr
		}
	}
	described := map[string]bool{}
	for _, d := range doc.DocumentDescribes {
		described[d] = true
	}
	for _, r := range doc.Relationships {
		if r.Type == "DESCRIBES" && strings.HasSuffix(r.Element, "DOCUMENT") {
			described[r.Related] = true
		}
	}
	g := model.NewGraph()
	idOf := map[string]string{}
	for _, p := range doc.Packages {
		if described[p.SPDXID] {
			id := "root:sbom:" + p.SPDXID
			g.AddRoot(id, p.Name)
			idOf[p.SPDXID] = id
			continue
		}
		var pu string
		for _, r := range p.ExternalRefs {
			if r.ReferenceType == "purl" {
				pu = r.ReferenceLocator
				break
			}
		}
		c := componentFromPURL(pu, p.Name, p.VersionInfo)
		if ex, ok := g.Components[c.ID]; ok {
			idOf[p.SPDXID] = ex.ID
			continue
		}
		lic := p.LicenseConcluded
		if isNoAssertion(lic) {
			lic = p.LicenseDeclared
		}
		if !isNoAssertion(lic) {
			c.Declared = []string{lic}
			lic = licenseRefRe.ReplaceAllStringFunc(lic, func(ref string) string {
				if v, ok := refMap[ref]; ok {
					if strings.Contains(v, " ") {
						return "(" + v + ")"
					}
					return v
				}
				return ref
			})
			setFromExprs(c, []string{lic}, "sbom")
		}
		if !isNoAssertion(p.CopyrightText) {
			c.Copyrights = []string{p.CopyrightText}
		}
		c.Homepage = p.Homepage
		if isNoAssertion(c.Homepage) {
			c.Homepage = ""
		}
		g.Components[c.ID] = c
		idOf[p.SPDXID] = c.ID
	}
	incoming := map[string]bool{}
	// Scope is decided by reachability, not by the first relationship
	// seen: a package that is a dev dependency of one thing but a normal
	// dependency of a runtime package ships.
	type kind int
	const (
		kNormal kind = iota
		kDev
		kOptional
	)
	kinds := map[[2]string]kind{}
	hasScopeInfo := false
	edge := func(from, to string, k kind) {
		f, t := idOf[from], idOf[to]
		if f == "" || t == "" {
			return
		}
		g.AddEdge(f, t)
		incoming[t] = true
		key := [2]string{f, t}
		if old, ok := kinds[key]; !ok || k < old {
			kinds[key] = k
		}
	}
	for _, r := range doc.Relationships {
		switch r.Type {
		case "DEPENDS_ON", "CONTAINS", "STATIC_LINK", "DYNAMIC_LINK":
			edge(r.Element, r.Related, kNormal)
		case "DEPENDENCY_OF", "RUNTIME_DEPENDENCY_OF", "CONTAINED_BY":
			edge(r.Related, r.Element, kNormal)
			hasScopeInfo = hasScopeInfo || r.Type == "RUNTIME_DEPENDENCY_OF"
		case "DEV_DEPENDENCY_OF", "TEST_DEPENDENCY_OF", "BUILD_DEPENDENCY_OF", "PROVIDED_DEPENDENCY_OF":
			edge(r.Related, r.Element, kDev)
			hasScopeInfo = true
		case "OPTIONAL_DEPENDENCY_OF":
			edge(r.Related, r.Element, kOptional)
			hasScopeInfo = true
		}
	}
	if len(g.Roots) == 0 {
		g.AddRoot("root:sbom", "project")
	}
	for id := range g.Components {
		if !incoming[id] {
			g.AddEdge(g.Roots[0], id)
		}
	}
	if hasScopeInfo {
		walk := func(allowed func(kind) bool) map[string]bool {
			seen := map[string]bool{}
			stack := append([]string(nil), g.Roots...)
			for len(stack) > 0 {
				n := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if seen[n] {
					continue
				}
				seen[n] = true
				for _, ch := range g.Edges[n] {
					if allowed(kinds[[2]string{n, ch}]) {
						stack = append(stack, ch)
					}
				}
			}
			return seen
		}
		rt := walk(func(k kind) bool { return k == kNormal })
		opt := walk(func(k kind) bool { return k != kDev })
		for id, c := range g.Components {
			switch {
			case rt[id]:
				c.Scope = model.ScopeRuntime
			case opt[id]:
				c.Scope = model.ScopeOptional
			default:
				c.Scope = model.ScopeDev
			}
		}
	}
	markDirect(g)
	return g, nil
}

func isNoAssertion(s string) bool {
	s = strings.TrimSpace(strings.ToUpper(s))
	return s == "" || s == "NOASSERTION" || s == "NONE"
}

func firstNonEmpty(s ...string) string {
	for _, x := range s {
		if x != "" {
			return x
		}
	}
	return ""
}

// ---- syft ------------------------------------------------------------------------

func runSyft(bin, dir string) (*model.Graph, error) {
	out, err := runCmd(dir, 15*time.Minute, nil, bin, "scan", "dir:"+dir, "-o", "cyclonedx-json", "-q")
	if err != nil {
		// syft < 1.0 has no "scan" subcommand.
		out, err = runCmd(dir, 15*time.Minute, nil, bin, "dir:"+dir, "-o", "cyclonedx-json", "-q")
		if err != nil {
			return nil, err
		}
	}
	g, err := IngestSBOM(out)
	if err != nil {
		return nil, err
	}
	g.Sources = append(g.Sources, "syft")
	return g, nil
}
