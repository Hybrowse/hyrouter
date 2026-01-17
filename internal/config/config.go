package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hybrowse/hyrouter/internal/routing"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/labels"
)

type Config struct {
	Listen    string           `json:"listen" yaml:"listen"`
	TLS       TLSConfig        `json:"tls" yaml:"tls"`
	QUIC      QUICConfig       `json:"quic" yaml:"quic"`
	Routing   routing.Config   `json:"routing" yaml:"routing"`
	Referral  *ReferralConfig  `json:"referral" yaml:"referral"`
	Plugins   []PluginConfig   `json:"plugins" yaml:"plugins"`
	Discovery *DiscoveryConfig `json:"discovery" yaml:"discovery"`
	Messages  MessagesConfig   `json:"messages" yaml:"messages"`
	Logging   LoggingConfig    `json:"logging" yaml:"logging"`
}

type LoggingConfig struct {
	LogClientIP bool `json:"log_client_ip" yaml:"log_client_ip"`
}

type ReferralConfig struct {
	KeyID      uint8  `json:"key_id" yaml:"key_id"`
	HMACSecret string `json:"hmac_secret" yaml:"hmac_secret"`
}

type MessagesConfig struct {
	Disconnect        DisconnectMessagesConfig            `json:"disconnect" yaml:"disconnect"`
	DisconnectLocales map[string]DisconnectMessagesConfig `json:"disconnect_locales" yaml:"disconnect_locales"`
}

type DisconnectMessagesConfig struct {
	NoRoute        string `json:"no_route" yaml:"no_route"`
	NoBackends     string `json:"no_backends" yaml:"no_backends"`
	RoutingError   string `json:"routing_error" yaml:"routing_error"`
	DiscoveryError string `json:"discovery_error" yaml:"discovery_error"`
}

type TLSConfig struct {
	CertFile string   `json:"cert_file" yaml:"cert_file"`
	KeyFile  string   `json:"key_file" yaml:"key_file"`
	ALPN     []string `json:"alpn" yaml:"alpn"`
}

type QUICConfig struct {
	MaxIdleTimeout string `json:"max_idle_timeout" yaml:"max_idle_timeout"`
}

type PluginConfig struct {
	Name   string            `json:"name" yaml:"name"`
	Type   string            `json:"type" yaml:"type"`
	Stage  string            `json:"stage" yaml:"stage"`
	Before []string          `json:"before" yaml:"before"`
	After  []string          `json:"after" yaml:"after"`
	GRPC   *GRPCPluginConfig `json:"grpc" yaml:"grpc"`
	WASM   *WASMPluginConfig `json:"wasm" yaml:"wasm"`
}

type GRPCPluginConfig struct {
	Address string `json:"address" yaml:"address"`
}

type WASMPluginConfig struct {
	Path string `json:"path" yaml:"path"`
}

