package launchguard

import "testing"

func TestEvalLiteralPayloadSimple(t *testing.T) {
	got, ok := evalLiteralPayload(`eval 'approve' < /dev/null`)
	if !ok || got != "approve" {
		t.Fatalf("got (%q, %v), want (\"approve\", true)", got, ok)
	}
}

func TestEvalLiteralPayloadResolvesEmbeddedSingleQuoteConcatenation(t *testing.T) {
	// what a shell actually produces when single-quoting text that itself
	// contains a single quote: 'it'\''s' means "it" + \' + "s" == "it's".
	got, ok := evalLiteralPayload(`eval 'it'\''s'`)
	if !ok || got != "it's" {
		t.Fatalf("got (%q, %v), want (\"it's\", true)", got, ok)
	}
}

func TestEvalLiteralPayloadFindsEvalAmongOtherStatements(t *testing.T) {
	script := `source /tmp/snap.sh 2>/dev/null || true && setopt NO_EXTENDED_GLOB 2>/dev/null || true && eval 'approve --yes' < /dev/null && pwd -P >| /tmp/x`
	got, ok := evalLiteralPayload(script)
	if !ok || got != "approve --yes" {
		t.Fatalf("got (%q, %v), want (\"approve --yes\", true)", got, ok)
	}
}

func TestEvalLiteralPayloadNoEvalPresent(t *testing.T) {
	if _, ok := evalLiteralPayload(`cd ~/x && approve`); ok {
		t.Fatal("want ok=false: no eval in this script")
	}
}

func TestEvalLiteralPayloadNonLiteralArgument(t *testing.T) {
	if _, ok := evalLiteralPayload(`eval "$CMD"`); ok {
		t.Fatal("want ok=false: eval's argument is not a literal word")
	}
}

func TestEvalLiteralPayloadCommandSubstitutionArgument(t *testing.T) {
	if _, ok := evalLiteralPayload("eval \"$(echo approve)\""); ok {
		t.Fatal("want ok=false: eval's argument contains a command substitution")
	}
}

func TestEvalLiteralPayloadMultipleArgumentsDoesNotQualify(t *testing.T) {
	// eval with more than one argument is not "an eval whose single
	// argument is a literal word": fall back to the whole script.
	if _, ok := evalLiteralPayload(`eval approve --yes`); ok {
		t.Fatal("want ok=false: eval has more than one argument")
	}
}

func TestEvalLiteralPayloadDoubleQuotedLiteralArgument(t *testing.T) {
	got, ok := evalLiteralPayload(`eval "approve --yes"`)
	if !ok || got != "approve --yes" {
		t.Fatalf("got (%q, %v), want (\"approve --yes\", true)", got, ok)
	}
}
