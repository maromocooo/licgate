// Package license knows about license identifiers: how to normalise messy
// declarations into SPDX, how to parse SPDX expressions, how to recognise a
// license from its text, and what kind of license each identifier is.
package license

import "strings"

// Category is the coarse family a license belongs to. Compatibility rules in
// the policy package are written against categories plus a few per-license
// flags, so adding a license usually means adding one catalog row.
type Category int

const (
	CatUnknown Category = iota
	CatPublicDomain
	CatPermissive
	CatWeakCopyleft
	CatStrongCopyleft
	CatNetworkCopyleft
	CatSourceAvailable // non-OSS: non-commercial, SSPL, BUSL, Elastic…
	CatProprietary     // all rights reserved / UNLICENSED
)

func (c Category) String() string {
	switch c {
	case CatPublicDomain:
		return "public-domain"
	case CatPermissive:
		return "permissive"
	case CatWeakCopyleft:
		return "weak-copyleft"
	case CatStrongCopyleft:
		return "strong-copyleft"
	case CatNetworkCopyleft:
		return "network-copyleft"
	case CatSourceAvailable:
		return "source-available"
	case CatProprietary:
		return "proprietary"
	}
	return "unknown"
}

// Info describes one license.
type Info struct {
	ID       string
	Name     string
	Category Category
	// GPL2 / GPL3: may code under this license be combined into a work
	// distributed under GPL-2.0 / GPL-3.0 (per FSF's published compatibility
	// list)? For GPL-family licenses this encodes which GPL versions they can
	// end up under.
	GPL2, GPL3 bool
	// Attribution: the license requires keeping copyright/license notices
	// when redistributing.
	Attribution bool
	// NoticeFile: Apache-2.0 style — a NOTICE file shipped with the work must
	// be passed on.
	NoticeFile bool
	// Ambiguous marks synthetic entries for declarations that do not pin a
	// single license (e.g. "BSD", "GPL" without a version).
	Ambiguous bool
	// FontOnly marks licenses that are normally only seen on fonts/assets.
	FontOnly bool
}

// Exception describes a WITH exception.
type Exception struct {
	ID string
	// Linking: the exception lets other code link against / be compiled with
	// the licensed code without the copyleft extending to it.
	Linking bool
	// GPL2: makes an Apache-2.0 base compatible with GPL-2.0 (LLVM-exception).
	GPL2 bool
}

const (
	RefPublicDomain = "LicenseRef-licgate-public-domain"
	RefBSD          = "LicenseRef-licgate-BSD-unspecified"
	RefGPL          = "LicenseRef-licgate-GPL-unversioned"
	RefLGPL         = "LicenseRef-licgate-LGPL-unversioned"
	RefAGPL         = "LicenseRef-licgate-AGPL-unversioned"
	RefProprietary  = "LicenseRef-licgate-proprietary"
	RefPermissive   = "LicenseRef-licgate-other-permissive"
	RefCopyleft     = "LicenseRef-licgate-other-copyleft"
	RefWeakCopyleft = "LicenseRef-licgate-other-weak-copyleft"
	RefNonFree      = "LicenseRef-licgate-source-available"
	RefCommons      = "LicenseRef-Commons-Clause"
	RefMPL          = "LicenseRef-licgate-MPL-unversioned"
	RefArtistic     = "LicenseRef-licgate-Artistic-unversioned"
)

func perm(id, name string, gpl2, gpl3 bool) Info {
	return Info{ID: id, Name: name, Category: CatPermissive, GPL2: gpl2, GPL3: gpl3, Attribution: true}
}

func pd(id, name string) Info {
	return Info{ID: id, Name: name, Category: CatPublicDomain, GPL2: true, GPL3: true}
}

