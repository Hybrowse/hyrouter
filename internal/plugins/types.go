package plugins

import (
	"context"

	"github.com/hybrowse/hyrouter/internal/routing"
)

type ConnectEvent struct {
	SNI                   string `json:"sni"`
	ClientCertFingerprint string `json:"client_cert_fingerprint,omitempty"`
	ProtocolHash          string `json:"protocol_hash,omitempty"`
	ClientType            uint8  `json:"client_type,omitempty"`
	UUID                  string `json:"uuid,omitempty"`
	Username              string `json:"username,omitempty"`
	Language              string `json:"language,omitempty"`
	IdentityTokenPresent  bool   `json:"identity_token_present,omitempty"`
}

type ConnectRequest struct {
	Event        ConnectEvent   `json:"event"`
	Target       routing.Target `json:"target"`
	ReferralData []byte         `json:"referral_data,omitempty"`
}

type ConnectResponse struct {
	Deny         bool            `json:"deny"`
	DenyReason   string          `json:"deny_reason,omitempty"`
	Target       *routing.Target `json:"target,omitempty"`
	ReferralData []byte          `json:"referral_data,omitempty"`
}

type Plugin interface {
	Name() string
	OnConnect(ctx context.Context, req ConnectRequest) (ConnectResponse, error)
	Close(ctx context.Context) error
}
