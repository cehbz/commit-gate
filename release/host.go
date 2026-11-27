package release

import (
	"os"
	"runtime"
	"strings"
)

// Mac is the Mac's name as a deploy host. Every other host's name is its
// short host name (DESIGN.md, Deploy).
const Mac = "mac"

// ThisHost is this host's name as a deploy host.
func ThisHost() (string, error) {
	if runtime.GOOS == "darwin" {
		return Mac, nil
	}
	h, err := os.Hostname()
	if err != nil {
		return "", err
	}
	short, _, _ := strings.Cut(h, ".")
	return short, nil
}
