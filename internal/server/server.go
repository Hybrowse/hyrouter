package server

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/hybrowse/hyrouter/internal/config"
	"github.com/hybrowse/hyrouter/internal/plugins"
	"github.com/hybrowse/hyrouter/internal/routing"
	"github.com/quic-go/quic-go"
)

type Server struct {
	cfg        *config.Config
	logger     *slog.Logger
	router     routing.Engine
	pluginCfgs []config.PluginConfig
	plugins    *plugins.Manager
}

func New(cfg *config.Config, logger *slog.Logger) *Server {
	var r routing.Engine
	var pcfgs []config.PluginConfig
	if cfg != nil {
		r = routing.NewStaticEngine(cfg.Routing)
		pcfgs = cfg.Plugins
	}
	return &Server{cfg: cfg, logger: logger, router: r, pluginCfgs: pcfgs}
}

func (s *Server) initPlugins(ctx context.Context) error {
	if s.plugins != nil {
		return nil
	}
	if len(s.pluginCfgs) == 0 {
		return nil
	}
	ordered, err := plugins.OrderPluginConfigs(s.pluginCfgs)
	if err != nil {
		return err
	}
	pls, err := plugins.LoadAll(ctx, ordered, s.logger)
	if err != nil {
		return err
	}
	s.plugins = plugins.NewManager(s.logger, pls)
	return nil
}

func (s *Server) Run(ctx context.Context) error {
	tlsConfig, err := s.buildTLSConfig()
	if err != nil {
		return err
	}
	if err := s.initPlugins(ctx); err != nil {
		return err
	}
	if s.plugins != nil {
		defer s.plugins.Close(ctx)
	}

	maxIdleTimeout := 30 * time.Second
	if s.cfg.QUIC.MaxIdleTimeout != "" {
		d, err := time.ParseDuration(s.cfg.QUIC.MaxIdleTimeout)
		if err != nil {
			return err
		}
		maxIdleTimeout = d
	}

	quicConfig := &quic.Config{
		MaxIdleTimeout: maxIdleTimeout,
	}

	listener, err := quic.ListenAddr(s.cfg.Listen, tlsConfig, quicConfig)
	if err != nil {
		return err
	}
	defer listener.Close()

	s.logger.Info("listening", "addr", s.cfg.Listen)

	for {
		conn, err := listener.Accept(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				listener.Close()
				return nil
			}
			return err
		}

		go s.handleConn(ctx, conn)
	}
}

func (s *Server) handleConn(ctx context.Context, conn *quic.Conn) {
	state := conn.ConnectionState()

	logger := s.logger.With(
		"remote_addr", conn.RemoteAddr().String(),
		"sni", state.TLS.ServerName,
		"alpn", state.TLS.NegotiatedProtocol,
	)

	decision := routing.Decision{}
	if s.router != nil {
		d, err := s.router.Decide(conn.Context(), routing.Request{SNI: state.TLS.ServerName})
		if err == nil {
			decision = d
		}
	}

	fp := ""
	if len(state.TLS.PeerCertificates) > 0 {
		fp = certificateFingerprint(state.TLS.PeerCertificates[0])
	}
	baseEvent := plugins.ConnectEvent{SNI: state.TLS.ServerName, ClientCertFingerprint: fp}

	logger.Info(
		"accepted connection",
		"client_cert_present", fp != "",
		"client_cert_fingerprint", fp,
	)

	connCtx := conn.Context()
	go s.acceptBidiStreams(connCtx, conn, logger, decision, baseEvent)
	go s.acceptUniStreams(connCtx, conn, logger, decision, baseEvent)

	select {
	case <-ctx.Done():
		_ = conn.CloseWithError(0, "shutdown")
	case <-connCtx.Done():
	}
}
