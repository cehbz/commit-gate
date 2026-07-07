package gatestate

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func AppendApprovals(manifestPath string, msgs []string) error {
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(manifestPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, m := range msgs {
		if _, err := f.WriteString(CanonicalHash(m) + "  " + Subject(m) + "\n"); err != nil {
			return err
		}
	}
	return nil
}

// ConsumeApproval removes every manifest line for hash (single-use approval;
// duplicates collapse, matching bash grep -v). Atomic: temp file + rename.
func ConsumeApproval(manifestPath, hash string) (bool, error) {
	raw, err := os.ReadFile(manifestPath)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	prefix := hash + "  "
	var kept []string
	matched := false
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, prefix) {
			matched = true
			continue
		}
		if line != "" {
			kept = append(kept, line)
		}
	}
	if !matched {
		return false, nil
	}
	tmp, err := os.CreateTemp(filepath.Dir(manifestPath), ".approved.*")
	if err != nil {
		return false, err
	}
	content := ""
	if len(kept) > 0 {
		content = strings.Join(kept, "\n") + "\n"
	}
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return false, err
	}
	tmp.Close()
	return true, os.Rename(tmp.Name(), manifestPath)
}

func ApprovedSubjects(manifestPath string) ([]string, error) {
	raw, err := os.ReadFile(manifestPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
		if line == "" {
			continue
		}
		if i := strings.Index(line, "  "); i == 64 {
			out = append(out, line[i+2:])
		} else {
			out = append(out, line)
		}
	}
	return out, nil
}
