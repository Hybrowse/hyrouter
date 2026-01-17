package routing

import (
	"context"
	"fmt"
	"path"
	"strings"
)

type Target struct {
	Host string `json:"host" yaml:"host"`
	Port int    `json:"port" yaml:"port"`
}

type Match struct {
	Hostname  string   `json:"hostname" yaml:"hostname"`
	Hostnames []string `json:"hostnames" yaml:"hostnames"`
}

type Route struct {
	Match  Match  `json:"match" yaml:"match"`
	Target Target `json:"target" yaml:"target"`
}

type Config struct {
	Default *Target `json:"default" yaml:"default"`
	Routes  []Route `json:"routes" yaml:"routes"`
}

func (c *Config) Validate() error {
	if c == nil {
		return nil
	}
	if c.Default != nil {
		if err := validateTarget(*c.Default); err != nil {
			return fmt.Errorf("routing.default: %w", err)
		}
	}
	for i, r := range c.Routes {
		if err := validateTarget(r.Target); err != nil {
			return fmt.Errorf("routing.routes[%d].target: %w", i, err)
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

type Request struct {
	SNI string
}

type Decision struct {
	Target     Target
	Matched    bool
	RouteIndex int
}

type Engine interface {
	Decide(ctx context.Context, req Request) (Decision, error)
}

type StaticEngine struct {
	cfg Config
}

func NewStaticEngine(cfg Config) *StaticEngine {
	return &StaticEngine{cfg: cfg}
}

func (e *StaticEngine) Decide(_ context.Context, req Request) (Decision, error) {
	sni := canonicalHost(req.SNI)

	for i, r := range e.cfg.Routes {
		patterns := matchPatterns(r.Match)
		for _, p := range patterns {
			if hostnameMatches(p, sni) {
				return Decision{Target: r.Target, Matched: true, RouteIndex: i}, nil
			}
		}
	}

	if e.cfg.Default != nil {
		return Decision{Target: *e.cfg.Default, Matched: false, RouteIndex: -1}, nil
	}

	return Decision{}, nil
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
