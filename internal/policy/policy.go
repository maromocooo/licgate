// Package policy decides, for every dependency, whether it can ship under
// the license you intend to publish with — given how you distribute
// (binary, library, SaaS, internal), how dependencies are linked, and
// whether a dependency actually ends up in what you ship.
//
// Rules are explicit and explainable rather than exhaustive: every verdict
// carries the reasons that produced it. Results are "no known
// incompatibility found", never legal advice.
package policy

import (
	"sort"
	"strings"

	"github.com/licgate/licgate/internal/i18n"
	"github.com/licgate/licgate/internal/license"
	"github.com/licgate/licgate/internal/model"
)

// Level is the severity of a finding. Higher is worse.
type Level int

const (
	LevelSkip   Level = iota // not shipped (dev-only) or ignored
	LevelOK                  // nothing to do
	LevelNotice              // fine, but there are obligations (attribution etc.)
	LevelReview              // a human should look at this
	LevelBlock               // known incompatibility with your intended license
)

func (l Level) String() string {
	switch l {
	case LevelSkip:
		return "SKIP"
	case LevelOK:
		return "OK"
	case LevelNotice:
		return "NOTICE"
	case LevelReview:
		return "REVIEW"
	case LevelBlock:
		return "BLOCK"
	}
	return "?"
}

// ParseLevel parses a level name (case-insensitive). "never" returns a
// level above BLOCK so nothing fails.
func ParseLevel(s string) (Level, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "block":
		return LevelBlock, true
	case "review":
		return LevelReview, true
	case "notice":
		return LevelNotice, true
	case "never", "none", "off":
		return LevelBlock + 1, true
	}
	return 0, false
}

// Distribution is how your project reaches other people.
type Distribution string

const (
	DistBinary   Distribution = "binary"   // you ship an artifact that contains the deps (app, CLI binary, container, bundled JS)
	DistLibrary  Distribution = "library"  // you publish source to a registry; users install deps themselves
	DistSaaS     Distribution = "saas"     // you run it as a network service; nothing is handed out
	DistInternal Distribution = "internal" // used only inside your organisation
)

// ParseDistribution validates a distribution name.
func ParseDistribution(s string) (Distribution, bool) {
	switch Distribution(strings.ToLower(s)) {
	case DistBinary, DistLibrary, DistSaaS, DistInternal:
		return Distribution(strings.ToLower(s)), true
	}
	return "", false
}

// Linking says how dependencies are combined with your code.
type Linking string

const (
	LinkAuto    Linking = "auto"
	LinkStatic  Linking = "static"
	LinkDynamic Linking = "dynamic"
)

// ParseLinking validates a linking mode.
func ParseLinking(s string) (Linking, bool) {
	switch Linking(strings.ToLower(s)) {
	case LinkAuto, LinkStatic, LinkDynamic:
		return Linking(strings.ToLower(s)), true
	}
	return "", false
}

// Override forces a license for matching packages.
type Override struct {
	Package string `toml:"package" json:"package"`
	License string `toml:"license" json:"license"`
	Reason  string `toml:"reason" json:"reason,omitempty"`
}

// Ignore excludes matching packages from evaluation.
type Ignore struct {
	Package string `toml:"package" json:"package"`
	Reason  string `toml:"reason" json:"reason,omitempty"`
}

// Settings is what the user tells us about their release.
type Settings struct {
	License      string       `json:"license"`
	Distribution Distribution `json:"distribution"`
	Linking      Linking      `json:"linking"`
	IncludeDev   bool         `json:"include_dev"`
	Allow        []string     `json:"allow,omitempty"`
	Deny         []string     `json:"deny,omitempty"`
	Overrides    []Override   `json:"overrides,omitempty"`
	Ignores      []Ignore     `json:"ignores,omitempty"`
}

