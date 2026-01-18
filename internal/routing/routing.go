package routing

import (
	"context"
	"fmt"
	"math/rand"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"
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
	Strategy string    `json:"strategy" yaml:"strategy"`
	Backends []Backend `json:"backends" yaml:"backends"`
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
	if len(p.Backends) == 0 {
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
}

func NewStaticEngine(cfg Config) *StaticEngine {
	return &StaticEngine{
		cfg: cfg,
		rr:  make([]atomic.Uint64, len(cfg.Routes)),
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (e *StaticEngine) Decide(_ context.Context, req Request) (Decision, error) {
	sni := canonicalHost(req.SNI)

	for i, r := range e.cfg.Routes {
		patterns := matchPatterns(r.Match)
		for _, p := range patterns {
			if hostnameMatches(p, sni) {
				idx, err := e.selectIndex(r.Pool, &e.rr[i])
				if err != nil {
					return Decision{}, err
				}
				b := r.Pool.Backends[idx]
				return Decision{
					Backend:       b,
					Candidates:    r.Pool.Backends,
					SelectedIndex: idx,
					Strategy:      normalizeStrategy(r.Pool.Strategy),
					Matched:       true,
					RouteIndex:    i,
				}, nil
			}
		}
	}

	if e.cfg.Default != nil {
		idx, err := e.selectIndex(*e.cfg.Default, &e.rrDefault)
		if err != nil {
			return Decision{}, err
		}
		b := e.cfg.Default.Backends[idx]
		return Decision{
			Backend:       b,
			Candidates:    e.cfg.Default.Backends,
			SelectedIndex: idx,
			Strategy:      normalizeStrategy(e.cfg.Default.Strategy),
			Matched:       false,
			RouteIndex:    -1,
		}, nil
	}

	return Decision{}, nil
}

func (e *StaticEngine) selectIndex(pool Pool, rr *atomic.Uint64) (int, error) {
	if len(pool.Backends) == 0 {
		return -1, fmt.Errorf("no backends")
	}
	strategy := normalizeStrategy(pool.Strategy)
	switch strategy {
	case "round_robin":
		v := rr.Add(1) - 1
		return int(v % uint64(len(pool.Backends))), nil
	case "random":
		e.rngMu.Lock()
		idx := e.rng.Intn(len(pool.Backends))
		e.rngMu.Unlock()
		return idx, nil
	case "weighted":
		total := 0
		for _, b := range pool.Backends {
			total += b.Weight
		}
		if total <= 0 {
			return -1, fmt.Errorf("invalid weighted pool")
		}
		e.rngMu.Lock()
		r := e.rng.Intn(total)
		e.rngMu.Unlock()
		acc := 0
		for i, b := range pool.Backends {
			acc += b.Weight
			if r < acc {
				return i, nil
			}
		}
		return len(pool.Backends) - 1, nil
	default:
		return -1, fmt.Errorf("unknown strategy %q", pool.Strategy)
	}
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
