package server

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hybrowse/hyrouter/internal/config"
)

func TestHasStringIntersection(t *testing.T) {
	if !hasStringIntersection([]string{"a", "b"}, []string{"c", "b"}) {
		t.Fatalf("expected intersection")
	}
	if hasStringIntersection([]string{"a"}, []string{"b"}) {
		t.Fatalf("expected no intersection")
	}
	if hasStringIntersection(nil, []string{"a"}) {
		t.Fatalf("expected no intersection")
	}
	if hasStringIntersection([]string{"a"}, nil) {
		t.Fatalf("expected no intersection")
	}
}

func TestBuildTLSConfig_SelectsALPN(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	s := &Server{cfg: config.Default(), logger: logger}
	cfg, err := s.buildTLSConfig()
	if err != nil {
		t.Fatalf("buildTLSConfig: %v", err)
	}
	if cfg.ClientAuth != tls.RequestClientCert {
		t.Fatalf("clientAuth=%v", cfg.ClientAuth)
	}

	chi := &tls.ClientHelloInfo{ServerName: "localhost", SupportedProtos: []string{"hytale/1"}}
	selected, err := cfg.GetConfigForClient(chi)
	if err != nil {
		t.Fatalf("GetConfigForClient: %v", err)
	}
	if len(selected.NextProtos) == 0 {
		t.Fatalf("expected next protos")
	}

	chi = &tls.ClientHelloInfo{ServerName: "localhost", SupportedProtos: []string{"other/1"}}
	selected, err = cfg.GetConfigForClient(chi)
	if err != nil {
		t.Fatalf("GetConfigForClient: %v", err)
	}
	if len(selected.NextProtos) != 2 || selected.NextProtos[0] != "hytale/2" || selected.NextProtos[1] != "hytale/1" {
		t.Fatalf("unexpected selected protos: %#v", selected.NextProtos)
	}
}

func TestBuildTLSConfig_WildcardALPN(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	c := config.Default()
	c.TLS.ALPN = []string{"hytale/*"}
	s := &Server{cfg: c, logger: logger}
	cfg, err := s.buildTLSConfig()
	if err != nil {
		t.Fatalf("buildTLSConfig: %v", err)
	}

	chi := &tls.ClientHelloInfo{ServerName: "localhost", SupportedProtos: []string{"hytale/1", "hytale/7", "other/1"}}
	selected, err := cfg.GetConfigForClient(chi)
	if err != nil {
		t.Fatalf("GetConfigForClient: %v", err)
	}
	if len(selected.NextProtos) != 2 || selected.NextProtos[0] != "hytale/7" || selected.NextProtos[1] != "hytale/1" {
		t.Fatalf("unexpected selected protos: %#v", selected.NextProtos)
	}
}

func TestCertificateFingerprint(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := x509.Certificate{SerialNumber: big.NewInt(3), NotBefore: time.Now().Add(-1 * time.Hour), NotAfter: time.Now().Add(1 * time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, priv.Public(), priv)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	fp := certificateFingerprint(cert)
	if fp == "" {
		t.Fatalf("expected fingerprint")
	}
}

func TestLoadCertificateFromFiles(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := x509.Certificate{SerialNumber: big.NewInt(4), NotBefore: time.Now().Add(-1 * time.Hour), NotAfter: time.Now().Add(1 * time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, priv.Public(), priv)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	pkcs8, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})

	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	cfg := config.Default()
	cfg.TLS.CertFile = certPath
	cfg.TLS.KeyFile = keyPath
	s := &Server{cfg: cfg, logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))}
	if _, err := s.loadCertificate(); err != nil {
		t.Fatalf("loadCertificate: %v", err)
	}
}

func TestCertificateFingerprintNil(t *testing.T) {
	if certificateFingerprint(nil) != "" {
		t.Fatalf("expected empty")
	}
}

func TestLoadCertificateFromFilesError(t *testing.T) {
	cfg := config.Default()
	cfg.TLS.CertFile = "missing"
	cfg.TLS.KeyFile = "missing"
	s := &Server{cfg: cfg, logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))}
	if _, err := s.loadCertificate(); err == nil {
		t.Fatalf("expected error")
	}
}

// Signature schemes captured from the actual 0.7.4 client's ClientHello.
// Its quiche transport does not advertise Ed25519 for TLS certificates.
func TestDefaultCertificateSupportsHytaleClient(t *testing.T) {
	s := &Server{cfg: config.Default()}
	cert, err := s.loadCertificate()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"localhost", "127.0.0.1", "::1"} {
		hello := &tls.ClientHelloInfo{
			ServerName:        name,
			SupportedVersions: []uint16{tls.VersionTLS13},
			CipherSuites:      []uint16{tls.TLS_AES_128_GCM_SHA256},
			SupportedCurves:   []tls.CurveID{tls.X25519, tls.CurveP256},
			SignatureSchemes: []tls.SignatureScheme{
				tls.ECDSAWithP256AndSHA256, tls.PSSWithSHA256, tls.PKCS1WithSHA256,
				tls.ECDSAWithP384AndSHA384, tls.PSSWithSHA384, tls.PKCS1WithSHA384,
				tls.PSSWithSHA512, tls.PKCS1WithSHA512, tls.PKCS1WithSHA1,
			},
		}
		if err := hello.SupportsCertificate(&cert); err != nil {
			t.Errorf("Hytale client cannot use default certificate for %s: %v", name, err)
		}
	}
}
