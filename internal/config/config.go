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
)

type Config struct {
	Listen  string         `json:"listen" yaml:"listen"`
	TLS     TLSConfig      `json:"tls" yaml:"tls"`
	QUIC    QUICConfig     `json:"quic" yaml:"quic"`
	Routing routing.Config `json:"routing" yaml:"routing"`
	Plugins []PluginConfig `json:"plugins" yaml:"plugins"`
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
			ALPN: []string{"hytale/1"},
		},
		QUIC: QUICConfig{
			MaxIdleTimeout: "30s",
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
	return nil
}
