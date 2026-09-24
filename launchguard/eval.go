package launchguard

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// evalLiteralPayload looks for `eval '<literal word>'` anywhere in script
// (including nested inside &&/||/;/pipelines/blocks) and, if found with
// exactly one literal argument, returns that argument's fully resolved
// value (quoting and concatenation resolved) and true. This is how
// Claude Code's `!` wrapper hands off the user's actual command: `eval
// '<user text>' < /dev/null && ...`. If no such eval is found, ok is
// false and the caller falls back to treating the whole script as the
// checked text.
func evalLiteralPayload(script string) (payload string, ok bool) {
	parser := syntax.NewParser(syntax.Variant(syntax.LangBash), syntax.KeepComments(false))
	file, err := parser.Parse(strings.NewReader(script), "")
	if err != nil {
		return "", false
	}
	syntax.Walk(file, func(n syntax.Node) bool {
		if ok {
			return false
		}
		call, isCall := n.(*syntax.CallExpr)
		if !isCall || len(call.Args) != 2 {
			return true
		}
		name, nameOK := literalWordValue(call.Args[0])
		if !nameOK || name != "eval" {
			return true
		}
		val, valOK := literalWordValue(call.Args[1])
		if !valOK {
			return true // this eval's argument isn't a literal word; keep looking
		}
		payload, ok = val, true
		return false
	})
	return payload, ok
}

// literalWordValue resolves w to its fully-quoting-and-concatenation
// resolved value, or ok=false if any part of it depends on something
// commit-gate cannot statically evaluate (a parameter/command/arithmetic
// expansion, process substitution, or extended glob).
func literalWordValue(w *syntax.Word) (string, bool) {
	if w == nil {
		return "", false
	}
	return literalParts(w.Parts)
}

func literalParts(parts []syntax.WordPart) (string, bool) {
	var b strings.Builder
	for _, part := range parts {
		switch x := part.(type) {
		case *syntax.Lit:
			s, ok := unescapeLit(x.Value)
			if !ok {
				return "", false
			}
			b.WriteString(s)
		case *syntax.SglQuoted:
			if x.Dollar {
				// $'...' ANSI-C quoting resolves C-style escapes; treat as
				// unresolvable rather than risk mis-decoding it.
				return "", false
			}
			b.WriteString(x.Value)
		case *syntax.DblQuoted:
			s, ok := literalParts(x.Parts)
			if !ok {
				return "", false
			}
			b.WriteString(s)
		default: // ParamExp, CmdSubst, ArithmExp, ProcSubst, ExtGlob, ...
			return "", false
		}
	}
	return b.String(), true
}

// unescapeLit resolves an unquoted/backslash-escaped Lit token's raw source
// text (mvdan.cc/sh keeps the backslash in Value for these) to its executed
// value: a backslash escapes the following character literally, except a
// backslash-newline pair, which is a line continuation and produces nothing.
// A trailing, unpaired backslash is unresolvable.
func unescapeLit(s string) (string, bool) {
	if !strings.ContainsRune(s, '\\') {
		return s, true
	}
	var b strings.Builder
	r := []rune(s)
	for i := 0; i < len(r); i++ {
		if r[i] != '\\' {
			b.WriteRune(r[i])
			continue
		}
		if i+1 >= len(r) {
			return "", false
		}
		i++
		if r[i] == '\n' {
			continue // line continuation: escapes the newline away
		}
		b.WriteRune(r[i])
	}
	return b.String(), true
}