var catalogList = []Info{
	// public domain & equivalent
	pd("CC0-1.0", "Creative Commons Zero v1.0 Universal"),
	pd("Unlicense", "The Unlicense"),
	pd("0BSD", "BSD Zero Clause License"),
	pd("MIT-0", "MIT No Attribution"),
	pd("WTFPL", "Do What The F*ck You Want To Public License"),
	pd(RefPublicDomain, "Public domain (declared)"),

	// permissive
	perm("MIT", "MIT License", true, true),
	perm("BlueOak-1.0.0", "Blue Oak Model License 1.0.0", true, true),
	perm("MIT-Modern-Variant", "MIT License Modern Variant", true, true),
	perm("MIT-open-group", "MIT Open Group variant", true, true),
	perm("MIT-feh", "feh License", true, true),
	perm("BSD-2-Clause-Patent", "BSD-2-Clause Plus Patent License", true, true),
	perm("BSD-2-Clause-Views", "BSD 2-Clause with views sentence", true, true),
	perm("BSD-3-Clause-LBNL", "Lawrence Berkeley National Labs BSD variant license", true, true),
	perm("BSD-3-Clause-Modification", "BSD 3-Clause Modification", true, true),
	perm("BSD-Source-Code", "BSD Source Code Attribution", true, true),
	perm("ISC", "ISC License", true, true),
	perm("BSD-1-Clause", "BSD 1-Clause License", true, true),
	perm("BSD-2-Clause", "BSD 2-Clause \"Simplified\" License", true, true),
	perm("BSD-3-Clause", "BSD 3-Clause \"New\" or \"Revised\" License", true, true),
	perm("BSD-3-Clause-Clear", "BSD 3-Clause Clear License", true, true),
	perm("BSD-4-Clause", "BSD 4-Clause \"Original\" License (advertising clause)", false, false),
	{ID: "Apache-2.0", Name: "Apache License 2.0", Category: CatPermissive, GPL2: false, GPL3: true, Attribution: true, NoticeFile: true},
	perm("Apache-1.1", "Apache License 1.1", false, false),
	perm("Apache-1.0", "Apache License 1.0", false, false),
	perm("Zlib", "zlib License", true, true),
	perm("BSL-1.0", "Boost Software License 1.0", true, true),
	perm("PSF-2.0", "Python Software Foundation License 2.0", true, true),
	perm("Python-2.0", "Python License 2.0", true, true),
	perm("X11", "X11 License", true, true),
	perm("MIT-CMU", "CMU License", true, true),
	perm("NCSA", "University of Illinois/NCSA Open Source License", true, true),
	perm("PostgreSQL", "PostgreSQL License", true, true),
	perm("curl", "curl License", true, true),
	perm("Unicode-DFS-2016", "Unicode License Agreement - Data Files and Software (2016)", true, true),
	perm("Unicode-3.0", "Unicode License v3", true, true),
	perm("Artistic-2.0", "Artistic License 2.0", true, true),
	perm("CC-BY-3.0", "Creative Commons Attribution 3.0", false, false),
	perm("CC-BY-4.0", "Creative Commons Attribution 4.0", true, true),
	{ID: "OFL-1.1", Name: "SIL Open Font License 1.1", Category: CatPermissive, Attribution: true, FontOnly: true},
	perm("OpenSSL", "OpenSSL License (advertising clause)", false, false),
	perm("AFL-3.0", "Academic Free License v3.0", false, false),
	perm("AFL-2.1", "Academic Free License v2.1", false, false),
	perm("ZPL-2.1", "Zope Public License 2.1", true, true),
	perm("W3C", "W3C Software Notice and License", true, true),
	perm("Libpng", "libpng License", true, true),
	perm("libpng-2.0", "PNG Reference Library version 2", true, true),
	perm("IJG", "Independent JPEG Group License", true, true),
	perm("HPND", "Historical Permission Notice and Disclaimer", true, true),
	perm("FTL", "Freetype Project License", false, true),
	perm("bzip2-1.0.6", "bzip2 and libbzip2 License", true, true),
	perm("MirOS", "The MirOS Licence", true, true),
	perm("UPL-1.0", "Universal Permissive License v1.0", true, true),
	perm("Vim", "Vim License", true, true),
	perm("Beerware", "Beerware License", true, true),
	perm("Ruby", "Ruby License", true, true),
	perm("MS-PL", "Microsoft Public License", false, false),
	perm("ECL-2.0", "Educational Community License v2.0", false, true),
	perm("Unicode-TOU", "Unicode Terms of Use", false, false),
	perm("CC-PDDC", "Creative Commons Public Domain Dedication and Certification", true, true),
	perm("CDLA-Permissive-2.0", "Community Data License Agreement Permissive 2.0", true, true),
	perm("CDLA-Permissive-1.0", "Community Data License Agreement Permissive 1.0", true, true),
	perm("NTP", "NTP License", true, true),
	perm("ICU", "ICU License", true, true),
	perm("OLDAP-2.8", "Open LDAP Public License v2.8", true, true),
	perm("Info-ZIP", "Info-ZIP License", true, true),
	perm("TCL", "TCL/TK License", true, true),
	perm("Unicode-DFS-2015", "Unicode License Agreement - Data Files and Software (2015)", true, true),
	perm("MulanPSL-2.0", "Mulan Permissive Software License v2", false, true),
	perm("PHP-3.01", "PHP License v3.01", false, false),
	perm("PHP-3.0", "PHP License v3.0", false, false),
	perm("SSH-OpenSSH", "SSH OpenSSH license", true, true),
	perm("Spencer-94", "Spencer License 94", true, true),
	perm("ISC-Veillard", "ISC Veillard variant", true, true),
	perm("X11-distribute-modifications-variant", "X11 License Distribution Modification Variant", true, true),
	{ID: "OFL-1.0", Name: "SIL Open Font License 1.0", Category: CatPermissive, Attribution: true, FontOnly: true},
	{ID: "Ubuntu-font-1.0", Name: "Ubuntu Font Licence v1.0", Category: CatPermissive, Attribution: true, FontOnly: true},
	{ID: RefBSD, Name: "BSD (variant not specified)", Category: CatPermissive, GPL2: true, GPL3: true, Attribution: true, Ambiguous: true},
	{ID: RefPermissive, Name: "Other permissive license", Category: CatPermissive, Attribution: true, Ambiguous: true},

	// weak copyleft
	{ID: "LGPL-2.0-only", Name: "GNU Library General Public License v2 only", Category: CatWeakCopyleft, GPL2: true, GPL3: true, Attribution: true},
	{ID: "LGPL-2.0-or-later", Name: "GNU Library General Public License v2 or later", Category: CatWeakCopyleft, GPL2: true, GPL3: true, Attribution: true},
	{ID: "LGPL-2.1-only", Name: "GNU Lesser General Public License v2.1 only", Category: CatWeakCopyleft, GPL2: true, GPL3: true, Attribution: true},
	{ID: "LGPL-2.1-or-later", Name: "GNU Lesser General Public License v2.1 or later", Category: CatWeakCopyleft, GPL2: true, GPL3: true, Attribution: true},
	{ID: "LGPL-3.0-only", Name: "GNU Lesser General Public License v3.0 only", Category: CatWeakCopyleft, GPL2: false, GPL3: true, Attribution: true},
	{ID: "LGPL-3.0-or-later", Name: "GNU Lesser General Public License v3.0 or later", Category: CatWeakCopyleft, GPL2: false, GPL3: true, Attribution: true},
	{ID: RefLGPL, Name: "LGPL (version not specified)", Category: CatWeakCopyleft, GPL2: true, GPL3: true, Attribution: true, Ambiguous: true},
	{ID: "MPL-2.0", Name: "Mozilla Public License 2.0", Category: CatWeakCopyleft, GPL2: true, GPL3: true, Attribution: true},
	{ID: "MPL-2.0-no-copyleft-exception", Name: "MPL 2.0 (Incompatible With Secondary Licenses)", Category: CatWeakCopyleft, Attribution: true},
	{ID: "MPL-1.1", Name: "Mozilla Public License 1.1", Category: CatWeakCopyleft, Attribution: true},
	{ID: "MPL-1.0", Name: "Mozilla Public License 1.0", Category: CatWeakCopyleft, Attribution: true},
	{ID: RefMPL, Name: "MPL (version not specified)", Category: CatWeakCopyleft, Attribution: true, Ambiguous: true},
	{ID: RefArtistic, Name: "Artistic License (version not specified)", Category: CatWeakCopyleft, Attribution: true, Ambiguous: true},
	{ID: "EPL-1.0", Name: "Eclipse Public License 1.0", Category: CatWeakCopyleft, Attribution: true},
	{ID: "EPL-2.0", Name: "Eclipse Public License 2.0", Category: CatWeakCopyleft, Attribution: true},
	{ID: "CDDL-1.0", Name: "Common Development and Distribution License 1.0", Category: CatWeakCopyleft, Attribution: true},
	{ID: "CDDL-1.1", Name: "Common Development and Distribution License 1.1", Category: CatWeakCopyleft, Attribution: true},
	{ID: "CPL-1.0", Name: "Common Public License 1.0", Category: CatWeakCopyleft, Attribution: true},
	{ID: "MS-RL", Name: "Microsoft Reciprocal License", Category: CatWeakCopyleft, Attribution: true},
	{ID: "Artistic-1.0", Name: "Artistic License 1.0", Category: CatWeakCopyleft, Attribution: true},
	{ID: "Artistic-1.0-Perl", Name: "Artistic License 1.0 (Perl)", Category: CatWeakCopyleft, Attribution: true},
	{ID: RefWeakCopyleft, Name: "Other weak (file-level) copyleft license", Category: CatWeakCopyleft, Attribution: true, Ambiguous: true},
	{ID: "LPPL-1.3c", Name: "LaTeX Project Public License v1.3c", Category: CatWeakCopyleft, Attribution: true},
	{ID: "APSL-2.0", Name: "Apple Public Source License 2.0", Category: CatWeakCopyleft, Attribution: true},

	// strong copyleft
	{ID: "GPL-2.0-only", Name: "GNU General Public License v2.0 only", Category: CatStrongCopyleft, GPL2: true, GPL3: false, Attribution: true},
	{ID: "GPL-2.0-or-later", Name: "GNU General Public License v2.0 or later", Category: CatStrongCopyleft, GPL2: true, GPL3: true, Attribution: true},
	{ID: "GPL-3.0-only", Name: "GNU General Public License v3.0 only", Category: CatStrongCopyleft, GPL3: true, Attribution: true},
	{ID: "GPL-3.0-or-later", Name: "GNU General Public License v3.0 or later", Category: CatStrongCopyleft, GPL3: true, Attribution: true},
	{ID: RefGPL, Name: "GPL (version not specified)", Category: CatStrongCopyleft, GPL2: true, GPL3: true, Attribution: true, Ambiguous: true},
	{ID: "CC-BY-SA-4.0", Name: "Creative Commons Attribution Share Alike 4.0", Category: CatStrongCopyleft, GPL3: true, Attribution: true},
	{ID: "CC-BY-SA-3.0", Name: "Creative Commons Attribution Share Alike 3.0", Category: CatStrongCopyleft, Attribution: true},
	{ID: "EUPL-1.1", Name: "European Union Public License 1.1", Category: CatStrongCopyleft, GPL2: true, Attribution: true},
	{ID: "EUPL-1.2", Name: "European Union Public License 1.2", Category: CatStrongCopyleft, GPL2: true, GPL3: true, Attribution: true},
	{ID: "CECILL-2.1", Name: "CeCILL Free Software License Agreement v2.1", Category: CatStrongCopyleft, GPL2: true, GPL3: true, Attribution: true},
	{ID: "Sleepycat", Name: "Sleepycat License", Category: CatStrongCopyleft, GPL2: true, GPL3: true, Attribution: true},
	{ID: "GFDL-1.3-only", Name: "GNU Free Documentation License v1.3 only", Category: CatStrongCopyleft, Attribution: true},
	{ID: "GFDL-1.3-or-later", Name: "GNU Free Documentation License v1.3 or later", Category: CatStrongCopyleft, Attribution: true},
	{ID: "GFDL-1.2-only", Name: "GNU Free Documentation License v1.2 only", Category: CatStrongCopyleft, Attribution: true},
	{ID: "ODbL-1.0", Name: "Open Data Commons Open Database License v1.0", Category: CatStrongCopyleft, Attribution: true},
	{ID: RefCopyleft, Name: "Other copyleft license", Category: CatStrongCopyleft, Attribution: true, Ambiguous: true},

	// network copyleft
	{ID: "AGPL-3.0-only", Name: "GNU Affero General Public License v3.0 only", Category: CatNetworkCopyleft, GPL3: true, Attribution: true},
	{ID: "AGPL-3.0-or-later", Name: "GNU Affero General Public License v3.0 or later", Category: CatNetworkCopyleft, GPL3: true, Attribution: true},
	{ID: "AGPL-1.0-only", Name: "Affero General Public License v1.0 only", Category: CatNetworkCopyleft, Attribution: true},
	{ID: RefAGPL, Name: "AGPL (version not specified)", Category: CatNetworkCopyleft, GPL3: true, Attribution: true, Ambiguous: true},
	{ID: "OSL-3.0", Name: "Open Software License 3.0", Category: CatNetworkCopyleft, Attribution: true},
	{ID: "RPL-1.5", Name: "Reciprocal Public License 1.5", Category: CatNetworkCopyleft, Attribution: true},

	// source-available / non-free
	{ID: "SSPL-1.0", Name: "Server Side Public License v1", Category: CatSourceAvailable, Attribution: true},
	{ID: "BUSL-1.1", Name: "Business Source License 1.1", Category: CatSourceAvailable, Attribution: true},
	{ID: "Elastic-2.0", Name: "Elastic License 2.0", Category: CatSourceAvailable, Attribution: true},
	{ID: "PolyForm-Noncommercial-1.0.0", Name: "PolyForm Noncommercial License 1.0.0", Category: CatSourceAvailable, Attribution: true},
	{ID: "PolyForm-Small-Business-1.0.0", Name: "PolyForm Small Business License 1.0.0", Category: CatSourceAvailable, Attribution: true},
	{ID: "PolyForm-Shield-1.0.0", Name: "PolyForm Shield License 1.0.0", Category: CatSourceAvailable, Attribution: true},
	{ID: "CC-BY-NC-4.0", Name: "Creative Commons Attribution Non Commercial 4.0", Category: CatSourceAvailable, Attribution: true},
	{ID: "CC-BY-NC-SA-4.0", Name: "Creative Commons Attribution Non Commercial Share Alike 4.0", Category: CatSourceAvailable, Attribution: true},
	{ID: "CC-BY-ND-4.0", Name: "Creative Commons Attribution No Derivatives 4.0", Category: CatSourceAvailable, Attribution: true},
	{ID: "CC-BY-NC-ND-4.0", Name: "Creative Commons Attribution Non Commercial No Derivatives 4.0", Category: CatSourceAvailable, Attribution: true},
	{ID: "CC-BY-NC-3.0", Name: "Creative Commons Attribution Non Commercial 3.0", Category: CatSourceAvailable, Attribution: true},
	{ID: "CC-BY-NC-SA-3.0", Name: "Creative Commons Attribution Non Commercial Share Alike 3.0", Category: CatSourceAvailable, Attribution: true},
	{ID: "CC-BY-ND-3.0", Name: "Creative Commons Attribution No Derivatives 3.0", Category: CatSourceAvailable, Attribution: true},
	{ID: "JSON", Name: "JSON License (\"shall be used for Good, not Evil\")", Category: CatSourceAvailable, Attribution: true},
	{ID: "Hippocratic-2.1", Name: "Hippocratic License 2.1", Category: CatSourceAvailable, Attribution: true},
	{ID: RefCommons, Name: "Commons Clause", Category: CatSourceAvailable, Attribution: true},
	{ID: RefNonFree, Name: "Source-available / non-commercial license", Category: CatSourceAvailable, Attribution: true, Ambiguous: true},

	// proprietary
	{ID: RefProprietary, Name: "Proprietary / all rights reserved", Category: CatProprietary},
}

