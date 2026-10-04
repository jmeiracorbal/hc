package coco

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPins(t *testing.T) {
	dir := t.TempDir()
	raw := []byte(`
cocos = ["acme/java-springboot"]
[extension_pins]
".java" = "acme/java-springboot"
`)
	if err := os.WriteFile(filepath.Join(dir, ProjectConfigFile), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	pins, err := LoadPins(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(pins.Cocos) != 1 || pins.Cocos[0] != "acme/java-springboot" {
		t.Fatalf("%+v", pins.Cocos)
	}
	if pins.ExtensionPins[".java"] != "acme/java-springboot" {
		t.Fatalf("%+v", pins.ExtensionPins)
	}
}

func TestLoadPinsMissing(t *testing.T) {
	pins, err := LoadPins(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(pins.Cocos) != 0 {
		t.Fatalf("%+v", pins)
	}
}
