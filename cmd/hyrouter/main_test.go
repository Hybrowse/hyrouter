package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/hybrowse/hyrouter/internal/config"
)

func TestRunLoadsConfigAndInvokesServer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	b := []byte("listen: ':5520'\ntls:\n  alpn:\n    - hytale/1\nrouting:\n  default:\n    host: play.hyvane.com\n    port: 5520\n")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	called := false
	prev := runServer
	runServer = func(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
		_ = logger
		called = true
		if cfg.Listen != ":5520" {
			return context.Canceled
		}
		return nil
	}
	defer func() { runServer = prev }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := run(ctx, []string{"-config", path}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !called {
		t.Fatalf("expected server to be called")
	}
}

func TestRunRejectsBadArgs(t *testing.T) {
	ctx := context.Background()
	if err := run(ctx, []string{"-unknown"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestMainMain_ReturnCodes(t *testing.T) {
	prevRun := runServer
	prevExit := osExit
	defer func() {
		runServer = prevRun
		osExit = prevExit
	}()

	runServer = func(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
		_ = logger
		return nil
	}
	osExit = func(code int) {}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	b := []byte("listen: ':5520'\ntls:\n  alpn:\n    - hytale/1\nrouting:\n  default:\n    host: play.hyvane.com\n    port: 5520\n")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	code := mainMain([]string{"-config", path})
	if code != 0 {
		t.Fatalf("code=%d", code)
	}

	code = mainMain([]string{"-config", filepath.Join(dir, "missing.yaml")})
	if code == 0 {
		t.Fatalf("expected non-zero")
	}
}

func TestMain_CallsExit(t *testing.T) {
	prevRun := runServer
	prevExit := osExit
	prevArgs := os.Args
	defer func() {
		runServer = prevRun
		osExit = prevExit
		os.Args = prevArgs
	}()

	runServer = func(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
		_ = logger
		return nil
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	b := []byte("listen: ':5520'\ntls:\n  alpn:\n    - hytale/1\nrouting:\n  default:\n    host: play.hyvane.com\n    port: 5520\n")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	code := -1
	osExit = func(c int) { code = c }
	os.Args = []string{"hyrouter", "-config", path}

	main()
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
}

func TestMain_CallsExitOnError(t *testing.T) {
	prevRun := runServer
	prevExit := osExit
	prevArgs := os.Args
	defer func() {
		runServer = prevRun
		osExit = prevExit
		os.Args = prevArgs
	}()

	runServer = func(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
		_ = logger
		return nil
	}

	code := -1
	osExit = func(c int) { code = c }
	os.Args = []string{"hyrouter", "-config", "missing.yaml"}

	main()
	if code == 0 {
		t.Fatalf("expected non-zero")
	}
}
