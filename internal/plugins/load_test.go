package plugins

import (
	"context"
	"net"
	"testing"

	"github.com/hybrowse/hyrouter/internal/config"
	"google.golang.org/grpc"
)

func TestLoadAll_UnknownType(t *testing.T) {
	_, err := LoadAll(context.Background(), []config.PluginConfig{{Name: "a", Type: "nope"}}, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestLoadAll_WASM_Error(t *testing.T) {
	_, err := LoadAll(context.Background(), []config.PluginConfig{{Name: "w", Type: "wasm", WASM: &config.WASMPluginConfig{Path: "missing.wasm"}}}, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestLoadAll_GRPC(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer lis.Close() // nolint:errcheck

	s := grpc.NewServer()
	RegisterGRPCServer(s, &testGRPCServer{})
	go s.Serve(lis) // nolint:errcheck
	defer s.Stop()

	pls, err := LoadAll(context.Background(), []config.PluginConfig{{Name: "p", Type: "grpc", GRPC: &config.GRPCPluginConfig{Address: lis.Addr().String()}}}, nil)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(pls) != 1 {
		t.Fatalf("len=%d", len(pls))
	}
	_ = pls[0].Close(context.Background())
}
