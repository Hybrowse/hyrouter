package config

import (
	"path/filepath"
	"testing"
)

func TestDevExampleConfigsLoadAndValidate(t *testing.T) {
	devDir := filepath.Join("..", "..", "dev")
	paths, err := filepath.Glob(filepath.Join(devDir, "*.dev.yaml"))
	if err != nil {
		t.Fatalf("glob dev configs: %v", err)
	}
	if len(paths) == 0 {
		t.Fatalf("expected dev example configs under %q", devDir)
	}
	for _, p := range paths {
		p := p
		t.Run(filepath.Base(p), func(t *testing.T) {
			if _, err := Load(p); err != nil {
				t.Fatalf("Load(%q): %v", p, err)
			}
		})
	}
}
