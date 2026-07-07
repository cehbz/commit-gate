package shellscan

import (
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

type Word struct {
	Text    string // best-effort text; exact when Literal
	Literal bool   // true iff composed only of literal/quoted-literal parts
}

type Redirect struct {
	Target Word
	Write  bool // >, >>, >|, &>, &>>, <> (output-capable); false for <, heredocs, fd-dups
}

type Invocation struct {
	Name      Word
	Args      []Word
	Redirects []Redirect
}

type Result struct {
	Invocations []Invocation
	ParseErr    bool
}

// Basename returns the path basename of a literal Word's text; "" if !Literal.
func Basename(w Word) string {
	if !w.Literal {
		return ""
	}
	return filepath.Base(w.Text)
}

func fromWord(w *syntax.Word) Word {
	if w == nil {
		return Word{}
	}
	var b strings.Builder
	lit := true
	escaped := false // backslash escapes or $'...' ANSI-C quoting: raw source != executed value
	var walkParts func(parts []syntax.WordPart)
	walkParts = func(parts []syntax.WordPart) {
		for _, p := range parts {
			switch x := p.(type) {
			case *syntax.Lit:
				if strings.ContainsRune(x.Value, '\\') {
					escaped = true // unquoted or in-double-quote backslash escape
				}
				b.WriteString(x.Value)
			case *syntax.SglQuoted:
				if x.Dollar {
					escaped = true // $'...' resolves C-style escapes; raw value is not the executed text
				}
				b.WriteString(x.Value)
			case *syntax.DblQuoted:
				walkParts(x.Parts)
			default: // ParamExp, CmdSubst, ArithmExp, ProcSubst, ExtGlob...
				lit = false
			}
		}
	}
	// Split brace expansions on a copy, not w itself: w is a live node inside
	// the AST that syntax.Walk (in Scan) is still descending into, and it does
	// not know how to walk the *syntax.BraceExp nodes SplitBraces introduces.
	// Mutating w in place corrupts the in-progress walk and panics.
	wc := *w
	syntax.SplitBraces(&wc) // valid brace expansions become BraceExp parts -> non-literal below
	walkParts(wc.Parts)
	// An escape-bearing word is treated as non-literal: its executed value cannot
	// be trusted to equal its source bytes, so policy must treat it as unresolvable
	// rather than as an exact literal match. Plain single-quoted strings (Dollar
	// false) and unescaped concatenations stay literal.
	return Word{Text: b.String(), Literal: lit && !escaped}
}

// writeOp reports whether a redirect operator targets a path (isRedirect) and,
// if so, whether it is output-capable (isWrite). Heredocs are path-target
// redirects (the delimiter word is surfaced) but are never writes; fd
// duplication (<&, >&) has no path target at all.
func writeOp(op syntax.RedirOperator) (isWrite, isRedirect bool) {
	switch op {
	case syntax.RdrOut, syntax.AppOut, syntax.RdrClob, syntax.RdrAll, syntax.AppAll, syntax.RdrInOut:
		return true, true
	case syntax.RdrIn, syntax.Hdoc, syntax.DashHdoc, syntax.WordHdoc:
		return false, true
	default: // DplIn, DplOut (fd duplication) — not path targets
		return false, false
	}
}
