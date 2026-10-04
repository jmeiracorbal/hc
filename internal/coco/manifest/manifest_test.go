package manifest

import "testing"

func TestValidateOK(t *testing.T) {
	m := &Manifest{
		ID: "hc/java", Language: "java", Extensions: []string{".java"},
		Priority: 10, Contract: "wasm/v1", Platforms: []string{"wasm32/wasi"},
		HCVersion: HCVersion{Version: "0.1.0", Constraint: "min"},
	}
	if err := m.Validate("v0.1.2", "wasm32/wasi"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsBadID(t *testing.T) {
	m := &Manifest{
		ID: "java", Language: "java", Extensions: []string{".java"},
		Contract: "wasm/v1", Platforms: []string{"wasm32/wasi"},
		HCVersion: HCVersion{Version: "0.1.0", Constraint: "min"},
	}
	if err := m.Validate("v0.1.2", "wasm32/wasi"); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateDevBinary(t *testing.T) {
	m := &Manifest{
		ID: "hc/java", Language: "java", Extensions: []string{".java"},
		Contract: "wasm/v1", Platforms: []string{"wasm32/wasi"},
		HCVersion: HCVersion{Version: "0.0.0-dev", Constraint: "min"},
	}
	if err := m.Validate("dev", "wasm32/wasi"); err != nil {
		t.Fatal(err)
	}
}
