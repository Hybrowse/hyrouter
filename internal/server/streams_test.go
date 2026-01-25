package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"testing"

	"github.com/hybrowse/hyrouter/internal/config"
	"github.com/hybrowse/hyrouter/internal/plugins"
	"github.com/hybrowse/hyrouter/internal/referral"
	"github.com/hybrowse/hyrouter/internal/routing"
)

type rw struct {
	r *bytes.Reader
	w bytes.Buffer
}

type denyPlugin struct{}

func (d *denyPlugin) Name() string { return "deny" }
func (d *denyPlugin) OnConnect(ctx context.Context, req plugins.ConnectRequest) (plugins.ConnectResponse, error) {
	_ = ctx
	_ = req
	return plugins.ConnectResponse{Deny: true, DenyReason: "no"}, nil
}
func (d *denyPlugin) Close(ctx context.Context) error { return nil }

func TestDumpFrames_PluginDenySendsDisconnect(t *testing.T) {
	connectPayload := buildConnectPayloadForTest(
		"6708f121966c1c443f4b0eb525b2f81d0a8dc61f5003a692a8fa157e5e02cea9",
		0,
		"d3e6ef90-e113-49a7-a845-1c11f24fe166",
		"de-DE",
		"tok",
		"Krymo",
	)

	frame := make([]byte, 8+len(connectPayload))
	binary.LittleEndian.PutUint32(frame[0:4], uint32(len(connectPayload)))
	binary.LittleEndian.PutUint32(frame[4:8], 0)
	copy(frame[8:], connectPayload)

	rx := &rw{r: bytes.NewReader(frame)}
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))

	s := &Server{logger: logger, plugins: plugins.NewManager(logger, []plugins.Plugin{&denyPlugin{}})}
	decision := routing.Decision{Backend: routing.Backend{Host: "play.hyvane.com", Port: 5520}}
	s.dumpFrames(context.Background(), nil, rx, logger, decision, nil, plugins.ConnectEvent{})

	out := rx.w.Bytes()
	if len(out) < 8 {
		t.Fatalf("short out")
	}
	if binary.LittleEndian.Uint32(out[4:8]) != 1 {
		t.Fatalf("expected packet 1, got %d", binary.LittleEndian.Uint32(out[4:8]))
	}
	payloadLen := int(binary.LittleEndian.Uint32(out[0:4]))
	if 8+payloadLen > len(out) {
		t.Fatalf("short payload")
	}
	p := out[8 : 8+payloadLen]
	if len(p) < 4 {
		t.Fatalf("short disconnect payload")
	}
	if p[0] != 0x01 {
		t.Fatalf("nullbits=%02x", p[0])
	}
}

func disconnectReasonFromFrameForTest(t *testing.T, out []byte) string {
	t.Helper()
	if len(out) < 8 {
		t.Fatalf("short out")
	}
	if binary.LittleEndian.Uint32(out[4:8]) != 1 {
		t.Fatalf("expected packet 1, got %d", binary.LittleEndian.Uint32(out[4:8]))
	}
	payloadLen := int(binary.LittleEndian.Uint32(out[0:4]))
	if 8+payloadLen > len(out) {
		t.Fatalf("short payload")
	}
	p := out[8 : 8+payloadLen]
	if len(p) == 2 && p[0] == 0 && p[1] == 0 {
		return ""
	}
	if len(p) < 3 {
		t.Fatalf("short disconnect payload")
	}
	if p[0] != 0x01 {
		t.Fatalf("nullbits=%02x", p[0])
	}
	if p[1] != 0 {
		t.Fatalf("type=%02x", p[1])
	}
	s, _, ok := readVarString(p, 2, 4096000)
	if !ok {
		t.Fatalf("failed to decode reason")
	}
	return s
}

