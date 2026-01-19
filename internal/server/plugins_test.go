package server

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"

	"github.com/hybrowse/hyrouter/internal/config"
	"github.com/hybrowse/hyrouter/internal/plugins"
	"google.golang.org/grpc"
)

type testPluginServer struct{}

func (t *testPluginServer) OnConnect(context.Context, *plugins.ConnectRequest) (*plugins.ConnectResponse, error) {
	return &plugins.ConnectResponse{}, nil
}

func TestInitPlugins_NoPlugins(t *testing.T) {
	s := &Server{logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))}
	if err := s.initPlugins(context.Background()); err != nil {
		t.Fatalf("initPlugins: %v", err)
	}
}

func TestInitPlugins_AlreadyInitialized(t *testing.T) {
	s := &Server{plugins: plugins.NewManager(nil, nil)}
	if err := s.initPlugins(context.Background()); err != nil {
		t.Fatalf("initPlugins: %v", err)
	}
}

func TestInitPlugins_OrderError(t *testing.T) {
	s := &Server{logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))}
	s.pluginCfgs = []config.PluginConfig{
		{Name: "a", Type: "grpc", After: []string{"b"}, GRPC: &config.GRPCPluginConfig{Address: "x"}},
		{Name: "b", Type: "grpc", After: []string{"a"}, GRPC: &config.GRPCPluginConfig{Address: "x"}},
	}
	if err := s.initPlugins(context.Background()); err == nil {
		t.Fatalf("expected error")
	}
}

func TestInitPlugins_LoadError(t *testing.T) {
	s := &Server{logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))}
	s.pluginCfgs = []config.PluginConfig{{Name: "a", Type: "grpc"}}
	if err := s.initPlugins(context.Background()); err == nil {
		t.Fatalf("expected error")
	}
}

func TestInitPlugins_Success(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer lis.Close() // nolint:errcheck

	g := grpc.NewServer()
	plugins.RegisterGRPCServer(g, &testPluginServer{})
	go g.Serve(lis) // nolint:errcheck
	defer g.Stop()

	s := &Server{logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))}
	s.pluginCfgs = []config.PluginConfig{{Name: "p", Type: "grpc", GRPC: &config.GRPCPluginConfig{Address: lis.Addr().String()}}}
	if err := s.initPlugins(context.Background()); err != nil {
		t.Fatalf("initPlugins: %v", err)
	}
	if s.plugins == nil {
		t.Fatalf("expected plugins")
	}
	s.plugins.Close(context.Background())
}
