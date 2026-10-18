package license

import (
	"regexp"
	"strings"
)

// Normalized is the result of cleaning up a raw license declaration.
type Normalized struct {
	// Expr is an SPDX expression ("" when nothing usable was found).
	Expr string
	// Unknown lists leaves that are neither in the catalog nor a family we
	// can classify.
	Unknown []string
	// SeeFile is set for npm-style "SEE LICENSE IN <file>".
	SeeFile string
	// Guessed is true when the expression came from fuzzy alias matching
	// rather than a valid SPDX expression.
	Guessed bool
}

// OK reports whether every leaf is classifiable.
func (n Normalized) OK() bool { return n.Expr != "" && len(n.Unknown) == 0 }

var emptyDecl = map[string]bool{
	"": true, "unknown": true, "noassertion": true, "none": true, "n/a": true, "na": true,
	"other": true, "other/proprietary license": false, "license": true, "see license": true,
	"see license file": true, "see license.txt": true, "see license.md": true, "custom": true,
}

// alias keys are compared after squash() (lower case, punctuation folded to
// single spaces).
var aliases = map[string]string{
	"mit": "MIT", "mit license": "MIT", "the mit license": "MIT", "mit licence": "MIT", "expat": "MIT",
	"mit x11": "MIT", "mit expat": "MIT", "mit style": "MIT", "mit like": "MIT", "mit 0": "MIT-0",
	"mit no attribution": "MIT-0",
	"isc":                "ISC", "isc license": "ISC", "isc license iscl": "ISC", "iscl": "ISC",
	"apache": "Apache-2.0", "apache 2": "Apache-2.0", "apache 2 0": "Apache-2.0", "apache2": "Apache-2.0",
	"apache license": "Apache-2.0", "apache license 2": "Apache-2.0", "apache license 2 0": "Apache-2.0",
	"apache license version 2 0": "Apache-2.0", "apache software license": "Apache-2.0",
	"apache software license 2 0": "Apache-2.0", "apache 2 0 license": "Apache-2.0", "asl 2 0": "Apache-2.0",
	"apache license v2": "Apache-2.0", "apache license v2 0": "Apache-2.0", "al2": "Apache-2.0",
	"apache software license version 2 0": "Apache-2.0", "the apache software license version 2 0": "Apache-2.0",
	"the apache license version 2 0": "Apache-2.0", "apache 2 license": "Apache-2.0",
	"bsd": RefBSD, "bsd license": RefBSD, "bsd style": RefBSD, "bsd like": RefBSD, "bsd style license": RefBSD,
	"new bsd": "BSD-3-Clause", "new bsd license": "BSD-3-Clause", "modified bsd": "BSD-3-Clause",
	"modified bsd license": "BSD-3-Clause", "revised bsd": "BSD-3-Clause", "3 clause bsd": "BSD-3-Clause",
	"bsd 3 clause": "BSD-3-Clause", "bsd 3": "BSD-3-Clause", "3 clause bsd license": "BSD-3-Clause",
	"bsd 3 clause license": "BSD-3-Clause", "the 3 clause bsd license": "BSD-3-Clause",
	"simplified bsd": "BSD-2-Clause", "simplified bsd license": "BSD-2-Clause", "2 clause bsd": "BSD-2-Clause",
	"bsd 2 clause": "BSD-2-Clause", "bsd 2": "BSD-2-Clause", "freebsd": "BSD-2-Clause",
	"bsd 2 clause license": "BSD-2-Clause", "the 2 clause bsd license": "BSD-2-Clause",
	"0bsd": "0BSD", "bsd zero clause": "0BSD",
	"zlib": "Zlib", "zlib license": "Zlib", "zlib libpng": "Zlib",
	"boost": "BSL-1.0", "boost software license": "BSL-1.0", "boost software license 1 0": "BSL-1.0", "bsl 1 0": "BSL-1.0",
	"psf": "PSF-2.0", "psfl": "PSF-2.0", "psf license": "PSF-2.0", "python software foundation license": "PSF-2.0",
	"python software foundation": "PSF-2.0", "psf 2 0": "PSF-2.0", "python": "Python-2.0",
	"public domain": RefPublicDomain, "publicdomain": RefPublicDomain, "public domain dedication": RefPublicDomain,
	"unlicense": "Unlicense", "the unlicense": "Unlicense", "the unlicense unlicense": "Unlicense",
	"cc0": "CC0-1.0", "cc0 1 0": "CC0-1.0", "cc0 1 0 universal": "CC0-1.0", "cc0 universal": "CC0-1.0",
	"cc0 1 0 universal cc0 1 0 public domain dedication": "CC0-1.0",
	"wtfpl": "WTFPL",
	"mpl":   RefMPL, "mpl 2": "MPL-2.0", "mpl 2 0": "MPL-2.0", "mpl2": "MPL-2.0", "mozilla public license 2 0": "MPL-2.0",
	"mozilla public license 2 0 mpl 2 0": "MPL-2.0", "mozilla public license version 2 0": "MPL-2.0",
	"mozilla public license 1 1": "MPL-1.1", "mozilla public license 1 1 mpl 1 1": "MPL-1.1",
	"lgpl": RefLGPL, "lgpl license": RefLGPL, "gnu lgpl": RefLGPL, "gnu lesser general public license": RefLGPL,
	"gnu library or lesser general public license lgpl": RefLGPL,
	"lgplv2": "LGPL-2.0-only", "lgpl 2": "LGPL-2.0-only", "lgplv2 1": "LGPL-2.1-only", "lgpl 2 1": "LGPL-2.1-only",
	"lgpl v2 1": "LGPL-2.1-only", "lgpl 2 1 only": "LGPL-2.1-only", "lgpl 2 1 or later": "LGPL-2.1-or-later",
	"lgplv2 1 plus": "LGPL-2.1-or-later", "lgplv2 plus": "LGPL-2.0-or-later", "lgplv3": "LGPL-3.0-only",
	"lgpl 3": "LGPL-3.0-only", "lgpl v3": "LGPL-3.0-only", "lgplv3 plus": "LGPL-3.0-or-later", "lgpl 3 0": "LGPL-3.0-only",
	"gnu lesser general public license v2 lgplv2":               "LGPL-2.0-only",
	"gnu lesser general public license v2 or later lgplv2 plus": "LGPL-2.0-or-later",
	"gnu lesser general public license v3 lgplv3":               "LGPL-3.0-only",
	"gnu lesser general public license v3 or later lgplv3 plus": "LGPL-3.0-or-later",
	"gnu lesser general public license v2 1":                    "LGPL-2.1-only",
	"gpl":                                                       RefGPL, "gpl license": RefGPL, "gnu gpl": RefGPL, "gnu general public license": RefGPL, "gnu general public license gpl": RefGPL,
	"gplv2": "GPL-2.0-only", "gpl 2": "GPL-2.0-only", "gpl v2": "GPL-2.0-only", "gpl 2 0": "GPL-2.0-only",
	"gplv2 plus": "GPL-2.0-or-later", "gpl 2 plus": "GPL-2.0-or-later", "gpl v2 plus": "GPL-2.0-or-later",
	"gpl 2 or later": "GPL-2.0-or-later", "gpl v2 or later": "GPL-2.0-or-later", "gplv2 or later": "GPL-2.0-or-later",
	"gpl v3 or later": "GPL-3.0-or-later", "gplv3 or later": "GPL-3.0-or-later", "lgpl v2 1 or later": "LGPL-2.1-or-later",
	"lgpl v3 or later": "LGPL-3.0-or-later", "lgplv3 or later": "LGPL-3.0-or-later", "gpl 2 0 or later": "GPL-2.0-or-later",
	"gplv3": "GPL-3.0-only", "gpl 3": "GPL-3.0-only", "gpl v3": "GPL-3.0-only", "gpl 3 0": "GPL-3.0-only",
	"gplv3 plus": "GPL-3.0-or-later", "gpl 3 plus": "GPL-3.0-or-later", "gpl v3 plus": "GPL-3.0-or-later",
	"gpl 3 or later": "GPL-3.0-or-later", "gpl 3 0 or later": "GPL-3.0-or-later",
	"gnu general public license v2 gplv2":               "GPL-2.0-only",
	"gnu general public license v2 or later gplv2 plus": "GPL-2.0-or-later",
	"gnu general public license v3 gplv3":               "GPL-3.0-only",
	"gnu general public license v3 or later gplv3 plus": "GPL-3.0-or-later",
	"gnu general public license version 2":              "GPL-2.0-only", "gnu general public license version 3": "GPL-3.0-only",
	"gnu gpl v3": "GPL-3.0-only", "gnu gplv3": "GPL-3.0-only", "gnu gpl v2": "GPL-2.0-only", "gnu gplv2": "GPL-2.0-only",
	"agpl": RefAGPL, "agplv3": "AGPL-3.0-only", "agpl 3": "AGPL-3.0-only", "agpl v3": "AGPL-3.0-only",
	"agplv3 plus": "AGPL-3.0-or-later", "agpl 3 0": "AGPL-3.0-only", "gnu agpl": RefAGPL, "gnu agplv3": "AGPL-3.0-only",
	"gnu affero general public license v3":                      "AGPL-3.0-only",
	"gnu affero general public license v3 agplv3":               "AGPL-3.0-only",
	"gnu affero general public license v3 or later agplv3 plus": "AGPL-3.0-or-later",
	"eclipse public license 1 0":                                "EPL-1.0", "eclipse public license 2 0": "EPL-2.0", "epl": "EPL-1.0", "epl 2": "EPL-2.0",
	"cddl": "CDDL-1.0", "cddl 1 0": "CDDL-1.0", "cddl 1 1": "CDDL-1.1",
	"artistic": RefArtistic, "artistic license": RefArtistic, "artistic 2": "Artistic-2.0", "artistic license 2 0": "Artistic-2.0",
	"eupl": "EUPL-1.2", "eupl 1 2": "EUPL-1.2", "european union public licence 1 2 eupl 1 2": "EUPL-1.2",
	"sspl": "SSPL-1.0", "server side public license": "SSPL-1.0", "busl": "BUSL-1.1", "bsl 1 1": "BUSL-1.1",
	"business source license": "BUSL-1.1", "elastic": "Elastic-2.0", "elastic license 2 0": "Elastic-2.0", "elv2": "Elastic-2.0",
	"commons clause": RefCommons,
	"ofl":            "OFL-1.1", "sil open font license 1 1": "OFL-1.1", "sil ofl 1 1": "OFL-1.1", "ofl 1 1": "OFL-1.1",
	"cc by 4 0": "CC-BY-4.0", "cc by 3 0": "CC-BY-3.0", "cc by sa 4 0": "CC-BY-SA-4.0", "cc by sa 3 0": "CC-BY-SA-3.0",
	"cc by nc 4 0": "CC-BY-NC-4.0", "cc by nc sa 4 0": "CC-BY-NC-SA-4.0", "cc by nd 4 0": "CC-BY-ND-4.0",
	"unlicensed": RefProprietary, "proprietary": RefProprietary, "commercial": RefProprietary,
	"other proprietary license": RefProprietary, "all rights reserved": RefProprietary,
	"copyright": RefProprietary, "private": RefProprietary,
	"hpnd": "HPND", "historical permission notice and disclaimer hpnd": "HPND",
	"upl": "UPL-1.0", "universal permissive license": "UPL-1.0",
	"ms pl": "MS-PL", "microsoft public license": "MS-PL",
	"zpl 2 1": "ZPL-2.1", "zope public license": "ZPL-2.1",
	"json": "JSON", "ncsa": "NCSA", "university of illinois ncsa open source license": "NCSA",
	"postgresql": "PostgreSQL", "postgresql license": "PostgreSQL",
}

