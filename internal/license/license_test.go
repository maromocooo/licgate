package license

import (
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"MIT":                         "MIT",
		"mit":                         "MIT",
		"MIT License":                 "MIT",
		"Apache 2.0":                  "Apache-2.0",
		"Apache License, Version 2.0": "Apache-2.0",
		"Apache Software License":     "Apache-2.0",
		"BSD":                         RefBSD,
		"BSD-3-Clause":                "BSD-3-Clause",
		"(MIT OR Apache-2.0)":         "MIT OR Apache-2.0",
		"MIT/Apache-2.0":              "MIT OR Apache-2.0",
		"MIT OR Apache-2.0":           "MIT OR Apache-2.0",
		"Apache-2.0 AND ISC":          "Apache-2.0 AND ISC",
		"GPL-2.0":                     "GPL-2.0-only",
		"GPL-2.0+":                    "GPL-2.0-or-later",
		"LGPL-2.1+":                   "LGPL-2.1-or-later",
		"GPLv3+":                      "GPL-3.0-or-later",
		"AGPL-3.0":                    "AGPL-3.0-only",
		"UNLICENSED":                  RefProprietary,
		"Public Domain":               RefPublicDomain,
		"GNU GPLv2 (with FOSS License Exception)": "GPL-2.0-only",
		"Apache-2.0 WITH LLVM-exception":          "Apache-2.0 WITH LLVM-exception",
		"(MIT OR Apache-2.0) AND Unicode-3.0":     "(MIT OR Apache-2.0) AND Unicode-3.0",
		"GPL-2.0-with-classpath-exception":        "GPL-2.0-only WITH Classpath-exception-2.0",
	}
	for in, want := range cases {
		got := Normalize(in)
		if got.Expr != want || !got.OK() {
			t.Errorf("Normalize(%q) = %q (unknown %v), want %q", in, got.Expr, got.Unknown, want)
		}
	}
	for _, empty := range []string{"", "UNKNOWN", "NOASSERTION", "none"} {
		if n := Normalize(empty); n.Expr != "" {
			t.Errorf("Normalize(%q) = %q, want empty", empty, n.Expr)
		}
	}
	if n := Normalize("SEE LICENSE IN LICENSE.txt"); n.SeeFile != "LICENSE.txt" {
		t.Errorf("SEE LICENSE IN: got %+v", n)
	}
	if n := Normalize("My Custom License v1"); n.OK() || len(n.Unknown) == 0 {
		t.Errorf("custom license should be unknown, got %+v", n)
	}
}

func TestParsePrecedence(t *testing.T) {
	e, err := Parse("MIT OR Apache-2.0 AND BSD-3-Clause")
	if err != nil {
		t.Fatal(err)
	}
	if got := e.String(); got != "MIT OR (Apache-2.0 AND BSD-3-Clause)" {
		t.Errorf("got %q", got)
	}
	for _, bad := range []string{"MIT OR", "(MIT", "AND MIT", "MIT WITH"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) should fail", bad)
		}
	}
}

const mitText = `MIT License

Copyright (c) 2020 Jane Doe

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.`

const bsd3Text = `Copyright (c) 2013 Someone. All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:
   * Neither the name of Someone nor the names of its
contributors may be used to endorse or promote products derived from
this software without specific prior written permission.`

var gpl2Head = `                    GNU GENERAL PUBLIC LICENSE
                       Version 2, June 1991

 Copyright (C) 1989, 1991 Free Software Foundation, Inc.
 Everyone is permitted to copy and distribute verbatim copies
 of this license document, but changing it is not allowed.

                            Preamble

  The licenses for most software are designed to take away your
freedom to share and change it.  By contrast, the GNU General Public
License is intended to guarantee your freedom to share and change free
software--to make sure the software is free for all its users.  This
General Public License applies to most of the Free Software
Foundation's software and to any other program whose authors commit to
using it.  (Some other Free Software Foundation software is covered by
the GNU Lesser General Public License instead.)  You can apply it to
your programs, too.` + "\n" + filler

// filler pushes the text over the "short notice" length so "any later
// version" inside the license body is not mistaken for a grant.
var filler = func() string {
	s := ""
	for i := 0; i < 80; i++ {
		s += "This is filler text from the body of the license used for testing.\n"
	}
	return s + "either version 2 of the License, or (at your option) any later version."
}()

const mpl2Text = `Mozilla Public License Version 2.0
==================================

1. Definitions
--------------
1.12. "Secondary License"
    means either the GNU General Public License, Version 2.0, the GNU Lesser
    General Public License, Version 2.1, the GNU Affero General Public
    License, Version 3.0, or any later versions of those licenses.`

