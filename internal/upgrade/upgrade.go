package upgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	RepoOwner = "jmeiracorbal"
	RepoName  = "hc"
)

// Client talks to GitHub releases. HTTPClient and BaseURL are injectable for tests.
type Client struct {
	HTTPClient *http.Client
	BaseURL    string
}

func NewClient() *Client {
	return &Client{
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
		BaseURL:    "https://api.github.com",
	}
}

type releaseResponse struct {
	TagName string `json:"tag_name"`
}

// LatestRelease returns the latest GitHub release tag (e.g. "v0.3.0").
func (c *Client) LatestRelease(ctx context.Context) (string, error) {
	if c == nil {
		return "", fmt.Errorf("upgrade client is required")
	}
	if c.HTTPClient == nil {
		return "", fmt.Errorf("http client is required")
	}
	if c.BaseURL == "" {
		return "", fmt.Errorf("base url is required")
	}
	url := strings.TrimRight(c.BaseURL, "/") + "/repos/" + RepoOwner + "/" + RepoName + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github releases: status %d", resp.StatusCode)
	}
	var rel releaseResponse
	if err := json.Unmarshal(body, &rel); err != nil {
		return "", err
	}
	if rel.TagName == "" {
		return "", fmt.Errorf("github releases: empty tag_name")
	}
	return rel.TagName, nil
}

// LatestRelease uses a default client.
func LatestRelease(ctx context.Context) (string, error) {
	return NewClient().LatestRelease(ctx)
}

// CompareVersions reports whether running is older than latest.
// running "dev" or empty is never outdated (caller should skip the check).
func CompareVersions(running, latest string) (outdated bool) {
	if running == "" || running == "dev" {
		return false
	}
	if latest == "" {
		return false
	}
	return compareSemver(normalizeVersion(running), normalizeVersion(latest)) < 0
}

func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	return v
}

func compareSemver(a, b string) int {
	ap := strings.Split(a, ".")
	bp := strings.Split(b, ".")
	n := len(ap)
	if len(bp) > n {
		n = len(bp)
	}
	for i := 0; i < n; i++ {
		var ai, bi int
		if i < len(ap) {
			ai, _ = strconv.Atoi(ap[i])
		}
		if i < len(bp) {
			bi, _ = strconv.Atoi(bp[i])
		}
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	return 0
}

// InstallHint is the curl|bash installer command.
func InstallHint() string {
	return "curl -sSf https://raw.githubusercontent.com/" + RepoOwner + "/" + RepoName + "/main/install.sh | bash"
}
