package routing

import (
	"context"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

type Target struct {
	Host string `json:"host" yaml:"host"`
	Port int    `json:"port" yaml:"port"`
}

type Backend struct {
	Host   string            `json:"host" yaml:"host"`
	Port   int               `json:"port" yaml:"port"`
	Weight int               `json:"weight" yaml:"weight"`
	Meta   map[string]string `json:"meta" yaml:"meta"`
}

func (b Backend) Target() Target {
	return Target{Host: b.Host, Port: b.Port}
}

type Pool struct {
	Strategy  string     `json:"strategy" yaml:"strategy"`
	Key       string     `json:"key" yaml:"key"`
	Sample    int        `json:"sample" yaml:"sample"`
	Sort      []SortKey  `json:"sort" yaml:"sort"`
	Limit     int        `json:"limit" yaml:"limit"`
	Filters   []Filter   `json:"filters" yaml:"filters"`
	Fallback  []Fallback `json:"fallback" yaml:"fallback"`
	Backends  []Backend  `json:"backends" yaml:"backends"`
	Discovery *Discovery `json:"discovery" yaml:"discovery"`
}

type Discovery struct {
	Provider string `json:"provider" yaml:"provider"`
	Mode     string `json:"mode" yaml:"mode"`
}

type SortKey struct {
	Key   string `json:"key" yaml:"key"`
	Order string `json:"order" yaml:"order"`
	Type  string `json:"type" yaml:"type"`
}

type Filter struct {
	Type       string `json:"type" yaml:"type"`
	Subject    string `json:"subject" yaml:"subject"`
	Left       string `json:"left" yaml:"left"`
	Op         string `json:"op" yaml:"op"`
	Right      string `json:"right" yaml:"right"`
	EnabledKey string `json:"enabled_key" yaml:"enabled_key"`
	ListKey    string `json:"list_key" yaml:"list_key"`
	Key        string `json:"key" yaml:"key"`
}

type Fallback struct {
	Strategy *string   `json:"strategy" yaml:"strategy"`
	Key      *string   `json:"key" yaml:"key"`
	Sample   *int      `json:"sample" yaml:"sample"`
	Sort     []SortKey `json:"sort" yaml:"sort"`
	Limit    *int      `json:"limit" yaml:"limit"`
	Filters  []Filter  `json:"filters" yaml:"filters"`
}

type Match struct {
	Hostname  string   `json:"hostname" yaml:"hostname"`
	Hostnames []string `json:"hostnames" yaml:"hostnames"`
}

type Route struct {
	Match Match `json:"match" yaml:"match"`
	Pool  Pool  `json:"pool" yaml:"pool"`
}

type Config struct {
	Default *Pool   `json:"default" yaml:"default"`
	Routes  []Route `json:"routes" yaml:"routes"`
}

type Request struct {
	SNI      string
	UUID     string
	Username string
	Language string
}

type Decision struct {
	Matched       bool      `json:"matched"`
	RouteIndex    int       `json:"route_index"`
	Strategy      string    `json:"strategy"`
	Candidates    []Backend `json:"candidates"`
	SelectedIndex int       `json:"selected_index"`
	Backend       Backend   `json:"backend"`
}

type Engine interface {
	Decide(ctx context.Context, req Request) (Decision, error)
}

type StaticEngine struct {
	cfg       Config
	rr        []atomic.Uint64
	rrDefault atomic.Uint64
	rngMu     sync.Mutex
	rng       *rand.Rand
	discovery func(ctx context.Context, provider string) ([]Backend, error)
}

func NewStaticEngine(cfg Config) *StaticEngine {
	return &StaticEngine{
		cfg: cfg,
		rr:  make([]atomic.Uint64, len(cfg.Routes)),
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (e *StaticEngine) SetDiscovery(fn func(ctx context.Context, provider string) ([]Backend, error)) {
	e.discovery = fn
}