const gplNotice = `This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.`

const proprietaryWithNotices = `Mapbox Web SDK

Copyright © 2021 Mapbox, Inc. All rights reserved.

The software and files in this repository (collectively, "Software") are
licensed under the Mapbox TOS for use only with the relevant Mapbox product(s).

-------------------------------------------------------------------------------
Contains code from some-lib

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:
Neither the name of the copyright holder nor the names of its contributors may be used to endorse`

func TestDetectText(t *testing.T) {
	cases := []struct {
		name, text, want string
		minConf          float64
	}{
		{"mit", mitText, "MIT", 0.9},
		{"bsd3 with all rights reserved", bsd3Text, "BSD-3-Clause", 0.9},
		{"gpl2 mentions lgpl in preamble", gpl2Head, "GPL-2.0-only", 0.8},
		{"mpl2 lists gpls as secondary", mpl2Text, "MPL-2.0", 0.9},
		{"gpl3 notice or later", gplNotice, "GPL-3.0-or-later", 0.8},
	}
	for _, c := range cases {
		d := DetectText(c.text)
		if d.ID != c.want || d.Confidence < c.minConf {
			t.Errorf("%s: got %s (%.2f), want %s", c.name, d.ID, d.Confidence, c.want)
		}
	}
	d := DetectText(proprietaryWithNotices)
	if d.Confidence >= 0.8 {
		t.Errorf("proprietary preamble must not be confidently detected, got %s (%.2f)", d.ID, d.Confidence)
	}
	if d := DetectText(gpl2Head); !d.VersionAmbiguous {
		t.Errorf("full GPL text should be version-ambiguous")
	}
}

func TestFromClassifiers(t *testing.T) {
	n := FromClassifiers([]string{
		"License :: OSI Approved :: Apache Software License",
		"License :: OSI Approved :: BSD License",
		"Programming Language :: Python",
	})
	if n.Expr != "Apache-2.0 OR "+RefBSD {
		t.Errorf("got %q", n.Expr)
	}
}

func TestExtractCopyrights(t *testing.T) {
	got := ExtractCopyrights(mitText)
	if len(got) != 1 || got[0] != "Copyright (c) 2020 Jane Doe" {
		t.Errorf("got %q", got)
	}
}

func TestRestrictionsSurvive(t *testing.T) {
	for in, want := range map[string]string{
		"Apache 2.0 / Commons Clause": "Apache-2.0 AND " + RefCommons,
		"MIT (see LICENSE)":           "MIT",
		"mpl":                         RefMPL,
	} {
		if got := Normalize(in); got.Expr != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got.Expr, want)
		}
	}
	for _, in := range []string{"MIT (non-commercial use only)", "Apache-2.0 (with Commons Clause)"} {
		if got := Normalize(in); got.OK() {
			t.Errorf("Normalize(%q) = %q must not be fully recognised", in, got.Expr)
		}
	}
	cases := map[string]string{
		mitText + "\nThe Software shall be used for Good, not Evil.":                      "MIT AND JSON",
		mitText + "\n\n-----\n\n\"Commons Clause\" License Condition v1.0\nThe Software…": "MIT AND " + RefCommons,
	}
	for text, want := range cases {
		if d := DetectText(text); d.ID != want {
			t.Errorf("got %q want %q", d.ID, want)
		}
	}
	eula := "Copyright 2024 Acme Corp. All rights reserved. This proprietary software includes components under the Apache License, Version 2.0, see below. " + strings.Repeat("Terms. ", 30)
	if d := DetectText(eula); d.ID == "Apache-2.0" && d.Confidence >= 0.8 {
		t.Errorf("EULA mentioning Apache detected as Apache")
	}
	if d := DetectText("Some project notes. This is not the MIT License, sorry, and no rights are granted to you at all here."); d.Confidence >= 0.8 {
		t.Errorf("bare 'mit license' mention accepted: %+v", d)
	}
	x11 := strings.Replace(mitText, "this permission notice shall be included", "this permission notice (including the next paragraph) shall be included", 1)
	if d := DetectText(x11); d.ID != "MIT" {
		t.Errorf("X11-style MIT: %+v", d)
	}
}

func TestProprietaryPreambleMentioningApache(t *testing.T) {
	eula := "Copyright (c) 2023 Acme Corp. All rights reserved.\n\nThis software is proprietary and confidential. You may not copy, modify or redistribute it without a written agreement with Acme. The software includes open source components licensed under the Apache License, Version 2.0; see THIRD_PARTY_NOTICES."
	if d := DetectText(eula); d.ID != RefProprietary {
		t.Errorf("got %+v", d)
	}
}
