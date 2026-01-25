package routing

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync/atomic"
)

func (e *StaticEngine) Decide(ctx context.Context, req Request) (Decision, error) {
	sni := canonicalHost(req.SNI)

	for i, r := range e.cfg.Routes {
		patterns := matchPatterns(r.Match)
		for _, p := range patterns {
			if hostnameMatches(p, sni) {
				cands, err := e.resolveCandidates(ctx, r.Pool)
				if err != nil {
					return Decision{}, err
				}
				cands, idx, err := e.selectCandidates(req, r.Pool, cands, &e.rr[i])
				if err != nil {
					return Decision{}, err
				}
				b := Backend{}
				if idx >= 0 && idx < len(cands) {
					b = cands[idx]
				}
				return Decision{
					Backend:       b,
					Candidates:    cands,
					SelectedIndex: idx,
					Strategy:      normalizeStrategy(r.Pool.Strategy),
					Matched:       true,
					RouteIndex:    i,
				}, nil
			}
		}
	}

	if e.cfg.Default != nil {
		cands, err := e.resolveCandidates(ctx, *e.cfg.Default)
		if err != nil {
			return Decision{}, err
		}
		cands, idx, err := e.selectCandidates(req, *e.cfg.Default, cands, &e.rrDefault)
		if err != nil {
			return Decision{}, err
		}
		b := Backend{}
		if idx >= 0 && idx < len(cands) {
			b = cands[idx]
		}
		return Decision{
			Backend:       b,
			Candidates:    cands,
			SelectedIndex: idx,
			Strategy:      normalizeStrategy(e.cfg.Default.Strategy),
			Matched:       false,
			RouteIndex:    -1,
		}, nil
	}

	return Decision{Matched: false, RouteIndex: -1, SelectedIndex: -1}, nil
}

func (e *StaticEngine) selectCandidates(req Request, pool Pool, backends []Backend, rr *atomic.Uint64) ([]Backend, int, error) {
	strategy := normalizeStrategy(pool.Strategy)
	if len(backends) == 0 {
		return nil, -1, fmt.Errorf("%w", ErrNoBackends)
	}
	base := selectionConfigFromPool(pool)
	selected, idx, err := e.selectWithConfig(req, base, backends, rr)
	if err == nil {
		return selected, idx, nil
	}
	if !errors.Is(err, ErrNoBackends) {
		return nil, -1, err
	}
	_ = strategy
	for _, fb := range pool.Fallback {
		cfg := base
		mergeFallback(&cfg, fb)
		selected, idx, err = e.selectWithConfig(req, cfg, backends, rr)
		if err == nil {
			return selected, idx, nil
		}
		if !errors.Is(err, ErrNoBackends) {
			return nil, -1, err
		}
	}
	return nil, -1, fmt.Errorf("%w", ErrNoBackends)
}

type selectionConfig struct {
	Strategy string
	Key      string
	Sample   int
	Sort     []SortKey
	Limit    int
	Filters  []Filter
}

func selectionConfigFromPool(p Pool) selectionConfig {
	return selectionConfig{
		Strategy: normalizeStrategy(p.Strategy),
		Key:      strings.TrimSpace(p.Key),
		Sample:   p.Sample,
		Sort:     p.Sort,
		Limit:    p.Limit,
		Filters:  p.Filters,
	}
}

func mergeFallback(dst *selectionConfig, fb Fallback) {
	if dst == nil {
		return
	}
	if fb.Strategy != nil {
		dst.Strategy = normalizeStrategy(*fb.Strategy)
	}
	if fb.Key != nil {
		dst.Key = strings.TrimSpace(*fb.Key)
	}
	if fb.Sample != nil {
		dst.Sample = *fb.Sample
	}
	if fb.Limit != nil {
		dst.Limit = *fb.Limit
	}
	if fb.Sort != nil {
		dst.Sort = fb.Sort
	}
	if fb.Filters != nil {
		dst.Filters = fb.Filters
	}
}

func (e *StaticEngine) selectWithConfig(req Request, cfg selectionConfig, backends []Backend, rr *atomic.Uint64) ([]Backend, int, error) {
	filtered := applyFilters(req, backends, cfg.Filters)
	if len(filtered) == 0 {
		return nil, -1, fmt.Errorf("%w", ErrNoBackends)
	}
	applySort(filtered, cfg.Sort)
	if cfg.Limit > 0 && len(filtered) > cfg.Limit {
		filtered = filtered[:cfg.Limit]
	}
	idx, err := e.selectIndex(cfg, filtered, rr)
	if err != nil {
		return nil, -1, err
	}
	return filtered, idx, nil
}

func (e *StaticEngine) selectIndex(cfg selectionConfig, backends []Backend, rr *atomic.Uint64) (int, error) {
	if len(backends) == 0 {
		return -1, fmt.Errorf("%w", ErrNoBackends)
	}
	switch cfg.Strategy {
	case "round_robin":
		v := rr.Add(1) - 1
		return int(v % uint64(len(backends))), nil
	case "random":
		e.rngMu.Lock()
		idx := e.rng.Intn(len(backends))
		e.rngMu.Unlock()
		return idx, nil
	case "weighted":
		total := 0
		for _, b := range backends {
			w := b.Weight
			if w <= 0 {
				w = 1
			}
			total += w
		}
		if total <= 0 {
			return -1, fmt.Errorf("%w", ErrInvalidWeightedPool)
		}
		e.rngMu.Lock()
		r := e.rng.Intn(total)
		e.rngMu.Unlock()
		acc := 0
		for i, b := range backends {
			w := b.Weight
			if w <= 0 {
				w = 1
			}
			acc += w
			if r < acc {
				return i, nil
			}
		}
		return len(backends) - 1, nil
	case "least_loaded":
		key := strings.TrimSpace(cfg.Key)
		if key == "" {
			return -1, fmt.Errorf("key must not be empty for strategy least_loaded")
		}
		bestIdx := -1
		best := math.Inf(1)
		for i, b := range backends {
			_, ok, n := sortValue(b, key, "number")
			if !ok {
				continue
			}
			if n < best {
				best = n
				bestIdx = i
			}
		}
		if bestIdx >= 0 {
			return bestIdx, nil
		}
		return 0, nil
	case "p2c":
		key := strings.TrimSpace(cfg.Key)
		if key == "" {
			return -1, fmt.Errorf("key must not be empty for strategy p2c")
		}
		sample := cfg.Sample
		if sample <= 0 {
			sample = 2
		}
		if sample > len(backends) {
			sample = len(backends)
		}
		if sample <= 0 {
			return -1, fmt.Errorf("%w", ErrNoBackends)
		}
		chosen := map[int]struct{}{}
		bestIdx := -1
		best := math.Inf(1)
		for len(chosen) < sample {
			e.rngMu.Lock()
			idx := e.rng.Intn(len(backends))
			e.rngMu.Unlock()
			if _, ok := chosen[idx]; ok {
				continue
			}
			chosen[idx] = struct{}{}
			_, ok, n := sortValue(backends[idx], key, "number")
			if !ok {
				n = math.Inf(1)
			}
			if n < best {
				best = n
				bestIdx = idx
			}
		}
		if bestIdx >= 0 {
			return bestIdx, nil
		}
		return 0, nil
	default:
		return -1, fmt.Errorf("%w %q", ErrUnknownStrategy, cfg.Strategy)
	}
}
