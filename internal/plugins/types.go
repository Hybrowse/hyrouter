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
	Event           ConnectEvent      `json:"event"`
	Strategy        string            `json:"strategy"`
	Candidates      []routing.Backend `json:"candidates"`
	SelectedIndex   int               `json:"selected_index"`
	Backend         routing.Backend   `json:"backend"`
	ReferralContent []byte            `json:"referral_content,omitempty"`
}

type ConnectResponse struct {
	Deny            bool              `json:"deny"`
	DenyReason      string            `json:"deny_reason,omitempty"`
	Candidates      []routing.Backend `json:"candidates,omitempty"`
	SelectedIndex   *int              `json:"selected_index,omitempty"`
	Backend         *routing.Backend  `json:"backend,omitempty"`
	ReferralContent []byte            `json:"referral_content,omitempty"`
}

type Plugin interface {
	Name() string
	OnConnect(ctx context.Context, req ConnectRequest) (ConnectResponse, error)
	Close(ctx context.Context) error
}
