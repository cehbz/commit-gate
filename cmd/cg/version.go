// version.go implements `commit-gate version` (DESIGN.md, Version check)
// and printCheck, its report, which install shares.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/cehbz/commit-gate/release"
)

// cmdVersion implements `commit-gate version`: the running build measured
// against the target, and the fix when it is out of date. It exits 0 only
// when the build is up to date.
func cmdVersion(_ []string) int {
	c := release.CheckBuild(release.ThisBuild(), release.PublicAddress)
	if err := printCheck(os.Stdout, c); err != nil {
		return dieApprove("%s", err)
	}
	if c.State != release.UpToDate {
		return 1
	}
	return 0
}

// printCheck writes the version check's report: the build, the target, the
// state and, for an out-of-date build, the fix, which names this host.
func printCheck(w io.Writer, c release.Check) error {
	fmt.Fprintf(w, "build:  %s\n", c.Build)
	switch {
	case errors.Is(c.Err, release.ErrNoRelease):
		fmt.Fprintf(w, "target: no release tag on %s\n", release.PublicAddress)
	case c.Err != nil:
		fmt.Fprintf(w, "target: can't be read: %v\n", c.Err)
	default:
		fmt.Fprintf(w, "target: %s %s\n", c.Target.Tag.Name, c.Target.Commit)
	}
	fmt.Fprintf(w, "state:  %s\n", c.State)
	if c.State != release.OutOfDate {
		return nil
	}
	host, err := release.ThisHost()
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "fix:    in the Mac's clone of commit-gate, run commit-gate approve-deploy, then make deploy HOST=%s\n", host)
	return nil
}
