package main_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// precheckOn asks the precheck about one tool call and returns its stdout.
func precheckOn(t *testing.T, tool string, input map[string]string) string {
	t.Helper()
	in, err := json.Marshal(map[string]any{"tool_name": tool, "tool_input": input})
	if err != nil {
		t.Fatal(err)
	}
	out, errStr, code := run(t, string(in), "precheck")
	if code != 0 {
		t.Fatalf("precheck: code=%d stderr=%q", code, errStr)
	}
	return out
}

func denied(out string) bool { return strings.Contains(out, `"permissionDecision":"deny"`) }

func TestPrecheckDeniesEditsOfTheGlobalGitConfig(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", "") // git then reads ~/.config/git/config
	for _, p := range []string{filepath.Join(home, ".gitconfig"), filepath.Join(home, ".config", "git", "config")} {
		if out := precheckOn(t, "Edit", map[string]string{"file_path": p}); !denied(out) || !strings.Contains(out, "global git config") {
			t.Errorf("Edit %s: %q", p, out)
		}
	}
	if out := precheckOn(t, "Bash", map[string]string{"command": "echo '[core]' >> ~/.gitconfig"}); !denied(out) {
		t.Errorf("a redirect into ~/.gitconfig: %q", out)
	}
	if out := precheckOn(t, "Read", map[string]string{"file_path": filepath.Join(home, ".gitconfig")}); out != "" {
		t.Errorf("reading ~/.gitconfig stays allowed: %q", out)
	}
}

func TestPrecheckDeniesEditsOfTheXDGGitConfig(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if out := precheckOn(t, "Write", map[string]string{"file_path": filepath.Join(xdg, "git", "config")}); !denied(out) {
		t.Errorf("Write $XDG_CONFIG_HOME/git/config: %q", out)
	}
	if out := precheckOn(t, "Write", map[string]string{"file_path": filepath.Join(xdg, "git", "ignore")}); out != "" {
		t.Errorf("its neighbour stays writable: %q", out)
	}
}
