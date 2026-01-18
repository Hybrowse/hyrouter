package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/hybrowse/hyrouter/internal/config"
	"github.com/hybrowse/hyrouter/internal/routing"
)

type Provider interface {
	Start(ctx context.Context) error
	Resolve(ctx context.Context) ([]routing.Backend, error)
}

type Manager struct {
	logger    *slog.Logger
	providers map[string]Provider

	startOnce sync.Once
	startErr  error
}

func New(cfg *config.DiscoveryConfig, logger *slog.Logger) (*Manager, error) {
	if cfg == nil {
		return &Manager{logger: logger, providers: map[string]Provider{}}, nil
	}
	if logger == nil {
		logger = slog.Default()
	}

	m := &Manager{logger: logger, providers: map[string]Provider{}}
	for i, p := range cfg.Providers {
		if p.Name == "" {
			return nil, fmt.Errorf("discovery.providers[%d].name must not be empty", i)
		}
		if _, ok := m.providers[p.Name]; ok {
			return nil, fmt.Errorf("discovery.providers[%d].name must be unique", i)
		}
		switch normalizeType(p.Type) {
		case "kubernetes":
			prov, err := newKubernetesProvider(p.Name, p.Kubernetes, logger)
			if err != nil {
				return nil, err
			}
			m.providers[p.Name] = prov
		case "agones":
			prov, err := newAgonesProvider(p.Name, p.Agones, logger)
			if err != nil {
				return nil, err
			}
			m.providers[p.Name] = prov
		default:
			return nil, fmt.Errorf("discovery.providers[%d].type must be one of: kubernetes, agones", i)
		}
	}
	return m, nil
}

func (m *Manager) Start(ctx context.Context) error {
	m.startOnce.Do(func() {
		for name, p := range m.providers {
			if err := p.Start(ctx); err != nil {
				m.startErr = fmt.Errorf("start discovery provider %q: %w", name, err)
				return
			}
		}
	})
	return m.startErr
}

func (m *Manager) Resolve(ctx context.Context, provider string) ([]routing.Backend, bool, error) {
	p, ok := m.providers[provider]
	if !ok {
		return nil, false, nil
	}
	bs, err := p.Resolve(ctx)
	return bs, true, err
}

func normalizeType(t string) string {
	return strings.ToLower(strings.TrimSpace(t))
}
