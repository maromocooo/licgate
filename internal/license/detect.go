package license

import (
	"regexp"
	"strings"
)

// Detection is the result of recognising a license from its text.
type Detection struct {
	ID         string
	Confidence float64 // 0..1
	// VersionAmbiguous is set for GPL-family texts: the license body alone
	// does not say whether "or later" applies, so we report -only (the
	// conservative choice) and flag it.
	VersionAmbiguous bool
}

var wsRe = regexp.MustCompile(`[\s\x{00a0}]+`)

func normText(s string) string {
	s = strings.ToLower(s)
	s = strings.NewReplacer("‘", "'", "’", "'", "“", `"`, "”", `"`, "`", "'", "*", " ", "#", " ").Replace(s)
	return wsRe.ReplaceAllString(s, " ")
}

func has(t string, phrases ...string) bool {
	for _, p := range phrases {
		if !strings.Contains(t, p) {
			return false
		}
	}
	return true
}

func hasAny(t string, phrases ...string) bool {
	for _, p := range phrases {
		if strings.Contains(t, p) {
			return true
		}
	}
	return false
}

// DetectText recognises common licenses from their full text or a standard
// short notice. It is deliberately phrase-based (no fuzzy diffing) so that
// results are explainable; anything it cannot place is left for the
// optional Jev classifier or manual review.
//
// Many license texts mention other licenses (GPL-2.0's preamble mentions the
// LGPL, MPL-2.0 lists the GPLs as "Secondary Licenses", SSPL is derived from
// the GPL…), so the family is chosen by whichever marker phrase occurs
// EARLIEST in the text — license files start with their own title.
func DetectText(raw string) Detection {
	// Files that start with their own terms and then append third-party
	// notices after a separator line: judge the leading part on its own so
	// a bundled BSD/MIT notice is not mistaken for the package's license.
	if main, ok := mainPart(raw); ok {
		t := normText(main)
		if d := detectNorm(t); d.ID != "" {
			return d
		}
		if restrictive(t) {
			return Detection{ID: RefProprietary, Confidence: 0.7}
		}
		d := detectNorm(normText(raw))
		if d.Confidence > 0.6 {
			d.Confidence = 0.6
		}
		return d
	}
	t := normText(raw)
	head := t
	if len(head) > 800 {
		head = head[:800]
	}
	if restrictive(head) {
		return Detection{ID: RefProprietary, Confidence: 0.7}
	}
	return detectNorm(t)
}

var separatorRe = regexp.MustCompile(`(?m)^\s*[-=_*#~]{5,}\s*$`)

// mainPart returns the text before the first separator line when that
// leading part is substantial.
func mainPart(raw string) (string, bool) {
	loc := separatorRe.FindStringIndex(raw)
	if loc == nil {
		return "", false
	}
	main := raw[:loc[0]]
	if len(strings.TrimSpace(main)) < 200 {
		return "", false
	}
	return main, true
}

// restrictive: "all rights reserved" without any open source grant.
func restrictive(t string) bool {
	return has(t, "all rights reserved") && !hasAny(t, grantPhrases...)
}

func detectNorm(t string) Detection {
	if len(strings.TrimSpace(t)) < 20 {
		return Detection{}
	}
	// Strong markers (license titles and characteristic grant phrases)
	// first; weak ones ("all rights reserved", "public domain", a bare
	// "MIT license" mention) only when nothing strong matched.
	for ti, tier := range [][]marker{markers, weakMarkers} {
		best, bestIdx := -1, len(t)+1
		for i, m := range tier {
			for _, p := range m.phrases {
				if idx := strings.Index(t, p); idx >= 0 && idx < bestIdx {
					best, bestIdx = i, idx
				}
			}
		}
		if best >= 0 {
			if d := tier[best].decide(t); d.ID != "" {
				// The license's own marker should be near the top; one
				// found deep inside a long file is probably a bundled
				// third-party notice.
				if ti == 0 && bestIdx > 1500 && d.Confidence > 0.6 {
					d.Confidence = 0.6
				}
				return d
			}
		}
	}
	return Detection{}
}

var grantPhrases = []string{
	"permission is hereby granted", "redistribution and use", "permission to use, copy",
	"is free software", "released under the", "apache license", "mozilla public license", "general public license",
	"this is free and unencumbered", "licensed under the mit", "licensed under the bsd", "licensed under the isc",
	"boost software license", "eclipse public license", "creative commons",
}

type marker struct {
	phrases []string
	decide  func(t string) Detection
}

func fixed(id string, conf float64) func(string) Detection {
	return func(string) Detection { return Detection{ID: id, Confidence: conf} }
}

