//go:build !darwin && !linux

package launchguard

import (
	"fmt"
	"runtime"
)

// defaultGetParentArgv has no implementation outside darwin/linux; Check
// fails open on the resulting error (see package doc).
func defaultGetParentArgv() ([]string, error) {
	return nil, fmt.Errorf("launchguard: reading the parent process's argv is not implemented on %s", runtime.GOOS)
}