var exceptionList = []Exception{
	{ID: "Classpath-exception-2.0", Linking: true},
	{ID: "GCC-exception-2.0", Linking: true},
	{ID: "GCC-exception-3.1", Linking: true},
	{ID: "LLVM-exception", Linking: true, GPL2: true},
	{ID: "Autoconf-exception-2.0", Linking: false}, // covers generated output / embedded fonts only
	{ID: "Autoconf-exception-3.0", Linking: false}, // covers generated output / embedded fonts only
	{ID: "Bison-exception-2.2", Linking: false},    // covers generated output / embedded fonts only
	{ID: "Libtool-exception", Linking: false},      // covers generated output / embedded fonts only
	{ID: "Linux-syscall-note", Linking: true},
	{ID: "OpenJDK-assembly-exception-1.0", Linking: true},
	{ID: "Font-exception-2.0", Linking: false}, // covers generated output / embedded fonts only
	{ID: "WxWindows-exception-3.1", Linking: true},
	{ID: "eCos-exception-2.0", Linking: true},
	{ID: "Qt-LGPL-exception-1.1", Linking: true},
	{ID: "Qt-GPL-exception-1.0", Linking: false}, // covers generated output / embedded fonts only
	{ID: "openvpn-openssl-exception", Linking: false},
	{ID: "Universal-FOSS-exception-1.0", Linking: false},
	{ID: "FLTK-exception", Linking: true},
	{ID: "u-boot-exception-2.0", Linking: true},
	{ID: "GPL-3.0-linking-exception", Linking: true},
	{ID: "GPL-3.0-linking-source-exception", Linking: true},
	{ID: "GPL-CC-1.0", Linking: false},
	{ID: "LGPL-3.0-linking-exception", Linking: true},
	{ID: "Swift-exception", Linking: true},
}

