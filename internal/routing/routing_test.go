package routing

import (
	"context"
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
		Default: &Target{Host: "default.example", Port: 1111},
		Routes: []Route{
			{
				Match:  Match{Hostnames: []string{"localhost"}},
				Target: Target{Host: "matched.example", Port: 2222},
			},
			{
				Match:  Match{Hostname: "api.example"},
				Target: Target{Host: "api-target", Port: 3333},
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
	if dec.Target.Host != "matched.example" || dec.Target.Port != 2222 {
		t.Fatalf("unexpected target: %#v", dec.Target)
	}

	dec, err = e.Decide(context.Background(), Request{SNI: "unknown"})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if dec.Matched {
		t.Fatalf("expected default (not matched): %#v", dec)
	}
	if dec.Target.Host != "default.example" || dec.Target.Port != 1111 {
		t.Fatalf("unexpected target: %#v", dec.Target)
	}

	dec, err = e.Decide(context.Background(), Request{SNI: "api.example"})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if !dec.Matched || dec.Target.Host != "api-target" {
		t.Fatalf("unexpected: %#v", dec)
	}

	e2 := NewStaticEngine(Config{})
	dec, err = e2.Decide(context.Background(), Request{SNI: "x"})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if dec.Target.Host != "" {
		t.Fatalf("expected empty decision")
	}
}

func TestConfigValidate(t *testing.T) {
	cfg := Config{Default: &Target{Host: "", Port: 1}}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error")
	}

	cfg = Config{Routes: []Route{{Target: Target{Host: "x", Port: 0}, Match: Match{Hostname: "a"}}}}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error")
	}

	cfg = Config{Routes: []Route{{Target: Target{Host: "x", Port: 1}}}}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error")
	}
}
