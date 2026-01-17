package routing

import (
	"context"
	"testing"
)

func TestStaticEngineDecide_LeastLoaded(t *testing.T) {
	e := NewStaticEngine(Config{Routes: []Route{{
		Match: Match{Hostname: "x"},
		Pool: Pool{
			Strategy: "least_loaded",
			Key:      "counter:players.count",
			Backends: []Backend{
				{Host: "a", Port: 1, Meta: map[string]string{"counter.players.count": "10"}},
				{Host: "b", Port: 2, Meta: map[string]string{"counter.players.count": "3"}},
				{Host: "c", Port: 3, Meta: map[string]string{"counter.players.count": "7"}},
			},
		},
	}}})

	dec, err := e.Decide(context.Background(), Request{SNI: "x"})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if dec.Backend.Host != "b" {
		t.Fatalf("expected least loaded backend b, got %#v", dec.Backend)
	}
}

func TestStaticEngineDecide_P2C(t *testing.T) {
	e := NewStaticEngine(Config{Routes: []Route{{
		Match: Match{Hostname: "x"},
		Pool: Pool{
			Strategy: "p2c",
			Key:      "counter:players.count",
			Sample:   2,
			Backends: []Backend{
				{Host: "a", Port: 1, Meta: map[string]string{"counter.players.count": "10"}},
				{Host: "b", Port: 2, Meta: map[string]string{"counter.players.count": "3"}},
				{Host: "c", Port: 3, Meta: map[string]string{"counter.players.count": "7"}},
			},
		},
	}}})

	// Make deterministic.
	e.rngMu.Lock()
	e.rng.Seed(1)
	e.rngMu.Unlock()

	dec, err := e.Decide(context.Background(), Request{SNI: "x"})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if dec.SelectedIndex < 0 || dec.SelectedIndex >= len(dec.Candidates) {
		t.Fatalf("selected_index=%d candidates=%d", dec.SelectedIndex, len(dec.Candidates))
	}
}
