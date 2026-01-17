package plugins

import "context"

type NoopPlugin struct {
	name string
}

func NewNoopPlugin(name string) *NoopPlugin {
	return &NoopPlugin{name: name}
}

func (p *NoopPlugin) Name() string { return p.name }

func (p *NoopPlugin) OnConnect(ctx context.Context, req ConnectRequest) (ConnectResponse, error) {
	return ConnectResponse{}, nil
}

func (p *NoopPlugin) Close(ctx context.Context) error { return nil }
