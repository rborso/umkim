package reconcile

import (
	"context"
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/clusters"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// stagingPromotionAction decides how to get a production certificate after
// Let's Encrypt staging preflight. Traefik publishes every staging certificate
// into the default TLS store, and the production resolver skips any name that
// already has a certificate there. The spoke deployment's init container
// deletes acme-staging.json, but only a restart picks that up. Restart only
// while the staging certificate is still being served, and only once the init
// container exists, so a ManifestWork rollout that already wiped staging is
// not interrupted mid-challenge.
func stagingPromotionAction(evictedHost, siteHost string, stagingCertServed, initDropsStaging bool) string {
	if siteHost != "" && strings.TrimSpace(evictedHost) == siteHost {
		return "noop"
	}
	if !initDropsStaging {
		return "wait-init"
	}
	if stagingCertServed {
		return "restart"
	}
	return "mark"
}

// promoteSpokeStagingCert restarts spoke Traefik once so the production
// resolver can issue. An empty return means the caller should keep the
// standard issuing message. A non-empty return is a status message for a
// restart that could not be completed yet.
func (r *RouteReconciler) promoteSpokeStagingCert(ctx context.Context, route *fleetv1alpha1.FleetAppRoute, clusterName string, regCluster clusters.Cluster, siteHost string) string {
	evicted := ""
	if route != nil && route.Annotations != nil {
		evicted = route.Annotations[annotationSpokeACMEStagingEvicted]
	}
	if stagingPromotionAction(evicted, siteHost, false, true) == "noop" {
		return ""
	}

	logger := log.FromContext(ctx).WithValues("cluster", clusterName, "host", siteHost)
	initDrops, stagingServed, err := r.spokeStagingEvictInputs(ctx, clusterName, regCluster, siteHost)
	if err != nil {
		logger.Error(err, "spoke staging promotion check failed")
		return "dropping spoke staging certificate so production Let's Encrypt can issue: " + err.Error()
	}
	switch stagingPromotionAction(evicted, siteHost, stagingServed, initDrops) {
	case "noop", "mark":
		_ = r.annotateSpokeStagingEvicted(ctx, route, siteHost)
		return ""
	case "wait-init":
		return ""
	}

	if !r.markStagingEvictOnce(clusterName, siteHost) {
		_ = r.annotateSpokeStagingEvicted(ctx, route, siteHost)
		return ""
	}
	if err := r.restartSpokeTraefik(ctx, clusterName, regCluster.UpstreamHost); err != nil {
		r.unmarkStagingEvict(clusterName, siteHost)
		logger.Error(err, "restarting spoke traefik to drop staging certificate")
		return "dropping spoke staging certificate so production Let's Encrypt can issue: " + err.Error()
	}
	logger.Info("restarted spoke traefik to drop staging certificate before production issuance")
	_ = r.annotateSpokeStagingEvicted(ctx, route, siteHost)
	return ""
}

func (r *RouteReconciler) markStagingEvictOnce(clusterName, siteHost string) bool {
	key := clusterName + "/" + siteHost
	r.stagingEvictMu.Lock()
	defer r.stagingEvictMu.Unlock()
	if r.stagingEvict == nil {
		r.stagingEvict = map[string]struct{}{}
	}
	if _, ok := r.stagingEvict[key]; ok {
		return false
	}
	r.stagingEvict[key] = struct{}{}
	return true
}

func (r *RouteReconciler) unmarkStagingEvict(clusterName, siteHost string) {
	r.stagingEvictMu.Lock()
	defer r.stagingEvictMu.Unlock()
	delete(r.stagingEvict, clusterName+"/"+siteHost)
}

func (r *RouteReconciler) annotateSpokeStagingEvicted(ctx context.Context, route *fleetv1alpha1.FleetAppRoute, siteHost string) error {
	if route == nil || siteHost == "" {
		return nil
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var latest fleetv1alpha1.FleetAppRoute
		if err := r.Get(ctx, client.ObjectKeyFromObject(route), &latest); err != nil {
			return err
		}
		if latest.Annotations != nil && latest.Annotations[annotationSpokeACMEStagingEvicted] == siteHost {
			return nil
		}
		base := latest.DeepCopy()
		if latest.Annotations == nil {
			latest.Annotations = map[string]string{}
		}
		latest.Annotations[annotationSpokeACMEStagingEvicted] = siteHost
		return r.Patch(ctx, &latest, client.MergeFrom(base))
	})
}

