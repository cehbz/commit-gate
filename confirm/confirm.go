// Package confirm implements the human-confirmation channel used by
// human-only commands (approve, approve-push). Behavioral contract =
// lib/common's confirm() shell function: macOS osascript dialog (works even
// over the tty-less `!` shell used by some agent harnesses) -> /dev/tty
// y/N prompt -> abort with "no confirmation channel".
package confirm

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// GOOS gates the osascript path to macOS. It is a package var (defaulting to
// runtime.GOOS) so tests can force either branch.
var GOOS = runtime.GOOS

// RunOsascript runs the exact AppleScript confirmation dialog from
// lib/common (buttons Cancel/Confirm, default button Cancel, title
// "commit-gate"), passing prompt as argv[0] of the `on run argv` handler.
// It is a package var so tests can stub it out.
var RunOsascript = func(prompt string) error {
	const script = `on run argv
  display dialog (item 1 of argv) buttons {"Cancel", "Confirm"} default button "Cancel" with title "commit-gate"
end run
`
	cmd := exec.Command("osascript", "-", prompt)
	cmd.Stdin = strings.NewReader(script)
	return cmd.Run()
}

// ReadTTY writes prompt to /dev/tty and reads one line of reply. A non-nil
// error means the channel itself is unavailable (no controlling terminal);
// a failed read after a successful prompt returns an empty reply with a
// nil error, matching bash's read -r reply </dev/tty 2>/dev/null || reply=(empty).
// It is a package var so tests can stub it out.
var ReadTTY = func(prompt string) (string, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", err
	}
	defer tty.Close()
	if _, err := tty.WriteString(prompt); err != nil {
		return "", err
	}
	reply, _ := bufio.NewReader(tty).ReadString('\n')
	return strings.TrimRight(reply, "\r\n"), nil
}

func hasOsascript() bool {
	_, err := exec.LookPath("osascript")
	return err == nil
}

// Confirm asks a human to confirm prompt via the best available channel and
// returns nil if confirmed, or an error describing why not:
//   - GOOS=="darwin" and osascript is on PATH: run the dialog; any error
//     (Cancel or osascript failure) -> "aborted; not confirmed".
//   - otherwise: prompt "<prompt> [y/N] " via /dev/tty; reply y/Y/yes/YES ->
//     confirmed, anything else -> "aborted; not confirmed".
//   - no channel available at all -> "no confirmation channel (no GUI, no
//     tty) — run from a terminal".
func Confirm(prompt string) error {
	if GOOS == "darwin" && hasOsascript() {
		if err := RunOsascript(prompt); err != nil {
			return errors.New("aborted; not confirmed")
		}
		return nil
	}
	reply, err := ReadTTY(prompt + " [y/N] ")
	if err != nil {
		return errors.New("no confirmation channel (no GUI, no tty) — run from a terminal")
	}
	switch reply {
	case "y", "Y", "yes", "YES":
		return nil
	default:
		return errors.New("aborted; not confirmed")
	}
}
