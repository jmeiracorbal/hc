package coco

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Resolver picks a coco for a file path from installed + built-in candidates.
type Resolver struct {
	byID  map[string]Coco
	byExt map[string][]Coco // extension -> candidates (unsorted)
}

// NewResolver builds a resolver from coco list. Duplicate ids: last wins.
func NewResolver(cocos []Coco) (*Resolver, error) {
	r := &Resolver{
		byID:  map[string]Coco{},
		byExt: map[string][]Coco{},
	}
	for _, c := range cocos {
		if c == nil {
			return nil, fmt.Errorf("nil coco")
		}
		id := c.ID()
		if id == "" {
			return nil, fmt.Errorf("coco id is required")
		}
		if c.Language() == "" {
			return nil, fmt.Errorf("coco %s: language is required", id)
		}
		exts := c.Extensions()
		if len(exts) == 0 {
			return nil, fmt.Errorf("coco %s: extensions are required", id)
		}
		r.byID[id] = c
		for _, ext := range exts {
			ext = normalizeExt(ext)
			if ext == "" {
				return nil, fmt.Errorf("coco %s: empty extension", id)
			}
			r.byExt[ext] = append(r.byExt[ext], c)
		}
	}
	return r, nil
}

// Resolve returns the coco for path, or nil if unsupported.
func (r *Resolver) Resolve(path string, pins Pins) (Coco, error) {
	if r == nil {
		return nil, fmt.Errorf("resolver is required")
	}
	ext := normalizeExt(filepath.Ext(path))
	if ext == "" {
		return nil, nil
	}
	if pins.ExtensionPins != nil {
		if id, ok := pins.ExtensionPins[ext]; ok {
			c, found := r.byID[id]
			if !found {
				return nil, fmt.Errorf("extension pin %s -> %s: coco not installed", ext, id)
			}
			if !cocoHasExt(c, ext) {
				return nil, fmt.Errorf("extension pin %s -> %s: coco does not claim extension", ext, id)
			}
			return c, nil
		}
	}
	for _, id := range pins.Cocos {
		c, found := r.byID[id]
		if !found {
			return nil, fmt.Errorf("pinned coco %s not installed", id)
		}
		if cocoHasExt(c, ext) {
			return c, nil
		}
	}
	cands := r.byExt[ext]
	if len(cands) == 0 {
		return nil, nil
	}
	best, err := pickHighestPriority(cands)
	if err != nil {
		return nil, fmt.Errorf("extension %s: %w", ext, err)
	}
	return best, nil
}

// DetectLanguage returns the language key for path under pins, or empty.
func (r *Resolver) DetectLanguage(path string, pins Pins) (string, error) {
	c, err := r.Resolve(path, pins)
	if err != nil {
		return "", err
	}
	if c == nil {
		return "", nil
	}
	return c.Language(), nil
}

// List returns all registered cocos.
func (r *Resolver) List() []Coco {
	out := make([]Coco, 0, len(r.byID))
	for _, c := range r.byID {
		out = append(out, c)
	}
	return out
}

func pickHighestPriority(cands []Coco) (Coco, error) {
	if len(cands) == 0 {
		return nil, fmt.Errorf("no candidates")
	}
	best := cands[0]
	tie := false
	for _, c := range cands[1:] {
		if c.Priority() > best.Priority() {
			best = c
			tie = false
			continue
		}
		if c.Priority() == best.Priority() && c.ID() != best.ID() {
			tie = true
		}
	}
	if tie {
		var ids []string
		for _, c := range cands {
			if c.Priority() == best.Priority() {
				ids = append(ids, c.ID())
			}
		}
		return nil, fmt.Errorf("priority tie between %s (use hc.toml pins)", strings.Join(ids, ", "))
	}
	return best, nil
}

func cocoHasExt(c Coco, ext string) bool {
	for _, e := range c.Extensions() {
		if normalizeExt(e) == ext {
			return true
		}
	}
	return false
}

func normalizeExt(ext string) string {
	ext = strings.ToLower(strings.TrimSpace(ext))
	if ext == "" {
		return ""
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return ext
}
