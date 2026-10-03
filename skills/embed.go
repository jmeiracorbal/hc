package skills

import (
	"embed"
	"fmt"
)

//go:embed hc-navigate/SKILL.md hc-graph/SKILL.md hc-index/SKILL.md hc-doctor/SKILL.md
var fsData embed.FS

// Names is the ordered list of shipped agent skills.
var Names = []string{"hc-navigate", "hc-graph", "hc-index", "hc-doctor"}

func MD(name string) ([]byte, error) {
	if !validName(name) {
		return nil, fmt.Errorf("unknown skill %q", name)
	}
	data, err := fsData.ReadFile(name + "/SKILL.md")
	if err != nil {
		return nil, fmt.Errorf("skill %s: %w", name, err)
	}
	return data, nil
}

func validName(name string) bool {
	for _, n := range Names {
		if n == name {
			return true
		}
	}
	return false
}
