//go:build linux

package launchguard

import (
	"fmt"
	"os"
	"strings"
)

// defaultGetParentArgv reads the parent process's exact argv from
// /proc/<ppid>/cmdline: NUL-separated arguments, NUL-terminated.
func defaultGetParentArgv() ([]string, error) {
	ppid := os.Getppid()
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", ppid))
	if err != nil {
		return nil, fmt.Errorf("read /proc/%d/cmdline: %w", ppid, err)
	}
	trimmed := strings.TrimRight(string(data), "\x00")
	if trimmed == "" {
		return nil, nil
	}
	return strings.Split(trimmed, "\x00"), nil
}