func gnu(base string, versions [][2]string, fallback string) func(string) Detection {
	return func(t string) Detection {
		orLater := len(t) < 4000 && hasAny(t, "any later version", "or later version")
		for _, v := range versions {
			if strings.Contains(t, v[0]) {
				id := base + "-" + v[1]
				if orLater {
					return Detection{ID: id + "-or-later", Confidence: 0.9}
				}
				return Detection{ID: id + "-only", Confidence: 0.9, VersionAmbiguous: true}
			}
		}
		return Detection{ID: fallback, Confidence: 0.7}
	}
}

var markers = []marker{
	{[]string{"server side public license"}, fixed("SSPL-1.0", 0.95)},
	{[]string{"business source license"}, fixed("BUSL-1.1", 0.9)},
	{[]string{"elastic license 2.0", "elastic license version 2.0", "elastic license, version 2.0"}, fixed("Elastic-2.0", 0.95)},
	{[]string{"polyform noncommercial license"}, fixed("PolyForm-Noncommercial-1.0.0", 0.95)},
	{[]string{"polyform small business license"}, fixed("PolyForm-Small-Business-1.0.0", 0.95)},
	{[]string{"polyform shield license"}, fixed("PolyForm-Shield-1.0.0", 0.95)},
	{[]string{`"commons clause" license condition`, "commons clause license condition"}, fixed(RefCommons, 0.95)},
	{[]string{"shall be used for good, not evil"}, fixed("JSON", 0.9)},
	{[]string{"hippocratic license"}, fixed("Hippocratic-2.1", 0.85)},
	{[]string{"creative commons"}, func(t string) Detection {
		switch {
		case hasAny(t, "noncommercial", "non-commercial", "noderivatives", "no derivatives", "noderivs"):
			return Detection{ID: RefNonFree, Confidence: 0.85}
		case hasAny(t, "cc0 1.0 universal", "cc0"):
			return Detection{ID: "CC0-1.0", Confidence: 0.95}
		case has(t, "attribution-sharealike 4.0"):
			return Detection{ID: "CC-BY-SA-4.0", Confidence: 0.9}
		case has(t, "attribution-sharealike 3.0"):
			return Detection{ID: "CC-BY-SA-3.0", Confidence: 0.9}
		case has(t, "attribution 4.0 international"):
			return Detection{ID: "CC-BY-4.0", Confidence: 0.9}
		case has(t, "attribution 3.0"):
			return Detection{ID: "CC-BY-3.0", Confidence: 0.85}
		}
		return Detection{ID: RefNonFree, Confidence: 0.4}
	}},
	{[]string{"cc0 1.0 universal"}, fixed("CC0-1.0", 0.95)},
	{[]string{"gnu affero general public license"}, gnu("AGPL", [][2]string{{"version 3", "3.0"}}, RefAGPL)},
	{[]string{"gnu lesser general public license"}, gnu("LGPL", [][2]string{{"version 2.1, february 1999", "2.1"}, {"version 3, 29 june 2007", "3.0"}, {"version 3", "3.0"}, {"version 2.1", "2.1"}}, RefLGPL)},
	{[]string{"gnu library general public license"}, gnu("LGPL", [][2]string{{"version 2", "2.0"}}, RefLGPL)},
	{[]string{"gnu general public license"}, gnu("GPL", [][2]string{{"version 3, 29 june 2007", "3.0"}, {"version 2, june 1991", "2.0"}, {"version 3 of the license", "3.0"}, {"version 2 of the license", "2.0"}, {"gplv3", "3.0"}, {"gplv2", "2.0"}}, RefGPL)},
	{[]string{"mozilla public license"}, func(t string) Detection {
		if hasAny(t, "version 2.0", "v. 2.0", "mpl-2.0", "mpl 2.0") {
			return Detection{ID: "MPL-2.0", Confidence: 0.95}
		}
		if hasAny(t, "version 1.1", "mpl-1.1", "mpl 1.1") {
			return Detection{ID: "MPL-1.1", Confidence: 0.9}
		}
		return Detection{ID: "MPL-2.0", Confidence: 0.6}
	}},
	{[]string{"eclipse public license"}, func(t string) Detection {
		if hasAny(t, "v 2.0", "v2.0", "version 2.0", "2.0") {
			return Detection{ID: "EPL-2.0", Confidence: 0.95}
		}
		return Detection{ID: "EPL-1.0", Confidence: 0.9}
	}},
	{[]string{"common development and distribution license"}, func(t string) Detection {
		if has(t, "version 1.1") {
			return Detection{ID: "CDDL-1.1", Confidence: 0.9}
		}
		return Detection{ID: "CDDL-1.0", Confidence: 0.9}
	}},
	{[]string{"european union public licence"}, func(t string) Detection {
		if has(t, "v. 1.1") || (has(t, "1.1") && !has(t, "1.2")) {
			return Detection{ID: "EUPL-1.1", Confidence: 0.9}
		}
		return Detection{ID: "EUPL-1.2", Confidence: 0.9}
	}},
	{[]string{"apache license"}, func(t string) Detection {
		if hasAny(t, "version 2.0", "2.0") {
			return Detection{ID: "Apache-2.0", Confidence: 0.95}
		}
		if has(t, "version 1.1") {
			return Detection{ID: "Apache-1.1", Confidence: 0.9}
		}
		return Detection{ID: "Apache-2.0", Confidence: 0.6}
	}},
	{[]string{"boost software license"}, fixed("BSL-1.0", 0.95)},
	{[]string{"python software foundation license"}, fixed("PSF-2.0", 0.9)},
	{[]string{"this is free and unencumbered software released into the public domain"}, fixed("Unlicense", 0.95)},
	{[]string{"do what the fuck you want to public license"}, fixed("WTFPL", 0.95)},
	{[]string{"blue oak model license"}, fixed("BlueOak-1.0.0", 0.95)},
	{[]string{"sil open font license"}, fixed("OFL-1.1", 0.9)},
	{[]string{"the artistic license 2.0"}, fixed("Artistic-2.0", 0.9)},
	{[]string{"unicode, inc. license agreement", "unicode license v3", "unicode license agreement"}, fixed("Unicode-3.0", 0.8)},
	{[]string{"universal permissive license"}, fixed("UPL-1.0", 0.9)},
	{[]string{"permission is hereby granted, free of charge, to any person obtaining a copy"}, func(t string) Detection {
		if has(t, "the above copyright notice and this permission notice shall be included") {
			return Detection{ID: "MIT", Confidence: 0.95}
		}
		return Detection{ID: "MIT-0", Confidence: 0.85}
	}},
	{[]string{"permission to use, copy, modify, and/or distribute this software for any purpose with or without fee is hereby granted",
		"permission to use, copy, modify, and distribute this software for any purpose with or without fee is hereby granted"}, func(t string) Detection {
		if has(t, "provided that the above copyright notice and this permission notice appear in all copies") {
			return Detection{ID: "ISC", Confidence: 0.95}
		}
		return Detection{ID: "0BSD", Confidence: 0.9}
	}},
	{[]string{"redistribution and use in source and binary forms, with or without modification, are permitted"}, func(t string) Detection {
		switch {
		case has(t, "all advertising materials mentioning features"):
			return Detection{ID: "BSD-4-Clause", Confidence: 0.9}
		case hasAny(t, "neither the name", "may not be used to endorse or promote", "may be used to endorse or promote", "names of its contributors may not"):
			return Detection{ID: "BSD-3-Clause", Confidence: 0.95}
		}
		return Detection{ID: "BSD-2-Clause", Confidence: 0.9}
	}},
	{[]string{"altered source versions must be plainly marked"}, fixed("Zlib", 0.95)},
	{[]string{"permission to use, copy, modify, distribute, and sell this software and its documentation for any purpose is hereby granted without fee"}, fixed("HPND", 0.8)},
}

