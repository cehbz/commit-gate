// Package shellscan parses a shell command string with mvdan.cc/sh/v3 and
// flattens every invocation (CallExpr) it contains — across statement
// separators, pipelines, subshells, brace groups, control-flow bodies, and
// function definitions — into a structured, best-effort representation for
// downstream policy checks.
package shellscan

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Scan parses command and returns every invocation it contains. On a shell
// syntax error, Result.ParseErr is true and Invocations is empty.
func Scan(command string) Result {
	var res Result
	parser := syntax.NewParser(syntax.Variant(syntax.LangBash), syntax.KeepComments(false))
	file, err := parser.Parse(strings.NewReader(command), "")
	if err != nil {
		return Result{ParseErr: true}
	}
	syntax.Walk(file, func(node syntax.Node) bool {
		stmt, ok := node.(*syntax.Stmt)
		if !ok {
			return true
		}
		var redirs []Redirect
		for _, r := range stmt.Redirs {
			w, isRedir := writeOp(r.Op)
			if !isRedir || r.Word == nil {
				continue
			}
			redirs = append(redirs, Redirect{Target: fromWord(r.Word), Write: w})
		}
		if call, ok := stmt.Cmd.(*syntax.CallExpr); ok && len(call.Args) > 0 {
			inv := Invocation{Name: fromWord(call.Args[0]), Redirects: redirs}
			for _, a := range call.Args[1:] {
				inv.Args = append(inv.Args, fromWord(a))
			}
			res.Invocations = append(res.Invocations, inv)
			return true
		}
		// Compound command (or assignment-only statement) with redirects:
		// surface the write targets on a nameless invocation so policy still
		// sees them, e.g. `{ ...; } > f`.
		if len(redirs) > 0 {
			res.Invocations = append(res.Invocations, Invocation{Redirects: redirs})
		}
		return true
	})
	return res
}
