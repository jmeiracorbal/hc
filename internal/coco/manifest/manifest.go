package manifest

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"golang.org/x/mod/semver"
)

const FileName = "coco.toml"

// ContractWASM is the only external coco transport in v1.
const ContractWASM = "wasm/v1"

// PlatformWASI is the release platform for wasm cocos.
const PlatformWASI = "wasm32/wasi"

var idRegex = regexp.MustCompile(`^[a-z0-9_-]+/[a-z0-9_-]+$`)

var availableContracts = []string{ContractWASM}

// Manifest is the required coco.toml schema.
type Manifest struct {
	ID         string    `toml:"id"`
	Language   string    `toml:"language"`
	Extensions []string  `toml:"extensions"`
	Priority   int       `toml:"priority"`
	Contract   string    `toml:"contract"`
	Platforms  []string  `toml:"platforms"`
	HCVersion  HCVersion `toml:"hc-version"`
}

// HCVersion declares which hc core versions may run this coco.
type HCVersion struct {
	Version    string `toml:"version"`
	Constraint string `toml:"constraint"` // min | exact
}

// ParseFile reads and decodes coco.toml.
func ParseFile(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Parse decodes coco.toml bytes.
func Parse(data []byte) (*Manifest, error) {
	var m Manifest
	if err := toml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("coco.toml: %w", err)
	}
	return &m, nil
}

// Validate checks manifest fields, platform, and hc version compatibility.
func (m *Manifest) Validate(runningHC, platform string) error {
	if m == nil {
		return fmt.Errorf("manifest is required")
	}
	if !idRegex.MatchString(m.ID) {
		return fmt.Errorf("id %q does not match naming rule author/name", m.ID)
	}
	if strings.TrimSpace(m.Language) == "" {
		return fmt.Errorf("language is required")
	}
	if len(m.Extensions) == 0 {
		return fmt.Errorf("extensions are required")
	}
	for _, ext := range m.Extensions {
		if strings.TrimSpace(ext) == "" {
			return fmt.Errorf("extensions must not contain empty values")
		}
	}
	if !contractSupported(m.Contract) {
		return fmt.Errorf("unsupported contract %q (supported: %s)", m.Contract, strings.Join(availableContracts, ", "))
	}
	if !semver.IsValid(normalizeSemver(m.HCVersion.Version)) {
		return fmt.Errorf("hc-version.version %q is not valid semver", m.HCVersion.Version)
	}
	if m.HCVersion.Constraint != "min" && m.HCVersion.Constraint != "exact" {
		return fmt.Errorf("hc-version.constraint must be min or exact")
	}
	if !platformListed(m.Platforms, platform) {
		return fmt.Errorf("platform %q not in manifest platforms", platform)
	}
	return m.validateHCVersion(runningHC)
}

func (m *Manifest) validateHCVersion(running string) error {
	want := normalizeSemver(m.HCVersion.Version)
	have := normalizeRunning(running)
	if !semver.IsValid(have) {
		return fmt.Errorf("running hc version %q is not valid semver", running)
	}
	switch m.HCVersion.Constraint {
	case "exact":
		if semver.Compare(have, want) != 0 {
			return fmt.Errorf("hc version %s required exactly, running %s", want, have)
		}
	case "min":
		if semver.Compare(have, want) < 0 {
			return fmt.Errorf("hc version >= %s required, running %s", want, have)
		}
	}
	return nil
}

func contractSupported(c string) bool {
	for _, v := range availableContracts {
		if v == c {
			return true
		}
	}
	return false
}

func platformListed(platforms []string, platform string) bool {
	for _, p := range platforms {
		if p == platform {
			return true
		}
	}
	return false
}

func normalizeSemver(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return v
	}
	if !strings.HasPrefix(v, "v") {
		return "v" + v
	}
	return v
}

// normalizeRunning maps the local "dev" binary label to a semver prerelease.
func normalizeRunning(v string) string {
	if strings.TrimSpace(v) == "dev" {
		return "v0.0.0-dev"
	}
	return normalizeSemver(v)
}
