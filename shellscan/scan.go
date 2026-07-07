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

// Scan parses command and returns every invocation it contains. A nested
// (-c payload) parse failure sets Result.ParseErr true while still
// returning invocations already collected from sibling statements.
func Scan(command string) Result {
	return scan(command, 0)
}

// scan is the depth-carrying implementation behind Scan. Invocations of
// bash/sh/zsh with a literal `-c` argument recurse into the next literal
// argument as shell source, up to a depth of 4; a non-literal `-c` payload
// (e.g. a variable or command substitution) is out of scope and is neither
// recursed into nor treated as a parse error.
func scan(command string, depth int) Result {
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
			inv := Invocation{Name: fromWord(call.Args[0]), Redirects: redirs, HasCommand: true}
			for _, a := range call.Args[1:] {
				inv.Args = append(inv.Args, fromWord(a))
			}
			res.Invocations = append(res.Invocations, inv)
			if bn := Basename(inv.Name); (bn == "bash" || bn == "sh" || bn == "zsh") && depth < 4 {
				for k := 0; k < len(inv.Args)-1; k++ {
					if inv.Args[k].Literal && inv.Args[k].Text == "-c" && inv.Args[k+1].Literal {
						inner := scan(inv.Args[k+1].Text, depth+1)
						res.Invocations = append(res.Invocations, inner.Invocations...)
						if inner.ParseErr {
							res.ParseErr = true
						}
						break
					}
				}
			}
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
