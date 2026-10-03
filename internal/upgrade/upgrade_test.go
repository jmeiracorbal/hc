package upgrade_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jmeiracorbal/hybrid-coco/internal/upgrade"
)

func TestLatestRelease_HTTPTest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/jmeiracorbal/hybrid-coco/releases/latest" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v0.9.0"}`))
	}))
	defer srv.Close()

	c := &upgrade.Client{
		HTTPClient: srv.Client(),
		BaseURL:    srv.URL,
	}
	tag, err := c.LatestRelease(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tag != "v0.9.0" {
		t.Fatalf("tag=%q", tag)
	}
}

func TestCompareVersions(t *testing.T) {
	if !upgrade.CompareVersions("0.1.0", "v0.2.0") {
		t.Fatal("expected outdated")
	}
	if upgrade.CompareVersions("v0.2.0", "v0.2.0") {
		t.Fatal("same version not outdated")
	}
	if upgrade.CompareVersions("dev", "v1.0.0") {
		t.Fatal("dev must not be outdated")
	}
	if upgrade.CompareVersions("", "v1.0.0") {
		t.Fatal("empty must not be outdated")
	}
}

func TestInstallHint(t *testing.T) {
	h := upgrade.InstallHint()
	if !strings.Contains(h, "jmeiracorbal/hybrid-coco") {
		t.Fatalf("hint missing repo: %s", h)
	}
	if !strings.Contains(h, "install.sh") {
		t.Fatalf("hint missing install.sh: %s", h)
	}
}
