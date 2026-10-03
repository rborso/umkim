package routesync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/clusters"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const spokeKubeconfigSecretNS = "fleet-system"

var managedClusterGVK = schema.GroupVersionKind{
	Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
}

type helmReleaseResp struct {
	Status string `json:"status"`
	Health *struct {
		Ready bool `json:"ready"`
	} `json:"health"`
}

func (s *Syncer) workloadServiceFromProvider(ctx context.Context, providerName, resourceID, releaseName, namespace string) (fleetv1alpha1.ServiceRef, bool) {
	endpoint, ok := s.providerEndpoint(ctx, providerName)
	if !ok || endpoint == "" {
		return fleetv1alpha1.ServiceRef{}, false
	}
	if strings.HasPrefix(providerName, "helm-") || strings.Contains(endpoint, "/releases") {
		return s.workloadServiceFromHelmRelease(ctx, providerName, endpoint, resourceID, releaseName, namespace)
	}
	if !strings.Contains(endpoint, "/containers") {
		return fleetv1alpha1.ServiceRef{}, false
	}
	base := strings.TrimSuffix(endpoint, "/")
	url := base + "/" + resourceID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fleetv1alpha1.ServiceRef{}, false
	}
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return fleetv1alpha1.ServiceRef{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fleetv1alpha1.ServiceRef{}, false
	}
	var body containerWorkloadResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fleetv1alpha1.ServiceRef{}, false
	}
	name := ""
	if body.Service != nil {
		name = strings.TrimSpace(body.Service.Name)
	}
	if name == "" {
		name = strings.TrimSpace(body.Spec.Metadata.Name)
	}
	if name == "" {
		return fleetv1alpha1.ServiceRef{}, false
	}
	ns := strings.TrimSpace(body.Spec.Metadata.Namespace)
	if ns == "" {
		ns = "default"
	}
	return fleetv1alpha1.ServiceRef{Namespace: ns, Name: name, Port: 80}, true
}

func (s *Syncer) workloadServiceFromHelmRelease(ctx context.Context, providerName, endpoint, resourceID, releaseName, namespace string) (fleetv1alpha1.ServiceRef, bool) {
	releaseName = strings.TrimSpace(releaseName)
	namespace = strings.TrimSpace(namespace)
	if releaseName == "" || namespace == "" || resourceID == "" {
		return fleetv1alpha1.ServiceRef{}, false
	}
	clusterName := providerToCluster(providerName)
	if clusterName == "" {
		return fleetv1alpha1.ServiceRef{}, false
	}
	cs, err := s.spokeClientset(ctx, clusterName)
	if err != nil {
		return fleetv1alpha1.ServiceRef{}, false
	}
	list, err := cs.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fleetv1alpha1.ServiceRef{}, false
	}
	name, port, ok := selectHelmWorkloadService(list.Items, releaseName)
	if !ok {
		return fleetv1alpha1.ServiceRef{}, false
	}
	// Prefer live Service ports even while Helm is still pending/failed
	// (e.g. post-install bootstrap hooks). Ready status is informational only.
	_ = s.helmReleaseReady(ctx, endpoint, resourceID)
	return fleetv1alpha1.ServiceRef{Namespace: namespace, Name: name, Port: port}, true
}

func (s *Syncer) helmReleaseReady(ctx context.Context, endpoint, resourceID string) bool {
	base := strings.TrimSuffix(endpoint, "/")
	url := base + "/" + resourceID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}
	var rel helmReleaseResp
	if err := json.Unmarshal(body, &rel); err != nil {
		return false
	}
	if rel.Health != nil && rel.Health.Ready {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(rel.Status), "active")
}

func (s *Syncer) spokeClientset(ctx context.Context, clusterName string) (kubernetes.Interface, error) {
	cfg, err := s.spokeRESTConfig(ctx, clusterName)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(cfg)
}

func (s *Syncer) spokeRESTConfig(ctx context.Context, clusterName string) (*rest.Config, error) {
	host := s.spokeAPIHost(ctx, clusterName)
	if host == "" {
		return nil, fmt.Errorf("no spoke API host for cluster %s", clusterName)
	}
	raw, err := s.spokeKubeconfigBytes(ctx, clusterName)
	if err != nil {
		return nil, err
	}
	kc := patchSpokeKubeconfigServer(string(raw), host)
	cfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(kc))
	if err != nil {
		return nil, fmt.Errorf("parse spoke kubeconfig: %w", err)
	}
	cfg.TLSClientConfig.Insecure = true
	cfg.TLSClientConfig.CAData = nil
	return cfg, nil
}

