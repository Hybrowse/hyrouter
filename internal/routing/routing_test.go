package routing

import (
	"context"
	"math/rand"
	"testing"
)

func TestHostnameMatches(t *testing.T) {
	cases := []struct {
		pattern  string
		hostname string
		want     bool
	}{
		{pattern: "localhost", hostname: "localhost", want: true},
		{pattern: "LOCALHOST", hostname: "localhost", want: true},
		{pattern: "localhost.", hostname: "localhost", want: true},
		{pattern: "*.example.com", hostname: "a.example.com", want: true},
		{pattern: "*.example.com", hostname: "example.com", want: false},
		{pattern: "*.example.com", hostname: "a.b.example.com", want: true},
		{pattern: "", hostname: "localhost", want: false},
		{pattern: "localhost", hostname: "", want: false},
	}
	for _, tc := range cases {
		got := hostnameMatches(tc.pattern, tc.hostname)
		if got != tc.want {
			t.Fatalf("hostnameMatches(%q,%q)=%v want %v", tc.pattern, tc.hostname, got, tc.want)
		}
	}
}

func TestMatchPatterns(t *testing.T) {
	ps := matchPatterns(Match{Hostnames: []string{"a", "b"}})
	if len(ps) != 2 {
		t.Fatalf("len=%d", len(ps))
	}
	ps = matchPatterns(Match{Hostname: "x"})
	if len(ps) != 1 || ps[0] != "x" {
		t.Fatalf("unexpected: %#v", ps)
	}
	ps = matchPatterns(Match{})
	if ps != nil {
		t.Fatalf("expected nil")
	}
}

func TestStaticEngineDecide(t *testing.T) {
	e := NewStaticEngine(Config{
		Default: &Pool{Strategy: "round_robin", Backends: []Backend{{Host: "default.example", Port: 1111}}},
		Routes: []Route{
			{
				Match: Match{Hostnames: []string{"localhost"}},
				Pool:  Pool{Strategy: "round_robin", Backends: []Backend{{Host: "matched.example", Port: 2222}}},
			},
			{
				Match: Match{Hostname: "api.example"},
				Pool:  Pool{Strategy: "round_robin", Backends: []Backend{{Host: "api-target", Port: 3333}}},
			},
		},
	})

	dec, err := e.Decide(context.Background(), Request{SNI: "LOCALHOST"})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if !dec.Matched || dec.RouteIndex != 0 {
		t.Fatalf("unexpected decision: %#v", dec)
	}
	if dec.Backend.Host != "matched.example" || dec.Backend.Port != 2222 {
		t.Fatalf("unexpected backend: %#v", dec.Backend)
	}

	dec, err = e.Decide(context.Background(), Request{SNI: "unknown"})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if dec.Matched {
		t.Fatalf("expected default (not matched): %#v", dec)
	}
	if dec.Backend.Host != "default.example" || dec.Backend.Port != 1111 {
		t.Fatalf("unexpected backend: %#v", dec.Backend)
	}

	dec, err = e.Decide(context.Background(), Request{SNI: "api.example"})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if !dec.Matched || dec.Backend.Host != "api-target" {
		t.Fatalf("unexpected: %#v", dec)
	}

	e2 := NewStaticEngine(Config{})
	dec, err = e2.Decide(context.Background(), Request{SNI: "x"})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if dec.Backend.Host != "" {
		t.Fatalf("expected empty decision")
	}
}

func TestStaticEngineDecide_RoundRobin(t *testing.T) {
	e := NewStaticEngine(Config{Routes: []Route{{
		Match: Match{Hostname: "x"},
		Pool:  Pool{Strategy: "round_robin", Backends: []Backend{{Host: "a", Port: 1}, {Host: "b", Port: 2}}},
	}}})

	dec1, err := e.Decide(context.Background(), Request{SNI: "x"})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	dec2, err := e.Decide(context.Background(), Request{SNI: "x"})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}

	if dec1.SelectedIndex == dec2.SelectedIndex {
		t.Fatalf("expected different selected indexes: %d vs %d", dec1.SelectedIndex, dec2.SelectedIndex)
	}
}

func TestStaticEngineDecide_RandomAndWeightedSmoke(t *testing.T) {
	e := NewStaticEngine(Config{Routes: []Route{{
		Match: Match{Hostname: "x"},
		Pool:  Pool{Strategy: "random", Backends: []Backend{{Host: "a", Port: 1}, {Host: "b", Port: 2}}},
	}}})
	e.rng = rand.New(rand.NewSource(1))

	dec, err := e.Decide(context.Background(), Request{SNI: "x"})
	if err != nil {
		t.Fatalf("Decide random: %v", err)
	}
	if dec.SelectedIndex < 0 || dec.SelectedIndex >= len(dec.Candidates) {
		t.Fatalf("selected_index=%d candidates=%d", dec.SelectedIndex, len(dec.Candidates))
	}

	e2 := NewStaticEngine(Config{Routes: []Route{{
		Match: Match{Hostname: "x"},
		Pool:  Pool{Strategy: "weighted", Backends: []Backend{{Host: "a", Port: 1, Weight: 1}, {Host: "b", Port: 2, Weight: 3}}},
	}}})
	e2.rng = rand.New(rand.NewSource(1))

	dec, err = e2.Decide(context.Background(), Request{SNI: "x"})
	if err != nil {
		t.Fatalf("Decide weighted: %v", err)
	}
	if dec.SelectedIndex < 0 || dec.SelectedIndex >= len(dec.Candidates) {
		t.Fatalf("selected_index=%d candidates=%d", dec.SelectedIndex, len(dec.Candidates))
	}
}

func TestConfigValidate(t *testing.T) {
	cfg := Config{Default: &Pool{Strategy: "round_robin", Backends: []Backend{{Host: "", Port: 1}}}}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error")
	}

	cfg = Config{Routes: []Route{{Pool: Pool{Strategy: "round_robin", Backends: []Backend{{Host: "x", Port: 0}}}, Match: Match{Hostname: "a"}}}}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error")
	}

	cfg = Config{Routes: []Route{{Pool: Pool{Strategy: "round_robin", Backends: []Backend{{Host: "x", Port: 1}}}}}}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error")
	}
}
