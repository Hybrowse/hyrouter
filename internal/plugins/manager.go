package plugins

import (
	"context"
	"log/slog"
	"time"

	"github.com/hybrowse/hyrouter/internal/routing"
)

const pluginCallTimeout = 1 * time.Second

type Manager struct {
	plugins []Plugin
	logger  *slog.Logger
}

type ApplyResult struct {
	Denied          bool
	DenyReason      string
	Strategy        string
	Candidates      []routing.Backend
	SelectedIndex   int
	Backend         routing.Backend
	ReferralContent []byte
}

func NewManager(logger *slog.Logger, plugins []Plugin) *Manager {
	return &Manager{plugins: plugins, logger: logger}
}

func (m *Manager) ApplyOnConnect(ctx context.Context, ev ConnectEvent, decision routing.Decision, referralContent []byte) ApplyResult {
	res := ApplyResult{
		Strategy:        decision.Strategy,
		Candidates:      decision.Candidates,
		SelectedIndex:   decision.SelectedIndex,
		Backend:         decision.Backend,
		ReferralContent: referralContent,
	}
	if m == nil {
		return res
	}
	for _, p := range m.plugins {
		pctx, cancel := context.WithTimeout(ctx, pluginCallTimeout)
		pr, err := p.OnConnect(pctx, ConnectRequest{
			Event:           ev,
			Strategy:        res.Strategy,
			Candidates:      res.Candidates,
			SelectedIndex:   res.SelectedIndex,
			Backend:         res.Backend,
			ReferralContent: res.ReferralContent,
		})
		cancel()
		if err != nil {
			if m.logger != nil {
				m.logger.Info("plugin error", "plugin", p.Name(), "error", err)
			}
			continue
		}
		if pr.Deny {
			res.Denied = true
			res.DenyReason = pr.DenyReason
			return res
		}
		if pr.Candidates != nil {
			if len(pr.Candidates) > 0 {
				res.Candidates = pr.Candidates
			}
		}
		if pr.SelectedIndex != nil {
			idx := *pr.SelectedIndex
			if idx >= 0 && idx < len(res.Candidates) {
				res.SelectedIndex = idx
				res.Backend = res.Candidates[idx]
			}
		}
		if pr.Backend != nil {
			res.Backend = *pr.Backend
			if len(res.Candidates) > 0 {
				for i, b := range res.Candidates {
					if b.Host == res.Backend.Host && b.Port == res.Backend.Port {
						res.SelectedIndex = i
						break
					}
				}
			}
		}
		if res.Backend.Host == "" && len(res.Candidates) > 0 {
			res.SelectedIndex = 0
			res.Backend = res.Candidates[0]
		}
		if pr.ReferralContent != nil {
			res.ReferralContent = pr.ReferralContent
		}
	}
	return res
}

func (m *Manager) Close(ctx context.Context) {
	if m == nil {
		return
	}
	for _, p := range m.plugins {
		_ = p.Close(ctx)
	}
}
