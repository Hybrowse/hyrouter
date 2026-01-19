package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/hybrowse/hyrouter/internal/config"
	"github.com/hybrowse/hyrouter/internal/discovery"
	"github.com/hybrowse/hyrouter/internal/plugins"
	"github.com/hybrowse/hyrouter/internal/routing"
	"github.com/quic-go/quic-go"
)

type Server struct {
	cfg        *config.Config
	logger     *slog.Logger
	router     routing.Engine
	discovery  *discovery.Manager
	initErr    error
	pluginCfgs []config.PluginConfig
	plugins    *plugins.Manager
}

func New(cfg *config.Config, logger *slog.Logger) *Server {
	var r routing.Engine
	var se *routing.StaticEngine
	var pcfgs []config.PluginConfig
	var dm *discovery.Manager
	var initErr error
	if cfg != nil {
		se = routing.NewStaticEngine(cfg.Routing)
		r = se
		pcfgs = cfg.Plugins
		if cfg.Discovery != nil {
			m, err := discovery.New(cfg.Discovery, logger)
			if err != nil {
				initErr = err
			} else {
				dm = m
				if se != nil {
					se.SetDiscovery(func(ctx context.Context, provider string) ([]routing.Backend, error) {
						bs, ok, err := dm.Resolve(ctx, provider)
						if err != nil {
							return nil, err
						}
						if !ok {
							return nil, fmt.Errorf("unknown discovery provider %q", provider)
						}
						return bs, nil
					})
				}
			}
		}
	}
	return &Server{cfg: cfg, logger: logger, router: r, pluginCfgs: pcfgs, discovery: dm, initErr: initErr}
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
	if s.initErr != nil {
		return s.initErr
	}
	tlsConfig, err := s.buildTLSConfig()
	if err != nil {
		return err
	}
	if s.discovery != nil {
		if err := s.discovery.Start(ctx); err != nil {
			return err
		}
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
	defer listener.Close() // nolint:errcheck

	s.logger.Info("listening", "addr", s.cfg.Listen)

	for {
		conn, err := listener.Accept(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				listener.Close() // nolint:errcheck
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

	decision := routing.Decision{Matched: false, RouteIndex: -1, SelectedIndex: -1}
	routeErr := error(nil)
	if s.router != nil {
		d, err := s.router.Decide(conn.Context(), routing.Request{SNI: state.TLS.ServerName})
		if err == nil {
			decision = d
		} else {
			routeErr = err
			logger.Info("routing error", "error", err)
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
	go s.acceptBidiStreams(connCtx, conn, logger, decision, routeErr, baseEvent)
	go s.acceptUniStreams(connCtx, conn, logger, decision, routeErr, baseEvent)

	select {
	case <-ctx.Done():
		_ = conn.CloseWithError(0, "shutdown")
	case <-connCtx.Done():
	}
}
