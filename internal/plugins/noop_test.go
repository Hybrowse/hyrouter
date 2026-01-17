package plugins

import (
	"context"
	"testing"
)

func TestNoopPlugin(t *testing.T) {
	p := NewNoopPlugin("n")
	if p.Name() != "n" {
		t.Fatalf("name=%q", p.Name())
	}
	resp, err := p.OnConnect(context.Background(), ConnectRequest{})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if resp.Deny {
		t.Fatalf("unexpected deny")
	}
	if err := p.Close(context.Background()); err != nil {
		t.Fatalf("close: %v", err)
	}
}
