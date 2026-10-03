package assets

import (
	"embed"
	"fmt"
)

//go:embed hybrid-coco.md hooks/*
var fsData embed.FS

func AwarenessMD() ([]byte, error) {
	return fsData.ReadFile("hybrid-coco.md")
}

func Hook(name string) ([]byte, error) {
	data, err := fsData.ReadFile("hooks/" + name)
	if err != nil {
		return nil, fmt.Errorf("hook %s: %w", name, err)
	}
	return data, nil
}
