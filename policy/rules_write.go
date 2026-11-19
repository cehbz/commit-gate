package policy

import (
	"strings"

	"github.com/cehbz/commit-gate/shellscan"
)

// protectedClass reports whether a RESOLVED absolute path is in the protected
// set (install dir, .git/config, native hooks, gate state, the user's global
// git config files), and returns the matching class ("" = not protected). R4
// always maps every class to MsgWriteTarget; R6 maps the same classes to
// per-target edit messages.
func protectedClass(resolved string, ctx Ctx) string {
	if ctx.InstallDir != "" && (resolved == ctx.InstallDir || strings.HasPrefix(resolved, ctx.InstallDir+"/")) {
		return "install"
	}
	for _, g := range ctx.GlobalGitConfigs {
		if resolved == g {
			return "globalgitconfig"
		}
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

// R4: write-verb args and Write redirect targets, literal or under $HOME,
// resolving into the protected set (install dir, .git/config, native hooks,
// gate state, the global git config). cp is checked at its destination
// only, since copying from a protected file reads it; mv at every argument,
// since moving removes the source.
func ruleWriteTargets(inv shellscan.Invocation, tc ToolCall, ctx Ctx) (Decision, bool) {
	check := func(w shellscan.Word) bool {
		if !(w.Literal || w.HomeRel) || w.Text == "" {
			return false // HomeRel's Text is "~/...", which Resolve expands
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
	if bn == "cp" || bn == "mv" {
		paths := cpDestinations(inv.Args)
		if bn == "mv" {
			paths = mvPaths(inv.Args)
		}
		for _, w := range paths {
			if check(w) {
				return deny(MsgWriteTarget), true
			}
		}
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

// copyShortValue and copyLongValue are cp's and mv's options that take a
// value, short (GNU -S SUFFIX, -t DIR) and long with a required one that may
// follow as the next word; a long option whose value is optional
// (--backup[=..]) takes it only attached.
var (
	copyShortValue = map[byte]bool{'S': true, 't': true}
	copyLongValue  = map[string]bool{"--suffix": true, "--target-directory": true, "--sparse": true, "--no-preserve": true}
)

// copyArgs is cp's or mv's arguments as GNU reads them (options anywhere,
// -- ends them, value options consuming their value): its operands, the
// directories -t/--target-directory name, and every option value, targets
// included.
type copyArgs struct {
	operands, targets, values []shellscan.Word
}

func parseCopyArgs(args []shellscan.Word) copyArgs {
	var c copyArgs
	value := func(v shellscan.Word, target bool) {
		c.values = append(c.values, v)
		if target {
			c.targets = append(c.targets, v)
		}
	}
	endOpts := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		// A word starting with "-" is a flag even when not literal, as in
		// --target-directory=$HOME/x.
		if endOpts || !strings.HasPrefix(a.Text, "-") || a.Text == "-" {
			c.operands = append(c.operands, a)
			continue
		}
		if a.Text == "--" {
			endOpts = true
			continue
		}
		if strings.HasPrefix(a.Text, "--") {
			name, _, attached := strings.Cut(a.Text, "=")
			if !copyLongValue[name] {
				continue
			}
			v := a.After(len(name) + 1)
			if !attached {
				if i+1 >= len(args) {
					continue
				}
				i++
				v = args[i]
			}
			value(v, name == "--target-directory")
			continue
		}
		for j := 1; j < len(a.Text); j++ { // a cluster of short flags
			f := a.Text[j]
			if !copyShortValue[f] {
				continue
			}
			// The value is the rest of the word; a literal word ending at
			// the flag takes the next word. A non-literal one has its value
			// attached, as an expansion (-t$DIR), which Text leaves out.
			v := a.After(j + 1)
			if j+1 == len(a.Text) && a.Literal {
				if i+1 >= len(args) {
					break
				}
				i++
				v = args[i]
			}
			value(v, f == 't')
			break
		}
	}
	return c
}

// cpDestinations returns the words cp may write to. As GNU cp reads its
// arguments: the -t/--target-directory directory, else the last operand.
// As BSD cp reads them (options end at the first operand, -S takes no
// value, no -t): the last word. Its other operands are sources, which it
// only reads.
func cpDestinations(args []shellscan.Word) []shellscan.Word {
	c := parseCopyArgs(args)
	if len(c.targets) > 0 {
		return c.targets // BSD cp has no -t
	}
	var dests []shellscan.Word
	if len(c.operands) > 0 {
		dests = append(dests, c.operands[len(c.operands)-1])
	}
	if len(args) > 0 {
		dests = append(dests, args[len(args)-1])
	}
	return dests
}

// mvPaths returns every word mv may write or remove: its operands (moving
// removes the source) and its option values, the target directory in
// every spelling included.
func mvPaths(args []shellscan.Word) []shellscan.Word {
	c := parseCopyArgs(args)
	return append(c.operands, c.values...)
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
