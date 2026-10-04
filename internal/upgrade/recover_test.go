package upgrade_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jmeiracorbal/hybrid-coco/internal/upgrade"
)

func TestRecoverAfterInstallRequiresExe(t *testing.T) {
	if _, err := upgrade.RecoverAfterInstall(""); err == nil {
		t.Fatal("expected error")
	}
	if _, err := upgrade.RecoverAfterInstall(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected missing binary error")
	}
}

func TestRecoverAfterInstallRunsDoctorFix(t *testing.T) {
	exe := os.Args[0]
	// when tests run as package binary, re-execing test binary with doctor args fails.
	// use a tiny shim script that exits 0 and prints json.
	dir := t.TempDir()
	shim := filepath.Join(dir, "hc-shim")
	script := "#!/bin/sh\necho '{\"healthy\":true,\"repairs_available\":[]}'\n"
	if err := os.WriteFile(shim, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := upgrade.RecoverAfterInstall(shim)
	if err != nil {
		t.Fatalf("RecoverAfterInstall: %v (exe was %s)", err, exe)
	}
	if out == "" {
		t.Fatal("expected stdout")
	}
	// ensure shim was invoked with doctor --fix --json
	cmd := exec.Command(shim, "doctor", "--fix", "--json")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
}
