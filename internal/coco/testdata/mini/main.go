//go:build wasip1

// mini wasm/v1 guest for host/install tests (not a product coco).
package main

import (
	"encoding/json"
	"unsafe"
)

const abiVersion = 1

var heap = make([]byte, 4*1024*1024)
var heapOff int = 64

type symbol struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	LineStart  int    `json:"line_start"`
	LineEnd    int    `json:"line_end"`
	Signature  string `json:"signature,omitempty"`
	ParentName string `json:"parent_name,omitempty"`
}

type ref struct {
	Kind     string `json:"kind"`
	FromName string `json:"from_name,omitempty"`
	ToName   string `json:"to_name"`
	Line     int    `json:"line"`
}

type parseResult struct {
	Symbols []symbol `json:"symbols"`
	Refs    []ref    `json:"refs"`
	Error   string   `json:"error,omitempty"`
}

//go:wasmexport coco_abi_version
func coco_abi_version() int32 { return abiVersion }

//go:wasmexport coco_alloc
func coco_alloc(size int32) int32 {
	if size <= 0 {
		return 0
	}
	if heapOff+int(size) > len(heap) {
		return 0
	}
	ptr := int32(uintptr(unsafe.Pointer(&heap[heapOff])))
	heapOff += int(size)
	return ptr
}

//go:wasmexport coco_free
func coco_free(ptr, size int32) {
	_ = ptr
	_ = size
}

//go:wasmexport coco_parse
func coco_parse(pathPtr, pathLen, srcPtr, srcLen int32) int64 {
	_ = pathPtr
	_ = pathLen
	_ = srcPtr
	_ = srcLen
	res := parseResult{
		Symbols: []symbol{
			{Name: "Probe", Kind: "class", LineStart: 1, LineEnd: 3},
			{Name: "run", Kind: "method", LineStart: 2, LineEnd: 2, ParentName: "Probe"},
		},
		Refs: []ref{
			{Kind: "contains", FromName: "Probe", ToName: "run", Line: 2},
		},
	}
	raw, err := json.Marshal(res)
	if err != nil {
		raw = []byte(`{"symbols":[],"refs":[],"error":"marshal failed"}`)
	}
	outPtr := coco_alloc(int32(len(raw)))
	if outPtr == 0 {
		return 0
	}
	out := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(outPtr))), len(raw))
	copy(out, raw)
	return (int64(outPtr) << 32) | int64(uint32(len(raw)))
}

func main() {}
