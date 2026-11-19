package testenv

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	ClearCallerRepo()
	os.Exit(m.Run())
}
