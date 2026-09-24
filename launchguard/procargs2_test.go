package launchguard

import (
	"encoding/binary"
	"reflect"
	"testing"
)

// buildProcArgs2 hand-builds a kern.procargs2-shaped buffer: argc (native
// int32), execPath\0, padNuls extra NUL padding bytes, then each of argv
// NUL-terminated, then (optionally) a trailing envp string to prove it's
// ignored.
func buildProcArgs2(execPath string, padNuls int, argv []string, envp ...string) []byte {
	var buf []byte
	hdr := make([]byte, 4)
	binary.LittleEndian.PutUint32(hdr, uint32(len(argv)))
	buf = append(buf, hdr...)
	buf = append(buf, execPath...)
	buf = append(buf, 0)
	for i := 0; i < padNuls; i++ {
		buf = append(buf, 0)
	}
	for _, a := range argv {
		buf = append(buf, a...)
		buf = append(buf, 0)
	}
	for _, e := range envp {
		buf = append(buf, e...)
		buf = append(buf, 0)
	}
	return buf
}

func TestParseProcArgs2RealWrapperShape(t *testing.T) {
	argv := []string{"/bin/zsh", "-c", "eval 'approve' < /dev/null && pwd -P >| /tmp/x"}
	buf := buildProcArgs2("/bin/zsh", 0, argv, "PATH=/usr/bin", "HOME=/Users/t")
	got, err := parseProcArgs2(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, argv) {
		t.Fatalf("got %#v, want %#v", got, argv)
	}
}

func TestParseProcArgs2ExtraPadding(t *testing.T) {
	// Real procargs2 buffers commonly carry several NUL padding bytes
	// between the saved exec path and argv[0] (word alignment).
	argv := []string{"-zsh"}
	buf := buildProcArgs2("/bin/zsh", 7, argv)
	got, err := parseProcArgs2(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, argv) {
		t.Fatalf("got %#v, want %#v", got, argv)
	}
}

func TestParseProcArgs2ArgWithSpacesAndPunctuation(t *testing.T) {
	argv := []string{"/bin/zsh", "-c", "approve && ./install.sh && approve-push"}
	buf := buildProcArgs2("/bin/zsh", 3, argv)
	got, err := parseProcArgs2(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, argv) {
		t.Fatalf("got %#v, want %#v", got, argv)
	}
}

func TestParseProcArgs2ZeroArgc(t *testing.T) {
	buf := buildProcArgs2("/bin/zsh", 2, nil)
	got, err := parseProcArgs2(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %#v, want empty", got)
	}
}

func TestParseProcArgs2TooShort(t *testing.T) {
	if _, err := parseProcArgs2([]byte{1, 2}); err == nil {
		t.Fatal("want error for a buffer too short to hold argc")
	}
}

func TestParseProcArgs2TruncatedArgv(t *testing.T) {
	// argc says 2 but only one argv string (no terminating NUL for the
	// second) is present.
	buf := buildProcArgs2("/bin/zsh", 0, []string{"-c"})
	hdr := make([]byte, 4)
	binary.LittleEndian.PutUint32(hdr, 2)
	copy(buf[:4], hdr)
	if _, err := parseProcArgs2(buf); err == nil {
		t.Fatal("want error for truncated argv")
	}
}