// Obligation keys.
const (
	ObAttribution = "attribution"
	ObNoticeFile  = "notice_file"
	ObLGPLRelink  = "lgpl_relink"
	ObFileSource  = "file_source"
	ObGPLSource   = "gpl_source"
	ObAGPLNetwork = "agpl_network"
	ObUseTerms    = "use_terms"
)

// Finding is the verdict for one component.
type Finding struct {
	Component   *model.Component `json:"component"`
	Level       Level            `json:"-"`
	LevelName   string           `json:"level"`
	Reasons     []string         `json:"reasons"`
	Obligations []string         `json:"obligations,omitempty"`
	// Chosen is the alternative picked from an OR expression.
	Chosen string   `json:"chosen,omitempty"`
	Path   []string `json:"path,omitempty"`
	// Linking actually assumed for this component.
	Linking Linking `json:"linking,omitempty"`
}

// Outbound describes the license you intend to publish under.
type Outbound struct {
	Expr        string   `json:"expr"`
	Proprietary bool     `json:"proprietary"`
	Leaves      []string `json:"leaves"`
	Unknown     []string `json:"unknown,omitempty"`
}

// Result is the full evaluation.
type Result struct {
	Settings Settings       `json:"settings"`
	Outbound Outbound       `json:"outbound"`
	Findings []*Finding     `json:"findings"`
	Counts   map[string]int `json:"counts"`
	Warnings []string       `json:"warnings,omitempty"`
	Graph    *model.Graph   `json:"-"`
}

// Incomplete reports whether part of the project could not be read.
func (r *Result) Incomplete() bool { return r.Graph != nil && len(r.Graph.Errors) > 0 }

// AllWarnings returns collection errors (first) and warnings.
func (r *Result) AllWarnings() []string {
	var out []string
	if r.Graph != nil {
		for _, e := range r.Graph.Errors {
			out = append(out, "ERROR: "+e)
		}
		out = append(out, r.Graph.Warnings...)
	}
	return append(out, r.Warnings...)
}

// Worst returns the highest level among findings.
func (r *Result) Worst() Level {
	w := LevelSkip
	for _, f := range r.Findings {
		if f.Level > w {
			w = f.Level
		}
	}
	return w
}

// ObligationText returns the human text for an obligation key.
func ObligationText(key string) string { return i18n.T("ob." + key) }

// ParseOutbound interprets the intended outbound license.
func ParseOutbound(s string) Outbound {
	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "", "proprietary", "closed", "closed-source", "commercial", "unlicensed", "private", "all-rights-reserved":
		return Outbound{Expr: "proprietary", Proprietary: true, Leaves: []string{"proprietary"}}
	}
	n := license.Normalize(s)
	if n.Expr == "" {
		return Outbound{Expr: s, Unknown: []string{s}, Leaves: []string{s}}
	}
	e, err := license.Parse(n.Expr)
	if err != nil {
		return Outbound{Expr: s, Unknown: []string{s}, Leaves: []string{s}}
	}
	out := Outbound{Expr: e.String(), Unknown: n.Unknown}
	for _, l := range license.Leaves(e) {
		if l.ID == license.RefProprietary {
			out.Proprietary = true
		}
		out.Leaves = append(out.Leaves, l.ID)
	}
	return out
}

