package upgrade

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// RecoverAfterInstall runs safe auto-recovery with the newly installed binary.
// The current process still executes the old image after replace, so schema
// migrations must run via re-exec of exe (doctor --fix applies apply_migrations).
func RecoverAfterInstall(exe string) (stdout string, err error) {
	if exe == "" {
		return "", fmt.Errorf("executable path is required")
	}
	if _, err := os.Stat(exe); err != nil {
		return "", fmt.Errorf("new binary: %w", err)
	}
	cmd := exec.Command(exe, "doctor", "--fix", "--json")
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(out.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return out.String(), fmt.Errorf("post-upgrade recovery: %s", msg)
	}
	return out.String(), nil
}
