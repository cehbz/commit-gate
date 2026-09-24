//go:build darwin

package launchguard

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// defaultGetParentArgv reads the parent process's exact argv via the
// kern.procargs2 sysctl (KERN_PROCARGS2) — the same mechanism `ps -o args=`
// uses, and the only way on macOS to see a process's argv without cgo.
func defaultGetParentArgv() ([]string, error) {
	ppid := os.Getppid()
	buf, err := unix.SysctlRaw("kern.procargs2", ppid)
	if err != nil {
		return nil, fmt.Errorf("sysctl kern.procargs2 for pid %d: %w", ppid, err)
	}
	return parseProcArgs2(buf)
}
