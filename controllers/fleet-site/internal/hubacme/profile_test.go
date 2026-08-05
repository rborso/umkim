package hubacme

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDetectLegacyHostNetwork(t *testing.T) {
	replicas := int32(1)
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "gateway", Namespace: gatewayNS},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{HostNetwork: true},
				},
			},
		},
	).Build()

	p := Detect(context.Background(), c)
	if p.SolverEnabled {
		t.Fatal("legacy hub should not use solver")
	}
	if !p.LegacyScale {
		t.Fatal("legacy hub should use legacy gateway scale")
	}
	if p.Preflight {
		t.Fatal("legacy hub should disable staging preflight")
	}
}

func TestDetectHASolver(t *testing.T) {
	replicas := int32(3)
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "gateway", Namespace: gatewayNS},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{HostNetwork: false},
				},
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "gateway-acme", Namespace: gatewayNS},
		},
	).Build()

	p := Detect(context.Background(), c)
	if !p.SolverEnabled {
		t.Fatal("HA hub with gateway-acme should use solver")
	}
	if p.LegacyScale {
		t.Fatal("HA solver hub should not use legacy scale")
	}
	if !p.Preflight {
		t.Fatal("HA solver hub should enable staging preflight")
	}
}

func TestInventoryYAMLLines(t *testing.T) {
	lines := InventoryYAMLLines(legacyProfile())
	if len(lines) != 3 {
		t.Fatalf("want 3 lines, got %d", len(lines))
	}
	if lines[0] != "dcm_gateway_acme_solver_enabled: false" {
		t.Fatalf("unexpected: %s", lines[0])
	}
}
