package shellscan

import (
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

type Word struct {
	Text    string // best-effort text; exact when Literal
	Literal bool   // true iff composed only of literal/quoted-literal parts
	HomeRel bool   // literal but for a leading $HOME or ${HOME} before a "/"; Text spells it "~"
}

type Redirect struct {
	Target Word
	Write  bool // >, >>, >|, &>, &>>, <> (output-capable); false for <, heredocs, fd-dups
}

// Assign is a `NAME=value` prefix assignment scoped to a single invocation,
// e.g. the GIT_CONFIG_COUNT=1 in `GIT_CONFIG_COUNT=1 git commit`.
type Assign struct {
	Name  string
	Value Word
}

type Invocation struct {
	Name      Word
	Args      []Word
	Redirects []Redirect
	Assigns   []Assign

	HasCommand bool // true for a real CallExpr-derived invocation (has a command word, even if non-literal); false for the synthetic redirect-only carrier
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
	escaped := false  // backslash escapes or $'...' ANSI-C quoting: raw source != executed value
	first := true     // the next part is the word's first; quotes are not parts
	homeLead := false // the first part is a plain $HOME or ${HOME}
	var walkParts func(parts []syntax.WordPart)
	walkParts = func(parts []syntax.WordPart) {
		for _, p := range parts {
			if dq, ok := p.(*syntax.DblQuoted); ok {
				walkParts(dq.Parts)
				continue
			}
			isFirst := first
			first = false
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
			case *syntax.ParamExp:
				if isFirst && plainHome(x) {
					homeLead = true
					continue
				}
				lit = false
			default: // CmdSubst, ArithmExp, ProcSubst, ExtGlob...
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
	// A leading plain $HOME before a literal "/..." is the home dir as ~ is:
	// such a word is HomeRel with Text "~/...", so path checks can resolve it.
	// An escape-bearing word is treated as non-literal: its executed value cannot
	// be trusted to equal its source bytes, so policy must treat it as unresolvable
	// rather than as an exact literal match. Plain single-quoted strings (Dollar
	// false) and unescaped concatenations stay literal.
	text := b.String()
	if homeLead && lit && !escaped && strings.HasPrefix(text, "/") {
		return Word{Text: "~" + text, HomeRel: true}
	}
	return Word{Text: text, Literal: lit && !escaped && !homeLead}
}

// plainHome reports whether x is $HOME or ${HOME}, with no operator.
func plainHome(x *syntax.ParamExp) bool {
	return x.Param != nil && x.Param.Value == "HOME" && x.Flags == nil &&
		!x.Excl && !x.Length && !x.Width && !x.IsSet &&
		x.NestedParam == nil && x.Index == nil && len(x.Modifiers) == 0 &&
		x.Slice == nil && x.Repl == nil && x.Names == 0 && x.Exp == nil
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
