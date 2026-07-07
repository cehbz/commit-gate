// Package harness owns the Claude Code hook JSON shapes.
package harness

import (
	"bytes"
	"encoding/json"
	"io"
)

type ToolInput struct {
	Command      string `json:"command"`
	FilePath     string `json:"file_path"`
	NotebookPath string `json:"notebook_path"`
}

type PreToolUseInput struct {
	ToolName  string    `json:"tool_name"`
	ToolInput ToolInput `json:"tool_input"`
	CWD       string    `json:"cwd"`
}

// Path returns the file path for file-tool calls (file_path, else notebook_path).
func (in PreToolUseInput) Path() string {
	if in.ToolInput.FilePath != "" {
		return in.ToolInput.FilePath
	}
	return in.ToolInput.NotebookPath
}

func ParseInput(r io.Reader) (PreToolUseInput, error) {
	var in PreToolUseInput
	err := json.NewDecoder(r).Decode(&in)
	return in, err
}

type denyOut struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

type allowOut struct {
	HookEventName      string `json:"hookEventName"`
	PermissionDecision string `json:"permissionDecision"`
	AdditionalContext  string `json:"additionalContext"`
}

type sessionOut struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

type envelope struct {
	HookSpecificOutput any `json:"hookSpecificOutput"`
}

// marshal emits compact one-line JSON with a trailing newline and NO HTML
// escaping, matching jq's output byte-for-byte (the bash implementation
// emitted these shapes via jq, which leaves & < > literal).
func marshal(e envelope) []byte {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(e); err != nil {
		return []byte(`{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"commit-gate: internal encoding error"}}` + "\n")
	}
	return b.Bytes()
}

func Deny(reason string) []byte {
	return marshal(envelope{denyOut{HookEventName: "PreToolUse", PermissionDecision: "deny", PermissionDecisionReason: reason}})
}

func AllowContext(ctx string) []byte {
	return marshal(envelope{allowOut{HookEventName: "PreToolUse", PermissionDecision: "allow", AdditionalContext: ctx}})
}

func SessionContext(ctx string) []byte {
	return marshal(envelope{sessionOut{HookEventName: "SessionStart", AdditionalContext: ctx}})
}