var weakMarkers = []marker{
	{[]string{"licensed under the mit license", "released under the mit license", "under the terms of the mit license", "mit license"}, fixed("MIT", 0.8)},
	{[]string{"public domain"}, func(t string) Detection {
		if len(t) < 1500 && !has(t, "all rights reserved") {
			return Detection{ID: RefPublicDomain, Confidence: 0.6}
		}
		return Detection{}
	}},
	{[]string{"all rights reserved"}, func(t string) Detection {
		if !hasAny(t, "permission is hereby granted", "redistribution and use", "licensed under", "permission to use") {
			return Detection{ID: RefProprietary, Confidence: 0.6}
		}
		return Detection{}
	}},
}

var copyrightRe = regexp.MustCompile(`(?im)^[\s#*/;-]*((?:copyright|\(c\)|©)\s*(?:\(c\)|©)?\s*[0-9][^\n]{0,200})$`)

// ExtractCopyrights returns distinct copyright lines from a license file.
func ExtractCopyrights(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range copyrightRe.FindAllStringSubmatch(text, 20) {
		line := strings.TrimSpace(m[1])
		lc := strings.ToLower(line)
		// Skip template lines from license bodies.
		if strings.Contains(lc, "<year>") || strings.Contains(lc, "[yyyy]") || strings.Contains(lc, "copyright holder") ||
			strings.Contains(lc, "copyright notice") || strings.Contains(lc, "free software foundation") && strings.Contains(lc, "2007") ||
			strings.Contains(lc, "[year]") || strings.Contains(lc, "{yyyy}") {
			continue
		}
		if !seen[line] {
			seen[line] = true
			out = append(out, line)
		}
	}
	return out
}