func (r *RouteReconciler) spokeStagingEvictInputs(ctx context.Context, clusterName string, regCluster clusters.Cluster, siteHost string) (initDrops, stagingServed bool, err error) {
	cs, err := r.spokeClientset(ctx, clusterName, regCluster.UpstreamHost)
	if err != nil {
		return false, false, err
	}
	ns := r.Config.SpokeIngressNS
	if ns == "" {
		ns = "fleet-ingress"
	}
	// A ready pod that already finished the init container means this process
	// started without staging storage. Checking the Deployment spec instead
	// would delete pods during the rollout that installs that init container.
	pods, err := cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: "app=" + spokeTraefikDeploymentName})
	if err != nil {
		return false, false, fmt.Errorf("list spoke traefik pods: %w", err)
	}
	for _, pod := range pods.Items {
		if !podReady(pod) || !podDroppedStagingCerts(pod) {
			continue
		}
		initDrops = true
		break
	}
	if !initDrops {
		return false, false, nil
	}
	probeHost := spokeProbeHost(ctx, siteHost, regCluster)
	probePort := spokeProbeHTTPSPort(regCluster)
	if probeHost == "" || probePort == 0 {
		return true, false, fmt.Errorf("cannot probe staging certificate for %s", siteHost)
	}
	stagingServed, _ = probeACMECertOnPort(probeHost, probePort, siteHost, true)
	return initDrops, stagingServed, nil
}

func podReady(pod corev1.Pod) bool {
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func podDroppedStagingCerts(pod corev1.Pod) bool {
	for _, st := range pod.Status.InitContainerStatuses {
		if st.Name != spokeDropStagingInitContainerName || st.State.Terminated == nil {
			continue
		}
		return st.State.Terminated.ExitCode == 0
	}
	return false
}

func (r *RouteReconciler) restartSpokeTraefik(ctx context.Context, clusterName, upstream string) error {
	cs, err := r.spokeClientset(ctx, clusterName, upstream)
	if err != nil {
		return err
	}
	ns := r.Config.SpokeIngressNS
	if ns == "" {
		ns = "fleet-ingress"
	}
	pods, err := cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: "app=traefik"})
	if err != nil {
		return fmt.Errorf("list spoke traefik pods: %w", err)
	}
	if len(pods.Items) == 0 {
		return fmt.Errorf("no spoke traefik pods in %s", ns)
	}
	for _, pod := range pods.Items {
		err := cs.CoreV1().Pods(ns).Delete(ctx, pod.Name, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete spoke traefik pod %s: %w", pod.Name, err)
		}
	}
	return nil
}

func (r *RouteReconciler) spokeClientset(ctx context.Context, clusterName, upstream string) (kubernetes.Interface, error) {
	var secret corev1.Secret
	key := client.ObjectKey{Namespace: "fleet-system", Name: "fleet-kubeconfig-" + clusterName}
	if err := r.Get(ctx, key, &secret); err != nil {
		return nil, fmt.Errorf("spoke kubeconfig: %w", err)
	}
	raw := secret.Data["kubeconfig"]
	if len(raw) == 0 {
		raw = secret.Data["value"]
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("spoke kubeconfig secret %s is empty", key.Name)
	}
	kc := string(raw)
	host := strings.TrimSpace(upstream)
	if host != "" {
		for _, old := range []string{
			"https://127.0.0.1:6443",
			"https://0.0.0.0:6443",
			"https://localhost:6443",
		} {
			kc = strings.ReplaceAll(kc, old, "https://"+host+":6443")
		}
	}
	cfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(kc))
	if err != nil {
		return nil, fmt.Errorf("parse spoke kubeconfig: %w", err)
	}
	// Spoke kubeconfigs are captured against localhost; the hub dials the
	// overlay address, whose certificate will not match that CA.
	cfg.TLSClientConfig.Insecure = true
	cfg.TLSClientConfig.CAData = nil
	return kubernetes.NewForConfig(cfg)
}
