package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hybrowse/hyrouter/internal/config"
	"github.com/hybrowse/hyrouter/internal/routing"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

type kubernetesProvider struct {
	name   string
	cfg    *config.KubernetesDiscoveryConfig
	logger *slog.Logger

	client kubernetes.Interface

	factory informers.SharedInformerFactory

	rebuildMu sync.Mutex
	snapshot  atomic.Value

	startOnce sync.Once
	startErr  error
}

func applyWeightFromMaps(b *routing.Backend, labelsMap map[string]string, ann map[string]string) {
	if b == nil {
		return
	}
	if b.Weight > 0 {
		return
	}
	if v := ann["hyrouter/weight"]; v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			b.Weight = n
			return
		}
	}
	if v := labelsMap["hyrouter/weight"]; v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			b.Weight = n
			return
		}
	}
}

func newKubernetesProvider(name string, cfg *config.KubernetesDiscoveryConfig, logger *slog.Logger) (*kubernetesProvider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("discovery provider %q: kubernetes config must be set", name)
	}
	if logger == nil {
		logger = slog.Default()
	}
	p := &kubernetesProvider{name: name, cfg: cfg, logger: logger}
	p.snapshot.Store([]routing.Backend(nil))
	return p, nil
}

func (p *kubernetesProvider) Start(ctx context.Context) error {
	p.startOnce.Do(func() {
		restCfg, err := kubeRESTConfig(p.cfg.Kubeconfig)
		if err != nil {
			p.startErr = err
			return
		}
		client, err := kubernetes.NewForConfig(restCfg)
		if err != nil {
			p.startErr = err
			return
		}
		p.client = client
		p.factory = informers.NewSharedInformerFactory(client, 0)

		podInf := p.factory.Core().V1().Pods().Informer()
		epInf := p.factory.Discovery().V1().EndpointSlices().Informer()

		h := cache.ResourceEventHandlerFuncs{
			AddFunc:    func(_ interface{}) { p.rebuild() },
			UpdateFunc: func(_, _ interface{}) { p.rebuild() },
			DeleteFunc: func(_ interface{}) { p.rebuild() },
		}
		podInf.AddEventHandler(h)
		epInf.AddEventHandler(h)

		p.factory.Start(ctx.Done())
		ok := cache.WaitForCacheSync(ctx.Done(), podInf.HasSynced, epInf.HasSynced)
		if !ok {
			p.startErr = fmt.Errorf("discovery provider %q: cache sync failed", p.name)
			return
		}
		p.rebuild()

		go wait.UntilWithContext(ctx, func(ctx context.Context) {
			p.rebuild()
		}, 30*time.Second)
	})
	return p.startErr
}

func (p *kubernetesProvider) Resolve(_ context.Context) ([]routing.Backend, error) {
	v := p.snapshot.Load()
	if v == nil {
		return nil, nil
	}
	bs := v.([]routing.Backend)
	out := make([]routing.Backend, len(bs))
	copy(out, bs)
	return out, nil
}

func (p *kubernetesProvider) rebuild() {
	p.rebuildMu.Lock()
	defer p.rebuildMu.Unlock()

	if p.factory == nil {
		return
	}

	podLister := p.factory.Core().V1().Pods().Lister()
	epLister := p.factory.Discovery().V1().EndpointSlices().Lister()

	namespaces := p.cfg.Namespaces
	if len(namespaces) == 0 {
		namespaces = []string{metav1.NamespaceAll}
	}

	var out []routing.Backend
	for _, r := range p.cfg.Resources {
		kind := strings.ToLower(strings.TrimSpace(r.Kind))
		switch kind {
		case "pods", "pod":
			ls := labels.Everything()
			labelExpr := ""
			if r.Selector != nil {
				labelExpr = strings.TrimSpace(r.Selector.Labels)
			}
			if labelExpr != "" {
				parsed, err := labels.Parse(labelExpr)
				if err == nil {
					ls = parsed
				}
			}
			for _, ns := range namespaces {
				var pods []*corev1.Pod
				var err error
				if ns == metav1.NamespaceAll {
					pods, err = podLister.List(ls)
				} else {
					pods, err = podLister.Pods(ns).List(ls)
				}
				if err != nil {
					continue
				}
				for _, pod := range pods {
					if !p.podAllowed(pod, r.Selector) {
						continue
					}
					port, ok := resolvePodPort(pod, r.Port)
					if !ok {
						continue
					}
					b := routing.Backend{Host: pod.Status.PodIP, Port: port, Meta: map[string]string{}}
					fillK8sMeta(b.Meta, pod.Namespace, pod.Name, pod.Spec.NodeName)
					copySelectedLabels(b.Meta, pod.Labels, p.cfg.Metadata.IncludeLabels)
					copySelectedAnnotations(b.Meta, pod.Annotations, p.cfg.Metadata.IncludeAnnotations)
					applyWeightFromMaps(&b, pod.Labels, pod.Annotations)
					out = append(out, b)
				}
			}
		case "endpointslices", "endpointslice":
			nsList := namespaces
			if r.Service != nil && r.Service.Namespace != "" {
				nsList = []string{r.Service.Namespace}
			}
			labelSel := labels.Everything()
			if r.Selector != nil {
				labelExpr := strings.TrimSpace(r.Selector.Labels)
				if labelExpr != "" {
					if parsed, err := labels.Parse(labelExpr); err == nil {
						labelSel = parsed
					}
				}
			}
			for _, ns := range nsList {
				var slices []*discoveryv1.EndpointSlice
				var err error
				if ns == metav1.NamespaceAll {
					slices, err = epLister.List(labels.Everything())
				} else {
					slices, err = epLister.EndpointSlices(ns).List(labels.Everything())
				}
				if err != nil {
					continue
				}
				for _, es := range slices {
					if !labelSel.Matches(labels.Set(es.Labels)) {
						continue
					}
					if r.Selector != nil {
						if !annotationsMatch(es.Annotations, r.Selector.Annotations) {
							continue
						}
					}
					if r.Service != nil {
						if es.Labels["kubernetes.io/service-name"] != r.Service.Name {
							continue
						}
					}
					port, ok := resolveEndpointSlicePort(es, r.Port)
					if !ok {
						continue
					}
					for _, ep := range es.Endpoints {
						if p.cfg.Filters.RequireEndpointReady {
							if ep.Conditions.Ready != nil && !*ep.Conditions.Ready {
								continue
							}
						}
						for _, addr := range ep.Addresses {
							b := routing.Backend{Host: addr, Port: port, Meta: map[string]string{}}
							fillK8sMeta(b.Meta, es.Namespace, es.Name, "")
							copySelectedLabels(b.Meta, es.Labels, p.cfg.Metadata.IncludeLabels)
							copySelectedAnnotations(b.Meta, es.Annotations, p.cfg.Metadata.IncludeAnnotations)
							applyWeightFromMaps(&b, es.Labels, es.Annotations)
							out = append(out, b)
						}
					}
				}
			}
		}
	}

	p.snapshot.Store(out)
}

