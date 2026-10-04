package coco

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const ProjectConfigFile = "hc.toml"

// ProjectConfig is optional project-level coco pins (hc.toml next to .hc).
type ProjectConfig struct {
	Cocos         []string          `toml:"cocos"`
	ExtensionPins map[string]string `toml:"extension_pins"`
}

// LoadPins reads hc.toml from project root. Missing file => empty pins.
func LoadPins(projectRoot string) (Pins, error) {
	if projectRoot == "" {
		return Pins{}, fmt.Errorf("project root is required")
	}
	path := filepath.Join(projectRoot, ProjectConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Pins{}, nil
		}
		return Pins{}, err
	}
	var cfg ProjectConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Pins{}, fmt.Errorf("%s: %w", ProjectConfigFile, err)
	}
	pins := Pins{
		Cocos:         append([]string(nil), cfg.Cocos...),
		ExtensionPins: map[string]string{},
	}
	for k, v := range cfg.ExtensionPins {
		if k == "" || v == "" {
			return Pins{}, fmt.Errorf("%s: extension_pins entries must be non-empty", ProjectConfigFile)
		}
		pins.ExtensionPins[normalizeExt(k)] = v
	}
	return pins, nil
}
