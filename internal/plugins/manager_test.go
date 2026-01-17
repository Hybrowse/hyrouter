package plugins

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/hybrowse/hyrouter/internal/routing"
)

type testPlugin struct {
	name   string
	resp   ConnectResponse
	err    error
	closed bool
}

func TestManagerNil(t *testing.T) {
	var m *Manager
	out := m.ApplyOnConnect(context.Background(), ConnectEvent{}, routing.Target{Host: "h", Port: 1}, []byte("x"))
	if out.Target.Host != "h" || string(out.ReferralData) != "x" {
		t.Fatalf("out=%#v", out)
	}
	m.Close(context.Background())
}

func (p *testPlugin) Name() string { return p.name }

func (p *testPlugin) OnConnect(ctx context.Context, req ConnectRequest) (ConnectResponse, error) {
	return p.resp, p.err
}

func (p *testPlugin) Close(ctx context.Context) error {
	p.closed = true
	return nil
}

func TestManagerApplyOnConnect(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	m := NewManager(logger, []Plugin{
		&testPlugin{name: "a", resp: ConnectResponse{ReferralData: []byte("x")}},
		&testPlugin{name: "b", resp: ConnectResponse{Target: &routing.Target{Host: "h", Port: 1}}},
	})

	out := m.ApplyOnConnect(context.Background(), ConnectEvent{SNI: "localhost"}, routing.Target{Host: "d", Port: 2}, nil)
	if out.Denied {
		t.Fatalf("unexpected deny")
	}
	if out.Target.Host != "h" || out.Target.Port != 1 {
		t.Fatalf("target=%#v", out.Target)
	}
	if string(out.ReferralData) != "x" {
		t.Fatalf("data=%q", string(out.ReferralData))
	}
}

func TestManagerApplyOnConnect_Deny(t *testing.T) {
	m := NewManager(nil, []Plugin{&testPlugin{name: "a", resp: ConnectResponse{Deny: true, DenyReason: "no"}}})
	out := m.ApplyOnConnect(context.Background(), ConnectEvent{}, routing.Target{}, nil)
	if !out.Denied || out.DenyReason != "no" {
		t.Fatalf("out=%#v", out)
	}
}

func TestManagerApplyOnConnect_PluginError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	m := NewManager(logger, []Plugin{&testPlugin{name: "a", err: context.Canceled}})
	out := m.ApplyOnConnect(context.Background(), ConnectEvent{}, routing.Target{}, nil)
	if out.Denied {
		t.Fatalf("unexpected")
	}
}

func TestManagerClose(t *testing.T) {
	p := &testPlugin{name: "a"}
	m := NewManager(nil, []Plugin{p})
	m.Close(context.Background())
	if !p.closed {
		t.Fatalf("expected closed")
	}
}
