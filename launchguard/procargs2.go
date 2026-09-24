package launchguard

import (
	"encoding/binary"
	"errors"
)

// parseProcArgs2 parses the raw byte layout the macOS kern.procargs2 sysctl
// returns for a given pid (the same data `ps` reconstructs argv from):
//
//	[0:4)  argc, native-endian int32
//	       the saved executable path, NUL-terminated
//	       NUL padding, up to the first argv byte
//	       argc NUL-terminated argv strings
//	       (environment strings may follow; ignored)
//
// macOS only runs on little-endian architectures (amd64, arm64), so argc is
// read little-endian.
func parseProcArgs2(buf []byte) ([]string, error) {
	if len(buf) < 4 {
		return nil, errors.New("procargs2: buffer too short for argc")
	}
	argc := int(int32(binary.LittleEndian.Uint32(buf[:4])))
	if argc < 0 {
		return nil, errors.New("procargs2: negative argc")
	}
	i := 4
	for i < len(buf) && buf[i] != 0 { // saved executable path
		i++
	}
	for i < len(buf) && buf[i] == 0 { // NUL padding before argv[0]
		i++
	}
	if argc == 0 {
		return nil, nil
	}
	argv := make([]string, 0, argc)
	for n := 0; n < argc; n++ {
		if i >= len(buf) {
			return nil, errors.New("procargs2: truncated argv")
		}
		start := i
		for i < len(buf) && buf[i] != 0 {
			i++
		}
		argv = append(argv, string(buf[start:i]))
		if i < len(buf) {
			i++ // skip the NUL terminator
		} else if n < argc-1 {
			return nil, errors.New("procargs2: truncated argv")
		}
	}
	return argv, nil
}
