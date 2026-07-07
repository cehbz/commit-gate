package shellscan

import "testing"

type inv struct {
	name string
	args []string
}

func names(r Result) []inv {
	var out []inv
	for _, i := range r.Invocations {
		var as []string
		for _, a := range i.Args {
			as = append(as, a.Text)
		}
		out = append(out, inv{i.Name.Text, as})
	}
	return out
}

func TestScanStatements(t *testing.T) {
	cases := []struct {
		cmd  string
		want []inv // nil means only check count==0
	}{
		{"git status", []inv{{"git", nil}}},
		{"git add x && git commit -F /tmp/m", []inv{{"git", []string{"add", "x"}}, {"git", []string{"commit", "-F", "/tmp/m"}}}},
		{"a; b | c & d || e", []inv{{"a", nil}, {"b", nil}, {"c", nil}, {"d", nil}, {"e", nil}}},
		{"foo\ngate-disable", []inv{{"foo", nil}, {"gate-disable", nil}}}, // newline is a separator
		{"(subshell) ; { grouped; }", []inv{{"subshell", nil}, {"grouped", nil}}},
		{"if x; then y; fi", []inv{{"x", nil}, {"y", nil}}},
		{"f() { hidden; }", []inv{{"hidden", nil}}}, // function bodies are scanned
		{"FOO=bar", nil}, // assignment-only: no invocation
		{"# just a comment", nil},
		{"", nil},
	}
	for _, c := range cases {
		r := Scan(c.cmd)
		if r.ParseErr {
			t.Errorf("%q: unexpected ParseErr", c.cmd)
			continue
		}
		got := names(r)
		if len(got) != len(c.want) {
			t.Errorf("%q: got %v want %v", c.cmd, got, c.want)
			continue
		}
		for i := range got {
			if got[i].name != c.want[i].name {
				t.Errorf("%q: inv %d name %q want %q", c.cmd, i, got[i].name, c.want[i].name)
			}
			if len(c.want[i].args) > 0 {
				for j, a := range c.want[i].args {
					if j >= len(got[i].args) || got[i].args[j] != a {
						t.Errorf("%q: inv %d args %v want %v", c.cmd, i, got[i].args, c.want[i].args)
						break
					}
				}
			}
		}
	}
}

func TestScanWordsAndRedirects(t *testing.T) {
	r := Scan(`echo "hi there" 'single' plain$var > /tmp/out 2>>/tmp/err <<EOF
heredoc body with rm .git/config inside
EOF
cat < /tmp/in`)
	if r.ParseErr {
		t.Fatal("ParseErr")
	}
	e := r.Invocations[0]
	if !e.Args[0].Literal || e.Args[0].Text != "hi there" {
		t.Errorf("dquoted: %+v", e.Args[0])
	}
	if !e.Args[1].Literal || e.Args[1].Text != "single" {
		t.Errorf("squoted: %+v", e.Args[1])
	}
	if e.Args[2].Literal {
		t.Errorf("expansion must be non-literal: %+v", e.Args[2])
	}
	var writes, nonwrites int
	for _, rd := range e.Redirects {
		if rd.Write {
			writes++
		} else {
			nonwrites++
		}
	}
	if writes != 2 { // > and 2>> ; heredoc is not a write
		t.Errorf("writes = %d, redirects: %+v", writes, e.Redirects)
	}
	// heredoc CONTENT must not appear as any invocation
	for _, i := range r.Invocations {
		if i.Name.Text == "rm" {
			t.Error("heredoc content was scanned as commands")
		}
	}
	if last := r.Invocations[len(r.Invocations)-1]; last.Name.Text != "cat" || len(last.Redirects) != 1 || last.Redirects[0].Write {
		t.Errorf("cat redirect: %+v", last)
	}
}

