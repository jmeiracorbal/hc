package wasmhost

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"

	"github.com/jmeiracorbal/hybrid-coco/internal/parsers"
)

const ABIVersion = 1

// Module is a loaded wasm coco with a cached instance.
type Module struct {
	id       string
	language string
	exts     []string
	priority int

	mu   sync.Mutex
	rt   wazero.Runtime
	mod  api.Module
	ctx  context.Context
	code wazero.CompiledModule
}

// Load compiles and instantiates a wasm/v1 coco module.
func Load(ctx context.Context, id, language string, exts []string, priority int, wasmPath string) (*Module, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context is required")
	}
	if id == "" || language == "" || wasmPath == "" {
		return nil, fmt.Errorf("id, language, wasmPath are required")
	}
	if len(exts) == 0 {
		return nil, fmt.Errorf("extensions are required")
	}
	wasm, err := os.ReadFile(wasmPath)
	if err != nil {
		return nil, err
	}
	rt := wazero.NewRuntime(ctx)
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		rt.Close(ctx)
		return nil, fmt.Errorf("wasi: %w", err)
	}
	code, err := rt.CompileModule(ctx, wasm)
	if err != nil {
		rt.Close(ctx)
		return nil, fmt.Errorf("compile: %w", err)
	}
	cfg := wazero.NewModuleConfig().WithName(id)
	// Go wasip1 + wasmexport is a reactor: run _initialize, not _start.
	if _, ok := code.ExportedFunctions()["_initialize"]; ok {
		cfg = cfg.WithStartFunctions("_initialize")
	} else {
		cfg = cfg.WithStartFunctions()
	}
	mod, err := rt.InstantiateModule(ctx, code, cfg)
	if err != nil {
		code.Close(ctx)
		rt.Close(ctx)
		return nil, fmt.Errorf("instantiate: %w", err)
	}
	m := &Module{
		id: id, language: language, exts: append([]string(nil), exts...), priority: priority,
		rt: rt, mod: mod, ctx: ctx, code: code,
	}
	if err := m.checkABI(); err != nil {
		m.Close(ctx)
		return nil, err
	}
	return m, nil
}

// Probe loads wasm, checks ABI, runs parse on fixture, then closes.
func Probe(ctx context.Context, wasmPath, fixturePath string, fixtureSource []byte) error {
	m, err := Load(ctx, "probe/tmp", "probe", []string{".probe"}, 0, wasmPath)
	if err != nil {
		return err
	}
	defer m.Close(ctx)
	_, err = m.Parse(fixtureSource, fixturePath)
	return err
}

func (m *Module) ID() string           { return m.id }
func (m *Module) Language() string     { return m.language }
func (m *Module) Extensions() []string { return append([]string(nil), m.exts...) }
func (m *Module) Priority() int        { return m.priority }

func (m *Module) checkABI() error {
	fn := m.mod.ExportedFunction("coco_abi_version")
	if fn == nil {
		return fmt.Errorf("missing export coco_abi_version")
	}
	for _, name := range []string{"coco_alloc", "coco_free", "coco_parse"} {
		if m.mod.ExportedFunction(name) == nil {
			return fmt.Errorf("missing export %s", name)
		}
	}
	res, err := fn.Call(m.ctx)
	if err != nil {
		return err
	}
	if len(res) != 1 || int32(res[0]) != ABIVersion {
		return fmt.Errorf("coco_abi_version: want %d", ABIVersion)
	}
	return nil
}

// Parse invokes coco_parse and unmarshals JSON ParseResult.
func (m *Module) Parse(source []byte, path string) (parsers.ParseResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	pathBytes := []byte(path)
	pathPtr, err := m.alloc(uint32(len(pathBytes)))
	if err != nil {
		return parsers.ParseResult{}, err
	}
	defer m.free(pathPtr, uint32(len(pathBytes)))
	if !m.mod.Memory().Write(pathPtr, pathBytes) {
		return parsers.ParseResult{}, fmt.Errorf("write path")
	}

	srcPtr, err := m.alloc(uint32(len(source)))
	if err != nil {
		return parsers.ParseResult{}, err
	}
	defer m.free(srcPtr, uint32(len(source)))
	if len(source) > 0 && !m.mod.Memory().Write(srcPtr, source) {
		return parsers.ParseResult{}, fmt.Errorf("write source")
	}

	parseFn := m.mod.ExportedFunction("coco_parse")
	res, err := parseFn.Call(m.ctx, uint64(pathPtr), uint64(len(pathBytes)), uint64(srcPtr), uint64(len(source)))
	if err != nil {
		return parsers.ParseResult{}, fmt.Errorf("coco_parse: %w", err)
	}
	if len(res) != 1 {
		return parsers.ParseResult{}, fmt.Errorf("coco_parse: expected i64 return")
	}
	packed := res[0]
	outPtr := uint32(packed >> 32)
	outLen := uint32(packed)
	if outLen == 0 {
		return parsers.ParseResult{}, fmt.Errorf("coco_parse: empty result")
	}
	raw, ok := m.mod.Memory().Read(outPtr, outLen)
	if !ok {
		return parsers.ParseResult{}, fmt.Errorf("coco_parse: cannot read result")
	}
	buf := append([]byte(nil), raw...)
	defer m.free(outPtr, outLen)

	var out parsers.ParseResult
	if err := json.Unmarshal(buf, &out); err != nil {
		return parsers.ParseResult{}, fmt.Errorf("coco_parse json: %w", err)
	}
	return out, nil
}

func (m *Module) alloc(size uint32) (uint32, error) {
	if size == 0 {
		size = 1
	}
	fn := m.mod.ExportedFunction("coco_alloc")
	res, err := fn.Call(m.ctx, uint64(size))
	if err != nil {
		return 0, err
	}
	ptr := uint32(res[0])
	if ptr == 0 {
		return 0, fmt.Errorf("coco_alloc returned null")
	}
	return ptr, nil
}

func (m *Module) free(ptr, size uint32) {
	fn := m.mod.ExportedFunction("coco_free")
	_, _ = fn.Call(m.ctx, uint64(ptr), uint64(size))
}

// Close releases the wasm runtime.
func (m *Module) Close(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var err error
	if m.mod != nil {
		err = m.mod.Close(ctx)
		m.mod = nil
	}
	if m.code != nil {
		_ = m.code.Close(ctx)
		m.code = nil
	}
	if m.rt != nil {
		_ = m.rt.Close(ctx)
		m.rt = nil
	}
	return err
}

// PackPtrLen packs ptr/len into i64 (hi=ptr, lo=len).
func PackPtrLen(ptr, length uint32) uint64 {
	return (uint64(ptr) << 32) | uint64(length)
}
