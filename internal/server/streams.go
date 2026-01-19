package server

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"strings"

	"github.com/hybrowse/hyrouter/internal/config"
	"github.com/hybrowse/hyrouter/internal/plugins"
	"github.com/hybrowse/hyrouter/internal/routing"
	"github.com/quic-go/quic-go"
)

func (s *Server) acceptBidiStreams(ctx context.Context, conn *quic.Conn, logger *slog.Logger, decision routing.Decision, routeErr error, baseEvent plugins.ConnectEvent) {
	for {
		stream, err := conn.AcceptStream(ctx)
		if err != nil {
			logger.Debug("accept bidi stream failed", "error", err)
			return
		}
		go s.handleBidiStream(ctx, stream, logger, decision, routeErr, baseEvent)
	}
}

func (s *Server) acceptUniStreams(ctx context.Context, conn *quic.Conn, logger *slog.Logger, decision routing.Decision, routeErr error, baseEvent plugins.ConnectEvent) {
	for {
		stream, err := conn.AcceptUniStream(ctx)
		if err != nil {
			logger.Debug("accept uni stream failed", "error", err)
			return
		}
		go s.handleUniStream(ctx, stream, logger, decision, routeErr, baseEvent)
	}
}

func (s *Server) handleBidiStream(ctx context.Context, stream *quic.Stream, logger *slog.Logger, decision routing.Decision, routeErr error, baseEvent plugins.ConnectEvent) {
	streamLogger := logger.With(
		"stream_id", stream.StreamID(),
		"stream_type", "bidi",
	)
	streamLogger.Debug("accepted stream")
	s.dumpFrames(ctx, stream, streamLogger, decision, routeErr, baseEvent)
}

func (s *Server) handleUniStream(ctx context.Context, stream *quic.ReceiveStream, logger *slog.Logger, decision routing.Decision, routeErr error, baseEvent plugins.ConnectEvent) {
	streamLogger := logger.With(
		"stream_id", stream.StreamID(),
		"stream_type", "uni",
	)
	streamLogger.Debug("accepted stream")
	s.dumpFrames(ctx, stream, streamLogger, decision, routeErr, baseEvent)
}

func (s *Server) dumpFrames(ctx context.Context, r io.Reader, logger *slog.Logger, decision routing.Decision, routeErr error, baseEvent plugins.ConnectEvent) {
	// Hytale packet framing: uint32le payloadLen + uint32le packetID + payload.
	// Hyrouter only needs the first Connect packet to either deny the connection or send a referral.
	buf := make([]byte, 4096)
	var pending []byte
	referralSent := false
	referralData := []byte(nil)
	backend := decision.Backend

	for {
		n, err := r.Read(buf)
		if n > 0 {
			pending = append(pending, buf[:n]...)

			for len(pending) >= 8 {

				payloadLen := int32(binary.LittleEndian.Uint32(pending[0:4]))
				packetID := int32(binary.LittleEndian.Uint32(pending[4:8]))

				if payloadLen < 0 || payloadLen > maxHytalePayloadLen || payloadLen > maxDebugBufferedPayload {
					logger.Info(
						"invalid frame",
						"payload_len", payloadLen,
						"packet_id", packetID,
						"buffered_bytes", len(pending),
					)
					return
				}

				frameLen := 8 + int(payloadLen)
				if len(pending) < frameLen {
					break
				}

				payload := pending[8:frameLen]
				sum := sha256.Sum256(payload)
				prefixLen := minInt(len(payload), maxDebugPayloadHexPrefix)

				logger.Debug(
					"rx packet",
					"packet_id", packetID,
					"packet_name", packetName(packetID),
					"payload_len", payloadLen,
					"payload_sha256", hex.EncodeToString(sum[:]),
					"payload_prefix_hex", hex.EncodeToString(payload[:prefixLen]),
				)

				if packetID == 0 {
					if info, ok := decodeConnectPayload(payload); ok {
						ev := baseEvent
						ev.ProtocolHash = info.protocolHash
						ev.ClientType = info.clientType
						ev.UUID = info.uuid
						ev.Username = info.username
						ev.Language = info.language
						ev.IdentityTokenPresent = info.identityTokenPresent
						if s.plugins != nil {
							res := s.plugins.ApplyOnConnect(ctx, ev, decision, referralData)
							if res.Denied {
								// Deny is terminal: send Disconnect and close the stream so the client can progress.
								w, ok := r.(io.Writer)
								if !ok {
									logger.Info("failed to send disconnect", "error", "stream is not writable")
									return
								}
								dp, derr := encodeDisconnectPayload(res.DenyReason)
								if derr != nil {
									logger.Info("failed to build disconnect", "error", derr)
									return
								}
								if err := writeFramedPacket(w, 1, dp); err != nil {
									logger.Info("failed to send disconnect", "error", err)
									return
								}
								logger.Info("tx disconnect", "reason", res.DenyReason)
								if c, ok := r.(interface{ Close() error }); ok {
									if err := c.Close(); err != nil {
										logger.Info("failed to close stream after disconnect", "error", err)
									}
								}
								return
							}
							backend = res.Backend
							referralData = res.ReferralData
						}
						logger.Info(
							"rx connect",
							"protocol_hash", info.protocolHash,
							"client_type", info.clientType,
							"uuid", info.uuid,
							"username", info.username,
							"language", info.language,
							"identity_token_present", info.identityTokenPresent,
							"referral_data_len", info.referralDataLen,
							"referral_source", info.referralSource,
						)

						if !referralSent && backend.Host != "" {
							w, ok := r.(io.Writer)
							if ok {
								refPayload, err := encodeClientReferralPayload(
									backend.Host,
									uint16(backend.Port),
									referralData,
								)
								if err != nil {
									logger.Info("failed to build referral", "error", err)
								} else if err := writeFramedPacket(w, 18, refPayload); err != nil {
									logger.Info("failed to send referral", "error", err)
								} else {
									referralSent = true
									logger.Info(
										"tx referral",
										"host", backend.Host,
										"port", backend.Port,
										"matched", decision.Matched,
										"route_index", decision.RouteIndex,
										"data_len", len(referralData),
									)
								}
							}
						}
						if !referralSent && backend.Host == "" {
							reason := s.disconnectReason(baseEvent.SNI, ev.Language, routeErr)
							if reason != "" {
								w, ok := r.(io.Writer)
								if !ok {
									logger.Info("failed to send disconnect", "error", "stream is not writable")
									return
								}
								dp, derr := encodeDisconnectPayload(reason)
								if derr != nil {
									logger.Info("failed to build disconnect", "error", derr)
									return
								}
								if err := writeFramedPacket(w, 1, dp); err != nil {
									logger.Info("failed to send disconnect", "error", err)
									return
								}
								logger.Info("tx disconnect", "reason", reason)
								if c, ok := r.(interface{ Close() error }); ok {
									if err := c.Close(); err != nil {
										logger.Info("failed to close stream after disconnect", "error", err)
									}
								}
								return
							}
						}
					}
				}

				pending = pending[frameLen:]
			}
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				logger.Debug("stream closed")
				return
			}
			var appErr *quic.ApplicationError
			if errors.As(err, &appErr) && appErr.Remote && appErr.ErrorCode == 0 {
				logger.Debug("stream closed (remote)", "error", err)
				return
			}
			var streamErr *quic.StreamError
			if errors.As(err, &streamErr) && streamErr.Remote {
				logger.Debug("stream canceled (remote)", "error", err, "code", streamErr.ErrorCode)
				return
			}
			logger.Warn("stream read error", "error", err)
			return
		}
	}
}