func TestScanCompoundRedirect(t *testing.T) {
	r := Scan(`{ echo a; } >> /x/.git/hooks/pre-commit`)
	found := false
	for _, i := range r.Invocations {
		for _, rd := range i.Redirects {
			if rd.Write && rd.Target.Literal && rd.Target.Text == "/x/.git/hooks/pre-commit" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("compound redirect target not surfaced: %+v", r.Invocations)
	}
}

func TestScanParseErr(t *testing.T) {
	for _, bad := range []string{`echo "unclosed`, `if ; then`, `foo )`} {
		if !Scan(bad).ParseErr {
			t.Errorf("%q: want ParseErr", bad)
		}
	}
}

func TestBasename(t *testing.T) {
	if Basename(Word{Text: "/usr/local/bin/approve", Literal: true}) != "approve" {
		t.Fail()
	}
	if Basename(Word{Text: "$CMD", Literal: false}) != "" {
		t.Fail()
	}
}

func TestScanEscapedWordsAreNonLiteral(t *testing.T) {
	// Escaped / ANSI-C command names must NOT be reported as exact literals,
	// or a literal-match policy rule (R5 human-only) would be bypassed.
	for _, cmd := range []string{`$'\x67ate-disable'`, `g\ate-disable`, `\gate-disable`} {
		inv := Scan(cmd).Invocations[0]
		if inv.Name.Literal {
			t.Errorf("%q: Name.Literal=true, want false (escape-bearing)", cmd)
		}
		if Basename(inv.Name) != "" {
			t.Errorf("%q: Basename=%q, want \"\" for non-literal", cmd, Basename(inv.Name))
		}
	}
	// Escaped content inside double quotes is also non-literal.
	if Scan(`echo "a\$b"`).Invocations[0].Args[0].Literal {
		t.Error(`"a\$b" arg reported literal`)
	}
}

func TestScanConcatenationStaysLiteral(t *testing.T) {
	// Quote-concatenation obfuscation resolves to a real literal and MUST still
	// be caught (no backslash involved).
	inv := Scan(`"app"rove --yes`).Invocations[0]
	if !inv.Name.Literal || inv.Name.Text != "approve" {
		t.Fatalf(`"app"rove: got {%q,%v} want {"approve",true}`, inv.Name.Text, inv.Name.Literal)
	}
}

func TestScanSubstitutionInnerCallsSurface(t *testing.T) {
	// The single most security-relevant guarantee: an executed command inside a
	// substitution surfaces as its own Invocation (R5 must catch $(approve)).
	for _, cmd := range []string{`cmd1 $(gate-disable)`, "cmd1 `gate-disable`", `tee >(gate-disable)`, `echo $(echo $(gate-disable))`} {
		found := false
		for _, inv := range Scan(cmd).Invocations {
			if inv.Name.Text == "gate-disable" {
				found = true
			}
		}
		if !found {
			t.Errorf("%q: inner gate-disable did not surface as an invocation", cmd)
		}
	}
}

func TestScanPlainSingleQuoteStaysLiteral(t *testing.T) {
	// Plain single quotes (no $) have no escape semantics: still an exact literal.
	inv := Scan(`'gate-disable'`).Invocations[0]
	if !inv.Name.Literal || Basename(inv.Name) != "gate-disable" {
		t.Fatalf("got {%q,%v}", inv.Name.Text, inv.Name.Literal)
	}
}

func TestScanHasCommand(t *testing.T) {
	// real commands (even non-literal name) carry HasCommand=true
	for _, cmd := range []string{`echo hi`, `$(which approve)`, `$CMD`, `$(which approve) > /tmp/out`} {
		inv := Scan(cmd).Invocations[0]
		if !inv.HasCommand {
			t.Errorf("%q: HasCommand=false, want true", cmd)
		}
	}
	// the synthetic redirect-only carrier carries HasCommand=false
	r := Scan(`{ echo a; } > /tmp/f`)
	var carrier *Invocation
	for i := range r.Invocations {
		if !r.Invocations[i].HasCommand {
			carrier = &r.Invocations[i]
		}
	}
	if carrier == nil {
		t.Fatal("no synthetic carrier found for compound redirect")
	}
}

func TestScanBraceExpansionIsNonLiteral(t *testing.T) {
	// Brace expansion is unconditional in bash: gat{e,e}-disable executes
	// gate-disable, so a literal-match denylist must treat it as unresolvable.
	for _, cmd := range []string{`gat{e,e}-disable`, `{gate-disable,x}`} {
		inv := Scan(cmd).Invocations[0]
		if inv.Name.Literal {
			t.Errorf("%q: Name.Literal=true, want false (brace expansion)", cmd)
		}
	}
	// Malformed/comma-less braces are NOT expanded by bash -> stay literal.
	for _, cmd := range []string{`a{b`, `{gate-disable}`} {
		inv := Scan(cmd).Invocations[0]
		if !inv.Name.Literal {
			t.Errorf("%q: Name.Literal=false, want true (no expansion in bash)", cmd)
		}
	}
}