// Evaluate runs the policy over every component of g.
func Evaluate(g *model.Graph, s Settings) *Result {
	out := ParseOutbound(s.License)
	res := &Result{Settings: s, Outbound: out, Counts: map[string]int{}, Graph: g}
	if len(out.Unknown) > 0 {
		res.Warnings = append(res.Warnings, i18n.T("warn.outbound_unknown", strings.Join(out.Unknown, ", ")))
	}
	ignored := func(c *model.Component) (string, bool) {
		for _, ig := range s.Ignores {
			if matchPackage(ig.Package, c) {
				return ig.Reason, true
			}
		}
		return "", false
	}
	for _, c := range g.Sorted() {
		for _, ov := range s.Overrides {
			if matchPackage(ov.Package, c) {
				// Work on a copy so re-evaluating with other settings (TUI)
				// starts from the collected data again.
				cc := *c
				c = &cc
				c.FileLicense = ""
				c.LicenseNotes = nil // notes about the collected license no longer apply
				n := license.Normalize(ov.License)
				c.License = n.Expr
				if c.License == "" {
					c.License = ov.License
				}
				c.LicenseSource = "override"
				if ov.Reason != "" {
					c.LicenseNotes = append(c.LicenseNotes, ov.Reason)
				}
			}
		}
		f := &Finding{Component: c}
		if p := g.PathTo(c.ID); p != nil {
			for _, id := range p {
				f.Path = append(f.Path, g.Label(id))
			}
		}
		switch reason, ig := ignored(c); {
		case ig:
			f.Level = LevelSkip
			f.Reasons = []string{i18n.T("r.ignored", reason)}
		case c.Scope == model.ScopeDev && !s.IncludeDev:
			f.Level = LevelSkip
			f.Reasons = []string{i18n.T("r.dev_only")}
		default:
			evaluateComponent(f, out, s)
		}
		f.LevelName = f.Level.String()
		res.Findings = append(res.Findings, f)
		res.Counts[f.LevelName]++
	}
	gplVersionConflicts(res)
	res.Counts = map[string]int{}
	for _, f := range res.Findings {
		f.LevelName = f.Level.String()
		res.Counts[f.LevelName]++
	}
	sort.SliceStable(res.Findings, func(i, j int) bool { return res.Findings[i].Level > res.Findings[j].Level })
	return res
}

// gplVersionConflicts catches combinations that are fine one by one but not
// together: with a "GPL-2.0-or-later" outbound license, a GPL-2.0-only
// dependency pins the whole work to GPL-2.0 while e.g. an Apache-2.0 or
// GPL-3.0 dependency pins it to GPL-3.0 — no version satisfies both.
func gplVersionConflicts(res *Result) {
	s := res.Settings
	if s.Distribution != DistBinary && s.Distribution != DistLibrary {
		return
	}
	var v2ok, v3ok bool
	for _, l := range res.Outbound.Leaves {
		a, b, _ := gplVersions(l)
		v2ok, v3ok = v2ok || a, v3ok || b
	}
	if !(v2ok && v3ok) {
		return
	}
	var only2, only3 []*Finding
	for _, f := range res.Findings {
		if f.Level == LevelSkip || f.Level == LevelBlock || f.Chosen == "" && f.Component.License == "" {
			continue
		}
		expr := f.Chosen
		if expr == "" {
			expr = f.Component.License
		}
		e, err := license.Parse(expr)
		if err != nil {
			continue
		}
		need2, need3 := false, false
		for _, l := range license.Leaves(e) {
			info, ok := license.Lookup(l.ID)
			if !ok || info.Category == license.CatPublicDomain {
				continue
			}
			g2, g3 := info.GPL2, info.GPL3
			if l.Exception != "" {
				if ex, ok := license.LookupException(l.Exception); ok && (ex.Linking || ex.GPL2) {
					continue
				}
			}
			if g2 && !g3 {
				need2 = true
			}
			if g3 && !g2 {
				need3 = true
			}
		}
		if need2 {
			only2 = append(only2, f)
		}
		if need3 {
			only3 = append(only3, f)
		}
	}
	if len(only2) == 0 || len(only3) == 0 {
		return
	}
	lvl := LevelBlock
	if s.Distribution == DistLibrary {
		lvl = LevelReview
	}
	ex := only3[0].Component.Key()
	for _, f := range only2 {
		if f.Level < lvl {
			f.Level = lvl
		}
		f.Reasons = append(f.Reasons, i18n.T("r.gpl_version_conflict", f.Component.Key(), ex, len(only3)))
	}
	res.Warnings = append(res.Warnings, i18n.T("warn.gpl_version_conflict", len(only2), len(only3)))
}

