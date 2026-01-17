package plugins

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/hybrowse/hyrouter/internal/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"
)

type grpcPlugin struct {
	name   string
	conn   *grpc.ClientConn
	logger *slog.Logger
}

func newGRPCPlugin(ctx context.Context, cfg config.PluginConfig, logger *slog.Logger) (Plugin, error) {
	if cfg.GRPC == nil || cfg.GRPC.Address == "" {
		return nil, fmt.Errorf("grpc.address must not be empty")
	}

	codec := jsonCodec{}
	encoding.RegisterCodec(codec)

	conn, err := grpc.DialContext(
		ctx,
		cfg.GRPC.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(codec)),
	)
	if err != nil {
		return nil, err
	}

	return &grpcPlugin{name: cfg.Name, conn: conn, logger: logger}, nil
}

func (p *grpcPlugin) Name() string { return p.name }

func (p *grpcPlugin) OnConnect(ctx context.Context, req ConnectRequest) (ConnectResponse, error) {
	var resp ConnectResponse
	codec := jsonCodec{}
	if err := p.conn.Invoke(ctx, "/hyrouter.Plugin/OnConnect", &req, &resp, grpc.ForceCodec(codec)); err != nil {
		return ConnectResponse{}, err
	}
	return resp, nil
}

func (p *grpcPlugin) Close(ctx context.Context) error {
	_ = ctx
	return p.conn.Close()
}
