package license

import (
	"fmt"
	"strings"
)

// Expr is a parsed SPDX license expression.
type Expr interface {
	String() string
	isExpr()
}

// Leaf is a single license, optionally with an exception.
type Leaf struct {
	ID        string
	Exception string // WITH <exception>, "" if none
}

// Op is an n-ary AND / OR node.
type Op struct {
	And  bool // true = AND, false = OR
	Kids []Expr
}

func (Leaf) isExpr() {}
func (*Op) isExpr()  {}

func (l Leaf) String() string {
	if l.Exception != "" {
		return l.ID + " WITH " + l.Exception
	}
	return l.ID
}

func (o *Op) String() string {
	word := " OR "
	if o.And {
		word = " AND "
	}
	parts := make([]string, len(o.Kids))
	for i, k := range o.Kids {
		s := k.String()
		// Parenthesise a nested op of the other kind.
		if ko, ok := k.(*Op); ok && ko.And != o.And {
			s = "(" + s + ")"
		}
		parts[i] = s
	}
	return strings.Join(parts, word)
}

// Leaves returns all leaves of e.
func Leaves(e Expr) []Leaf {
	switch v := e.(type) {
	case Leaf:
		return []Leaf{v}
	case *Op:
		var out []Leaf
		for _, k := range v.Kids {
			out = append(out, Leaves(k)...)
		}
		return out
	}
	return nil
}

type tokenizer struct {
	toks []string
	pos  int
}

func tokenize(s string) []string {
	var toks []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			toks = append(toks, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case r == '(' || r == ')':
			flush()
			toks = append(toks, string(r))
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return toks
}

func (t *tokenizer) peek() string {
	if t.pos < len(t.toks) {
		return t.toks[t.pos]
	}
	return ""
}

func (t *tokenizer) next() string {
	s := t.peek()
	t.pos++
	return s
}

func isOp(tok, op string) bool { return strings.EqualFold(tok, op) }

// Parse parses an SPDX expression. Operators are accepted in any case.
// Leaves are canonicalised (deprecated ids like GPL-2.0 and "+" suffixes
// become -only / -or-later forms) but unknown ids are kept verbatim.
func Parse(s string) (Expr, error) {
	t := &tokenizer{toks: tokenize(s)}
	if len(t.toks) == 0 {
		return nil, fmt.Errorf("empty expression")
	}
	e, err := t.parseOr()
	if err != nil {
		return nil, err
	}
	if t.pos != len(t.toks) {
		return nil, fmt.Errorf("unexpected token %q", t.peek())
	}
	return e, nil
}

func (t *tokenizer) parseOr() (Expr, error) {
	first, err := t.parseAnd()
	if err != nil {
		return nil, err
	}
	kids := []Expr{first}
	for isOp(t.peek(), "OR") {
		t.next()
		k, err := t.parseAnd()
		if err != nil {
			return nil, err
		}
		kids = append(kids, k)
	}
	if len(kids) == 1 {
		return first, nil
	}
	return flatten(&Op{And: false, Kids: kids}), nil
}

func (t *tokenizer) parseAnd() (Expr, error) {
	first, err := t.parseWith()
	if err != nil {
		return nil, err
	}
	kids := []Expr{first}
	for isOp(t.peek(), "AND") {
		t.next()
		k, err := t.parseWith()
		if err != nil {
			return nil, err
		}
		kids = append(kids, k)
	}
	if len(kids) == 1 {
		return first, nil
	}
	return flatten(&Op{And: true, Kids: kids}), nil
}

func (t *tokenizer) parseWith() (Expr, error) {
	tok := t.next()
	switch {
	case tok == "":
		return nil, fmt.Errorf("unexpected end of expression")
	case tok == "(":
		e, err := t.parseOr()
		if err != nil {
			return nil, err
		}
		if t.next() != ")" {
			return nil, fmt.Errorf("missing )")
		}
		return e, nil
	case tok == ")" || isOp(tok, "AND") || isOp(tok, "OR") || isOp(tok, "WITH"):
		return nil, fmt.Errorf("unexpected %q", tok)
	}
	leaf := Leaf{ID: canonicalLeafID(tok)}
	// Deprecated ids such as "GPL-2.0-with-classpath-exception" expand to
	// "<id> WITH <exception>".
	if id, exc, ok := strings.Cut(leaf.ID, " WITH "); ok {
		leaf = Leaf{ID: id, Exception: exc}
	}
	if isOp(t.peek(), "WITH") {
		t.next()
		exc := t.next()
		if exc == "" || exc == "(" || exc == ")" {
			return nil, fmt.Errorf("missing exception after WITH")
		}
		leaf.Exception = CanonicalID(exc)
	}
	return leaf, nil
}

func flatten(o *Op) Expr {
	var kids []Expr
	for _, k := range o.Kids {
		if ko, ok := k.(*Op); ok && ko.And == o.And {
			kids = append(kids, ko.Kids...)
		} else {
			kids = append(kids, k)
		}
	}
	o.Kids = kids
	return o
}

// deprecated SPDX ids and the "+" operator.
var deprecatedIDs = map[string]string{
	"gpl-1.0":                          "GPL-1.0-only",
	"gpl-1.0+":                         "GPL-1.0-or-later",
	"gpl-2.0":                          "GPL-2.0-only",
	"gpl-2.0+":                         "GPL-2.0-or-later",
	"gpl-3.0":                          "GPL-3.0-only",
	"gpl-3.0+":                         "GPL-3.0-or-later",
	"lgpl-2.0":                         "LGPL-2.0-only",
	"lgpl-2.0+":                        "LGPL-2.0-or-later",
	"lgpl-2.1":                         "LGPL-2.1-only",
	"lgpl-2.1+":                        "LGPL-2.1-or-later",
	"lgpl-3.0":                         "LGPL-3.0-only",
	"lgpl-3.0+":                        "LGPL-3.0-or-later",
	"agpl-1.0":                         "AGPL-1.0-only",
	"agpl-3.0":                         "AGPL-3.0-only",
	"agpl-3.0+":                        "AGPL-3.0-or-later",
	"gpl-2.0-with-classpath-exception": "GPL-2.0-only WITH Classpath-exception-2.0",
	"gpl-2.0-with-gcc-exception":       "GPL-2.0-only WITH GCC-exception-2.0",
	"gpl-3.0-with-gcc-exception":       "GPL-3.0-only WITH GCC-exception-3.1",
	"gpl-2.0-with-autoconf-exception":  "GPL-2.0-only WITH Autoconf-exception-2.0",
	"gpl-3.0-with-autoconf-exception":  "GPL-3.0-only WITH Autoconf-exception-3.0",
}

func canonicalLeafID(tok string) string {
	lc := strings.ToLower(tok)
	if v, ok := deprecatedIDs[lc]; ok {
		return v
	}
	if strings.HasSuffix(tok, "+") {
		base := strings.TrimSuffix(tok, "+")
		// "GPL-2.0-only+" and similar oddities: drop the -only.
		blc := strings.TrimSuffix(strings.ToLower(base), "-only")
		if v, ok := deprecatedIDs[blc+"+"]; ok {
			return v
		}
		// MPL-2.0+, Apache-2.0+ …: "or later" of a permissive or weak
		// license changes nothing for us; treat as the base version.
		return CanonicalID(base)
	}
	return CanonicalID(tok)
}
