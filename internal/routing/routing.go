package routing

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrNoBackends           = errors.New("no backends")
	ErrUnknownStrategy      = errors.New("unknown strategy")
	ErrInvalidWeightedPool  = errors.New("invalid weighted pool")
	ErrDiscovery            = errors.New("discovery error")
	ErrDiscoveryNotSet      = errors.New("discovery resolver not set")
	ErrInvalidDiscoveryMode = errors.New("invalid discovery mode")
)

type Target struct {
	Host string `json:"host" yaml:"host"`
	Port int    `json:"port" yaml:"port"`
}

type Backend struct {
	Host   string            `json:"host" yaml:"host"`
	Port   int               `json:"port" yaml:"port"`
	Weight int               `json:"weight" yaml:"weight"`
	Meta   map[string]string `json:"meta" yaml:"meta"`
}

func (b Backend) Target() Target {
	return Target{Host: b.Host, Port: b.Port}
}

type Pool struct {
	Strategy  string     `json:"strategy" yaml:"strategy"`
	Backends  []Backend  `json:"backends" yaml:"backends"`
	Discovery *Discovery `json:"discovery" yaml:"discovery"`
}

type Discovery struct {
	Provider string    `json:"provider" yaml:"provider"`
	Mode     string    `json:"mode" yaml:"mode"`
	Limit    int       `json:"limit" yaml:"limit"`
	Sort     []SortKey `json:"sort" yaml:"sort"`
}

type SortKey struct {
	Key   string `json:"key" yaml:"key"`
	Order string `json:"order" yaml:"order"`
	Type  string `json:"type" yaml:"type"`
}

type Match struct {
	Hostname  string   `json:"hostname" yaml:"hostname"`
	Hostnames []string `json:"hostnames" yaml:"hostnames"`
}

type Route struct {
	Match Match `json:"match" yaml:"match"`
	Pool  Pool  `json:"pool" yaml:"pool"`
}

type Config struct {
	Default *Pool   `json:"default" yaml:"default"`
	Routes  []Route `json:"routes" yaml:"routes"`
}

func (c *Config) Validate() error {
	if c == nil {
		return nil
	}
	if c.Default != nil {
		if err := validatePool(*c.Default); err != nil {
			return fmt.Errorf("routing.default: %w", err)
		}
	}
	for i, r := range c.Routes {
		if err := validatePool(r.Pool); err != nil {
			return fmt.Errorf("routing.routes[%d].pool: %w", i, err)
		}
		if len(r.Match.Hostnames) == 0 && r.Match.Hostname == "" {
			return fmt.Errorf("routing.routes[%d].match must not be empty", i)
		}
	}
	return nil
}

func validateTarget(t Target) error {
	if t.Host == "" {
		return fmt.Errorf("host must not be empty")
	}
	if t.Port <= 0 || t.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	return nil
}

func validateBackend(b Backend) error {
	if err := validateTarget(b.Target()); err != nil {
		return err
	}
	if b.Weight < 0 {
		return fmt.Errorf("weight must be >= 0")
	}
	return nil
}

func normalizeStrategy(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "-", "_")
	return s
}

func validatePool(p Pool) error {
	if len(p.Backends) == 0 && p.Discovery == nil {
		return fmt.Errorf("backends must not be empty")
	}
	strategy := normalizeStrategy(p.Strategy)
	if strategy == "" {
		return fmt.Errorf("strategy must not be empty")
	}
	switch strategy {
	case "round_robin", "random", "weighted":
	default:
		return fmt.Errorf("unknown strategy %q", p.Strategy)
	}
	if p.Discovery != nil {
		if strings.TrimSpace(p.Discovery.Provider) == "" {
			return fmt.Errorf("discovery.provider must not be empty")
		}
		mode := normalizeStrategy(p.Discovery.Mode)
		if mode == "" {
			mode = "union"
		}
		switch mode {
		case "union", "prefer":
		default:
			return fmt.Errorf("discovery.mode must be one of: union, prefer")
		}
		if p.Discovery.Limit < 0 {
			return fmt.Errorf("discovery.limit must be >= 0")
		}
		for i, s := range p.Discovery.Sort {
			if strings.TrimSpace(s.Key) == "" {
				return fmt.Errorf("discovery.sort[%d].key must not be empty", i)
			}
			order := normalizeStrategy(s.Order)
			if order == "" {
				order = "asc"
			}
			switch order {
			case "asc", "desc":
			default:
				return fmt.Errorf("discovery.sort[%d].order must be one of: asc, desc", i)
			}
			typeHint := normalizeStrategy(s.Type)
			switch typeHint {
			case "", "string", "number":
			default:
				return fmt.Errorf("discovery.sort[%d].type must be one of: string, number", i)
			}
		}
	}

	for i, b := range p.Backends {
		if err := validateBackend(b); err != nil {
			return fmt.Errorf("backends[%d]: %w", i, err)
		}
		if strategy == "weighted" && b.Weight <= 0 {
			return fmt.Errorf("backends[%d].weight must be > 0 for weighted strategy", i)
		}
	}
	return nil
}

