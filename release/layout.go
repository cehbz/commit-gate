package release

import "path/filepath"

// Layout is where a build goes live on a host and how the host is wired to
// it, fixed under the home directory Home (DESIGN.md, Install). The zero
// Layout's paths are relative to the home directory, as ssh and scp read
// them.
type Layout struct{ Home string }

// Dir is the install directory.
func (l Layout) Dir() string { return filepath.Join(l.Home, ".local", "libexec", "commit-gate") }

// Live is the live binary.
func (l Layout) Live() string { return filepath.Join(l.Dir(), "commit-gate") }

// Hooks is the hook directory the global core.hooksPath names.
func (l Layout) Hooks() string { return filepath.Join(l.Dir(), "hooks") }

// Staging is where a deploy delivers a build before running its install.
func (l Layout) Staging() string { return filepath.Join(l.Dir(), "staging") }

// OnPath is the commit-gate link on PATH.
func (l Layout) OnPath() string { return filepath.Join(l.Home, ".local", "bin", "commit-gate") }

// Settings is Claude Code's settings.json.
func (l Layout) Settings() string { return filepath.Join(l.Home, ".claude", "settings.json") }