var (
	catalog    = map[string]Info{}
	catalogLC  = map[string]string{} // lower-case id -> canonical id
	exceptions = map[string]Exception{}
	excLC      = map[string]string{}
)

func init() {
	for _, i := range catalogList {
		catalog[i.ID] = i
		catalogLC[strings.ToLower(i.ID)] = i.ID
	}
	for _, e := range exceptionList {
		exceptions[e.ID] = e
		excLC[strings.ToLower(e.ID)] = e.ID
	}
}

// Lookup returns catalog info for an SPDX (or licgate LicenseRef) id.
func Lookup(id string) (Info, bool) {
	if i, ok := catalog[id]; ok {
		return i, true
	}
	if c, ok := catalogLC[strings.ToLower(id)]; ok {
		return catalog[c], true
	}
	// Families we can classify by prefix even if the exact variant is not
	// in the catalog.
	lc := strings.ToLower(id)
	switch {
	case strings.HasPrefix(lc, "cc-by-nc") || strings.HasPrefix(lc, "cc-by-nd"):
		return Info{ID: id, Name: id, Category: CatSourceAvailable, Attribution: true}, true
	case strings.HasPrefix(lc, "polyform-"):
		return Info{ID: id, Name: id, Category: CatSourceAvailable, Attribution: true}, true
		// Note: no catch-all for "BSD-3-Clause-*" / "MIT-*": some variants add
		// field-of-use restrictions (e.g. BSD-3-Clause-No-Military-License), so
		// unlisted variants stay unknown and go to REVIEW.
	}
	return Info{}, false
}

// LookupException returns info for a WITH exception id.
func LookupException(id string) (Exception, bool) {
	if e, ok := exceptions[id]; ok {
		return e, true
	}
	if c, ok := excLC[strings.ToLower(id)]; ok {
		return exceptions[c], true
	}
	return Exception{}, false
}

// CanonicalID returns the catalog spelling of id if known, else id.
func CanonicalID(id string) string {
	if c, ok := catalogLC[strings.ToLower(id)]; ok {
		return c
	}
	if c, ok := excLC[strings.ToLower(id)]; ok {
		return c
	}
	return id
}

// IsKnown reports whether id is a recognised license (catalog or family).
func IsKnown(id string) bool {
	_, ok := Lookup(id)
	return ok
}

// SPDXURL returns the spdx.org page for a real SPDX id, or "".
func SPDXURL(id string) string {
	if strings.HasPrefix(id, "LicenseRef-") {
		return ""
	}
	return "https://spdx.org/licenses/" + id + ".html"
}

// DisplayName returns a human name for id.
func DisplayName(id string) string {
	if i, ok := Lookup(id); ok && i.Name != "" {
		return i.Name
	}
	return id
}