var squashRe = regexp.MustCompile(`[^a-z0-9+]+`)

func squash(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "+", " plus ")
	s = squashRe.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}

var seeFileRe = regexp.MustCompile(`(?i)^\s*see\s+licen[cs]e\s+in\s+(.+?)\s*$`)

// Normalize turns a raw declaration (package.json "license", PyPI "License",
// Cargo "license", a CycloneDX license name …) into an SPDX expression.
func Normalize(raw string) Normalized {
	s := strings.TrimSpace(raw)
	s = strings.Trim(s, `"'`)
	if m := seeFileRe.FindStringSubmatch(s); m != nil {
		return Normalized{SeeFile: strings.TrimSpace(m[1])}
	}
	if emptyDecl[strings.ToLower(s)] {
		return Normalized{}
	}
	// Long free text: probably a full license body pasted into metadata.
	if len(s) > 200 {
		if d := DetectText(s); d.ID != "" && d.Confidence >= 0.8 {
			return Normalized{Expr: d.ID, Guessed: true}
		}
		return Normalized{}
	}
	// 1) strict SPDX
	if e, err := Parse(s); err == nil {
		if unk := unknownLeaves(e); len(unk) == 0 {
			return Normalized{Expr: e.String()}
		}
	}
	// 2) whole-string alias
	if v, ok := aliases[squash(s)]; ok {
		return Normalized{Expr: v, Guessed: true}
	}
	// 2b) drop parentheticals such as "(with FOSS License Exception)" or
	// "(see LICENSE)": exceptions only grant extra permissions, so ignoring
	// them is the conservative reading. Any other parenthetical ("MIT
	// (non-commercial use only)") may restrict and is kept.
	if i := strings.Index(s, "("); i > 0 && droppableParen(s[i:]) {
		if v, ok := aliases[squash(s[:i])]; ok {
			return Normalized{Expr: v, Guessed: true}
		}
		if e, err := Parse(strings.TrimSpace(s[:i])); err == nil && len(unknownLeaves(e)) == 0 {
			return Normalized{Expr: e.String(), Guessed: true}
		}
	}
	// 3) split on common separators and alias each part.
	if expr, ok := splitAlias(s); ok {
		return Normalized{Expr: expr, Guessed: true}
	}
	// 4) strict parse that has unknown leaves: keep, report unknowns.
	if e, err := Parse(s); err == nil {
		return Normalized{Expr: e.String(), Unknown: unknownLeaves(e)}
	}
	// 5) give up: keep the text as a LicenseRef so it shows up in reports.
	ref := "LicenseRef-" + strings.Trim(squashRe.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(ref) > 80 {
		ref = ref[:80]
	}
	return Normalized{Expr: ref, Unknown: []string{ref}, Guessed: true}
}

func droppableParen(p string) bool {
	lc := strings.ToLower(p)
	if restrictiveWords(lc) {
		return false
	}
	return strings.Contains(lc, "exception") || strings.HasPrefix(strings.TrimLeft(lc, "( "), "see ") ||
		strings.Contains(lc, "license file") || strings.Contains(lc, "licence file")
}

// HasRestrictiveWords reports whether a declaration mentions terms that
// usually restrict use (commercial, military, …).
func HasRestrictiveWords(s string) bool { return restrictiveWords(strings.ToLower(s)) }

func restrictiveWords(lc string) bool {
	for _, w := range []string{"commercial", "evil", "military", "nuclear", "personal use", "educational", "non-profit", "nonprofit", "commons clause", "proprietary"} {
		if strings.Contains(lc, w) {
			return true
		}
	}
	return false
}

var splitRe = regexp.MustCompile(`(?i)\s+or\s+|\s*/\s*|\s+and\s+|\s*;\s*|\s*,\s*|\s*&\s*`)

func splitAlias(s string) (string, bool) {
	isOr := regexp.MustCompile(`(?i)\s+or\s+|\s*/\s*`).MatchString(s)
	isAnd := regexp.MustCompile(`(?i)\s+and\s+|\s*;\s*|\s*,\s*|\s*&\s*`).MatchString(s)
	if !isOr && !isAnd {
		return "", false
	}
	parts := splitRe.Split(strings.Trim(s, "()"), -1)
	var ids []string
	for _, p := range parts {
		p = strings.TrimSpace(strings.Trim(p, "()"))
		if p == "" {
			continue
		}
		if v, ok := aliases[squash(p)]; ok {
			ids = append(ids, v)
			continue
		}
		if e, err := Parse(p); err == nil && len(unknownLeaves(e)) == 0 {
			ids = append(ids, e.String())
			continue
		}
		return "", false
	}
	if len(ids) < 2 {
		return "", false
	}
	op := " AND "
	if isOr && !isAnd {
		op = " OR "
	}
	// "Apache 2.0 / Commons Clause": a restriction is an add-on, never an
	// alternative.
	for _, id := range ids {
		if info, ok := Lookup(id); ok && (info.Category == CatSourceAvailable || info.Category == CatProprietary) {
			op = " AND "
		}
	}
	// Mixed separators: be conservative (AND).
	for i, id := range ids {
		if strings.Contains(id, " ") {
			ids[i] = "(" + id + ")"
		}
	}
	return strings.Join(ids, op), true
}

func unknownLeaves(e Expr) []string {
	var out []string
	for _, l := range Leaves(e) {
		if !IsKnown(l.ID) {
			out = append(out, l.ID)
		}
		if l.Exception != "" {
			if _, ok := LookupException(l.Exception); !ok {
				out = append(out, l.Exception)
			}
		}
	}
	return out
}

// FromClassifiers maps PyPI trove classifiers ("License :: OSI Approved ::
// MIT License") to SPDX ids. Multiple license classifiers are combined with
// OR, which is how dual-licensed Python projects use them.
func FromClassifiers(cls []string) Normalized {
	var ids []string
	seen := map[string]bool{}
	for _, c := range cls {
		if !strings.HasPrefix(c, "License ::") {
			continue
		}
		parts := strings.Split(c, "::")
		last := strings.TrimSpace(parts[len(parts)-1])
		if last == "OSI Approved" || last == "" {
			continue
		}
		var id string
		if v, ok := classifierMap[last]; ok {
			id = v
		} else if n := Normalize(last); n.OK() {
			id = n.Expr
		} else {
			continue
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return Normalized{}
	}
	return Normalized{Expr: strings.Join(ids, " OR "), Guessed: true}
}

var classifierMap = map[string]string{
	"MIT License":                                             "MIT",
	"MIT No Attribution License (MIT-0)":                      "MIT-0",
	"Apache Software License":                                 "Apache-2.0",
	"BSD License":                                             RefBSD,
	"ISC License (ISCL)":                                      "ISC",
	"Python Software Foundation License":                      "PSF-2.0",
	"Mozilla Public License 2.0 (MPL 2.0)":                    "MPL-2.0",
	"Mozilla Public License 1.1 (MPL 1.1)":                    "MPL-1.1",
	"GNU General Public License (GPL)":                        RefGPL,
	"GNU General Public License v2 (GPLv2)":                   "GPL-2.0-only",
	"GNU General Public License v2 or later (GPLv2+)":         "GPL-2.0-or-later",
	"GNU General Public License v3 (GPLv3)":                   "GPL-3.0-only",
	"GNU General Public License v3 or later (GPLv3+)":         "GPL-3.0-or-later",
	"GNU Lesser General Public License v2 (LGPLv2)":           "LGPL-2.0-only",
	"GNU Lesser General Public License v2 or later (LGPLv2+)": "LGPL-2.0-or-later",
	"GNU Lesser General Public License v3 (LGPLv3)":           "LGPL-3.0-only",
	"GNU Lesser General Public License v3 or later (LGPLv3+)": "LGPL-3.0-or-later",
	"GNU Library or Lesser General Public License (LGPL)":     RefLGPL,
	"GNU Affero General Public License v3":                    "AGPL-3.0-only",
	"GNU Affero General Public License v3 or later (AGPLv3+)": "AGPL-3.0-or-later",
	"Eclipse Public License 1.0 (EPL-1.0)":                    "EPL-1.0",
	"Eclipse Public License 2.0 (EPL-2.0)":                    "EPL-2.0",
	"The Unlicense (Unlicense)":                               "Unlicense",
	"Public Domain":                                           RefPublicDomain,
	"CC0 1.0 Universal (CC0 1.0) Public Domain Dedication":    "CC0-1.0",
	"zlib/libpng License":                                     "Zlib",
	"Boost Software License 1.0 (BSL-1.0)":                    "BSL-1.0",
	"Academic Free License (AFL)":                             "AFL-3.0",
	"Artistic License":                                        RefArtistic,
	"European Union Public Licence 1.2 (EUPL 1.2)":            "EUPL-1.2",
	"Historical Permission Notice and Disclaimer (HPND)":      "HPND",
	"Universal Permissive License (UPL)":                      "UPL-1.0",
	"Other/Proprietary License":                               RefProprietary,
	"Freely Distributable":                                    RefPermissive,
	"Freeware":                                                RefProprietary,
}
