package plugins

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/hybrowse/hyrouter/internal/config"
)

func LoadAll(ctx context.Context, cfgs []config.PluginConfig, logger *slog.Logger) ([]Plugin, error) {
	out := make([]Plugin, 0, len(cfgs))
	for _, c := range cfgs {
		switch strings.ToLower(c.Type) {
		case "grpc":
			p, err := newGRPCPlugin(ctx, c, logger)
			if err != nil {
				return nil, err
			}
			out = append(out, p)
		case "wasm":
			p, err := newWASMPlugin(ctx, c, logger)
			if err != nil {
				return nil, err
			}
			out = append(out, p)
		default:
			return nil, fmt.Errorf("unknown plugin type: %q", c.Type)
		}
	}
	return out, nil
}
