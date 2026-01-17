package server

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/hybrowse/hyrouter/internal/config"
)

func TestRun_InvalidIdleTimeout(t *testing.T) {
	cfg := config.Default()
	cfg.QUIC.MaxIdleTimeout = "notaduration"

	s := New(cfg, slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{})))
	if err := s.Run(context.Background()); err == nil {
		t.Fatalf("expected error")
	}
}

func TestRun_InvalidListenAddr(t *testing.T) {
	cfg := config.Default()
	cfg.Listen = "not-an-addr"
	s := New(cfg, slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{})))
	if err := s.Run(context.Background()); err == nil {
		t.Fatalf("expected error")
	}
}

func TestRun_ContextCanceledReturnsNil(t *testing.T) {
	cfg := config.Default()
	cfg.Listen = "127.0.0.1:0"
	s := New(cfg, slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{})))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Run(ctx); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}