func (s *Server) disconnectReason(sni string, language string, routeErr error) string {
	if routeErr == nil {
		msg := s.templateOrDefault(s.templateNoRoute(language), "no route")
		return formatTemplate(msg, sni, nil)
	}
	if errors.Is(routeErr, routing.ErrNoBackends) {
		msg := s.templateOrDefault(s.templateNoBackends(language), "no backends")
		return formatTemplate(msg, sni, routeErr)
	}
	if errors.Is(routeErr, routing.ErrDiscovery) || errors.Is(routeErr, routing.ErrDiscoveryNotSet) || errors.Is(routeErr, routing.ErrInvalidDiscoveryMode) {
		msg := s.templateOrDefault(s.templateDiscoveryError(language), "discovery error")
		return formatTemplate(msg, sni, routeErr)
	}
	msg := s.templateOrDefault(s.templateRoutingError(language), "routing error")
	return formatTemplate(msg, sni, routeErr)
}

func (s *Server) templateNoRoute(language string) string {
	return s.disconnectMessagesForLanguage(language).NoRoute
}

func (s *Server) templateNoBackends(language string) string {
	return s.disconnectMessagesForLanguage(language).NoBackends
}

func (s *Server) templateRoutingError(language string) string {
	return s.disconnectMessagesForLanguage(language).RoutingError
}

func (s *Server) templateDiscoveryError(language string) string {
	return s.disconnectMessagesForLanguage(language).DiscoveryError
}

func (s *Server) disconnectMessagesForLanguage(language string) config.DisconnectMessagesConfig {
	if s == nil || s.cfg == nil {
		return config.DisconnectMessagesConfig{}
	}
	base := s.cfg.Messages.Disconnect
	locales := s.cfg.Messages.DisconnectLocales
	if len(locales) == 0 {
		return base
	}

	lang := strings.TrimSpace(language)
	if lang == "" {
		return base
	}
	lang = strings.ReplaceAll(lang, "_", "-")
	baseLang := lang
	if i := strings.Index(baseLang, "-"); i >= 0 {
		baseLang = baseLang[:i]
	}

	var loc config.DisconnectMessagesConfig
	var ok bool
	if loc, ok = lookupDisconnectLocale(locales, lang); !ok {
		loc, ok = lookupDisconnectLocale(locales, baseLang)
	}
	if !ok {
		return base
	}

	if strings.TrimSpace(loc.NoRoute) != "" {
		base.NoRoute = loc.NoRoute
	}
	if strings.TrimSpace(loc.NoBackends) != "" {
		base.NoBackends = loc.NoBackends
	}
	if strings.TrimSpace(loc.RoutingError) != "" {
		base.RoutingError = loc.RoutingError
	}
	if strings.TrimSpace(loc.DiscoveryError) != "" {
		base.DiscoveryError = loc.DiscoveryError
	}
	return base
}

func lookupDisconnectLocale(locales map[string]config.DisconnectMessagesConfig, key string) (config.DisconnectMessagesConfig, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return config.DisconnectMessagesConfig{}, false
	}
	for k, v := range locales {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}
	return config.DisconnectMessagesConfig{}, false
}

func (s *Server) templateOrDefault(v string, def string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return def
}

func formatTemplate(tpl string, sni string, err error) string {
	e := ""
	if err != nil {
		e = err.Error()
	}
	r := strings.NewReplacer(
		"${sni}", sni,
		"${error}", e,
	)
	return r.Replace(tpl)
}
