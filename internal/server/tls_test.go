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
	if len(selected.NextProtos) != 1 || selected.NextProtos[0] != "other/1" {
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
