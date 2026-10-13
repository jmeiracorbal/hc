package setup

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ExcludeLocal adds files under dir to the repository's local ignore list
// (.git/info/exclude), which is never committed. Patterns are anchored to the
// repo root so only these exact paths are hidden. Returns the patterns added;
// nil without error when dir is not inside a git work tree or git is missing.
func ExcludeLocal(dir string, names ...string) ([]string, error) {
	top, err := gitOutput(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, nil
	}
	excludePath, err := gitOutput(dir, "rev-parse", "--path-format=absolute", "--git-path", "info/exclude")
	if err != nil {
		return nil, err
	}
	topReal, err := filepath.EvalSymlinks(top)
	if err != nil {
		return nil, err
	}
	dirReal, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(topReal, dirReal)
	if err != nil {
		return nil, err
	}

	existing := map[string]struct{}{}
	raw, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		existing[strings.TrimSpace(sc.Text())] = struct{}{}
	}

	var added []string
	for _, name := range names {
		pattern := "/" + filepath.ToSlash(filepath.Join(rel, name))
		if _, ok := existing[pattern]; ok {
			continue
		}
		existing[pattern] = struct{}{}
		added = append(added, pattern)
	}
	if len(added) == 0 {
		return nil, nil
	}

	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if len(raw) > 0 && !bytes.HasSuffix(raw, []byte("\n")) {
		buf.WriteByte('\n')
	}
	for _, p := range added {
		buf.WriteString(p + "\n")
	}
	f, err := os.OpenFile(excludePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	return added, nil
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}
