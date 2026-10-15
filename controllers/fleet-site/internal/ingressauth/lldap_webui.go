package ingressauth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	AnsibleSpecHashAnnotation = "fleet.umkim.com/ansible-spec-hash"
	lldapServiceName            = "lldap"
	lldapWebUIServiceName       = "lldap-webui"
	lldapHTTPPortName           = "http"
	lldapAppLabel               = "lldap"
)

// ConsoleLLDAPDeployed reports whether fleet-console LLDAP is installed.
func ConsoleLLDAPDeployed(ctx context.Context, c client.Client, ns string) (bool, error) {
	var dep appsv1.Deployment
	err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: lldapServiceName}, &dep)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

// AnsibleSpecHash fingerprints spec fields and computed route auth rules that
// require an Ansible reconcile job.
func AnsibleSpecHash(fia *fleetv1alpha1.FleetIngressAuth, desired DesiredState) string {
	h := sha256.New()
	parts := []string{
		fmt.Sprintf("hubEnabled=%v", fia.Spec.HubEnabled),
		fia.Spec.PortalHost,
		fia.Spec.CookieDomain,
		fia.Spec.SecretsRef,
	}
	for _, d := range fia.Spec.AccessDomains {
		parts = append(parts, "ad="+d)
	}
	for _, r := range desired.AccessRules {
		parts = append(parts, "hr="+r.Domain+"="+strings.Join(r.Groups, ","))
	}
	// The enabled cluster set is part of the rendered state: a cluster with no
	// access rules or OIDC clients still needs its own ManifestWork, so adding or
	// removing one must change the hash and re-trigger the reconcile.
	for _, name := range sortedKeys(sliceToSet(desired.EnabledClusters)) {
		parts = append(parts, "cl="+name)
	}
	for _, name := range sortedKeys(sliceToSet(desired.EnabledClusters)) {
		cc := desired.ClusterConfigs[name]
		for _, r := range cc.AccessRules {
			parts = append(parts, "cr="+name+"/"+r.Domain+"="+strings.Join(r.Groups, ","))
		}
		for _, oc := range cc.OIDCClients {
			pkce := "true"
			if oc.RequirePKCE != nil && !*oc.RequirePKCE {
				pkce = "false"
			}
			parts = append(parts, "oc="+name+"/"+oc.ClientID+"/"+pkce+"/"+strings.Join(oc.RedirectURIs, ","))
		}
	}
	_, _ = h.Write([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(h.Sum(nil))
}

func storedAnsibleSpecHash(fia *fleetv1alpha1.FleetIngressAuth) string {
	if fia.Annotations == nil {
		return ""
	}
	return fia.Annotations[AnsibleSpecHashAnnotation]
}

// AnsibleSpecChanged reports whether Ansible-relevant spec differs from the last applied hash.
func AnsibleSpecChanged(fia *fleetv1alpha1.FleetIngressAuth, desired DesiredState) bool {
	current := AnsibleSpecHash(fia, desired)
	stored := storedAnsibleSpecHash(fia)
	return stored == "" || stored != current
}

// ReconcileHubLLDAPWebUI publishes the LLDAP web UI according to the configured
// exposure mode: disabled, wireguard (NodePort reachable only over the overlay),
// or authelia (hub gateway route behind Authelia). The LDAP service itself stays
// ClusterIP.
func ReconcileHubLLDAPWebUI(ctx context.Context, c client.Client, cfg config.Config, fia *fleetv1alpha1.FleetIngressAuth) error {
	consoleNS := cfg.ConsoleNamespace
	if consoleNS == "" {
		consoleNS = "fleet-console"
	}
	nodePort := int32(cfg.LldapWebUINodePort)
	if nodePort <= 0 {
		nodePort = 30917
	}

	mode := fia.Spec.EffectiveHubLldapWebUIExposure()
	want := mode != fleetv1alpha1.LldapWebUIExposureDisabled
	hub := fia.Status.Hub
	host := lldapWebUIHostForFIA(fia)
	hub.LLDAPWebUIExposure = mode

	if want {
		if mode == fleetv1alpha1.LldapWebUIExposureAuthelia && !fia.Spec.HubEnabled {
			hub.LLDAPWebUIPhase = fleetv1alpha1.LldapWebUIPhaseFailed
			hub.LLDAPWebUIMessage = "hub ingress auth (Authelia) must be enabled, or use exposure wireguard"
			hub.LLDAPWebUIEnabled = false
			hub.LLDAPWebUIURL = ""
			fia.Status.Hub = hub
			return c.Status().Update(ctx, fia)
		}
		if mode == fleetv1alpha1.LldapWebUIExposureAuthelia && host == "" {
			hub.LLDAPWebUIPhase = fleetv1alpha1.LldapWebUIPhaseFailed
			hub.LLDAPWebUIMessage = "no access domain configured for the LLDAP web UI host"
			hub.LLDAPWebUIEnabled = false
			hub.LLDAPWebUIURL = ""
			fia.Status.Hub = hub
			return c.Status().Update(ctx, fia)
		}
		deployed, err := ConsoleLLDAPDeployed(ctx, c, consoleNS)
		if err != nil {
			return err
		}
		if !deployed {
			hub.LLDAPWebUIPhase = fleetv1alpha1.LldapWebUIPhaseFailed
			hub.LLDAPWebUIMessage = "Console LLDAP is not deployed (enable fleet_auth_lldap_enabled on the hub)"
			hub.LLDAPWebUIEnabled = false
			hub.LLDAPWebUIURL = ""
			fia.Status.Hub = hub
			return c.Status().Update(ctx, fia)
		}
	}

	if err := restorePrimaryLLDAPClusterIP(ctx, c, consoleNS); err != nil {
		return err
	}

	var webUI corev1.Service
	err := c.Get(ctx, types.NamespacedName{Namespace: consoleNS, Name: lldapWebUIServiceName}, &webUI)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	if !want {
		if err := ensureHubLLDAPWebUIRoute(ctx, c, cfg, host, false); err != nil {
			return err
		}
		if apierrors.IsNotFound(err) {
			hub.LLDAPWebUIPhase = fleetv1alpha1.LldapWebUIPhaseDisabled
			hub.LLDAPWebUIMessage = ""
			hub.LLDAPWebUIEnabled = false
			hub.LLDAPWebUIURL = ""
			fia.Status.Hub = hub
			return c.Status().Update(ctx, fia)
		}
		if err := c.Delete(ctx, &webUI); err != nil && !apierrors.IsNotFound(err) {
			hub.LLDAPWebUIPhase = fleetv1alpha1.LldapWebUIPhaseFailed
			hub.LLDAPWebUIMessage = err.Error()
			fia.Status.Hub = hub
			_ = c.Status().Update(ctx, fia)
			return err
		}
		hub.LLDAPWebUIPhase = fleetv1alpha1.LldapWebUIPhaseDisabled
		hub.LLDAPWebUIMessage = ""
		hub.LLDAPWebUIEnabled = false
		hub.LLDAPWebUIURL = ""
		fia.Status.Hub = hub
		return c.Status().Update(ctx, fia)
	}

	desired := desiredLLDAPWebUIService(consoleNS, mode, nodePort)
	if apierrors.IsNotFound(err) {
		hub.LLDAPWebUIPhase = fleetv1alpha1.LldapWebUIPhaseEnabling
		fia.Status.Hub = hub
		if err := c.Status().Update(ctx, fia); err != nil {
			return err
		}
		if err := c.Create(ctx, desired); err != nil {
			hub.LLDAPWebUIPhase = fleetv1alpha1.LldapWebUIPhaseFailed
			hub.LLDAPWebUIMessage = err.Error()
			fia.Status.Hub = hub
			_ = c.Status().Update(ctx, fia)
			return err
		}
	} else if !lldapWebUIServiceEqual(&webUI, desired) {
		hub.LLDAPWebUIPhase = fleetv1alpha1.LldapWebUIPhaseEnabling
		fia.Status.Hub = hub
		if err := c.Status().Update(ctx, fia); err != nil {
			return err
		}
		// Service type cannot be patched in place (NodePort -> ClusterIP), so
		// replace it. A ClusterIP is reallocated on create, which is fine here.
		if err := c.Delete(ctx, &webUI); err != nil && !apierrors.IsNotFound(err) {
			hub.LLDAPWebUIPhase = fleetv1alpha1.LldapWebUIPhaseFailed
			hub.LLDAPWebUIMessage = err.Error()
			fia.Status.Hub = hub
			_ = c.Status().Update(ctx, fia)
			return err
		}
		if err := c.Create(ctx, desired); err != nil {
			hub.LLDAPWebUIPhase = fleetv1alpha1.LldapWebUIPhaseFailed
			hub.LLDAPWebUIMessage = err.Error()
			fia.Status.Hub = hub
			_ = c.Status().Update(ctx, fia)
			return err
		}
	}

	if err := ensureHubLLDAPWebUIRoute(ctx, c, cfg, host, mode == fleetv1alpha1.LldapWebUIExposureAuthelia); err != nil {
		hub.LLDAPWebUIPhase = fleetv1alpha1.LldapWebUIPhaseFailed
		hub.LLDAPWebUIMessage = err.Error()
		fia.Status.Hub = hub
		_ = c.Status().Update(ctx, fia)
		return err
	}

	hub.LLDAPWebUIPhase = fleetv1alpha1.LldapWebUIPhaseReady
	hub.LLDAPWebUIMessage = ""
	hub.LLDAPWebUIEnabled = true
	if mode == fleetv1alpha1.LldapWebUIExposureAuthelia {
		hub.LLDAPWebUIURL = "https://" + host
	} else {
		hub.LLDAPWebUIURL = fmt.Sprintf("http://%s:%d", cfg.HubWireguardIP, nodePort)
	}
	fia.Status.Hub = hub
	return c.Status().Update(ctx, fia)
}

// lldapWebUIHostForFIA derives the LLDAP web UI host from the configured zone
// (lldap.<zone>), preferring the cookie domain.
func lldapWebUIHostForFIA(fia *fleetv1alpha1.FleetIngressAuth) string {
	zone := strings.TrimSpace(fia.Spec.CookieDomain)
	if zone == "" {
		for _, ad := range fia.Spec.AccessDomains {
			if d := strings.TrimPrefix(strings.TrimSpace(ad), "*."); d != "" {
				zone = d
				break
			}
		}
	}
	if zone == "" {
		return ""
	}
	return LLDAPWebUIHost(zone)
}

// hubDynamicCMPlaceholder is the hub's own dynamic-routes file when it carries
// nothing but the placeholder comment.
const hubDynamicCMPlaceholder = "# Hub ingress placeholder dynamic routes (fleet-edge updates fleet-traefik-fleet-routes).\n" +
	"# Traefik v3 rejects empty http/middlewares-only fragments; fleet routes live in fleet-routes.yml.\n"

const (
	hubDynamicCMKey       = "routes.yml"
	hubGatewayReloadAnnot = "fleet.umkim.com/lldap-webui-routes"
	lldapWebUIRouteMarker = "# Managed by fleet-site-controller: LLDAP web UI behind Authelia."
)

// ensureHubLLDAPWebUIRoute publishes (or withdraws) the LLDAP web UI on the hub
// gateway. The TLS router chains the fleet-edge Authelia forward-auth
// middleware, so only authenticated directory users reach the UI.
func ensureHubLLDAPWebUIRoute(ctx context.Context, c client.Client, cfg config.Config, host string, enabled bool) error {
	ns := cfg.HubGatewayNamespace
	if ns == "" {
		ns = "fleet-ingress"
	}
	name := cfg.HubDynamicCMName
	if name == "" {
		name = "fleet-traefik-dynamic"
	}
	var cm corev1.ConfigMap
	err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &cm)
	if apierrors.IsNotFound(err) {
		return nil // no hub gateway (spoke-only deployment)
	}
	if err != nil {
		return err
	}
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	current := cm.Data[hubDynamicCMKey]
	want := current
	if enabled && host != "" {
		want = hubLLDAPWebUIRoutesYAML(host, cfg.ConsoleNamespace)
	} else if strings.Contains(current, lldapWebUIRouteMarker) {
		want = hubDynamicCMPlaceholder
	}
	if want == current {
		return nil
	}
	cm.Data[hubDynamicCMKey] = want
	if err := c.Update(ctx, &cm); err != nil {
		return err
	}
	return reloadHubGateway(ctx, c, cfg)
}

func hubLLDAPWebUIRoutesYAML(host, consoleNS string) string {
	if consoleNS == "" {
		consoleNS = "fleet-console"
	}
	return fmt.Sprintf(`%s
http:
  routers:
    fleet-lldap-webui:
      rule: Host(`+"`%s`"+`) && !PathPrefix(`+"`/.well-known/acme-challenge/`"+`)
      service: fleet-lldap-webui
      entryPoints:
        - web
      middlewares:
        - fleet-edge-redirect-https
    fleet-lldap-webui-tls:
      rule: Host(`+"`%s`"+`)
      service: fleet-lldap-webui
      entryPoints:
        - websecure
      middlewares:
        - fleet-edge-forward-headers-https
        - fleet-edge-authelia
      tls:
        certResolver: letsencrypt
  services:
    fleet-lldap-webui:
      loadBalancer:
        servers:
          - url: http://%s.%s.svc.cluster.local:17170
`, lldapWebUIRouteMarker, host, host, lldapWebUIServiceName, consoleNS)
}

// reloadHubGateway bumps a pod-template annotation so the gateway picks up the
// changed file-provider config (the hub Traefik runs with file watch disabled).
func reloadHubGateway(ctx context.Context, c client.Client, cfg config.Config) error {
	ns := cfg.HubGatewayNamespace
	if ns == "" {
		ns = "fleet-ingress"
	}
	name := cfg.HubGatewayDeployment
	if name == "" {
		name = "gateway"
	}
	var dep appsv1.Deployment
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &dep); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if dep.Spec.Template.Annotations == nil {
		dep.Spec.Template.Annotations = map[string]string{}
	}
	dep.Spec.Template.Annotations[hubGatewayReloadAnnot] = time.Now().UTC().Format(time.RFC3339Nano)
	return c.Update(ctx, &dep)
}