func matchPackage(pattern string, c *model.Component) bool {
	p := strings.TrimSpace(pattern)
	if p == "" {
		return false
	}
	lc := strings.ToLower(p)
	cands := []string{c.Name, c.Key(), c.ID, c.PURL, c.Ecosystem + "/" + c.Name, c.Ecosystem + "/" + c.Key()}
	for _, x := range cands {
		if x != "" && strings.ToLower(x) == lc {
			return true
		}
	}
	// purl without version
	if c.PURL != "" {
		base := c.PURL
		if i := strings.LastIndex(base, "@"); i > strings.Index(base, "/") {
			base = base[:i]
		}
		if strings.ToLower(base) == lc {
			return true
		}
	}
	// trailing * wildcard
	if strings.HasSuffix(p, "*") {
		pre := strings.ToLower(strings.TrimSuffix(p, "*"))
		for _, x := range cands {
			if x != "" && strings.HasPrefix(strings.ToLower(x), pre) {
				return true
			}
		}
	}
	return false
}

func effectiveLinking(s Settings, c *model.Component) Linking {
	if s.Linking == LinkStatic || s.Linking == LinkDynamic {
		return s.Linking
	}
	switch c.Ecosystem {
	case "golang", "cargo", "swift", "cocoapods", "conan", "github", "generic":
		return LinkStatic
	case "npm":
		// Shipped JS is almost always bundled into one artifact.
		return LinkStatic
	case "pypi", "gem", "composer", "maven", "nuget", "hex", "pub", "cran", "conda", "deb", "rpm", "apk":
		return LinkDynamic
	}
	return LinkStatic
}

// verdict is an intermediate result for one expression node.
type verdict struct {
	level   Level
	reasons []string
	obs     []string
	chosen  string
}

func (v *verdict) add(l Level, reason string, obs ...string) {
	if l > v.level {
		v.level = l
	}
	if reason != "" {
		v.reasons = append(v.reasons, reason)
	}
	v.obs = append(v.obs, obs...)
}

func evaluateComponent(f *Finding, out Outbound, s Settings) {
	c := f.Component
	f.Linking = effectiveLinking(s, c)
	if c.License == "" {
		f.Level = LevelReview
		var files []string
		for _, lf := range c.LicenseFiles {
			if !lf.IsNotice {
				files = append(files, lf.Path)
			}
		}
		switch {
		case len(c.Declared) > 0:
			f.Reasons = []string{i18n.T("r.license_unrecognised", strings.Join(c.Declared, " | "))}
		case len(files) > 0:
			f.Reasons = []string{i18n.T("r.license_file_unrecognised", strings.Join(files, ", "))}
		default:
			f.Reasons = []string{i18n.T("r.no_license_info")}
		}
		f.Reasons = append(f.Reasons, c.LicenseNotes...)
		return
	}
	e, err := license.Parse(c.License)
	if err != nil {
		f.Level = LevelReview
		f.Reasons = []string{i18n.T("r.bad_expr", c.License)}
		return
	}
	v := evalOutbound(e, out, s, c, f.Linking)
	if c.FileLicense != "" {
		// Metadata and the bundled license file disagree: judge both, keep
		// the worse, and make sure a human looks at it.
		if fe, err := license.Parse(c.FileLicense); err == nil {
			fv := evalOutbound(fe, out, s, c, f.Linking)
			if fv.level > v.level {
				fv.chosen = v.chosen
				v = fv
			}
		}
		v.add(LevelReview, i18n.T("r.file_mismatch", c.License, c.FileLicense))
	}
	f.Level, f.Reasons, f.Chosen = v.level, dedupe(v.reasons), v.chosen
	f.Obligations = dedupe(v.obs)
	if v.chosen == c.License {
		f.Chosen = ""
	}
	// Notices on the component (e.g. guessed license) become reasons.
	for _, n := range c.LicenseNotes {
		f.Reasons = append(f.Reasons, n)
	}
	if f.Level == LevelOK && len(f.Obligations) > 0 {
		f.Level = LevelNotice
	}
	// For a BLOCK the obligations are moot until the conflict is resolved.
	if f.Level == LevelBlock {
		f.Obligations = nil
	}
}

