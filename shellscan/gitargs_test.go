package shellscan

import "testing"

func TestGitSubcommand(t *testing.T) {
	cases := []struct {
		cmd, want string
		ok        bool
	}{
		{"git commit -F /tmp/m", "commit", true},
		{"git -C /x -c a.b=c push origin main", "push", true},
		{"git --git-dir /g status", "status", true},
		{"git log --grep approve", "log", true},
		{"notgit commit", "", false},
		{"git", "", false},
	}
	for _, c := range cases {
		r := Scan(c.cmd)
		sub, ok := GitSubcommand(r.Invocations[0])
		if sub != c.want || ok != c.ok {
			t.Errorf("%q: got %q,%v want %q,%v", c.cmd, sub, ok, c.want, c.ok)
		}
	}
}

func TestGitConfigKVs(t *testing.T) {
	r := Scan(`git -c core.hooksPath=/tmp/evil -c commit-gate.disabled=true commit -F /tmp/m`)
	kvs := GitConfigKVs(r.Invocations[0])
	if len(kvs) != 2 || kvs[0].Key.Text != "core.hooksPath" || kvs[0].Value.Text != "/tmp/evil" || kvs[1].Key.Text != "commit-gate.disabled" {
		t.Fatalf("kvs: %+v", kvs)
	}
}

func TestGitSubcommandNonLiteralIsUnresolvable(t *testing.T) {
	for _, cmd := range []string{`git $SUB`, `git -$(echo c foo=bar) push`} {
		sub, ok := GitSubcommand(Scan(cmd).Invocations[0])
		if ok {
			t.Errorf("%q: got (%q,true), want ok=false (unresolvable subcommand region)", cmd, sub)
		}
	}
	// value of -C may be non-literal without making the whole thing unresolvable
	sub, ok := GitSubcommand(Scan(`git -C $DIR status`).Invocations[0])
	if !ok || sub != "status" {
		t.Errorf(`git -C $DIR status: got (%q,%v) want ("status",true)`, sub, ok)
	}
}

func TestGitConfigKVsStopsAtSubcommand(t *testing.T) {
	// subcommand-local -c (git commit -c <commit> reuse-message) is NOT a global config KV
	if kvs := GitConfigKVs(Scan(`git commit -c HEAD^ -m msg`).Invocations[0]); len(kvs) != 0 {
		t.Errorf("git commit -c HEAD^: got %+v, want no global KVs", kvs)
	}
	// global -c before the subcommand is still collected
	kvs := GitConfigKVs(Scan(`git -c core.hooksPath=/x commit -c HEAD^`).Invocations[0])
	if len(kvs) != 1 || kvs[0].Key.Text != "core.hooksPath" {
		t.Fatalf("got %+v, want one global KV core.hooksPath", kvs)
	}
}

func TestGitSubcommandArgs(t *testing.T) {
	// -C's value token "config" must not be mistaken for the subcommand.
	sub, rest, ok := GitSubcommandArgs(Scan(`git -C config config core.hooksPath /evil`).Invocations[0])
	if !ok || sub != "config" || len(rest) != 2 || rest[0].Text != "core.hooksPath" || rest[1].Text != "/evil" {
		t.Fatalf("got sub=%q rest=%+v ok=%v", sub, rest, ok)
	}
	// non-literal subcommand position -> unresolvable
	if _, _, ok := GitSubcommandArgs(Scan(`git $SUB`).Invocations[0]); ok {
		t.Error("non-literal subcommand must be unresolvable")
	}
}

func TestHasArg(t *testing.T) {
	inv := Scan(`git commit --no-verify -m x`).Invocations[0]
	if !HasArg(inv, "--no-verify") {
		t.Error("HasArg should find --no-verify")
	}
	if HasArg(inv, "-n") {
		t.Error("HasArg must not find -n when absent")
	}
	// a non-literal arg is never an exact match
	if HasArg(Scan(`git commit $FLAG`).Invocations[0], "$FLAG") {
		t.Error("HasArg must not match non-literal args")
	}
}
