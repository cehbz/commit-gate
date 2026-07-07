// approve.go implements the `approve` command: human approval of one or
// more pending commit messages. Behavioral contract = reference file
// bin/approve (sourcing lib/common's require_enabled/confirm/die).
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/cehbz/commit-gate/confirm"
	"github.com/cehbz/commit-gate/gatestate"
)

// approveSep is the literal line that separates messages in a -F batch file.
const approveSep = "@@COMMIT-GATE-SEP@@"

const approveUsage = "usage: approve [-F file | --plan plan.md] [--yes]   (bare: reads the preloaded pending file)"

// dieApprove prints "commit-gate: <formatted message>" to stderr (matching
// bash's die()) and returns the exit code callers should propagate.
func dieApprove(format string, a ...any) int {
	fmt.Fprintf(os.Stderr, "commit-gate: "+format+"\n", a...)
	return 1
}

// cmdApprove implements the `approve` command.
func cmdApprove(args []string) int {
	mode := "stdin"
	src := ""
	assumeYes := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-F":
			if i+1 >= len(args) {
				return dieApprove("%s", approveUsage)
			}
			mode, src = "file", args[i+1]
			i++
		case "--plan":
			if i+1 >= len(args) {
				return dieApprove("%s", approveUsage)
			}
			mode, src = "plan", args[i+1]
			i++
		case "--yes":
			assumeYes = true
		default:
			return dieApprove("%s", approveUsage)
		}
	}

	cwd, _ := os.Getwd()
	repo, err := gatestate.Open(cwd)
	if err != nil {
		return dieApprove("not inside a git repository")
	}
	if repo.Disabled() {
		return dieApprove("repo is opted out (commit-gate.disabled); run: gate-enable")
	}
	if !repo.Enabled(liveHooksDir()) {
		return dieApprove("repo not enabled — run: gate-enable")
	}
	if err := os.MkdirAll(repo.GateDir(), 0o755); err != nil {
		return dieApprove("%s", err)
	}

	// Bare `approve` (no -F/--plan) uses the agent-preloaded pending file if
	// present, else stdin.
	consumePending := false
	if mode == "stdin" {
		if _, err := os.Stat(repo.PendingPath()); err == nil {
			mode, src, consumePending = "file", repo.PendingPath(), true
		}
	}

	var msgs []string
	switch mode {
	case "stdin":
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return dieApprove("%s", err)
		}
		if m := strings.TrimRight(string(b), "\n"); m != "" {
			msgs = append(msgs, m)
		}
	case "file":
		b, err := os.ReadFile(src)
		if err != nil {
			return dieApprove("cannot read: %s", src)
		}
		msgs = append(msgs, splitOnApproveSep(string(b))...)
	case "plan":
		b, err := os.ReadFile(src)
		if err != nil {
			return dieApprove("cannot read: %s", src)
		}
		msgs = append(msgs, extractPlanMessages(string(b))...)
	}

	if len(msgs) == 0 {
		return dieApprove("no messages to approve")
	}

	fmt.Printf("commit-gate: about to approve %d message(s):\n", len(msgs))
	for i, m := range msgs {
		fmt.Printf("--- [%d] ---\n", i+1)
		fmt.Println(m)
	}
	fmt.Println("-----------")

	if !assumeYes {
		prompt := fmt.Sprintf("commit-gate — approve %d message(s)?", len(msgs))
		for _, m := range msgs {
			prompt += "\n" + gatestate.Subject(m)
		}
		if err := confirm.Confirm(prompt); err != nil {
			return dieApprove("%s", err)
		}
	}

	if err := gatestate.AppendApprovals(repo.ManifestPath(), msgs); err != nil {
		return dieApprove("%s", err)
	}
	fmt.Printf("commit-gate: recorded %d approval(s)\n", len(msgs))

	// New approvals mean new content is coming: a push authorized before
	// them is stale.
	if _, err := os.Stat(repo.PushTokenPath()); err == nil {
		os.Remove(repo.PushTokenPath())
		fmt.Println("commit-gate: push token invalidated (approvals postdate it — re-run approve-push)")
	}

	if consumePending {
		os.Remove(repo.PendingPath())
	}
	return 0
}

// splitOnApproveSep splits content on lines exactly equal to approveSep,
// joining the lines of each message with "\n" (no trailing newline), and
// drops empty messages. This is a direct transcription of bin/approve's
// read loop, including its quirk: if a message's first accumulated line is
// itself empty, the "cur is empty" check can't distinguish that from "no
// message started yet" -- matched here deliberately for parity.
func splitOnApproveSep(content string) []string {
	var msgs []string
	cur := ""
	flush := func() {
		if cur != "" {
			msgs = append(msgs, cur)
		}
		cur = ""
	}
	sc := bufio.NewScanner(strings.NewReader(content))
	sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == approveSep:
			flush()
		case cur == "":
			cur = line
		default:
			cur = cur + "\n" + line
		}
	}
	flush()
	return msgs
}

// planMsgRe mirrors the two regexes on bin/approve line 41 (double- and
// single-quoted -m payloads), applied per-line to match grep -oE's
// line-oriented semantics.
var planMsgRe = regexp.MustCompile(`git commit[^"']*-m[ \t]*"([^"]*)"|git commit[^"']*-m[ \t]*'([^']*)'`)

// extractPlanMessages extracts every `git commit ... -m "..."` / `-m '...'`
// payload from content, skipping empty payloads.
func extractPlanMessages(content string) []string {
	var msgs []string
	for _, line := range strings.Split(content, "\n") {
		for _, idx := range planMsgRe.FindAllStringSubmatchIndex(line, -1) {
			var payload string
			switch {
			case idx[2] != -1:
				payload = line[idx[2]:idx[3]]
			case idx[4] != -1:
				payload = line[idx[4]:idx[5]]
			}
			if payload != "" {
				msgs = append(msgs, payload)
			}
		}
	}
	return msgs
}
