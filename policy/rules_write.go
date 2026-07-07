package policy

import (
	"strings"

	"github.com/cehbz/commit-gate/shellscan"
)

// protectedClass reports whether a RESOLVED absolute path is in the protected
// set, and returns the matching class ("" = not protected). R4 always maps
// every class to MsgWriteTarget; R6 (Task 9) maps the same classes to
// per-target edit messages.
func protectedClass(resolved string, ctx Ctx) string {
	if ctx.InstallDir != "" && (resolved == ctx.InstallDir || strings.HasPrefix(resolved, ctx.InstallDir+"/")) {
		return "install"
	}
	parts := strings.Split(resolved, "/")
	for i, p := range parts {
		if p != ".git" || i+1 >= len(parts) {
			continue
		}
		switch parts[i+1] {
		case "config":
			if i+2 == len(parts) {
				return "gitconfig"
			}
		case "hooks":
			return "githooks"
		case "commit-gate":
			return "gatestate"
		}
	}
	return ""
}

var writeVerbs = map[string]bool{"rm": true, "mv": true, "cp": true, "tee": true, "truncate": true, "sed": true}

// R4: write-verb args and Write redirect targets resolving into the
// protected set (install dir, .git/config, native hooks, gate state).
func ruleWriteTargets(inv shellscan.Invocation, tc ToolCall, ctx Ctx) (Decision, bool) {
	check := func(w shellscan.Word) bool {
		if !w.Literal || w.Text == "" {
			return false
		}
		return protectedClass(ctx.Resolve(tc.CWD, w.Text), ctx) != ""
	}
	for _, r := range inv.Redirects {
		if r.Write && check(r.Target) {
			return deny(MsgWriteTarget), true
		}
	}
	bn := shellscan.Basename(inv.Name)
	if !writeVerbs[bn] {
		return Decision{}, false
	}
	if bn == "sed" && !sedInPlace(inv) {
		return Decision{}, false
	}
	skipNext := false
	sawScript := false
	explicitScript := sedHasExplicitScript(inv)
	for _, a := range inv.Args {
		if skipNext {
			skipNext = false
			continue
		}
		if a.Literal && strings.HasPrefix(a.Text, "-") {
			if bn == "truncate" && a.Text == "-s" {
				skipNext = true
			}
			continue
		}
		if bn == "sed" && !explicitScript && !sawScript {
			sawScript = true // first non-flag arg is the sed script
			continue
		}
		if check(a) {
			return deny(MsgWriteTarget), true
		}
	}
	return Decision{}, false
}

func sedInPlace(inv shellscan.Invocation) bool {
	for _, a := range inv.Args {
		if !a.Literal {
			continue
		}
		flag, _, _ := strings.Cut(a.Text, "=") // --in-place=bak -> --in-place
		if flag == "-i" || flag == "--in-place" || strings.HasPrefix(a.Text, "-i") {
			return true
		}
	}
	return false
}

func sedHasExplicitScript(inv shellscan.Invocation) bool {
	for _, a := range inv.Args {
		if !a.Literal {
			continue
		}
		flag, _, _ := strings.Cut(a.Text, "=") // --file=x -> --file, --expression=y -> --expression
		switch flag {
		case "-e", "--expression", "-f", "--file":
			return true
		}
		if strings.HasPrefix(a.Text, "-e") || strings.HasPrefix(a.Text, "-f") {
			return true // attached short forms -escript / -ffile
		}
	}
	return false
}
