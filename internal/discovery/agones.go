package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hybrowse/hyrouter/internal/config"
	"github.com/hybrowse/hyrouter/internal/routing"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	dynamicinformer "k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

type agonesProvider struct {
	name   string
	cfg    *config.AgonesDiscoveryConfig
	logger *slog.Logger

	client  dynamic.Interface
	factory dynamicinformer.DynamicSharedInformerFactory
	inf     cache.SharedIndexInformer

	rebuildMu sync.Mutex
	snapshot  atomic.Value

	allocateMu   sync.Mutex
	nextAllocate time.Time

	startOnce sync.Once
	startErr  error
}

var (
	gsGVR  = schema.GroupVersionResource{Group: "agones.dev", Version: "v1", Resource: "gameservers"}
	gsaGVR = schema.GroupVersionResource{Group: "allocation.agones.dev", Version: "v1", Resource: "gameserverallocations"}
)

func newAgonesProvider(name string, cfg *config.AgonesDiscoveryConfig, logger *slog.Logger) (*agonesProvider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("discovery provider %q: agones config must be set", name)
	}
	if logger == nil {
		logger = slog.Default()
	}
	p := &agonesProvider{name: name, cfg: cfg, logger: logger}
	p.snapshot.Store([]routing.Backend(nil))
	return p, nil
}

func (p *agonesProvider) Start(ctx context.Context) error {
	p.startOnce.Do(func() {
		restCfg, err := agonesRESTConfig(p.cfg.Kubeconfig)
		if err != nil {
			p.startErr = err
			return
		}
		client, err := dynamic.NewForConfig(restCfg)
		if err != nil {
			p.startErr = err
			return
		}
		p.client = client

		selector := ""
		if p.cfg.Selector != nil {
			selector = strings.TrimSpace(p.cfg.Selector.Labels)
			if selector != "" {
				if _, err := labels.Parse(selector); err != nil {
					p.startErr = fmt.Errorf("discovery provider %q: invalid selector.labels: %w", p.name, err)
					return
				}
			}
		}

		p.factory = dynamicinformer.NewFilteredDynamicSharedInformerFactory(client, 0, metav1.NamespaceAll, func(lo *metav1.ListOptions) {
			if selector != "" {
				lo.LabelSelector = selector
			}
		})
		p.inf = p.factory.ForResource(gsGVR).Informer()
		p.inf.AddEventHandler(cache.ResourceEventHandlerFuncs{ // nolint:errcheck
			AddFunc:    func(_ interface{}) { p.rebuild() },
			UpdateFunc: func(_, _ interface{}) { p.rebuild() },
			DeleteFunc: func(_ interface{}) { p.rebuild() },
		})

		p.factory.Start(ctx.Done())
		if !cache.WaitForCacheSync(ctx.Done(), p.inf.HasSynced) {
			p.startErr = fmt.Errorf("discovery provider %q: cache sync failed", p.name)
			return
		}
		p.rebuild()
		go wait.UntilWithContext(ctx, func(ctx context.Context) { p.rebuild() }, 30*time.Second)
	})
	return p.startErr
}

func (p *agonesProvider) Resolve(ctx context.Context) ([]routing.Backend, error) {
	mode := strings.ToLower(strings.TrimSpace(p.cfg.Mode))
	if mode == "" || mode == "observe" {
		v := p.snapshot.Load()
		if v == nil {
			return nil, nil
		}
		bs := v.([]routing.Backend)
		out := make([]routing.Backend, len(bs))
		copy(out, bs)
		return out, nil
	}
	if mode != "allocate" {
		return nil, fmt.Errorf("discovery provider %q: unknown agones mode %q", p.name, p.cfg.Mode)
	}
	return p.allocate(ctx)
}

func (p *agonesProvider) rebuild() {
	p.rebuildMu.Lock()
	defer p.rebuildMu.Unlock()

	if p.inf == nil {
		return
	}

	allowedNS := map[string]struct{}{}
	for _, ns := range p.cfg.Namespaces {
		ns = strings.TrimSpace(ns)
		if ns != "" {
			allowedNS[ns] = struct{}{}
		}
	}

	allowedStates := map[string]struct{}{}
	for _, st := range p.cfg.State {
		st = strings.ToLower(strings.TrimSpace(st))
		if st != "" {
			allowedStates[st] = struct{}{}
		}
	}
	if len(allowedStates) == 0 {
		allowedStates["ready"] = struct{}{}
	}

	var out []routing.Backend
	for _, obj := range p.inf.GetStore().List() {
		u, ok := obj.(*unstructured.Unstructured)
		if !ok {
			continue
		}
		ns := u.GetNamespace()
		if len(allowedNS) > 0 {
			if _, ok := allowedNS[ns]; !ok {
				continue
			}
		}
		if p.cfg.Selector != nil {
			if !annotationsMatch(u.GetAnnotations(), p.cfg.Selector.Annotations) {
				continue
			}
		}
		b, ok := toBackendFromGameServer(u, p.cfg, allowedStates)
		if !ok {
			continue
		}
		applyWeight(&b)
		out = append(out, b)
	}

	p.snapshot.Store(out)
}