func (p *kubernetesProvider) podAllowed(pod *corev1.Pod, sel *config.KubernetesSelector) bool {
	if pod == nil {
		return false
	}
	if pod.Status.PodIP == "" {
		return false
	}
	if len(p.cfg.Filters.RequirePodPhase) > 0 {
		ok := false
		for _, ph := range p.cfg.Filters.RequirePodPhase {
			if strings.EqualFold(string(pod.Status.Phase), ph) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if p.cfg.Filters.RequirePodReady {
		if !podReady(pod) {
			return false
		}
	}
	if sel == nil {
		return true
	}
	return annotationsMatch(pod.Annotations, sel.Annotations)
}

func podReady(pod *corev1.Pod) bool {
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func resolvePodPort(pod *corev1.Pod, cfg config.KubernetesPortConfig) (int, bool) {
	if cfg.ContainerPort > 0 {
		return cfg.ContainerPort, true
	}
	if cfg.Number > 0 {
		return cfg.Number, true
	}
	if strings.TrimSpace(cfg.Name) == "" {
		return 0, false
	}
	name := strings.TrimSpace(cfg.Name)
	for _, c := range pod.Spec.Containers {
		for _, p := range c.Ports {
			if p.Name == name {
				if p.ContainerPort > 0 {
					return int(p.ContainerPort), true
				}
			}
		}
	}
	return 0, false
}

func resolveEndpointSlicePort(es *discoveryv1.EndpointSlice, cfg config.KubernetesPortConfig) (int, bool) {
	if es == nil {
		return 0, false
	}
	if cfg.Number > 0 {
		return cfg.Number, true
	}
	if strings.TrimSpace(cfg.Name) != "" {
		want := strings.TrimSpace(cfg.Name)
		for _, p := range es.Ports {
			if p.Name != nil && *p.Name == want {
				if p.Port != nil {
					return int(*p.Port), true
				}
			}
		}
	}
	for _, p := range es.Ports {
		if p.Port != nil {
			return int(*p.Port), true
		}
	}
	return 0, false
}

func kubeRESTConfig(kubeconfig string) (*rest.Config, error) {
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

func fillK8sMeta(meta map[string]string, ns string, name string, node string) {
	if meta == nil {
		return
	}
	if ns != "" {
		meta["k8s.namespace"] = ns
	}
	if name != "" {
		meta["k8s.name"] = name
	}
	if node != "" {
		meta["k8s.node"] = node
	}
}

func copySelectedLabels(meta map[string]string, labelsMap map[string]string, include []string) {
	for _, k := range include {
		if v, ok := labelsMap[k]; ok {
			meta["label."+k] = v
		}
	}
}

func copySelectedAnnotations(meta map[string]string, ann map[string]string, include []string) {
	for _, k := range include {
		if v, ok := ann[k]; ok {
			meta["annotation."+k] = v
		}
	}
}

func applyWeight(b *routing.Backend) {
	if b == nil {
		return
	}
	if b.Weight > 0 {
		return
	}
	if b.Meta == nil {
		return
	}
	if v := b.Meta["annotation.hyrouter/weight"]; v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			b.Weight = n
			return
		}
	}
	if v := b.Meta["label.hyrouter/weight"]; v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			b.Weight = n
			return
		}
	}
}

func annotationsMatch(annotations map[string]string, expr string) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true
	}
	parts := strings.Split(expr, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 {
			return false
		}
		k := strings.TrimSpace(kv[0])
		v := strings.TrimSpace(kv[1])
		if annotations == nil {
			return false
		}
		if annotations[k] != v {
			return false
		}
	}
	return true
}
