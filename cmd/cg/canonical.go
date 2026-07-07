// canonical.go implements the `canonical` subcommand, a thin CLI wrapper
// around gatestate.CanonicalHash used by the acceptance tests (and
// installed as build/lib/canonical) to mirror bash lib/canonical: read a
// commit message on stdin, write its canonical sha256 (64 hex chars)
// followed by a newline on stdout.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/cehbz/commit-gate/gatestate"
)

// cmdCanonical implements the `canonical` subcommand.
func cmdCanonical() int {
	msg, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "commit-gate: %s\n", err)
		return 1
	}
	fmt.Println(gatestate.CanonicalHash(string(msg)))
	return 0
}
