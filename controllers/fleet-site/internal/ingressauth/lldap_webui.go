package ingressauth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

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
	for _, name := range sortedKeys(sliceToSet(desired.EnabledClusters)) {
		cc := desired.ClusterConfigs[name]
		for _, r := range cc.AccessRules {
			parts = append(parts, "cr="+name+"/"+r.Domain+"="+strings.Join(r.Groups, ","))
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

// ReconcileHubLLDAPWebUI manages fleet-console/lldap-webui NodePort (http only; LDAP stays ClusterIP).
func ReconcileHubLLDAPWebUI(ctx context.Context, c client.Client, cfg config.Config, fia *fleetv1alpha1.FleetIngressAuth) error {
	consoleNS := cfg.ConsoleNamespace
	if consoleNS == "" {
		consoleNS = "fleet-console"
	}
	nodePort := int32(cfg.LldapWebUINodePort)
	if nodePort <= 0 {
		nodePort = 30917
	}

	want := fia.Spec.HubExposeLldapWebUI
	hub := fia.Status.Hub

	if want {
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

	desired := desiredLLDAPWebUIService(consoleNS, nodePort)
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
		patch := desired.DeepCopy()
		patch.ResourceVersion = webUI.ResourceVersion
		if err := c.Patch(ctx, patch, client.MergeFrom(&webUI)); err != nil {
			hub.LLDAPWebUIPhase = fleetv1alpha1.LldapWebUIPhaseFailed
			hub.LLDAPWebUIMessage = err.Error()
			fia.Status.Hub = hub
			_ = c.Status().Update(ctx, fia)
			return err
		}
	}

	hub.LLDAPWebUIPhase = fleetv1alpha1.LldapWebUIPhaseReady
	hub.LLDAPWebUIMessage = ""
	hub.LLDAPWebUIEnabled = true
	hub.LLDAPWebUIURL = fmt.Sprintf("http://%s:%d", cfg.HubWireguardIP, nodePort)
	fia.Status.Hub = hub
	return c.Status().Update(ctx, fia)
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

func desiredLLDAPWebUIService(ns string, nodePort int32) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      lldapWebUIServiceName,
			Namespace: ns,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "fleet-console",
				"app.kubernetes.io/component":  "lldap-webui",
				"fleet.umkim.com/managed-by": "fleet-site-controller",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Selector: map[string]string{
				"app": lldapAppLabel,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       lldapHTTPPortName,
					Port:       17170,
					TargetPort: intstr.FromString(lldapHTTPPortName),
					NodePort:   nodePort,
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
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