func (p *agonesProvider) allocate(ctx context.Context) ([]routing.Backend, error) {
	if p.client == nil {
		return nil, fmt.Errorf("discovery provider %q: not started", p.name)
	}
	if d, ok, err := p.allocateMinInterval(); err != nil {
		return nil, err
	} else if ok {
		wait := p.reserveAllocateSlot(d)
		if wait > 0 {
			t := time.NewTimer(wait)
			defer t.Stop()
			select {
			case <-t.C:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	ns := "default"
	if len(p.cfg.Namespaces) > 0 {
		ns = p.cfg.Namespaces[0]
	}
	state := "Ready"
	if len(p.cfg.State) > 0 {
		state = p.cfg.State[0]
	}

	labelsMap := parseLabelEqualsMap("")
	if p.cfg.Selector != nil {
		labelsMap = parseLabelEqualsMap(p.cfg.Selector.Labels)
	}

	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "allocation.agones.dev/v1",
		"kind":       "GameServerAllocation",
		"metadata": map[string]interface{}{
			"generateName": "hyrouter-",
		},
		"spec": map[string]interface{}{
			"gameServerState": state,
			"required": map[string]interface{}{
				"matchLabels": labelsMap,
			},
		},
	}}

	created, err := p.client.Resource(gsaGVR).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	status, _, _ := unstructured.NestedMap(created.Object, "status")
	addr := resolveAgonesAddress(status, p.cfg)
	ports, _, _ := unstructured.NestedSlice(status, "ports")
	port := resolveAgonesPorts(ports, p.cfg.Port)
	if addr == "" || port == 0 {
		return nil, fmt.Errorf("allocation returned empty address/port")
	}
	b := routing.Backend{Host: addr, Port: port, Meta: map[string]string{}}
	fillK8sMeta(b.Meta, ns, created.GetName(), "")
	return []routing.Backend{b}, nil
}

func (p *agonesProvider) allocateMinInterval() (time.Duration, bool, error) {
	if p == nil || p.cfg == nil {
		return 0, false, nil
	}
	v := strings.TrimSpace(p.cfg.AllocateMinInterval)
	if v == "" {
		return 0, false, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, false, fmt.Errorf("discovery provider %q: invalid agones.allocate_min_interval: %w", p.name, err)
	}
	if d <= 0 {
		return 0, false, nil
	}
	return d, true, nil
}

func (p *agonesProvider) reserveAllocateSlot(minInterval time.Duration) time.Duration {
	p.allocateMu.Lock()
	defer p.allocateMu.Unlock()

	now := time.Now()
	wait := time.Duration(0)
	if !p.nextAllocate.IsZero() && now.Before(p.nextAllocate) {
		wait = p.nextAllocate.Sub(now)
		now = p.nextAllocate
	}
	p.nextAllocate = now.Add(minInterval)
	return wait
}

func toBackendFromGameServer(u *unstructured.Unstructured, cfg *config.AgonesDiscoveryConfig, allowedStates map[string]struct{}) (routing.Backend, bool) {
	status, _, _ := unstructured.NestedMap(u.Object, "status")
	state, _, _ := unstructured.NestedString(status, "state")
	stateLower := strings.ToLower(strings.TrimSpace(state))
	if stateLower == "shutdown" {
		return routing.Backend{}, false
	}
	if len(allowedStates) > 0 {
		if _, ok := allowedStates[stateLower]; !ok {
			return routing.Backend{}, false
		}
	}
	addr := resolveAgonesAddress(status, cfg)
	ports, _, _ := unstructured.NestedSlice(status, "ports")
	portCfg := config.KubernetesPortConfig{}
	if cfg != nil {
		portCfg = cfg.Port
	}
	port := resolveAgonesPorts(ports, portCfg)
	if addr == "" || port == 0 {
		return routing.Backend{}, false
	}

	meta := map[string]string{}
	fillK8sMeta(meta, u.GetNamespace(), u.GetName(), "")
	meta["gameserver.state"] = state
	if v := u.GetLabels()["hyrouter/weight"]; v != "" {
		meta["label.hyrouter/weight"] = v
	}
	if v := u.GetAnnotations()["hyrouter/weight"]; v != "" {
		meta["annotation.hyrouter/weight"] = v
	}

	counters, _, _ := unstructured.NestedMap(status, "counters")
	for name, raw := range counters {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if c, ok := m["count"]; ok {
			meta["counter."+name+".count"] = fmt.Sprint(c)
		}
		if c, ok := m["capacity"]; ok {
			meta["counter."+name+".capacity"] = fmt.Sprint(c)
		}
	}

	if cfg != nil {
		copySelectedLabels(meta, u.GetLabels(), cfg.Metadata.IncludeLabels)
		copySelectedAnnotations(meta, u.GetAnnotations(), cfg.Metadata.IncludeAnnotations)
	}
	appendAgonesLists(meta, status)

	return routing.Backend{Host: addr, Port: port, Meta: meta}, true
}

func resolveAgonesAddress(status map[string]interface{}, cfg *config.AgonesDiscoveryConfig) string {
	if status == nil {
		return ""
	}
	addr, _, _ := unstructured.NestedString(status, "address")
	addrSrc := ""
	pref := []string(nil)
	if cfg != nil && cfg.Address != nil {
		addrSrc = strings.ToLower(strings.TrimSpace(cfg.Address.Source))
		pref = cfg.Address.Preference
	}
	if addrSrc == "" {
		if len(pref) == 0 {
			return addr
		}
		addrSrc = "addresses"
	}
	if addrSrc == "address" {
		return addr
	}

	addrs, _, _ := unstructured.NestedSlice(status, "addresses")
	if len(addrs) == 0 {
		return addr
	}
	byType := map[string]string{}
	order := make([]string, 0, len(addrs))
	for _, a := range addrs {
		m, ok := a.(map[string]interface{})
		if !ok {
			continue
		}
		t, _ := m["type"].(string)
		v, _ := m["address"].(string)
		t = strings.TrimSpace(t)
		v = strings.TrimSpace(v)
		if t == "" || v == "" {
			continue
		}
		if _, exists := byType[t]; !exists {
			order = append(order, t)
		}
		byType[t] = v
	}
	for _, t := range pref {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if v, ok := byType[t]; ok && v != "" {
			return v
		}
	}
	if addrSrc == "addresses" {
		for _, t := range order {
			if v := byType[t]; v != "" {
				return v
			}
		}
	}
	return addr
}

func appendAgonesLists(meta map[string]string, status map[string]interface{}) {
	if meta == nil || status == nil {
		return
	}
	lists, _, _ := unstructured.NestedMap(status, "lists")
	for name, raw := range lists {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		vals, _, _ := unstructured.NestedSlice(m, "values")
		if len(vals) > 0 {
			xs := make([]string, 0, len(vals))
			for _, v := range vals {
				if s, ok := v.(string); ok {
					xs = append(xs, s)
				}
			}
			if b, err := json.Marshal(xs); err == nil {
				meta["list."+name+".values"] = string(b)
			}
		}
		if capAny, ok := m["capacity"]; ok {
			meta["list."+name+".capacity"] = fmt.Sprint(capAny)
		}
	}
}

func resolveAgonesPorts(ports []interface{}, cfg config.KubernetesPortConfig) int {
	if cfg.Number > 0 {
		return cfg.Number
	}
	wantName := strings.TrimSpace(cfg.Name)
	for _, p := range ports {
		m, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		portAny := m["port"]
		port := toInt(portAny)
		if wantName != "" {
			if name == wantName && port > 0 {
				return port
			}
			continue
		}
		if port > 0 {
			return port
		}
	}
	return 0
}

func toInt(v interface{}) int {
	switch t := v.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float64:
		return int(t)
	case string:
		n, _ := strconv.Atoi(t)
		return n
	default:
		return 0
	}
}

func agonesRESTConfig(kubeconfig string) (*rest.Config, error) {
	kubeconfig = strings.TrimSpace(kubeconfig)
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	cfg, err := rest.InClusterConfig()
	if err == nil {
		return cfg, nil
	}
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
}

func parseLabelEqualsMap(s string) map[string]interface{} {
	s = strings.TrimSpace(s)
	out := map[string]interface{}{}
	if s == "" {
		return out
	}
	parts := strings.Split(s, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.TrimSpace(kv[0])
		v := strings.TrimSpace(kv[1])
		if k == "" {
			continue
		}
		out[k] = v
	}
	return out
}