func TestDumpFrames_DisconnectLocalized_ExactLocale(t *testing.T) {
	connectPayload := buildConnectPayloadForTest(
		"6708f121966c1c443f4b0eb525b2f81d0a8dc61f5003a692a8fa157e5e02cea9",
		0,
		"d3e6ef90-e113-49a7-a845-1c11f24fe166",
		"de-DE",
		"tok",
		"Krymo",
	)
	frame := make([]byte, 8+len(connectPayload))
	binary.LittleEndian.PutUint32(frame[0:4], uint32(len(connectPayload)))
	binary.LittleEndian.PutUint32(frame[4:8], 0)
	copy(frame[8:], connectPayload)

	rx := &rw{r: bytes.NewReader(frame)}
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))

	cfg := &config.Config{Messages: config.MessagesConfig{
		Disconnect: config.DisconnectMessagesConfig{RoutingError: "EN ${sni}"},
		DisconnectLocales: map[string]config.DisconnectMessagesConfig{
			"de":    {RoutingError: "DE ${sni}"},
			"de-DE": {RoutingError: "DE-DE ${sni}"},
		},
	}}

	s := &Server{logger: logger, cfg: cfg}
	s.dumpFrames(context.Background(), nil, rx, logger, routing.Decision{Matched: false, RouteIndex: -1, SelectedIndex: -1}, routing.ErrUnknownStrategy, plugins.ConnectEvent{SNI: "example"})

	reason := disconnectReasonFromFrameForTest(t, rx.w.Bytes())
	if reason != "DE-DE example" {
		t.Fatalf("reason=%q", reason)
	}
}

func TestDumpFrames_DisconnectLocalized_BaseLanguageFallback(t *testing.T) {
	connectPayload := buildConnectPayloadForTest(
		"6708f121966c1c443f4b0eb525b2f81d0a8dc61f5003a692a8fa157e5e02cea9",
		0,
		"d3e6ef90-e113-49a7-a845-1c11f24fe166",
		"de-AT",
		"tok",
		"Krymo",
	)
	frame := make([]byte, 8+len(connectPayload))
	binary.LittleEndian.PutUint32(frame[0:4], uint32(len(connectPayload)))
	binary.LittleEndian.PutUint32(frame[4:8], 0)
	copy(frame[8:], connectPayload)

	rx := &rw{r: bytes.NewReader(frame)}
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))

	cfg := &config.Config{Messages: config.MessagesConfig{
		Disconnect: config.DisconnectMessagesConfig{RoutingError: "EN ${sni}"},
		DisconnectLocales: map[string]config.DisconnectMessagesConfig{
			"de": {RoutingError: "DE ${sni}"},
		},
	}}

	s := &Server{logger: logger, cfg: cfg}
	s.dumpFrames(context.Background(), nil, rx, logger, routing.Decision{Matched: false, RouteIndex: -1, SelectedIndex: -1}, routing.ErrUnknownStrategy, plugins.ConnectEvent{SNI: "example"})

	reason := disconnectReasonFromFrameForTest(t, rx.w.Bytes())
	if reason != "DE example" {
		t.Fatalf("reason=%q", reason)
	}
}

func TestDumpFrames_DisconnectLocalized_DefaultFallback(t *testing.T) {
	connectPayload := buildConnectPayloadForTest(
		"6708f121966c1c443f4b0eb525b2f81d0a8dc61f5003a692a8fa157e5e02cea9",
		0,
		"d3e6ef90-e113-49a7-a845-1c11f24fe166",
		"fr-FR",
		"tok",
		"Krymo",
	)
	frame := make([]byte, 8+len(connectPayload))
	binary.LittleEndian.PutUint32(frame[0:4], uint32(len(connectPayload)))
	binary.LittleEndian.PutUint32(frame[4:8], 0)
	copy(frame[8:], connectPayload)

	rx := &rw{r: bytes.NewReader(frame)}
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))

	cfg := &config.Config{Messages: config.MessagesConfig{
		Disconnect: config.DisconnectMessagesConfig{RoutingError: "EN ${sni}"},
		DisconnectLocales: map[string]config.DisconnectMessagesConfig{
			"de": {RoutingError: "DE ${sni}"},
		},
	}}

	s := &Server{logger: logger, cfg: cfg}
	s.dumpFrames(context.Background(), nil, rx, logger, routing.Decision{Matched: false, RouteIndex: -1, SelectedIndex: -1}, routing.ErrUnknownStrategy, plugins.ConnectEvent{SNI: "example"})

	reason := disconnectReasonFromFrameForTest(t, rx.w.Bytes())
	if reason != "EN example" {
		t.Fatalf("reason=%q", reason)
	}
}

type mutatePlugin struct{}

func (m *mutatePlugin) Name() string { return "mut" }
func (m *mutatePlugin) OnConnect(ctx context.Context, req plugins.ConnectRequest) (plugins.ConnectResponse, error) {
	_ = ctx
	_ = req
	return plugins.ConnectResponse{ReferralContent: []byte{1, 2, 3}}, nil
}
func (m *mutatePlugin) Close(ctx context.Context) error { return nil }