// evalOutbound judges a dependency expression against every alternative of
// a dual-licensed outbound license: the project must be distributable under
// each of them, so the worst result counts — but for each outbound
// alternative the dependency may pick its own best alternative.
func evalOutbound(e license.Expr, out Outbound, s Settings, c *model.Component, link Linking) verdict {
	var worst verdict
	for i, ol := range out.Leaves {
		v := evalExpr(e, ol, out.Proprietary && ol == "proprietary", s, c, link)
		if i == 0 || v.level > worst.level {
			reasons, obs := worst.reasons, worst.obs
			worst = v
			worst.reasons = append(reasons, v.reasons...)
			worst.obs = append(obs, v.obs...)
		} else {
			worst.reasons = append(worst.reasons, v.reasons...)
			worst.obs = append(worst.obs, v.obs...)
		}
	}
	return worst
}

func evalExpr(e license.Expr, outLeaf string, outIsProp bool, s Settings, c *model.Component, link Linking) verdict {
	switch x := e.(type) {
	case license.Leaf:
		return judgeLeaf(x, outLeaf, outIsProp, s, c, link)
	case *license.Op:
		var kids []verdict
		for _, k := range x.Kids {
			kids = append(kids, evalExpr(k, outLeaf, outIsProp, s, c, link))
		}
		if x.And {
			v := verdict{level: LevelOK}
			var parts []string
			for _, k := range kids {
				v.add(k.level, "")
				v.reasons = append(v.reasons, k.reasons...)
				v.obs = append(v.obs, k.obs...)
				parts = append(parts, k.chosen)
			}
			v.chosen = strings.Join(parts, " AND ")
			return v
		}
		// OR: pick the best alternative.
		best := 0
		for i, k := range kids {
			if k.level < kids[best].level || (k.level == kids[best].level && len(k.obs) < len(kids[best].obs)) {
				best = i
			}
		}
		v := kids[best]
		if len(kids) > 1 && v.level < LevelBlock {
			v.reasons = append([]string{i18n.T("r.or_choice", v.chosen)}, v.reasons...)
		} else if len(kids) > 1 {
			v.reasons = append([]string{i18n.T("r.or_none")}, v.reasons...)
		}
		return v
	}
	return verdict{level: LevelReview}
}

type outClass int

const (
	outProprietary outClass = iota
	outPermissive
	outWeak
	outGPL
	outOtherCopyleft
	outUnknown
)

// gplVersions returns which GPL versions a GPL-family outbound license can
// be combined under, and whether it is AGPL.
func gplVersions(id string) (v2, v3, agpl bool) {
	switch id {
	case "GPL-2.0-only":
		return true, false, false
	case "GPL-2.0-or-later":
		return true, true, false
	case "GPL-3.0-only", "GPL-3.0-or-later":
		return false, true, false
	case "AGPL-3.0-only", "AGPL-3.0-or-later":
		return false, true, true
	}
	return false, false, false
}

func classifyOutbound(id string, proprietary bool) (outClass, license.Info) {
	if proprietary || id == license.RefProprietary {
		return outProprietary, license.Info{ID: "proprietary", Category: license.CatProprietary}
	}
	info, ok := license.Lookup(id)
	if !ok {
		return outUnknown, license.Info{ID: id}
	}
	if v2, v3, _ := gplVersions(info.ID); v2 || v3 {
		return outGPL, info
	}
	switch info.Category {
	case license.CatPublicDomain, license.CatPermissive:
		return outPermissive, info
	case license.CatWeakCopyleft:
		return outWeak, info
	case license.CatSourceAvailable, license.CatProprietary:
		return outProprietary, info
	}
	return outOtherCopyleft, info
}

