package plugins

import (
	"context"
	"net"
	"testing"

	"github.com/hybrowse/hyrouter/internal/config"
	"google.golang.org/grpc"
)

type testGRPCServer struct{}

func TestGRPCPlugin_ConfigError(t *testing.T) {
	if _, err := newGRPCPlugin(context.Background(), config.PluginConfig{Name: "p", Type: "grpc"}, nil); err == nil {
		t.Fatalf("expected error")
	}
}

func (s *testGRPCServer) OnConnect(_ context.Context, req *ConnectRequest) (*ConnectResponse, error) {
	resp := &ConnectResponse{}
	if req.Event.Username == "deny" {
		resp.Deny = true
		resp.DenyReason = "no"
		return resp, nil
	}
	resp.ReferralData = []byte("x")
	return resp, nil
}

func TestGRPCPlugin_OnConnect(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer lis.Close() // nolint:errcheck

	s := grpc.NewServer()
	RegisterGRPCServer(s, &testGRPCServer{})
	go s.Serve(lis) // nolint:errcheck
	defer s.Stop()

	p, err := newGRPCPlugin(context.Background(), config.PluginConfig{Name: "p", Type: "grpc", GRPC: &config.GRPCPluginConfig{Address: lis.Addr().String()}}, nil)
	if err != nil {
		t.Fatalf("newGRPCPlugin: %v", err)
	}
	defer p.Close(context.Background()) // nolint:errcheck
	if p.Name() != "p" {
		t.Fatalf("name=%q", p.Name())
	}

	resp, err := p.OnConnect(context.Background(), ConnectRequest{Event: ConnectEvent{Username: "ok"}})
	if err != nil {
		t.Fatalf("OnConnect: %v", err)
	}
	if string(resp.ReferralData) != "x" {
		t.Fatalf("ref=%q", string(resp.ReferralData))
	}

	resp, err = p.OnConnect(context.Background(), ConnectRequest{Event: ConnectEvent{Username: "deny"}})
	if err != nil {
		t.Fatalf("OnConnect deny: %v", err)
	}
	if !resp.Deny || resp.DenyReason != "no" {
		t.Fatalf("resp=%#v", resp)
	}

	s.Stop()
	if _, err := p.OnConnect(context.Background(), ConnectRequest{Event: ConnectEvent{Username: "ok"}}); err == nil {
		t.Fatalf("expected error")
	}
}
