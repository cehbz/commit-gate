package gatestate

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// CanonicalHash mirrors lib/canonical: CRLF->LF, strip trailing
// horizontal whitespace per line, drop trailing blank lines, every kept
// line newline-terminated.
func CanonicalHash(msg string) string {
	s := strings.ReplaceAll(msg, "\r", "")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t\v\f")
	}
	last := len(lines)
	for last > 0 && lines[last-1] == "" {
		last--
	}
	var b strings.Builder
	for _, l := range lines[:last] {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// Subject is the message's first line.
func Subject(msg string) string {
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		return msg[:i]
	}
	return msg
}
