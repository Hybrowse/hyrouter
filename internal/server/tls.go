package server

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"math/big"
	"time"
)

func (s *Server) buildTLSConfig() (*tls.Config, error) {
	cert, err := s.loadCertificate()
	if err != nil {
		return nil, err
	}

	nextProtos := s.cfg.TLS.ALPN
	if len(nextProtos) == 0 {
		nextProtos = []string{"hytale/1"}
	}

	baseConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   nextProtos,
		ClientAuth:   tls.RequestClientCert,
	}

	baseConfig.GetConfigForClient = func(info *tls.ClientHelloInfo) (*tls.Config, error) {
		remote := ""
		if info.Conn != nil && info.Conn.RemoteAddr() != nil {
			remote = info.Conn.RemoteAddr().String()
		}

		s.logger.Debug(
			"client hello",
			"remote_addr", remote,
			"sni", info.ServerName,
			"alpn_offered", info.SupportedProtos,
		)

		selected := nextProtos
		if len(info.SupportedProtos) > 0 && !hasStringIntersection(nextProtos, info.SupportedProtos) {
			selected = info.SupportedProtos
		}

		cfg := baseConfig.Clone()
		cfg.GetConfigForClient = nil
		cfg.NextProtos = selected
		return cfg, nil
	}

	return baseConfig, nil
}

func hasStringIntersection(a []string, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(a))
	for _, s := range a {
		set[s] = struct{}{}
	}
	for _, s := range b {
		if _, ok := set[s]; ok {
			return true
		}
	}
	return false
}

func (s *Server) loadCertificate() (tls.Certificate, error) {
	if s.cfg.TLS.CertFile != "" || s.cfg.TLS.KeyFile != "" {
		return tls.LoadX509KeyPair(s.cfg.TLS.CertFile, s.cfg.TLS.KeyFile)
	}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, priv.Public(), priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	return tls.Certificate{Certificate: [][]byte{certDER}, PrivateKey: priv}, nil
}

func certificateFingerprint(cert *x509.Certificate) string {
	if cert == nil {
		return ""
	}
	sum := sha256.Sum256(cert.Raw)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