func judgeLeaf(l license.Leaf, outID string, outIsProp bool, s Settings, c *model.Component, link Linking) verdict {
	v := verdict{level: LevelOK, chosen: l.String()}
	id := l.ID
	for _, d := range s.Deny {
		if listMatches(d, id) {
			v.add(LevelBlock, i18n.T("r.denied", id))
			return v
		}
	}
	for _, a := range s.Allow {
		if listMatches(a, id) {
			v.add(LevelOK, i18n.T("r.allowed", id))
			return v
		}
	}
	di, known := license.Lookup(id)
	if !known {
		v.add(LevelReview, i18n.T("r.unknown_license", id))
		return v
	}
	if di.Ambiguous {
		v.add(LevelOK, i18n.T("r.ambiguous", di.Name))
	}
	cat := di.Category
	gpl2, gpl3 := di.GPL2, di.GPL3
	linkExc := false
	if l.Exception != "" {
		if ex, ok := license.LookupException(l.Exception); ok {
			if ex.Linking && (cat == license.CatStrongCopyleft || cat == license.CatWeakCopyleft || cat == license.CatNetworkCopyleft) {
				linkExc = true
			}
			if ex.GPL2 {
				gpl2 = true
			}
		} else {
			v.add(LevelReview, i18n.T("r.unknown_exception", l.Exception))
		}
	}
	oc, oi := classifyOutbound(outID, outIsProp)
	ov2, ov3, oagpl := gplVersions(oi.ID)
	dist := s.Distribution
	name := di.ID
	if strings.HasPrefix(name, "LicenseRef-") {
		name = di.Name
	}

	gplCompatible := func() (ok bool, onlyV3 bool) {
		ok = (ov2 && gpl2) || (ov3 && gpl3)
		onlyV3 = ok && ov2 && ov3 && !gpl2
		return
	}
	// incompatible returns the level for a GPL incompatibility: a
	// placeholder ("other permissive license") only gets REVIEW because we
	// don't actually know its terms.
	incompatible := func() Level {
		if di.Ambiguous {
			return LevelReview
		}
		return LevelBlock
	}

	// Things that don't depend on the outbound license much.
	switch cat {
	case license.CatProprietary:
		switch dist {
		case DistBinary:
			if oc == outProprietary {
				v.add(LevelReview, i18n.T("r.dep_proprietary_review"))
			} else {
				v.add(LevelBlock, i18n.T("r.dep_proprietary_block"))
			}
		default:
			v.add(LevelReview, i18n.T("r.dep_proprietary_review"))
		}
		return v
	case license.CatSourceAvailable:
		switch {
		case dist == DistBinary && oc != outProprietary:
			v.add(LevelBlock, i18n.T("r.source_available_oss", name), ObUseTerms)
		default:
			v.add(LevelReview, i18n.T("r.source_available", name), ObUseTerms)
		}
		return v
	}

	if di.FontOnly {
		v.add(LevelOK, i18n.T("r.font"))
	}

	switch dist {
	case DistInternal:
		switch cat {
		case license.CatNetworkCopyleft:
			v.add(LevelNotice, i18n.T("r.internal_network", name))
		default:
			v.add(LevelOK, "")
		}
		return v

	case DistSaaS:
		switch {
		case cat == license.CatNetworkCopyleft && !linkExc:
			switch {
			case oc == outGPL && (oagpl || ov3):
				v.add(LevelNotice, i18n.T("r.saas_agpl_ok", name), ObAGPLNetwork)
			case oc == outProprietary:
				v.add(LevelBlock, i18n.T("r.saas_agpl_block", name), ObAGPLNetwork)
			default:
				v.add(LevelReview, i18n.T("r.saas_agpl_review", name), ObAGPLNetwork)
			}
		case cat == license.CatStrongCopyleft && !linkExc && strings.HasPrefix(di.ID, "EUPL"):
			// EUPL's "Communication" includes providing online access.
			v.add(LevelReview, i18n.T("r.saas_eupl", name))
		case cat == license.CatStrongCopyleft && !linkExc && c.Ecosystem == "npm":
			// JS that runs in the browser is shipped to every visitor.
			v.add(LevelReview, i18n.T("r.saas_frontend", name))
		case cat == license.CatStrongCopyleft && !linkExc:
			v.add(LevelOK, i18n.T("r.saas_gpl", name))
		default:
			v.add(LevelOK, "")
		}
		return v

	case DistLibrary:
		switch oc {
		case outGPL:
			if cat == license.CatPublicDomain {
				return v
			}
			if linkExc {
				v.add(LevelOK, i18n.T("r.link_exception", l.Exception))
				return v
			}
			if di.FontOnly {
				v.add(LevelOK, i18n.T("r.font_aggregate", name))
				return v
			}
			if ok, onlyV3 := gplCompatible(); !ok {
				v.add(incompatible(), i18n.T("r.gpl_incompatible", name, oi.ID))
			} else if onlyV3 {
				v.add(LevelNotice, i18n.T("r.gpl_becomes_v3", name))
			}
			if cat == license.CatNetworkCopyleft && !oagpl {
				v.add(LevelNotice, i18n.T("r.agpl_in_gpl3", name), ObAGPLNetwork)
			}
		case outUnknown, outOtherCopyleft:
			if cat != license.CatPublicDomain && cat != license.CatPermissive {
				v.add(LevelReview, i18n.T("r.outbound_rules_missing", outID))
			}
		default: // proprietary, permissive, weak
			if r := lgplOutboundConflict(oi.ID, di, gpl2, gpl3); r != "" && (cat == license.CatPermissive || cat == license.CatWeakCopyleft) {
				v.add(LevelReview, i18n.T(r, name, oi.ID))
			}
			switch {
			case linkExc:
				v.add(LevelOK, i18n.T("r.link_exception", l.Exception))
			case cat == license.CatStrongCopyleft:
				v.add(LevelReview, i18n.T("r.lib_gpl", name, outLabel(outID, oc)))
			case cat == license.CatNetworkCopyleft:
				v.add(LevelReview, i18n.T("r.lib_agpl", name, outLabel(outID, oc)))
			case cat == license.CatWeakCopyleft && oc == outProprietary:
				v.add(LevelOK, i18n.T("r.lib_weak", name))
			}
		}
		return v
	}

	// DistBinary: the deps are inside what you hand out.
	if di.Attribution {
		v.obs = append(v.obs, ObAttribution)
	}
	if di.NoticeFile && hasNotice(c) {
		v.obs = append(v.obs, ObNoticeFile)
	}
	switch oc {
	case outGPL:
		if cat == license.CatPublicDomain {
			return v
		}
		if linkExc {
			v.add(LevelNotice, i18n.T("r.link_exception", l.Exception), ObFileSource)
			return v
		}
		if di.FontOnly {
			v.add(LevelNotice, i18n.T("r.font_aggregate", name))
			return v
		}
		ok, onlyV3 := gplCompatible()
		switch {
		case !ok:
			if l.ID == "EPL-2.0" {
				v.add(LevelBlock, i18n.T("r.epl2_secondary", oi.ID))
			} else {
				v.add(incompatible(), i18n.T("r.gpl_incompatible", name, oi.ID))
			}
			return v
		case onlyV3:
			v.add(LevelNotice, i18n.T("r.gpl_becomes_v3", name))
		}
		switch cat {
		case license.CatStrongCopyleft:
			v.add(LevelNotice, i18n.T("r.gpl_in_gpl", name), ObGPLSource)
		case license.CatNetworkCopyleft:
			if oagpl {
				v.add(LevelNotice, i18n.T("r.gpl_in_gpl", name), ObGPLSource, ObAGPLNetwork)
			} else {
				v.add(LevelNotice, i18n.T("r.agpl_in_gpl3", name), ObGPLSource, ObAGPLNetwork)
			}
		case license.CatWeakCopyleft:
			v.add(LevelNotice, "", ObGPLSource)
		}
		return v

	case outUnknown, outOtherCopyleft:
		if cat != license.CatPublicDomain && cat != license.CatPermissive {
			v.add(LevelReview, i18n.T("r.outbound_rules_missing", outID))
		}
		return v
	}

	// proprietary / permissive / weak outbound, binary distribution
	if r := lgplOutboundConflict(oi.ID, di, gpl2, gpl3); r != "" && (cat == license.CatPermissive || cat == license.CatWeakCopyleft) {
		v.add(LevelReview, i18n.T(r, name, oi.ID))
	}
	switch {
	case cat == license.CatPublicDomain || cat == license.CatPermissive:
		// attribution handled above
	case linkExc:
		v.add(LevelNotice, i18n.T("r.link_exception", l.Exception), ObFileSource)
	case cat == license.CatWeakCopyleft:
		if strings.HasPrefix(name, "LGPL") || name == license.RefLGPL {
			if link == LinkStatic {
				v.add(LevelReview, i18n.T("r.lgpl_static", name), ObLGPLRelink)
			} else {
				v.add(LevelNotice, i18n.T("r.lgpl_dynamic", name), ObLGPLRelink)
			}
		} else {
			v.add(LevelNotice, i18n.T("r.file_copyleft", name), ObFileSource)
		}
	case cat == license.CatStrongCopyleft:
		v.add(LevelBlock, i18n.T("r.bin_gpl", name, outLabel(outID, oc)))
	case cat == license.CatNetworkCopyleft:
		v.add(LevelBlock, i18n.T("r.bin_agpl", name, outLabel(outID, oc)))
	}
	return v
}

