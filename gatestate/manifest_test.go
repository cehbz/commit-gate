package gatestate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	mf := filepath.Join(dir, "approved")
	msgs := []string{"feat: a\n\nbody", "fix: b"}
	if err := AppendApprovals(mf, msgs); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(mf)
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 manifest lines, got %d: %q", len(lines), raw)
	}
	if !strings.HasSuffix(lines[0], "  feat: a") || !strings.HasSuffix(lines[1], "  fix: b") {
		t.Fatalf("subject columns wrong: %q", lines)
	}
	subj, err := ApprovedSubjects(mf)
	if err != nil || len(subj) != 2 || subj[0] != "feat: a" {
		t.Fatalf("subjects: %v %v", subj, err)
	}
	ok, err := ConsumeApproval(mf, CanonicalHash(msgs[0]))
	if !ok || err != nil {
		t.Fatalf("consume: %v %v", ok, err)
	}
	ok, _ = ConsumeApproval(mf, CanonicalHash(msgs[0])) // single-use
	if ok {
		t.Fatal("second consume must miss")
	}
	subj, _ = ApprovedSubjects(mf)
	if len(subj) != 1 || subj[0] != "fix: b" {
		t.Fatalf("remaining: %v", subj)
	}
}

func TestConsumeRemovesDuplicatesAndMissingFile(t *testing.T) {
	dir := t.TempDir()
	mf := filepath.Join(dir, "approved")
	if ok, err := ConsumeApproval(mf, strings.Repeat("a", 64)); ok || err != nil {
		t.Fatalf("missing manifest: %v %v", ok, err)
	}
	AppendApprovals(mf, []string{"dup", "dup", "keep"})
	ok, _ := ConsumeApproval(mf, CanonicalHash("dup"))
	if !ok {
		t.Fatal("consume dup")
	}
	subj, _ := ApprovedSubjects(mf)
	if len(subj) != 1 || subj[0] != "keep" { // bash grep -v removes all matches
		t.Fatalf("want only keep, got %v", subj)
	}
}
