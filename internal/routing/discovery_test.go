package routing

import (
	"context"
	"errors"
	"testing"
)

func TestResolveCandidates_DiscoveryNotSet(t *testing.T) {
	e := NewStaticEngine(Config{Routes: []Route{{
		Match: Match{Hostname: "x"},
		Pool:  Pool{Strategy: "round_robin", Backends: []Backend{{Host: "a", Port: 1}}, Discovery: &Discovery{Provider: "p", Mode: "union"}},
	}}})
	_, err := e.Decide(context.Background(), Request{SNI: "x"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, ErrDiscoveryNotSet) {
		t.Fatalf("expected ErrDiscoveryNotSet, got %v", err)
	}
}