// lgplOutboundConflict: an LGPL project has the same "no further
// restrictions" rule as the GPL version it derives from, so e.g. Apache-2.0
// code cannot simply be folded into an LGPL-2.1-only library. Returns a
// message key or "".
func lgplOutboundConflict(outID string, di license.Info, gpl2, gpl3 bool) string {
	var v2, v3 bool
	switch outID {
	case "LGPL-2.0-only", "LGPL-2.1-only":
		v2 = true
	case "LGPL-2.0-or-later", "LGPL-2.1-or-later":
		v2, v3 = true, true
	case "LGPL-3.0-only", "LGPL-3.0-or-later":
		v3 = true
	default:
		return ""
	}
	if di.Category == license.CatPublicDomain || (v2 && gpl2) || (v3 && gpl3) {
		return ""
	}
	return "r.lgpl_out_incompatible"
}

// listMatches compares an allow/deny entry with a license id. Entries are
// normalised ("GPL-3.0" = "GPL-3.0-only"), and an entry without -only /
// -or-later matches the whole family ("GPL-3.0" also denies
// GPL-3.0-or-later).
func listMatches(entry, id string) bool {
	if strings.EqualFold(entry, id) {
		return true
	}
	n := license.Normalize(entry)
	if n.Expr != "" && strings.EqualFold(n.Expr, id) {
		return true
	}
	base := func(x string) string {
		x = strings.TrimSuffix(strings.TrimSuffix(x, "-only"), "-or-later")
		return strings.ToLower(strings.TrimSuffix(x, "+"))
	}
	if !strings.HasSuffix(entry, "-only") && !strings.HasSuffix(entry, "-or-later") {
		return base(entry) == base(id)
	}
	return false
}

func outLabel(id string, oc outClass) string {
	if oc == outProprietary {
		return i18n.T("w.proprietary")
	}
	return id
}

func hasNotice(c *model.Component) bool {
	for _, f := range c.LicenseFiles {
		if f.IsNotice {
			return true
		}
	}
	return false
}

func dedupe(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
