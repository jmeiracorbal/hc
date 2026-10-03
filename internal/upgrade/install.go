package upgrade

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const DefaultDownloadBase = "https://github.com/" + RepoOwner + "/" + RepoName + "/releases/download"

// DetectPlatform returns os-arch as used by install.sh (e.g. darwin-arm64).
func DetectPlatform() (string, error) {
	var osName string
	switch runtime.GOOS {
	case "darwin":
		osName = "darwin"
	case "linux":
		osName = "linux"
	default:
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	var arch string
	switch runtime.GOARCH {
	case "amd64":
		arch = "amd64"
	case "arm64":
		arch = "arm64"
	default:
		return "", fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
	}
	return osName + "-" + arch, nil
}

// InstallOpts configures binary download + replace.
type InstallOpts struct {
	Version      string // release tag, required
	DestPath     string // binary path to replace, required
	Platform     string // e.g. darwin-arm64; empty = DetectPlatform
	DownloadBase string // empty = DefaultDownloadBase
}

// Install downloads hc for Version, verifies sha256, and atomically replaces DestPath.
func (c *Client) Install(ctx context.Context, opts InstallOpts) error {
	if c == nil {
		return fmt.Errorf("upgrade client is required")
	}
	if c.HTTPClient == nil {
		return fmt.Errorf("http client is required")
	}
	if opts.Version == "" {
		return fmt.Errorf("version is required")
	}
	if opts.DestPath == "" {
		return fmt.Errorf("dest path is required")
	}
	platform := opts.Platform
	if platform == "" {
		var err error
		platform, err = DetectPlatform()
		if err != nil {
			return err
		}
	}
	base := opts.DownloadBase
	if base == "" {
		base = DefaultDownloadBase
	}
	base = strings.TrimRight(base, "/")
	binURL := base + "/" + opts.Version + "/hc-" + platform
	sumURL := binURL + ".sha256"

	tmpDir, err := os.MkdirTemp("", "hc-upgrade-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	tmpBin := filepath.Join(tmpDir, "hc")

	if err := c.downloadFile(ctx, binURL, tmpBin); err != nil {
		return fmt.Errorf("download binary: %w", err)
	}
	expected, err := c.fetchChecksum(ctx, sumURL)
	if err != nil {
		return fmt.Errorf("download checksum: %w", err)
	}
	actual, err := fileSHA256(tmpBin)
	if err != nil {
		return err
	}
	if !strings.EqualFold(expected, actual) {
		return fmt.Errorf("checksum mismatch: expected %s got %s", expected, actual)
	}
	if err := os.Chmod(tmpBin, 0o755); err != nil {
		return err
	}

	destDir := filepath.Dir(opts.DestPath)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	staged := opts.DestPath + ".new"
	_ = os.Remove(staged)
	if err := copyReplace(tmpBin, staged); err != nil {
		return err
	}
	if err := os.Chmod(staged, 0o755); err != nil {
		_ = os.Remove(staged)
		return err
	}
	if err := os.Rename(staged, opts.DestPath); err != nil {
		_ = os.Remove(staged)
		return fmt.Errorf("replace binary at %s: %w (dir may not be writable; use install.sh)", opts.DestPath, err)
	}
	return nil
}

// InstallBinary is a convenience using a client with longer timeout for downloads.
func InstallBinary(ctx context.Context, opts InstallOpts) error {
	c := &Client{
		HTTPClient: &http.Client{Timeout: 120 * time.Second},
		BaseURL:    "https://api.github.com",
	}
	return c.Install(ctx, opts)
}

func (c *Client) downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d for %s", resp.StatusCode, url)
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, io.LimitReader(resp.Body, 256<<20)); err != nil {
		_ = os.Remove(dest)
		return err
	}
	return out.Close()
}

func (c *Client) fetchChecksum(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d for %s", resp.StatusCode, url)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(body))
	if len(fields) == 0 {
		return "", fmt.Errorf("empty checksum file")
	}
	sum := fields[0]
	if len(sum) != 64 {
		return "", fmt.Errorf("invalid sha256 length in checksum file")
	}
	return sum, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyReplace(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		_ = os.Remove(dst)
		return err
	}
	return out.Close()
}
