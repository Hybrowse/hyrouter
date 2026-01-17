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
	Denied       bool
	DenyReason   string
	Target       routing.Target
	ReferralData []byte
}

func NewManager(logger *slog.Logger, plugins []Plugin) *Manager {
	return &Manager{plugins: plugins, logger: logger}
}

func (m *Manager) ApplyOnConnect(ctx context.Context, ev ConnectEvent, target routing.Target, referralData []byte) ApplyResult {
	res := ApplyResult{Target: target, ReferralData: referralData}
	if m == nil {
		return res
	}
	for _, p := range m.plugins {
		pctx, cancel := context.WithTimeout(ctx, pluginCallTimeout)
		pr, err := p.OnConnect(pctx, ConnectRequest{Event: ev, Target: res.Target, ReferralData: res.ReferralData})
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
		if pr.Target != nil {
			res.Target = *pr.Target
		}
		if pr.ReferralData != nil {
			res.ReferralData = pr.ReferralData
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
