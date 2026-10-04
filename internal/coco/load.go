package coco

import (
	"context"
	"fmt"
	"sync"

	"github.com/jmeiracorbal/hybrid-coco/internal/coco/manifest"
	"github.com/jmeiracorbal/hybrid-coco/internal/coco/registry"
	"github.com/jmeiracorbal/hybrid-coco/internal/coco/wasmhost"
)

// Cache holds loaded wasm modules for reuse across files.
type Cache struct {
	mu   sync.Mutex
	mods map[string]*wasmhost.Module
	ctx  context.Context
}

// NewCache creates an empty wasm module cache.
func NewCache(ctx context.Context) *Cache {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Cache{mods: map[string]*wasmhost.Module{}, ctx: ctx}
}

// Close closes all cached modules.
func (c *Cache) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var first error
	for id, m := range c.mods {
		if err := m.Close(c.ctx); err != nil && first == nil {
			first = err
		}
		delete(c.mods, id)
	}
	return first
}

func (c *Cache) getOrLoad(rec registry.Record, man *manifest.Manifest) (*wasmhost.Module, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if m, ok := c.mods[rec.ID]; ok {
		return m, nil
	}
	m, err := wasmhost.Load(c.ctx, rec.ID, man.Language, man.Extensions, man.Priority, rec.WasmPath)
	if err != nil {
		return nil, err
	}
	c.mods[rec.ID] = m
	return m, nil
}

// LoadResolver builds a resolver from builtins + installed wasm cocos.
func LoadResolver(cache *Cache) (*Resolver, error) {
	builtins, err := Builtins()
	if err != nil {
		return nil, err
	}
	cocos := append([]Coco(nil), builtins...)

	db, err := registry.Open()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	recs, err := registry.List(db)
	if err != nil {
		return nil, err
	}
	if cache == nil {
		cache = NewCache(context.Background())
	}
	for _, rec := range recs {
		if rec.Contract != ContractWASM {
			return nil, fmt.Errorf("coco %s: unsupported contract %q", rec.ID, rec.Contract)
		}
		man, err := manifest.ParseFile(rec.ManifestPath)
		if err != nil {
			return nil, fmt.Errorf("coco %s manifest: %w", rec.ID, err)
		}
		mod, err := cache.getOrLoad(rec, man)
		if err != nil {
			return nil, fmt.Errorf("coco %s: %w", rec.ID, err)
		}
		cocos = append(cocos, mod)
	}
	return NewResolver(cocos)
}

// ListInstalled returns registry records (may be empty).
func ListInstalled() ([]registry.Record, error) {
	db, err := registry.Open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return registry.List(db)
}