func TestDumpFrames_PluginMutatesReferralContent(t *testing.T) {
	connectPayload := buildConnectPayloadForTest(
		"6708f121966c1c443f4b0eb525b2f81d0a8dc61f5003a692a8fa157e5e02cea9",
		0,
		"d3e6ef90-e113-49a7-a845-1c11f24fe166",
		"de-DE",
		"tok",
		"Krymo",
	)

	frame := make([]byte, 8+len(connectPayload))
	binary.LittleEndian.PutUint32(frame[0:4], uint32(len(connectPayload)))
	binary.LittleEndian.PutUint32(frame[4:8], 0)
	copy(frame[8:], connectPayload)

	rx := &rw{r: bytes.NewReader(frame)}
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))

	s := &Server{logger: logger, plugins: plugins.NewManager(logger, []plugins.Plugin{&mutatePlugin{}})}
	decision := routing.Decision{Backend: routing.Backend{Host: "play.hyvane.com", Port: 5520}}
	s.dumpFrames(context.Background(), nil, rx, logger, decision, nil, plugins.ConnectEvent{})

	out := rx.w.Bytes()
	if len(out) < 8 {
		t.Fatalf("short out")
	}
	if binary.LittleEndian.Uint32(out[4:8]) != 18 {
		t.Fatalf("expected packet 18, got %d", binary.LittleEndian.Uint32(out[4:8]))
	}
	payloadLen := int(binary.LittleEndian.Uint32(out[0:4]))
	p := out[8 : 8+payloadLen]
	if p[0]&0x02 == 0 {
		t.Fatalf("expected data bit")
	}
	dataOffset := int(int32(binary.LittleEndian.Uint32(p[5:9])))
	if dataOffset < 0 {
		t.Fatalf("expected data offset")
	}
	varStart := 9
	pos := varStart + dataOffset
	l, sz, ok := readVarInt(p, pos)
	if !ok || l <= 0 {
		t.Fatalf("len=%d ok=%v", l, ok)
	}
	if pos+sz+l > len(p) {
		t.Fatalf("short data")
	}
	data := p[pos+sz : pos+sz+l]
	env, err := referral.Parse(data)
	if err != nil {
		t.Fatalf("parse envelope: %v", err)
	}
	if len(env.Content) != 3 || env.Content[0] != 1 || env.Content[1] != 2 || env.Content[2] != 3 {
		t.Fatalf("content=%v", env.Content)
	}
}

type errReader struct{}

func (errReader) Read(_ []byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestDumpFrames_InvalidFrameReturns(t *testing.T) {
	frame := make([]byte, 8)
	binary.LittleEndian.PutUint32(frame[0:4], uint32(maxDebugBufferedPayload+1))
	binary.LittleEndian.PutUint32(frame[4:8], 1)

	rx := &rw{r: bytes.NewReader(frame)}
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	s := &Server{logger: logger}
	s.dumpFrames(context.Background(), nil, rx, logger, routing.Decision{}, nil, plugins.ConnectEvent{})
}

func TestDumpFrames_ReadError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	s := &Server{logger: logger}
	s.dumpFrames(context.Background(), nil, errReader{}, logger, routing.Decision{}, nil, plugins.ConnectEvent{})
}

func TestDumpFrames_StreamClosedEOF(t *testing.T) {
	rx := &rw{r: bytes.NewReader(nil)}
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	s := &Server{logger: logger}
	s.dumpFrames(context.Background(), nil, rx, logger, routing.Decision{}, nil, plugins.ConnectEvent{})
}

func (x *rw) Read(p []byte) (int, error)  { return x.r.Read(p) }
func (x *rw) Write(p []byte) (int, error) { return x.w.Write(p) }

