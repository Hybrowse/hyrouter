package routing

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func (e *StaticEngine) resolveCandidates(ctx context.Context, pool Pool) ([]Backend, error) {
	strategy := normalizeStrategy(pool.Strategy)
	static := pool.Backends

	if pool.Discovery == nil {
		if len(static) == 0 {
			return nil, fmt.Errorf("%w", ErrNoBackends)
		}
		return static, nil
	}

	if e.discovery == nil {
		return nil, fmt.Errorf("%w", ErrDiscoveryNotSet)
	}

	provider := strings.TrimSpace(pool.Discovery.Provider)
	disc, err := e.discovery(ctx, provider)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDiscovery, err)
	}

	mode := normalizeStrategy(pool.Discovery.Mode)
	if mode == "" {
		mode = "union"
	}
	var merged []Backend
	switch mode {
	case "prefer":
		if len(disc) > 0 {
			merged = disc
		} else {
			merged = static
		}
	case "union":
		merged = append(append([]Backend(nil), disc...), static...)
	default:
		return nil, fmt.Errorf("%w %q", ErrInvalidDiscoveryMode, pool.Discovery.Mode)
	}

	merged = dedupeBackends(merged)
	if strategy == "weighted" {
		for i := range merged {
			if merged[i].Weight <= 0 {
				merged[i].Weight = 1
			}
		}
	}
	if len(merged) == 0 {
		return nil, fmt.Errorf("%w", ErrNoBackends)
	}
	return merged, nil
}

func dedupeBackends(in []Backend) []Backend {
	seen := map[string]struct{}{}
	out := make([]Backend, 0, len(in))
	for _, b := range in {
		k := b.Host + ":" + strconv.Itoa(b.Port)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, b)
	}
	return out
}
