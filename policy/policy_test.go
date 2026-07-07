package policy

import (
	"strings"
	"testing"
)

func fakeCtx() Ctx {
	return Ctx{
		InstallDir: "/inst",
		HooksDir:   "/inst/hooks",
		Resolve: func(cwd, p string) string {
			// pure fake: /inst/hooks resolves to itself; relative joins cwd
			if strings.HasPrefix(p, "/") {
				return p
			}
			return cwd + "/" + p
		},
	}
}

func decideBash(t *testing.T, cmd string) Decision {
	t.Helper()
	return Decide(ToolCall{ToolName: "Bash", Command: cmd, CWD: "/w"}, fakeCtx())
}

func wantDeny(t *testing.T, cmd, msg string) {
	t.Helper()
	d := decideBash(t, cmd)
	if d.Kind != Deny || d.Reason != msg {
		t.Errorf("%q: got %+v, want deny %q", cmd, d, msg)
	}
}

func wantSilent(t *testing.T, cmd string) {
	t.Helper()
	if d := decideBash(t, cmd); d.Kind != Silent {
		t.Errorf("%q: got %+v, want silent", cmd, d)
	}
}

func TestParseFailureDenies(t *testing.T) {
	wantDeny(t, `echo "unclosed`, MsgParse)
}
