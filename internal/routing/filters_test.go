package routing

import (
	"context"
	"strconv"
	"testing"
	"time"
)

func TestFilters_Whitelist(t *testing.T) {
	b := Backend{Host: "a", Port: 1, Meta: map[string]string{
		"annotation.agones.dev/sdk-whitelist-enabled": "true",
		"list.whitelistedPlayers.values":              "[\"u1\",\"u2\"]",
	}}

	f := Filter{Type: "whitelist", EnabledKey: "annotation.agones.dev/sdk-whitelist-enabled", ListKey: "list.whitelistedPlayers.values"}

	if !filterMatches(Request{UUID: "u1"}, b, f) {
		t.Fatalf("expected whitelisted user to match")
	}
	if filterMatches(Request{UUID: "u3"}, b, f) {
		t.Fatalf("expected non-whitelisted user to be rejected")
	}
}

func TestFilters_Whitelist_SubjectUsername(t *testing.T) {
	b := Backend{Host: "a", Port: 1, Meta: map[string]string{
		"annotation.agones.dev/sdk-whitelist-enabled": "true",
		"list.whitelistedPlayers.values":              "[\"alice\",\"bob\"]",
	}}

	f := Filter{Type: "whitelist", Subject: "username", EnabledKey: "annotation.agones.dev/sdk-whitelist-enabled", ListKey: "list.whitelistedPlayers.values"}

	if !filterMatches(Request{Username: "alice"}, b, f) {
		t.Fatalf("expected whitelisted username to match")
	}
	if filterMatches(Request{Username: "carol"}, b, f) {
		t.Fatalf("expected non-whitelisted username to be rejected")
	}
}

func TestFilters_GameStartNotPast(t *testing.T) {
	now := time.Now().UnixMilli()

	bFuture := Backend{Host: "a", Port: 1, Meta: map[string]string{"annotation.agones.dev/sdk-game-start": ""}}
	f := Filter{Type: "game_start_not_past", Key: "annotation.agones.dev/sdk-game-start"}
	if !filterMatches(Request{}, bFuture, f) {
		t.Fatalf("expected empty game start to pass")
	}

	bPast := Backend{Host: "b", Port: 2, Meta: map[string]string{"annotation.agones.dev/sdk-game-start": strconv.FormatInt(now-1000, 10)}}
	if filterMatches(Request{}, bPast, f) {
		t.Fatalf("expected past game start to be rejected")
	}
}

func TestEngine_FallbackWhenFiltersEliminateAll(t *testing.T) {
	e := NewStaticEngine(Config{Routes: []Route{{
		Match: Match{Hostname: "x"},
		Pool: Pool{
			Strategy: "round_robin",
			Filters:  []Filter{{Type: "compare", Left: "counter:players.count", Op: "lt", Right: "counter:players.capacity"}},
			Fallback: []Fallback{{Filters: []Filter{}}},
			Backends: []Backend{{Host: "a", Port: 1, Meta: map[string]string{"counter.players.count": "10", "counter.players.capacity": "1"}}},
		},
	}}})

	dec, err := e.Decide(context.Background(), Request{SNI: "x"})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if dec.Backend.Host != "a" {
		t.Fatalf("expected fallback to allow backend, got %#v", dec.Backend)
	}
}
