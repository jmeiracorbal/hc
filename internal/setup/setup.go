package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmeiracorbal/hybrid-coco/assets"
	"github.com/jmeiracorbal/hybrid-coco/internal/config"
	"github.com/jmeiracorbal/hybrid-coco/internal/store"
	"github.com/jmeiracorbal/hybrid-coco/skills"
)

const (
	hookPre  = "hc-pre-tool-use.sh"
	hookPost = "hc-post-tool-use.sh"
)

var mcpEntry = map[string]any{
	"command": "hc",
	"args":    []string{"serve"},
	"type":    "stdio",
}

var mcpTools = []string{"hc_search", "hc_symbol", "hc_file_context", "hc_package", "hc_explore", "hc_impact", "hc_path", "hc_status"}

type Result struct {
	AwarenessWritten bool
	ClaudeMDUpdated  bool
	HooksInstalled   bool
	SettingsPatched  bool
	SkillsInstalled  []string
	MCPRegistered    bool
	MCPPath          string
	Tools            []string
	SharedIndexPath  string
}

func InstallGlobal(claudeDir string) (Result, error) {
	var r Result
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		return r, err
	}

	awareness, err := assets.AwarenessMD()
	if err != nil {
		return r, err
	}
	dstAwareness := filepath.Join(claudeDir, "hybrid-coco.md")
	if err := os.WriteFile(dstAwareness, awareness, 0o644); err != nil {
		return r, err
	}
	r.AwarenessWritten = true

	claudeMD := filepath.Join(claudeDir, "CLAUDE.md")
	tag := "@hybrid-coco.md"
	content, err := os.ReadFile(claudeMD)
	if err != nil && !os.IsNotExist(err) {
		return r, err
	}
	text := string(content)
	if !strings.Contains(text, tag) {
		sep := ""
		if len(text) > 0 && !strings.HasSuffix(text, "\n") {
			sep = "\n"
		}
		if err := os.WriteFile(claudeMD, []byte(text+sep+tag+"\n"), 0o644); err != nil {
			return r, err
		}
		r.ClaudeMDUpdated = true
	}

	hooksDir := filepath.Join(claudeDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return r, err
	}
	for _, name := range []string{hookPre, hookPost} {
		data, err := assets.Hook(name)
		if err != nil {
			return r, err
		}
		dst := filepath.Join(hooksDir, name)
		if err := os.WriteFile(dst, data, 0o755); err != nil {
			return r, err
		}
	}
	r.HooksInstalled = true

	home, err := os.UserHomeDir()
	if err != nil {
		return r, err
	}
	if home == "" {
		return r, fmt.Errorf("home directory is required")
	}
	skills, err := installSkills(home, claudeDir)
	if err != nil {
		return r, err
	}
	r.SkillsInstalled = skills

	settingsPath := filepath.Join(claudeDir, "settings.json")
	data, err := readJSON(settingsPath)
	if err != nil {
		return r, err
	}
	hooks, _ := data["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		data["hooks"] = hooks
	}
	pre := asSlice(hooks["PreToolUse"])
	post := asSlice(hooks["PostToolUse"])
	preCmd := filepath.Join(hooksDir, hookPre)
	postCmd := filepath.Join(hooksDir, hookPost)
	patched := false
	if !entryPresent(pre, preCmd) {
		pre = append(pre, map[string]any{
			"matcher": "Read|Grep",
			"hooks":   []any{map[string]any{"type": "command", "command": preCmd}},
		})
		patched = true
	}
	if !entryPresent(post, postCmd) {
		post = append(post, map[string]any{
			"matcher": "Write|Edit",
			"hooks":   []any{map[string]any{"type": "command", "command": postCmd}},
		})
		patched = true
	}
	hooks["PreToolUse"] = pre
	hooks["PostToolUse"] = post
	if err := writeJSON(settingsPath, data); err != nil {
		return r, err
	}
	r.SettingsPatched = patched

	dbPath, err := config.EnsureSharedIndex()
	if err != nil {
		return r, err
	}
	st, err := store.Open(dbPath)
	if err != nil {
		return r, err
	}
	if err := st.Close(); err != nil {
		return r, err
	}
	r.SharedIndexPath = dbPath
	return r, nil
}

func installSkills(home, claudeDir string) ([]string, error) {
	var installed []string
	agentsRoot := filepath.Join(home, ".agents", "skills")
	claudeSkills := filepath.Join(claudeDir, "skills")
	if err := os.MkdirAll(agentsRoot, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(claudeSkills, 0o755); err != nil {
		return nil, err
	}

	for _, name := range skills.Names {
		data, err := skills.MD(name)
		if err != nil {
			return nil, err
		}
		canonDir := filepath.Join(agentsRoot, name)
		if err := os.MkdirAll(canonDir, 0o755); err != nil {
			return nil, err
		}
		canonFile := filepath.Join(canonDir, "SKILL.md")
		if err := os.WriteFile(canonFile, data, 0o644); err != nil {
			return nil, err
		}
		link := filepath.Join(claudeSkills, name)
		if err := ensureDirSymlink(link, canonDir); err != nil {
			return nil, err
		}
		installed = append(installed, name)
	}
	return installed, nil
}

func ensureDirSymlink(link, target string) error {
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	// resolve for comparison on platforms where /var -> /private/var
	resolvedWant, err := filepath.EvalSymlinks(absTarget)
	if err != nil {
		resolvedWant = absTarget
	}

	info, err := os.Lstat(link)
	switch {
	case err == nil:
		if info.Mode()&os.ModeSymlink != 0 {
			current, readErr := filepath.EvalSymlinks(link)
			if readErr == nil && current == resolvedWant {
				return nil
			}
			if removeErr := os.Remove(link); removeErr != nil {
				return fmt.Errorf("replace skill symlink %s: %w", link, removeErr)
			}
		} else {
			return fmt.Errorf("skill link path %s exists and is not a symlink", link)
		}
	case !os.IsNotExist(err):
		return fmt.Errorf("stat skill link %s: %w", link, err)
	}

	if err := os.Symlink(resolvedWant, link); err != nil {
		return fmt.Errorf("create skill symlink %s -> %s: %w", link, resolvedWant, err)
	}
	return nil
}

func RegisterMCP(settingsPath string) (Result, error) {
	var r Result
	data, err := readJSON(settingsPath)
	if err != nil {
		return r, err
	}
	servers, _ := data["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
		data["mcpServers"] = servers
	}
	servers["hybrid-coco"] = mcpEntry
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return r, err
	}
	if err := writeJSON(settingsPath, data); err != nil {
		return r, err
	}
	r.MCPRegistered = true
	r.MCPPath = settingsPath
	r.Tools = append([]string{}, mcpTools...)
	return r, nil
}

func readJSON(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return map[string]any{}, nil
	}
	if data == nil {
		data = map[string]any{}
	}
	return data, nil
}

func writeJSON(path string, data map[string]any) error {
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(path, out, 0o644)
}

func asSlice(v any) []any {
	if v == nil {
		return nil
	}
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}

func entryPresent(entries []any, command string) bool {
	for _, entry := range entries {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		hooks := asSlice(m["hooks"])
		for _, h := range hooks {
			hm, ok := h.(map[string]any)
			if !ok {
				continue
			}
			if fmt.Sprint(hm["command"]) == command {
				return true
			}
		}
	}
	return false
}
