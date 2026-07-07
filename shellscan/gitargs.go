package shellscan

import "strings"

// KV is a single `-c key=value` (or `-c key`) per-invocation git config
// override, as parsed from a git Invocation's Args.
type KV struct {
	Key      Word
	HasValue bool
	Value    Word
}

var gitValueFlags = map[string]bool{
	"-C": true, "--git-dir": true, "--work-tree": true, "--namespace": true, "--exec-path": true,
}

func isGit(inv Invocation) bool { return Basename(inv.Name) == "git" }

// GitSubcommand returns the first non-flag argument of a git Invocation (its
// subcommand, e.g. "commit", "push"), skipping the values of global flags
// that take one (-C, --git-dir, --work-tree, --namespace, --exec-path) and
// skipping `-c k=v` (single-arg form). ok is false if inv is not a git
// invocation or no subcommand argument is found.
func GitSubcommand(inv Invocation) (string, bool) {
	if !isGit(inv) {
		return "", false
	}
	skip := false
	for _, a := range inv.Args {
		if skip {
			skip = false
			continue
		}
		if a.Literal && gitValueFlags[a.Text] {
			skip = true // -C/--git-dir/... consume their value
			continue
		}
		if a.Literal && a.Text == "-c" {
			skip = true // -c consumes its key=value
			continue
		}
		if !a.Literal {
			return "", false // unresolvable word in the subcommand region: can't prove what runs
		}
		if strings.HasPrefix(a.Text, "-") {
			continue // other global flag (guaranteed literal here)
		}
		return a.Text, true
	}
	return "", false
}

// GitConfigKVs returns every `-c` per-invocation config override on a git
// Invocation, key split at the first '=' (no '=' means HasValue is false).
func GitConfigKVs(inv Invocation) []KV {
	if !isGit(inv) {
		return nil
	}
	var out []KV
	skip := false
	for i := 0; i < len(inv.Args); i++ {
		a := inv.Args[i]
		if skip {
			skip = false
			continue
		}
		if a.Literal && a.Text == "-c" {
			if i+1 < len(inv.Args) {
				v := inv.Args[i+1]
				key, val, found := strings.Cut(v.Text, "=")
				out = append(out, KV{Key: Word{Text: key, Literal: v.Literal}, HasValue: found, Value: Word{Text: val, Literal: v.Literal}})
			}
			skip = true
			continue
		}
		if a.Literal && gitValueFlags[a.Text] {
			skip = true
			continue
		}
		if !a.Literal {
			break // unresolvable region; an obfuscated override here is out of scope (sabotage)
		}
		if strings.HasPrefix(a.Text, "-") {
			continue
		}
		break // reached the subcommand: no more global -c options
	}
	return out
}

// HasArg reports whether inv has any literal argument exactly equal to s.
func HasArg(inv Invocation, s string) bool {
	for _, a := range inv.Args {
		if a.Literal && a.Text == s {
			return true
		}
	}
	return false
}
