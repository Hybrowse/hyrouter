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
	out := m.ApplyOnConnect(context.Background(), ConnectEvent{}, routing.Decision{Backend: routing.Backend{Host: "h", Port: 1}}, []byte("x"))
	if out.Backend.Host != "h" || string(out.ReferralContent) != "x" {
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
		&testPlugin{name: "a", resp: ConnectResponse{ReferralContent: []byte("x")}},
		&testPlugin{name: "b", resp: ConnectResponse{Backend: &routing.Backend{Host: "h", Port: 1}}},
	})

	out := m.ApplyOnConnect(context.Background(), ConnectEvent{SNI: "localhost"}, routing.Decision{Backend: routing.Backend{Host: "d", Port: 2}}, nil)
	if out.Denied {
		t.Fatalf("unexpected deny")
	}
	if out.Backend.Host != "h" || out.Backend.Port != 1 {
		t.Fatalf("backend=%#v", out.Backend)
	}
	if string(out.ReferralContent) != "x" {
		t.Fatalf("content=%q", string(out.ReferralContent))
	}
}

func TestManagerApplyOnConnect_Deny(t *testing.T) {
	m := NewManager(nil, []Plugin{&testPlugin{name: "a", resp: ConnectResponse{Deny: true, DenyReason: "no"}}})
	out := m.ApplyOnConnect(context.Background(), ConnectEvent{}, routing.Decision{}, nil)
	if !out.Denied || out.DenyReason != "no" {
		t.Fatalf("out=%#v", out)
	}
}

func TestManagerApplyOnConnect_PluginError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	m := NewManager(logger, []Plugin{&testPlugin{name: "a", err: context.Canceled}})
	out := m.ApplyOnConnect(context.Background(), ConnectEvent{}, routing.Decision{}, nil)
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
