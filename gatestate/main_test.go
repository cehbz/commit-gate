package gatestate

import (
	"os"
	"testing"
)

// TestMain points git at an empty global config, so these tests never read
// the user's ~/.gitconfig (whose core.hooksPath would run the live gate on
// their commits).
func TestMain(m *testing.M) {
	f, err := os.CreateTemp("", "gitconfig")
	if err != nil {
		panic(err)
	}
	f.Close()
	os.Setenv("GIT_CONFIG_GLOBAL", f.Name())
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	code := m.Run()
	os.Remove(f.Name())
	os.Exit(code)
}
