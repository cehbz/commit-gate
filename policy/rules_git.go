package policy

import (
	"strings"

	"github.com/cehbz/commit-gate/shellscan"
)

// R1: git commit/push carrying --no-verify (or commit's -n alias) on the same
// invocation. For push, -n means --dry-run (harmless) and is NOT no-verify —
// only the explicit --no-verify flag skips the pre-push hook.
func ruleNoVerify(inv shellscan.Invocation, _ ToolCall, _ Ctx) (Decision, bool) {
	sub, ok := shellscan.GitSubcommand(inv)
	if !ok {
		return Decision{}, false
	}
	switch sub {
	case "commit":
		if shellscan.HasArg(inv, "-n") || shellscan.HasArg(inv, "--no-verify") {
			return deny(MsgNoVerify), true
		}
	case "push":
		if shellscan.HasArg(inv, "--no-verify") {
			return deny(MsgNoVerify), true
		}
	}
	return Decision{}, false
}

// R2/R3 (environment channel): git config injection via GIT_CONFIG_*
// environment assignments — the same effective power as `-c` to disable the
// gate or redirect core.hooksPath, but on the assignment prefix that is not an
// Arg. Denied on any git invocation carrying it.
func ruleEnvConfigInjection(inv shellscan.Invocation, _ ToolCall, _ Ctx) (Decision, bool) {
	if shellscan.GitConfigEnvInjection(inv) {
		return deny(MsgEnvConfig), true
	}
	return Decision{}, false
}

type configOp struct {
	key      string // lowercased
	hasValue bool
	value    shellscan.Word
	unset    bool
	read     bool
}

// parse `git config ...` args; ok=false when inv is not a git config invocation.
func parseGitConfig(inv shellscan.Invocation) (configOp, bool) {
	sub, rest, ok := shellscan.GitSubcommandArgs(inv)
	if !ok || sub != "config" {
		return configOp{}, false
	}
	var op configOp
	var nonFlags []shellscan.Word
	skip := false
	for _, a := range rest {
		if skip {
			skip = false // consumed the value of --file/-f/--blob
			continue
		}
		if a.Literal && strings.HasPrefix(a.Text, "-") {
			switch a.Text {
			case "--unset", "--unset-all":
				op.unset = true
			case "--get", "--get-all", "--get-regexp", "-l", "--list":
				op.read = true
			case "--file", "-f", "--blob":
				skip = true // these consume the next token (a path/blob), not the key
			}
			continue
		}
		nonFlags = append(nonFlags, a)
	}
	if len(nonFlags) > 0 {
		op.key = strings.ToLower(nonFlags[0].Text)
	}
	if len(nonFlags) > 1 {
		op.hasValue = true
		op.value = nonFlags[1]
	}
	return op, true
}

// R2: writes of commit-gate.disabled (config subcommand or -c override).
func ruleDisabledKey(inv shellscan.Invocation, _ ToolCall, _ Ctx) (Decision, bool) {
	for _, kv := range shellscan.GitConfigKVs(inv) {
		if strings.EqualFold(kv.Key.Text, "commit-gate.disabled") {
			return deny(MsgDisabled), true
		}
	}
	op, ok := parseGitConfig(inv)
	if !ok || op.read || op.key != "commit-gate.disabled" {
		return Decision{}, false
	}
	if op.unset || op.hasValue {
		return deny(MsgDisabled), true
	}
	return Decision{}, false // bare read of the key
}

// R3: writes of core.hooksPath — only a literal value resolving exactly to
// the live hooks dir is a permitted enable.
func ruleHooksPath(inv shellscan.Invocation, tc ToolCall, ctx Ctx) (Decision, bool) {
	enableOK := func(v shellscan.Word) bool {
		return v.Literal && ctx.Resolve(tc.CWD, v.Text) == ctx.HooksDir
	}
	for _, kv := range shellscan.GitConfigKVs(inv) {
		if strings.EqualFold(kv.Key.Text, "core.hookspath") {
			if kv.HasValue && enableOK(kv.Value) {
				continue
			}
			return deny(MsgHooksPath), true
		}
	}
	op, ok := parseGitConfig(inv)
	if !ok || op.read || op.key != "core.hookspath" {
		return Decision{}, false
	}
	if op.unset || !op.hasValue || !enableOK(op.value) {
		if op.unset || op.hasValue {
			return deny(MsgHooksPath), true
		}
		return Decision{}, false // bare read of the key
	}
	return Decision{}, false // enable
}
