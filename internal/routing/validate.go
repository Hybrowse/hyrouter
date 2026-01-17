package routing

import (
	"fmt"
	"strings"
)

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
	case "round_robin", "random", "weighted", "least_loaded", "p2c":
	default:
		return fmt.Errorf("unknown strategy %q", p.Strategy)
	}
	if strategy == "least_loaded" || strategy == "p2c" {
		if strings.TrimSpace(p.Key) == "" {
			return fmt.Errorf("key must not be empty for strategy %q", p.Strategy)
		}
	}
	if strategy == "p2c" {
		if p.Sample < 0 {
			return fmt.Errorf("sample must be >= 0")
		}
	}
	if p.Limit < 0 {
		return fmt.Errorf("limit must be >= 0")
	}
	for i, s := range p.Sort {
		if strings.TrimSpace(s.Key) == "" {
			return fmt.Errorf("sort[%d].key must not be empty", i)
		}
		order := normalizeStrategy(s.Order)
		if order == "" {
			order = "asc"
		}
		switch order {
		case "asc", "desc":
		default:
			return fmt.Errorf("sort[%d].order must be one of: asc, desc", i)
		}
		typeHint := normalizeStrategy(s.Type)
		switch typeHint {
		case "", "string", "number":
		default:
			return fmt.Errorf("sort[%d].type must be one of: string, number", i)
		}
	}
	for i, f := range p.Filters {
		if err := validateFilter(f); err != nil {
			return fmt.Errorf("filters[%d]: %w", i, err)
		}
	}
	for i, fb := range p.Fallback {
		if err := validateFallback(fb); err != nil {
			return fmt.Errorf("fallback[%d]: %w", i, err)
		}
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

func validateFilter(f Filter) error {
	t := normalizeStrategy(f.Type)
	if t == "" {
		return fmt.Errorf("type must not be empty")
	}
	switch t {
	case "compare":
		if strings.TrimSpace(f.Left) == "" {
			return fmt.Errorf("left must not be empty")
		}
		op := normalizeStrategy(f.Op)
		switch op {
		case "lt", "lte", "gt", "gte", "eq", "neq":
		default:
			return fmt.Errorf("op must be one of: lt, lte, gt, gte, eq, neq")
		}
		if strings.TrimSpace(f.Right) == "" {
			return fmt.Errorf("right must not be empty")
		}
	case "whitelist":
		if strings.TrimSpace(f.EnabledKey) == "" {
			return fmt.Errorf("enabled_key must not be empty")
		}
		if strings.TrimSpace(f.ListKey) == "" {
			return fmt.Errorf("list_key must not be empty")
		}
		subject := normalizeStrategy(f.Subject)
		switch subject {
		case "", "uuid", "username":
		default:
			return fmt.Errorf("subject must be one of: uuid, username")
		}
		return nil
	case "game_start_not_past":
		if strings.TrimSpace(f.Key) == "" {
			return fmt.Errorf("key must not be empty")
		}
		return nil
	default:
		return fmt.Errorf("unknown type %q", f.Type)
	}
	return nil
}

func validateFallback(fb Fallback) error {
	if fb.Strategy != nil {
		strategy := normalizeStrategy(*fb.Strategy)
		switch strategy {
		case "round_robin", "random", "weighted", "least_loaded", "p2c":
		default:
			return fmt.Errorf("unknown strategy %q", *fb.Strategy)
		}
		if strategy == "least_loaded" || strategy == "p2c" {
			if fb.Key == nil || strings.TrimSpace(*fb.Key) == "" {
				return fmt.Errorf("key must not be empty for strategy %q", *fb.Strategy)
			}
		}
	}
	if fb.Sample != nil && *fb.Sample < 0 {
		return fmt.Errorf("sample must be >= 0")
	}
	if fb.Limit != nil && *fb.Limit < 0 {
		return fmt.Errorf("limit must be >= 0")
	}
	for i, s := range fb.Sort {
		if strings.TrimSpace(s.Key) == "" {
			return fmt.Errorf("sort[%d].key must not be empty", i)
		}
		order := normalizeStrategy(s.Order)
		if order == "" {
			order = "asc"
		}
		switch order {
		case "asc", "desc":
		default:
			return fmt.Errorf("sort[%d].order must be one of: asc, desc", i)
		}
		typeHint := normalizeStrategy(s.Type)
		switch typeHint {
		case "", "string", "number":
		default:
			return fmt.Errorf("sort[%d].type must be one of: string, number", i)
		}
	}
	for i, f := range fb.Filters {
		if err := validateFilter(f); err != nil {
			return fmt.Errorf("filters[%d]: %w", i, err)
		}
	}
	return nil
}
