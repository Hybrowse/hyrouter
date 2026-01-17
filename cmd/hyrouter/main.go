package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/hybrowse/hyrouter/internal/config"
	"github.com/hybrowse/hyrouter/internal/server"
)

var runServer = func(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	srv := server.New(cfg, logger)
	return srv.Run(ctx)
}

var osExit = os.Exit

func main() {
	osExit(mainMain(os.Args[1:]))
}

func mainMain(args []string) int {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx, args); err != nil {
		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))
		logger.Error("hyrouter failed", "error", err)
		return 1
	}
	return 0
}

func run(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("hyrouter", flag.ContinueOnError)
	configPath := fs.String("config", "config.yaml", "Path to config file")
	logLevel := fs.String("log-level", "info", "Log level (debug|info|warn|error)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	lvl, err := parseLogLevel(*logLevel)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	return runServer(ctx, cfg, logger)
}

func parseLogLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid log level %q", s)
	}
}
