package server

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hybrowse/hyrouter/internal/config"
	"github.com/hybrowse/hyrouter/internal/routing"
	"github.com/quic-go/quic-go"
)

type captureHandler struct {
	state *captureState
	attrs []slog.Attr
}

type captureState struct {
	uniAccepted atomic.Bool
}

func newCaptureHandler() *captureHandler {
	return &captureHandler{state: &captureState{}}
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Message == "accepted stream" {
		streamType := ""
		for _, a := range h.attrs {
			if a.Key == "stream_type" {
				streamType = a.Value.String()
			}
		}
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "stream_type" {
				streamType = a.Value.String()
			}
			return true
		})
		if streamType == "uni" {
			h.state.uniAccepted.Store(true)
		}
	}
	return nil
}

func (h *captureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	n := &captureHandler{state: h.state}
	if len(h.attrs) > 0 {
		n.attrs = append(n.attrs, h.attrs...)
	}
	if len(attrs) > 0 {
		n.attrs = append(n.attrs, attrs...)
	}
	return n
}

func (h *captureHandler) WithGroup(_ string) slog.Handler { return h }

func (h *captureHandler) waitUniAccepted(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if h.state.uniAccepted.Load() {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("timeout")
}

func TestEndToEndReferralRedirect(t *testing.T) {
	addr := reserveUDPAddr(t)

	cfg := config.Default()
	cfg.Listen = addr
	cfg.Routing.Default = &routing.Target{Host: "play.hyvane.com", Port: 5520}

	h := newCaptureHandler()
	logger := slog.New(h)
	srv := New(cfg, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = srv.Run(ctx)
	}()

	conn := dialQUIC(t, addr)
	defer conn.CloseWithError(0, "done")

	uni, err := conn.OpenUniStreamSync(ctx)
	if err != nil {
		t.Fatalf("OpenUniStreamSync: %v", err)
	}
	if _, err := uni.Write([]byte{0, 0, 0, 0}); err != nil {
		t.Fatalf("uni write: %v", err)
	}
	_ = uni.Close()
	if err := h.waitUniAccepted(2 * time.Second); err != nil {
		t.Fatalf("wait uni accepted: %v", err)
	}

	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		t.Fatalf("OpenStreamSync: %v", err)
	}

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

	if _, err := stream.Write(frame); err != nil {
		t.Fatalf("Write connect frame: %v", err)
	}

	buf := make([]byte, 4096)
	n, err := stream.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if n < 8 {
		t.Fatalf("short read: %d", n)
	}

	payloadLen := int(binary.LittleEndian.Uint32(buf[0:4]))
	packetID := int(binary.LittleEndian.Uint32(buf[4:8]))
	if packetID != 18 {
		t.Fatalf("expected referral packet 18, got %d", packetID)
	}
	if 8+payloadLen > n {
		deadline := time.Now().Add(2 * time.Second)
		for 8+payloadLen > n && time.Now().Before(deadline) {
			more, e := stream.Read(buf[n:])
			n += more
			if e != nil {
				break
			}
		}
	}
	if 8+payloadLen > n {
		t.Fatalf("incomplete referral frame: have %d want %d", n, 8+payloadLen)
	}

	payload := buf[8 : 8+payloadLen]

	nullBits := payload[0]
	if nullBits != 0x01 {
		t.Fatalf("nullBits=%02x", nullBits)
	}

	hostToOff := int32(binary.LittleEndian.Uint32(payload[1:5]))
	if hostToOff != 0 {
		t.Fatalf("hostToOff=%d", hostToOff)
	}

	haPos := 9 + int(hostToOff)
	ha, _, ok := decodeHostAddress(payload, haPos)
	if !ok {
		t.Fatalf("decodeHostAddress failed")
	}
	if ha.Host != "play.hyvane.com" {
		t.Fatalf("host=%q", ha.Host)
	}
	if ha.Port != 5520 {
		t.Fatalf("port=%d", ha.Port)
	}
}

func makeClientCertificate(t *testing.T) tls.Certificate {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(2),
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}

	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, priv.Public(), priv)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}

	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv}
}

func dialQUIC(t *testing.T, addr string) *quic.Conn {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientCert := makeClientCertificate(t)

	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"hytale/1"},
		ServerName:         "localhost",
		Certificates:       []tls.Certificate{clientCert},
	}

	var lastErr error
	for i := 0; i < 50; i++ {
		c, err := quic.DialAddr(ctx, addr, tlsConf, &quic.Config{})
		if err == nil {
			return c
		}
		lastErr = err
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("DialAddr: %v", lastErr)
	return nil
}

func reserveUDPAddr(t *testing.T) string {
	l, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("ListenUDP: %v", err)
	}
	port := l.LocalAddr().(*net.UDPAddr).Port
	_ = l.Close()
	return fmt.Sprintf("127.0.0.1:%d", port)
}