func Default() *Config {
	return &Config{
		Listen: ":5520",
		TLS: TLSConfig{
			ALPN: []string{"hytale/*"},
		},
		QUIC: QUICConfig{
			MaxIdleTimeout: "30s",
		},
		Logging: LoggingConfig{
			LogClientIP: true,
		},
		Messages: MessagesConfig{
			Disconnect: DisconnectMessagesConfig{
				NoRoute:        "The server is currently unavailable.",
				NoBackends:     "The server is full or restarting. Please try again in a moment.",
				RoutingError:   "The server is currently unreachable. Please try again later.",
				DiscoveryError: "The server is looking for an available instance. Please try again in a moment.",
			},
		},
	}
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := Default()

	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(b, cfg); err != nil {
			return nil, fmt.Errorf("parse yaml config: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(b, cfg); err != nil {
			return nil, fmt.Errorf("parse json config: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported config extension: %q", filepath.Ext(path))
	}

	if cfg.Messages.DisconnectLocales == nil {
		cfg.Messages.DisconnectLocales = map[string]DisconnectMessagesConfig{
			"de": {
				NoRoute:        "Der Server ist aktuell nicht verfügbar.",
				NoBackends:     "Der Server ist gerade voll oder startet neu. Bitte versuche es gleich erneut.",
				RoutingError:   "Der Server ist aktuell nicht erreichbar. Bitte versuche es später erneut.",
				DiscoveryError: "Der Server sucht gerade eine freie Instanz. Bitte versuche es gleich erneut.",
			},
			"fr": {
				NoRoute:        "Le serveur est actuellement indisponible.",
				NoBackends:     "Le serveur est plein ou redémarre. Réessaie dans un instant.",
				RoutingError:   "Le serveur est actuellement inaccessible. Réessaie plus tard.",
				DiscoveryError: "Le serveur cherche une instance disponible. Réessaie dans un instant.",
			},
			"es": {
				NoRoute:        "El servidor no está disponible en este momento.",
				NoBackends:     "El servidor está lleno o reiniciándose. Inténtalo de nuevo en un momento.",
				RoutingError:   "No se puede acceder al servidor en este momento. Inténtalo más tarde.",
				DiscoveryError: "El servidor está buscando una instancia disponible. Inténtalo de nuevo en un momento.",
			},
			"pt": {
				NoRoute:        "O servidor não está disponível no momento.",
				NoBackends:     "O servidor está cheio ou reiniciando. Tente novamente em instantes.",
				RoutingError:   "Não foi possível acessar o servidor no momento. Tente novamente mais tarde.",
				DiscoveryError: "O servidor está procurando uma instância disponível. Tente novamente em instantes.",
			},
			"pt-BR": {
				NoRoute:        "O servidor está indisponível no momento.",
				NoBackends:     "O servidor está cheio ou reiniciando. Tente novamente em instantes.",
				RoutingError:   "O servidor está inacessível no momento. Tente novamente mais tarde.",
				DiscoveryError: "O servidor está procurando uma instância disponível. Tente novamente em instantes.",
			},
			"it": {
				NoRoute:        "Il server non è disponibile al momento.",
				NoBackends:     "Il server è pieno o si sta riavviando. Riprova tra un momento.",
				RoutingError:   "Il server non è raggiungibile al momento. Riprova più tardi.",
				DiscoveryError: "Il server sta cercando un'istanza disponibile. Riprova tra un momento.",
			},
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.Listen == "" {
		return fmt.Errorf("listen must not be empty")
	}
	if (c.TLS.CertFile == "") != (c.TLS.KeyFile == "") {
		return fmt.Errorf("tls.cert_file and tls.key_file must be set together")
	}
	if len(c.TLS.ALPN) == 0 {
		return fmt.Errorf("tls.alpn must not be empty")
	}
	if c.QUIC.MaxIdleTimeout != "" {
		if _, err := time.ParseDuration(c.QUIC.MaxIdleTimeout); err != nil {
			return fmt.Errorf("invalid quic.max_idle_timeout: %w", err)
		}
	}
	if err := c.Routing.Validate(); err != nil {
		return err
	}
	if c.Referral != nil {
		_ = c.Referral.KeyID
		_ = c.Referral.HMACSecret
	}
	seen := map[string]struct{}{}
	for i, p := range c.Plugins {
		if p.Name == "" {
			return fmt.Errorf("plugins[%d].name must not be empty", i)
		}
		if _, ok := seen[p.Name]; ok {
			return fmt.Errorf("plugins[%d].name must be unique", i)
		}
		seen[p.Name] = struct{}{}
		if p.Stage != "" {
			s := strings.ToLower(p.Stage)
			if s != "deny" && s != "route" && s != "mutate" {
				return fmt.Errorf("plugins[%d].stage must be one of: deny, route, mutate", i)
			}
		}
		switch strings.ToLower(p.Type) {
		case "grpc":
			if p.GRPC == nil || p.GRPC.Address == "" {
				return fmt.Errorf("plugins[%d].grpc.address must not be empty", i)
			}
		case "wasm":
			if p.WASM == nil || p.WASM.Path == "" {
				return fmt.Errorf("plugins[%d].wasm.path must not be empty", i)
			}
		default:
			return fmt.Errorf("plugins[%d].type must be one of: grpc, wasm", i)
		}
	}
	providers := map[string]struct{}{}
	if c.Discovery != nil {
		if err := c.Discovery.Validate(); err != nil {
			return err
		}
		for _, p := range c.Discovery.Providers {
			providers[p.Name] = struct{}{}
		}
	}
	if err := validateRoutingDiscoveryRefs(c.Routing, providers); err != nil {
		return err
	}
	return nil
}

func validateRoutingDiscoveryRefs(r routing.Config, providers map[string]struct{}) error {
	checkPool := func(path string, p routing.Pool) error {
		if p.Discovery == nil {
			return nil
		}
		if len(providers) == 0 {
			return fmt.Errorf("%s: discovery is configured but top-level discovery section is missing", path)
		}
		if _, ok := providers[p.Discovery.Provider]; !ok {
			return fmt.Errorf("%s: unknown discovery provider %q", path, p.Discovery.Provider)
		}
		return nil
	}
	if r.Default != nil {
		if err := checkPool("routing.default", *r.Default); err != nil {
			return err
		}
	}
	for i, rt := range r.Routes {
		if err := checkPool(fmt.Sprintf("routing.routes[%d].pool", i), rt.Pool); err != nil {
			return err
		}
	}
	return nil
}

type DiscoveryConfig struct {
	Providers []DiscoveryProviderConfig `json:"providers" yaml:"providers"`
}

type DiscoveryProviderConfig struct {
	Name       string                     `json:"name" yaml:"name"`
	Type       string                     `json:"type" yaml:"type"`
	Kubernetes *KubernetesDiscoveryConfig `json:"kubernetes" yaml:"kubernetes"`
	Agones     *AgonesDiscoveryConfig     `json:"agones" yaml:"agones"`
}

type KubernetesDiscoveryConfig struct {
	Kubeconfig string                     `json:"kubeconfig" yaml:"kubeconfig"`
	Namespaces []string                   `json:"namespaces" yaml:"namespaces"`
	Resources  []KubernetesResourceConfig `json:"resources" yaml:"resources"`
	Filters    KubernetesFilterConfig     `json:"filters" yaml:"filters"`
	Metadata   KubernetesMetadataConfig   `json:"metadata" yaml:"metadata"`
}

type KubernetesResourceConfig struct {
	Kind     string                `json:"kind" yaml:"kind"`
	Service  *KubernetesServiceRef `json:"service" yaml:"service"`
	Selector *KubernetesSelector   `json:"selector" yaml:"selector"`
	Port     KubernetesPortConfig  `json:"port" yaml:"port"`
}

type KubernetesServiceRef struct {
	Name      string `json:"name" yaml:"name"`
	Namespace string `json:"namespace" yaml:"namespace"`
}

type KubernetesSelector struct {
	Labels      string `json:"labels" yaml:"labels"`
	Annotations string `json:"annotations" yaml:"annotations"`
}

type KubernetesPortConfig struct {
	Name          string `json:"name" yaml:"name"`
	Number        int    `json:"number" yaml:"number"`
	ContainerPort int    `json:"container_port" yaml:"container_port"`
}

type KubernetesFilterConfig struct {
	RequirePodReady      bool     `json:"require_pod_ready" yaml:"require_pod_ready"`
	RequirePodPhase      []string `json:"require_pod_phase" yaml:"require_pod_phase"`
	RequireEndpointReady bool     `json:"require_endpoint_ready" yaml:"require_endpoint_ready"`
}

type KubernetesMetadataConfig struct {
	IncludeLabels      []string `json:"include_labels" yaml:"include_labels"`
	IncludeAnnotations []string `json:"include_annotations" yaml:"include_annotations"`
}

type AgonesAddressConfig struct {
	Source     string   `json:"source" yaml:"source"`
	Preference []string `json:"preference" yaml:"preference"`
}

type AgonesDiscoveryConfig struct {
	Kubeconfig          string                   `json:"kubeconfig" yaml:"kubeconfig"`
	Namespaces          []string                 `json:"namespaces" yaml:"namespaces"`
	Mode                string                   `json:"mode" yaml:"mode"`
	AllocateMinInterval string                   `json:"allocate_min_interval" yaml:"allocate_min_interval"`
	State               []string                 `json:"state" yaml:"state"`
	Selector            *KubernetesSelector      `json:"selector" yaml:"selector"`
	Metadata            KubernetesMetadataConfig `json:"metadata" yaml:"metadata"`
	Address             *AgonesAddressConfig     `json:"address" yaml:"address"`
	Port                KubernetesPortConfig     `json:"port" yaml:"port"`
}

func (c *DiscoveryConfig) Validate() error {
	if c == nil {
		return nil
	}
	seen := map[string]struct{}{}
	for i, p := range c.Providers {
		if p.Name == "" {
			return fmt.Errorf("discovery.providers[%d].name must not be empty", i)
		}
		if _, ok := seen[p.Name]; ok {
			return fmt.Errorf("discovery.providers[%d].name must be unique", i)
		}
		seen[p.Name] = struct{}{}
		switch strings.ToLower(strings.TrimSpace(p.Type)) {
		case "kubernetes":
			if p.Kubernetes == nil {
				return fmt.Errorf("discovery.providers[%d].kubernetes must be set", i)
			}
			for j, r := range p.Kubernetes.Resources {
				if r.Selector != nil {
					labelExpr := strings.TrimSpace(r.Selector.Labels)
					if labelExpr != "" {
						if _, err := labels.Parse(labelExpr); err != nil {
							return fmt.Errorf("discovery.providers[%d].kubernetes.resources[%d].selector.labels is invalid: %w", i, j, err)
						}
					}
					annExpr := strings.TrimSpace(r.Selector.Annotations)
					if annExpr != "" {
						if err := validateAnnotationSelector(annExpr); err != nil {
							return fmt.Errorf("discovery.providers[%d].kubernetes.resources[%d].selector.annotations is invalid: %w", i, j, err)
						}
					}
				}
			}
		case "agones":
			if p.Agones == nil {
				return fmt.Errorf("discovery.providers[%d].agones must be set", i)
			}
			if p.Agones.Selector != nil {
				labelExpr := strings.TrimSpace(p.Agones.Selector.Labels)
				if labelExpr != "" {
					if _, err := labels.Parse(labelExpr); err != nil {
						return fmt.Errorf("discovery.providers[%d].agones.selector.labels is invalid: %w", i, err)
					}
				}
				annExpr := strings.TrimSpace(p.Agones.Selector.Annotations)
				if annExpr != "" {
					if err := validateAnnotationSelector(annExpr); err != nil {
						return fmt.Errorf("discovery.providers[%d].agones.selector.annotations is invalid: %w", i, err)
					}
				}
			}
			if p.Agones.Address != nil {
				addrSrc := strings.ToLower(strings.TrimSpace(p.Agones.Address.Source))
				if addrSrc != "" {
					if addrSrc != "address" && addrSrc != "addresses" {
						return fmt.Errorf("discovery.providers[%d].agones.address.source must be one of: address, addresses", i)
					}
				}
			}
			if strings.TrimSpace(p.Agones.AllocateMinInterval) != "" {
				if _, err := time.ParseDuration(p.Agones.AllocateMinInterval); err != nil {
					return fmt.Errorf("discovery.providers[%d].agones.allocate_min_interval is invalid: %w", i, err)
				}
			}
		default:
			return fmt.Errorf("discovery.providers[%d].type must be one of: kubernetes, agones", i)
		}
	}
	return nil
}

func validateAnnotationSelector(expr string) error {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil
	}
	parts := strings.Split(expr, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 {
			return fmt.Errorf("invalid selector token %q", p)
		}
		k := strings.TrimSpace(kv[0])
		v := strings.TrimSpace(kv[1])
		if k == "" || v == "" {
			return fmt.Errorf("invalid selector token %q", p)
		}
	}
	return nil
}
