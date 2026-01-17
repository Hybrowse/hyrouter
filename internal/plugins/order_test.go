package plugins

import (
	"testing"

	"github.com/hybrowse/hyrouter/internal/config"
)

func TestOrderPluginConfigs(t *testing.T) {
	cfgs := []config.PluginConfig{
		{Name: "b", Type: "grpc", Stage: "route", After: []string{"a"}, GRPC: &config.GRPCPluginConfig{Address: "x"}},
		{Name: "a", Type: "grpc", Stage: "route", GRPC: &config.GRPCPluginConfig{Address: "x"}},
		{Name: "c", Type: "grpc", Stage: "deny", GRPC: &config.GRPCPluginConfig{Address: "x"}},
	}
	out, err := OrderPluginConfigs(cfgs)
	if err != nil {
		t.Fatalf("OrderPluginConfigs: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("len=%d", len(out))
	}
	if out[0].Name != "c" {
		t.Fatalf("expected deny first, got %q", out[0].Name)
	}
	if out[1].Name != "a" || out[2].Name != "b" {
		t.Fatalf("unexpected order: %q,%q", out[1].Name, out[2].Name)
	}
}

func TestOrderPluginConfigs_NonStandardStageAndMissingDepsIgnored(t *testing.T) {
	cfgs := []config.PluginConfig{
		{Name: "z", Type: "grpc", Stage: "custom", After: []string{"missing"}, GRPC: &config.GRPCPluginConfig{Address: "x"}},
		{Name: "r", Type: "grpc", GRPC: &config.GRPCPluginConfig{Address: "x"}},
		{Name: "d", Type: "grpc", Stage: "deny", GRPC: &config.GRPCPluginConfig{Address: "x"}},
	}
	out, err := OrderPluginConfigs(cfgs)
	if err != nil {
		t.Fatalf("OrderPluginConfigs: %v", err)
	}
	if out[0].Name != "d" {
		t.Fatalf("expected deny first, got %q", out[0].Name)
	}
}

func TestOrderPluginConfigs_Cycle(t *testing.T) {
	cfgs := []config.PluginConfig{
		{Name: "a", Type: "grpc", Stage: "route", After: []string{"b"}, GRPC: &config.GRPCPluginConfig{Address: "x"}},
		{Name: "b", Type: "grpc", Stage: "route", After: []string{"a"}, GRPC: &config.GRPCPluginConfig{Address: "x"}},
	}
	if _, err := OrderPluginConfigs(cfgs); err == nil {
		t.Fatalf("expected error")
	}
}

func TestOrderPluginConfigs_BeforeConstraint(t *testing.T) {
	cfgs := []config.PluginConfig{
		{Name: "b", Type: "grpc", Stage: "route", GRPC: &config.GRPCPluginConfig{Address: "x"}},
		{Name: "a", Type: "grpc", Stage: "route", Before: []string{"b"}, GRPC: &config.GRPCPluginConfig{Address: "x"}},
	}
	out, err := OrderPluginConfigs(cfgs)
	if err != nil {
		t.Fatalf("OrderPluginConfigs: %v", err)
	}
	if out[0].Name != "a" {
		t.Fatalf("expected a first, got %q", out[0].Name)
	}
}
