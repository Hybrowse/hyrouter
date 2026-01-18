package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"testing"

	"github.com/hybrowse/hyrouter/internal/plugins"
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
	s.dumpFrames(context.Background(), rx, logger, decision, plugins.ConnectEvent{})

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

type mutatePlugin struct{}

func (m *mutatePlugin) Name() string { return "mut" }
func (m *mutatePlugin) OnConnect(ctx context.Context, req plugins.ConnectRequest) (plugins.ConnectResponse, error) {
	_ = ctx
	_ = req
	return plugins.ConnectResponse{ReferralData: []byte{1, 2, 3}}, nil
}
func (m *mutatePlugin) Close(ctx context.Context) error { return nil }

func TestDumpFrames_PluginMutatesReferralData(t *testing.T) {
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
	s.dumpFrames(context.Background(), rx, logger, decision, plugins.ConnectEvent{})

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
	if !ok || l != 3 {
		t.Fatalf("len=%d ok=%v", l, ok)
	}
	data := p[pos+sz : pos+sz+l]
	if len(data) != 3 || data[0] != 1 || data[1] != 2 || data[2] != 3 {
		t.Fatalf("data=%v", data)
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
	s.dumpFrames(context.Background(), rx, logger, routing.Decision{}, plugins.ConnectEvent{})
}

func TestDumpFrames_ReadError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	s := &Server{logger: logger}
	s.dumpFrames(context.Background(), errReader{}, logger, routing.Decision{}, plugins.ConnectEvent{})
}

func TestDumpFrames_StreamClosedEOF(t *testing.T) {
	rx := &rw{r: bytes.NewReader(nil)}
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	s := &Server{logger: logger}
	s.dumpFrames(context.Background(), rx, logger, routing.Decision{}, plugins.ConnectEvent{})
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
	s.dumpFrames(context.Background(), rx, logger, decision, plugins.ConnectEvent{})

	out := rx.w.Bytes()
	if len(out) == 0 {
		t.Fatalf("expected referral write")
	}
	if binary.LittleEndian.Uint32(out[4:8]) != 18 {
		t.Fatalf("expected packet 18, got %d", binary.LittleEndian.Uint32(out[4:8]))
	}
}
