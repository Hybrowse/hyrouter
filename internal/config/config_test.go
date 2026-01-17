package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hybrowse/hyrouter/internal/routing"
)

func TestLoadYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("listen: ':5520'\ntls:\n  alpn:\n    - hytale/1\nrouting:\n  default:\n    host: play.hyvane.com\n    port: 5520\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Listen != ":5520" {
		t.Fatalf("unexpected listen: %q", cfg.Listen)
	}
	if len(cfg.TLS.ALPN) != 1 || cfg.TLS.ALPN[0] != "hytale/1" {
		t.Fatalf("unexpected alpn: %#v", cfg.TLS.ALPN)
	}
	if cfg.Routing.Default == nil || cfg.Routing.Default.Host != "play.hyvane.com" {
		t.Fatalf("unexpected routing: %#v", cfg.Routing)
	}
}

func TestLoadJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"listen":":5520","tls":{"alpn":["hytale/1"]},"routing":{"default":{"host":"play.hyvane.com","port":5520}}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Listen != ":5520" {
		t.Fatalf("unexpected listen: %q", cfg.Listen)
	}
	if len(cfg.TLS.ALPN) != 1 || cfg.TLS.ALPN[0] != "hytale/1" {
		t.Fatalf("unexpected alpn: %#v", cfg.TLS.ALPN)
	}
}

func TestLoadUnsupportedExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.txt")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatalf("expected error")
	}
}

func TestValidate_Errors(t *testing.T) {
	cfg := Default()
	cfg.Listen = ""
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error")
	}

	cfg = Default()
	cfg.TLS.CertFile = "x"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error")
	}

	cfg = Default()
	cfg.TLS.ALPN = nil
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error")
	}

	cfg = Default()
	cfg.QUIC.MaxIdleTimeout = "1s"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}

	cfg = Default()
	cfg.Routing = routing.Config{Default: &routing.Target{Host: "", Port: 1}}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error")
	}

	cfg = Default()
	cfg.Plugins = []PluginConfig{{Name: "a", Type: "grpc", GRPC: &GRPCPluginConfig{Address: "127.0.0.1:1"}}, {Name: "a", Type: "grpc", GRPC: &GRPCPluginConfig{Address: "127.0.0.1:1"}}}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error")
	}

	cfg = Default()
	cfg.Plugins = []PluginConfig{{Name: "a", Type: "grpc", Stage: "nope", GRPC: &GRPCPluginConfig{Address: "127.0.0.1:1"}}}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error")
	}

	cfg = Default()
	cfg.Plugins = []PluginConfig{{Name: "a", Type: "grpc"}}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error")
	}

	cfg = Default()
	cfg.Plugins = []PluginConfig{{Name: "a", Type: "wasm"}}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error")
	}

	cfg = Default()
	cfg.Plugins = []PluginConfig{{Name: "a", Type: "nope"}}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestValidate_ValidWithPlugins(t *testing.T) {
	cfg := Default()
	cfg.Listen = ":1"
	cfg.QUIC.MaxIdleTimeout = (10 * time.Second).String()
	cfg.Plugins = []PluginConfig{{Name: "a", Type: "grpc", Stage: strings.ToUpper("deny"), GRPC: &GRPCPluginConfig{Address: "127.0.0.1:1"}}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestLoadInvalidMaxIdleTimeout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	b := []byte("listen: ':5520'\ntls:\n  alpn:\n    - hytale/1\nquic:\n  max_idle_timeout: notaduration\n")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatalf("expected error")
	}
}
