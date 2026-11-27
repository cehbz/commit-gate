// Command release makes a release of the gate (DESIGN.md, Release). Run by
// make release VERSION=<tag> in the Mac's clone, it pushes the head to
// origin's main and a release tag on it in one atomic push, under one push
// authorization, and tags the clone once that push lands. It refuses,
// changing nothing, when the clone has uncommitted changes, when origin has
// commits the head lacks, when the head is the target's commit, when the
// tree holds pending model-level tests, or when the version is not a
// release tag above every release tag.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/cehbz/commit-gate/release"
)

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) != 1 {
		return refuse("usage: make release VERSION=vMAJOR.MINOR.PATCH")
	}
	tag, ok := release.ParseTag(args[0])
	if !ok {
		return refuse("VERSION=%s is not a release tag (vMAJOR.MINOR.PATCH)", args[0])
	}
	changes, err := git("status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return refuse("%s", err)
	}
	if changes != "" {
		return refuse("the clone has uncommitted changes:\n%s", changes)
	}
	head, err := git("rev-parse", "HEAD")
	if err != nil {
		return refuse("%s", err)
	}
	if err := originBehindHead(); err != nil {
		return refuse("%s", err)
	}
	t, err := release.ReadTarget(release.PublicAddress)
	switch {
	case errors.Is(err, release.ErrNoRelease):
	case err != nil:
		return refuse("%s", err)
	case t.Commit == head:
		return refuse("the head is already the target, %s; there is nothing to release", t.Tag.Name)
	case !t.Tag.Less(tag):
		return refuse("%s is not above the highest release tag, %s", tag.Name, t.Tag.Name)
	}
	marks, err := pendingMarks()
	if err != nil {
		return refuse("%s", err)
	}
	if marks != "" {
		return refuse("the tree holds pending model-level tests:\n%s", marks)
	}
	push := exec.Command("git", "push", "--atomic", "origin", "HEAD:refs/heads/main", "HEAD:refs/tags/"+tag.Name)
	push.Stdout, push.Stderr = os.Stderr, os.Stderr
	if err := push.Run(); err != nil {
		return refuse("the push of main and %s did not land (%v); the clone has no tag %s", tag.Name, err, tag.Name)
	}
	if _, err := git("tag", tag.Name, head); err != nil {
		return refuse("%s landed, but tagging the clone failed: %s", tag.Name, err)
	}
	fmt.Printf("release: %s is %s\n", tag.Name, head)
	return 0
}

func refuse(format string, a ...any) int {
	fmt.Fprintf(os.Stderr, "release: "+format+"\n", a...)
	return 1
}

// git runs git in the clone and returns its trimmed output.
func git(args ...string) (string, error) {
	var out, errb bytes.Buffer
	cmd := exec.Command("git", args...)
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// originBehindHead fails unless origin's main is the head or an ancestor of
// it: a commit there that the clone lacks, or one the head doesn't contain,
// would be lost or force-pushed over.
func originBehindHead() error {
	ref, err := git("ls-remote", "origin", "refs/heads/main")
	if err != nil {
		return err
	}
	remote, _, _ := strings.Cut(ref, "\t")
	if remote == "" {
		return nil // origin has no main yet
	}
	if _, err := git("merge-base", "--is-ancestor", remote, "HEAD"); err != nil {
		return fmt.Errorf("origin's main has commits the head lacks; pull them first")
	}
	return nil
}

// pendingMarks lists the head's pending marks: a pending(t) call opening a
// line of a Go test, and a case run through pending in an acceptance
// script. A mark inside a string, mid-line, is not one.
func pendingMarks() (string, error) {
	var found []string
	for _, g := range []struct{ pattern, pathspec string }{
		{`^[[:space:]]*pending\(t\)`, "*_test.go"},
		{`^[[:space:]]*pending `, "tests/test_*.sh"},
	} {
		out, err := git("grep", "-n", "-E", g.pattern, "HEAD", "--", g.pathspec)
		var exit *exec.ExitError
		switch {
		case err == nil:
			found = append(found, out)
		case errors.As(err, &exit) && exit.ExitCode() == 1:
			// git grep: no match
		default:
			return "", err
		}
	}
	return strings.Join(found, "\n"), nil
}