func (s *Syncer) spokeAPIHost(ctx context.Context, clusterName string) string {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(managedClusterGVK)
	if err := s.Client.Get(ctx, client.ObjectKey{Name: clusterName}, obj); err == nil {
		if labels := obj.GetLabels(); labels != nil {
			if host := strings.TrimSpace(labels["fleet.topology/wireguard_ip"]); host != "" {
				return host
			}
		}
	}
	reg, err := clusters.Load(ctx, s.Client, s.Config.ClusterConfigMapNS, s.Config.ClusterConfigMapName)
	if err != nil {
		return ""
	}
	if c, ok := reg.Get(clusterName); ok {
		return strings.TrimSpace(c.UpstreamHost)
	}
	return ""
}

func (s *Syncer) spokeKubeconfigBytes(ctx context.Context, clusterName string) ([]byte, error) {
	var secret corev1.Secret
	key := client.ObjectKey{Namespace: spokeKubeconfigSecretNS, Name: "fleet-kubeconfig-" + clusterName}
	if err := s.Client.Get(ctx, key, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("kubeconfig secret %s/%s not found", key.Namespace, key.Name)
		}
		return nil, err
	}
	raw := secret.Data["kubeconfig"]
	if len(raw) == 0 {
		raw = secret.Data["value"]
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("kubeconfig secret %s/%s is empty", key.Namespace, key.Name)
	}
	return raw, nil
}

func patchSpokeKubeconfigServer(kc, host string) string {
	for _, old := range []string{
		"https://127.0.0.1:6443",
		"https://0.0.0.0:6443",
		"https://localhost:6443",
	} {
		kc = strings.ReplaceAll(kc, old, "https://"+host+":6443")
	}
	return kc
}

type helmServiceCandidate struct {
	name     string
	port     int32
	priority int
}

func selectHelmWorkloadService(services []corev1.Service, releaseName string) (string, int32, bool) {
	releaseName = strings.TrimSpace(releaseName)
	if releaseName == "" {
		return "", 0, false
	}
	var candidates []helmServiceCandidate
	for _, svc := range services {
		if !helmServiceMatchesRelease(svc, releaseName) {
			continue
		}
		if helmServiceExcluded(svc.Name) {
			continue
		}
		port := serviceHTTPPort(svc)
		if port == 0 {
			continue
		}
		candidates = append(candidates, helmServiceCandidate{
			name:     svc.Name,
			port:     port,
			priority: helmServicePriority(svc.Name, releaseName),
		})
	}
	if len(candidates) == 0 {
		return "", 0, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].priority != candidates[j].priority {
			return candidates[i].priority < candidates[j].priority
		}
		return candidates[i].name < candidates[j].name
	})
	return candidates[0].name, candidates[0].port, true
}

func helmServiceExcluded(name string) bool {
	n := strings.ToLower(name)
	for _, skip := range []string{
		"-ssh", "-postgresql", "-mariadb", "-mysql", "-redis", "-valkey",
		"-mongodb", "-mongo", "-rabbitmq", "-headless", "-hl",
		"machine-learning", "-ml", "-metrics", "-exporter",
	} {
		if strings.Contains(n, skip) || strings.HasSuffix(n, skip) {
			return true
		}
	}
	return false
}

func helmServiceMatchesRelease(svc corev1.Service, releaseName string) bool {
	if svc.Name == releaseName || strings.HasPrefix(svc.Name, releaseName+"-") {
		return true
	}
	if svc.Labels != nil && svc.Labels["app.kubernetes.io/instance"] == releaseName {
		return true
	}
	return false
}

func helmServicePriority(name, releaseName string) int {
	n := strings.ToLower(name)
	switch {
	case name == releaseName:
		return 0
	case name == releaseName+"-http":
		return 1
	case name == releaseName+"-server" || strings.HasSuffix(n, "-server"):
		return 1
	case name == releaseName+"-web" || strings.HasSuffix(n, "-web") || strings.HasSuffix(n, "-frontend"):
		return 1
	case strings.Contains(n, "http"):
		return 2
	case strings.HasSuffix(n, "-app") || n == releaseName+"-nextcloud":
		return 2
	default:
		return 3
	}
}

func serviceHTTPPort(svc corev1.Service) int32 {
	var httpPort int32
	for _, p := range svc.Spec.Ports {
		if p.Port == 80 {
			return 80
		}
		if p.Name == "http" || strings.Contains(strings.ToLower(p.Name), "http") {
			httpPort = p.Port
		}
	}
	if httpPort != 0 {
		return httpPort
	}
	if len(svc.Spec.Ports) > 0 {
		return svc.Spec.Ports[0].Port
	}
	return 0
}

func placementTargetActive(t placementTarget) bool {
	switch strings.ToLower(strings.TrimSpace(t.Status)) {
	case "deleted", "delete", "failed":
		return false
	default:
		return true
	}
}