func TestDumpFrames_SendsReferralOnConnect(t *testing.T) {
	connectPayload := buildConnectPayloadForTest(
		"6708f121966c1c443f4b0eb525b2f81d0a8dc61f5003a692a8fa157e5e02cea9",
		0,
		"d3e6ef90-e113-49a7-a845-1c11f24fe166",
		"de-DE",
		"tok",
		"Krymo",
	)

	frame := make([]byte, 8+len(connectPayload))
	binary.LittleEndian.PutUint32(frame[0:4], uint32(len(connectPayload)))
	binary.LittleEndian.PutUint32(frame[4:8], 0)
	copy(frame[8:], connectPayload)

	rx := &rw{r: bytes.NewReader(frame)}
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))

	s := &Server{logger: logger}
	decision := routing.Decision{Backend: routing.Backend{Host: "play.hyvane.com", Port: 5520}, Matched: false, RouteIndex: -1}
	s.dumpFrames(context.Background(), nil, rx, logger, decision, nil, plugins.ConnectEvent{})

	out := rx.w.Bytes()
	if len(out) == 0 {
		t.Fatalf("expected referral write")
	}
	if binary.LittleEndian.Uint32(out[4:8]) != 18 {
		t.Fatalf("expected packet 18, got %d", binary.LittleEndian.Uint32(out[4:8]))
	}
	payloadLen := int(binary.LittleEndian.Uint32(out[0:4]))
	p := out[8 : 8+payloadLen]
	// No referral data is expected by default; the server may omit it entirely.
	if p[0]&0x02 != 0 {
		dataOffset := int(int32(binary.LittleEndian.Uint32(p[5:9])))
		if dataOffset < 0 {
			t.Fatalf("expected data offset")
		}
		varStart := 9
		pos := varStart + dataOffset
		l, sz, ok := readVarInt(p, pos)
		if !ok || l < 0 {
			t.Fatalf("len=%d ok=%v", l, ok)
		}
		if pos+sz+l > len(p) {
			t.Fatalf("short data")
		}
		data := p[pos+sz : pos+sz+l]
		env, err := referral.Parse(data)
		if err != nil {
			t.Fatalf("parse envelope: %v", err)
		}
		if len(env.Content) != 0 {
			t.Fatalf("content=%v", env.Content)
		}
	}
}

func TestDumpFrames_NoRouteSendsDisconnect(t *testing.T) {
	connectPayload := buildConnectPayloadForTest(
		"6708f121966c1c443f4b0eb525b2f81d0a8dc61f5003a692a8fa157e5e02cea9",
		0,
		"d3e6ef90-e113-49a7-a845-1c11f24fe166",
		"de-DE",
		"tok",
		"Krymo",
	)
	frame := make([]byte, 8+len(connectPayload))
	binary.LittleEndian.PutUint32(frame[0:4], uint32(len(connectPayload)))
	binary.LittleEndian.PutUint32(frame[4:8], 0)
	copy(frame[8:], connectPayload)

	rx := &rw{r: bytes.NewReader(frame)}
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))

	s := &Server{logger: logger}
	s.dumpFrames(context.Background(), nil, rx, logger, routing.Decision{Matched: false, RouteIndex: -1, SelectedIndex: -1}, nil, plugins.ConnectEvent{SNI: "x"})

	out := rx.w.Bytes()
	if len(out) < 8 {
		t.Fatalf("short out")
	}
	if binary.LittleEndian.Uint32(out[4:8]) != 1 {
		t.Fatalf("expected packet 1, got %d", binary.LittleEndian.Uint32(out[4:8]))
	}
}

func TestDumpFrames_RoutingErrorUsesTemplate(t *testing.T) {
	connectPayload := buildConnectPayloadForTest(
		"6708f121966c1c443f4b0eb525b2f81d0a8dc61f5003a692a8fa157e5e02cea9",
		0,
		"d3e6ef90-e113-49a7-a845-1c11f24fe166",
		"de-DE",
		"tok",
		"Krymo",
	)
	frame := make([]byte, 8+len(connectPayload))
	binary.LittleEndian.PutUint32(frame[0:4], uint32(len(connectPayload)))
	binary.LittleEndian.PutUint32(frame[4:8], 0)
	copy(frame[8:], connectPayload)

	rx := &rw{r: bytes.NewReader(frame)}
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))

	s := &Server{logger: logger, cfg: &config.Config{Messages: config.MessagesConfig{Disconnect: config.DisconnectMessagesConfig{RoutingError: "oops ${sni} ${error}"}}}}
	s.dumpFrames(context.Background(), nil, rx, logger, routing.Decision{Matched: false, RouteIndex: -1, SelectedIndex: -1}, routing.ErrUnknownStrategy, plugins.ConnectEvent{SNI: "example"})

	out := rx.w.Bytes()
	if len(out) < 8 {
		t.Fatalf("short out")
	}
	if binary.LittleEndian.Uint32(out[4:8]) != 1 {
		t.Fatalf("expected packet 1")
	}
	payloadLen := int(binary.LittleEndian.Uint32(out[0:4]))
	p := out[8 : 8+payloadLen]
	if len(p) < 3 {
		t.Fatalf("short payload")
	}
	if p[0] != 0x01 {
		t.Fatalf("nullbits=%02x", p[0])
	}
}
