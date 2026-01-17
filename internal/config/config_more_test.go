package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hybrowse/hyrouter/internal/routing"
)

func TestValidateRouting(t *testing.T) {
	cfg := Default()
	cfg.Routing.Default = nil
	cfg.Routing.Routes = nil
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	cfg.Routing.Default = &routing.Pool{Strategy: "round_robin", Backends: []routing.Backend{{Host: "", Port: 1}}}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error")
	}

	cfg.Routing.Default = &routing.Pool{Strategy: "round_robin", Backends: []routing.Backend{{Host: "example", Port: 0}}}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestLoadRejectsBadTLSConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	b := []byte("listen: ':5520'\ntls:\n  cert_file: foo\n")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatalf("expected error")
	}
}

func TestValidateErrors(t *testing.T) {
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
	cfg.QUIC.MaxIdleTimeout = "notaduration"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestValidateDiscoveryRefs(t *testing.T) {
	cfg := Default()
	cfg.Routing.Default = &routing.Pool{
		Strategy:  "round_robin",
		Discovery: &routing.Discovery{Provider: "missing"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error")
	}

	cfg = Default()
	cfg.Discovery = &DiscoveryConfig{Providers: []DiscoveryProviderConfig{{Name: "p", Type: "kubernetes", Kubernetes: &KubernetesDiscoveryConfig{}}}}
	cfg.Routing.Default = &routing.Pool{
		Strategy:  "round_robin",
		Discovery: &routing.Discovery{Provider: "p"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateDiscoveryAgonesAllocateMinIntervalInvalid(t *testing.T) {
	cfg := Default()
	cfg.Discovery = &DiscoveryConfig{Providers: []DiscoveryProviderConfig{{
		Name:   "a",
		Type:   "agones",
		Agones: &AgonesDiscoveryConfig{AllocateMinInterval: "nope"},
	}}}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error")
	}
}