type Request struct {
	SNI string
}

type Decision struct {
	Matched       bool      `json:"matched"`
	RouteIndex    int       `json:"route_index"`
	Strategy      string    `json:"strategy"`
	Candidates    []Backend `json:"candidates"`
	SelectedIndex int       `json:"selected_index"`
	Backend       Backend   `json:"backend"`
}

type Engine interface {
	Decide(ctx context.Context, req Request) (Decision, error)
}

type StaticEngine struct {
	cfg       Config
	rr        []atomic.Uint64
	rrDefault atomic.Uint64
	rngMu     sync.Mutex
	rng       *rand.Rand
	discovery func(ctx context.Context, provider string) ([]Backend, error)
}

func NewStaticEngine(cfg Config) *StaticEngine {
	return &StaticEngine{
		cfg: cfg,
		rr:  make([]atomic.Uint64, len(cfg.Routes)),
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (e *StaticEngine) SetDiscovery(fn func(ctx context.Context, provider string) ([]Backend, error)) {
	e.discovery = fn
}

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
				idx, err := e.selectIndex(normalizeStrategy(r.Pool.Strategy), cands, &e.rr[i])
				if err != nil {
					return Decision{}, err
				}
				b := cands[idx]
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
		idx, err := e.selectIndex(normalizeStrategy(e.cfg.Default.Strategy), cands, &e.rrDefault)
		if err != nil {
			return Decision{}, err
		}
		b := cands[idx]
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

func (e *StaticEngine) selectIndex(strategy string, backends []Backend, rr *atomic.Uint64) (int, error) {
	if len(backends) == 0 {
		return -1, fmt.Errorf("%w", ErrNoBackends)
	}
	switch strategy {
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
	default:
		return -1, fmt.Errorf("%w %q", ErrUnknownStrategy, strategy)
	}
}

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
	applyDiscoverySort(merged, pool.Discovery.Sort)
	if pool.Discovery.Limit > 0 && len(merged) > pool.Discovery.Limit {
		merged = merged[:pool.Discovery.Limit]
	}
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

func applyDiscoverySort(backends []Backend, keys []SortKey) {
	if len(keys) == 0 {
		return
	}
	sort.SliceStable(backends, func(i, j int) bool {
		a := backends[i]
		b := backends[j]
		for _, k := range keys {
			order := normalizeStrategy(k.Order)
			if order == "" {
				order = "asc"
			}
			typeHint := normalizeStrategy(k.Type)
			av, aok, an := sortValue(a, k.Key, typeHint)
			bv, bok, bn := sortValue(b, k.Key, typeHint)
			if aok != bok {
				return aok && !bok
			}
			if typeHint == "number" {
				if an != bn {
					if order == "desc" {
						return an > bn
					}
					return an < bn
				}
				continue
			}
			if av != bv {
				if order == "desc" {
					return av > bv
				}
				return av < bv
			}
		}
		return false
	})
}

func sortValue(b Backend, key string, typeHint string) (string, bool, float64) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", false, 0
	}

	if normalizeStrategy(typeHint) == "number" {
		if key == "port" {
			return "", true, float64(b.Port)
		}
		if key == "weight" {
			return "", true, float64(b.Weight)
		}
	}

	var raw string
	switch {
	case key == "host":
		raw = b.Host
	case key == "port":
		raw = strconv.Itoa(b.Port)
	case key == "weight":
		raw = strconv.Itoa(b.Weight)
	case strings.HasPrefix(key, "label:"):
		raw = metaGet(b.Meta, "label."+strings.TrimSpace(strings.TrimPrefix(key, "label:")))
	case strings.HasPrefix(key, "annotation:"):
		raw = metaGet(b.Meta, "annotation."+strings.TrimSpace(strings.TrimPrefix(key, "annotation:")))
	case strings.HasPrefix(key, "counter:"):
		raw = metaGet(b.Meta, "counter."+strings.TrimSpace(strings.TrimPrefix(key, "counter:")))
	default:
		raw = metaGet(b.Meta, key)
	}
	if raw == "" {
		return "", false, 0
	}
	if normalizeStrategy(typeHint) == "number" {
		n, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return "", false, 0
		}
		return "", true, n
	}
	return raw, true, 0
}

func metaGet(m map[string]string, k string) string {
	if m == nil {
		return ""
	}
	return m[k]
}

func matchPatterns(m Match) []string {
	if len(m.Hostnames) > 0 {
		return m.Hostnames
	}
	if m.Hostname != "" {
		return []string{m.Hostname}
	}
	return nil
}

func canonicalHost(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, ".")
	s = strings.ToLower(s)
	return s
}

func hostnameMatches(pattern string, hostname string) bool {
	pattern = canonicalHost(pattern)
	if pattern == "" || hostname == "" {
		return false
	}
	if pattern == hostname {
		return true
	}
	ok, err := path.Match(pattern, hostname)
	if err != nil {
		return false
	}
	return ok
}
