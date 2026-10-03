package upgrade_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmeiracorbal/hybrid-coco/internal/upgrade"
)

func TestInstall_ChecksumOK(t *testing.T) {
	payload := []byte("#!/bin/sh\necho hc\n")
	sum := sha256.Sum256(payload)
	hexSum := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ".sha256"):
			_, _ = w.Write([]byte(hexSum + "  hc-test\n"))
		case strings.Contains(r.URL.Path, "/hc-"):
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	destDir := t.TempDir()
	dest := filepath.Join(destDir, "hc")
	if err := os.WriteFile(dest, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	c := &upgrade.Client{HTTPClient: srv.Client()}
	err := c.Install(context.Background(), upgrade.InstallOpts{
		Version:      "v9.9.9",
		DestPath:     dest,
		Platform:     "darwin-arm64",
		DownloadBase: srv.URL + "/download",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("dest content mismatch")
	}
}

func TestInstall_ChecksumFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".sha256") {
			_, _ = w.Write([]byte(strings.Repeat("0", 64) + "\n"))
			return
		}
		_, _ = w.Write([]byte("binary"))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "hc")
	c := &upgrade.Client{HTTPClient: srv.Client()}
	err := c.Install(context.Background(), upgrade.InstallOpts{
		Version:      "v1.0.0",
		DestPath:     dest,
		Platform:     "linux-amd64",
		DownloadBase: srv.URL,
	})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("err=%v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("dest must not exist after failed install")
	}
}

func TestDetectPlatform(t *testing.T) {
	p, err := upgrade.DetectPlatform()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p, "-") {
		t.Fatalf("platform=%s", p)
	}
}