func restorePrimaryLLDAPClusterIP(ctx context.Context, c client.Client, ns string) error {
	var svc corev1.Service
	err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: lldapServiceName}, &svc)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if svc.Spec.Type == corev1.ServiceTypeClusterIP && httpNodePort(&svc) == 0 {
		return nil
	}
	patch := svc.DeepCopy()
	patch.Spec.Type = corev1.ServiceTypeClusterIP
	clearHTTPNodePort(patch)
	for i := range patch.Spec.Ports {
		patch.Spec.Ports[i].NodePort = 0
	}
	return c.Patch(ctx, patch, client.MergeFrom(&svc))
}

func desiredLLDAPWebUIService(ns, mode string, nodePort int32) *corev1.Service {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      lldapWebUIServiceName,
			Namespace: ns,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "fleet-console",
				"app.kubernetes.io/component": "lldap-webui",
				"fleet.umkim.com/managed-by":  "fleet-site-controller",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app": lldapAppLabel,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       lldapHTTPPortName,
					Port:       17170,
					TargetPort: intstr.FromString(lldapHTTPPortName),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
	// wireguard mode: a NodePort the hub firewall keeps off the public NIC, so it
	// is reachable only over the overlay. authelia mode stays ClusterIP and is
	// reached through the hub gateway route.
	if mode == fleetv1alpha1.LldapWebUIExposureWireguard {
		svc.Spec.Type = corev1.ServiceTypeNodePort
		svc.Spec.Ports[0].NodePort = nodePort
	}
	return svc
}

func lldapWebUIServiceEqual(a, b *corev1.Service) bool {
	if a.Spec.Type != b.Spec.Type {
		return false
	}
	return httpNodePort(a) == httpNodePort(b) && len(a.Spec.Ports) == len(b.Spec.Ports)
}

func clearHTTPNodePort(svc *corev1.Service) {
	for i := range svc.Spec.Ports {
		if svc.Spec.Ports[i].Name == lldapHTTPPortName {
			svc.Spec.Ports[i].NodePort = 0
		}
	}
}

func httpNodePort(svc *corev1.Service) int32 {
	for _, p := range svc.Spec.Ports {
		if p.Name == lldapHTTPPortName {
			return p.NodePort
		}
	}
	return 0
}
