package routesync

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSelectHelmWorkloadService(t *testing.T) {
	services := []corev1.Service{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "gitea-test-ssh"},
			Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 22}}},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "gitea-test-http",
				Labels: map[string]string{"app.kubernetes.io/instance": "gitea-test"},
			},
			Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 80, Name: "http"}}},
		},
	}
	name, port, ok := selectHelmWorkloadService(services, "gitea-test")
	if !ok {
		t.Fatal("expected service")
	}
	if name != "gitea-test-http" || port != 80 {
		t.Fatalf("got %s:%d", name, port)
	}
}

func TestSelectHelmWorkloadServiceExactReleaseNameNon80(t *testing.T) {
	services := []corev1.Service{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "uptime-kuma-smoke",
				Labels: map[string]string{"app.kubernetes.io/instance": "uptime-kuma-smoke"},
			},
			Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3001, Name: "http"}}},
		},
	}
	name, port, ok := selectHelmWorkloadService(services, "uptime-kuma-smoke")
	if !ok || name != "uptime-kuma-smoke" || port != 3001 {
		t.Fatalf("got %s:%d ok=%v want uptime-kuma-smoke:3001", name, port, ok)
	}
}


func TestSelectHelmWorkloadServicePrefersServerOverML(t *testing.T) {
	services := []corev1.Service{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "immich2-machine-learning",
				Labels: map[string]string{"app.kubernetes.io/instance": "immich2"},
			},
			Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3003}}},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "immich2-postgresql",
				Labels: map[string]string{"app.kubernetes.io/instance": "immich2"},
			},
			Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 5432}}},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "immich2-server",
				Labels: map[string]string{"app.kubernetes.io/instance": "immich2"},
			},
			Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 2283}}},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "immich2-valkey",
				Labels: map[string]string{"app.kubernetes.io/instance": "immich2"},
			},
			Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 6379}}},
		},
	}
	name, port, ok := selectHelmWorkloadService(services, "immich2")
	if !ok {
		t.Fatal("expected service")
	}
	if name != "immich2-server" || port != 2283 {
		t.Fatalf("got %s:%d want immich2-server:2283", name, port)
	}
}

func TestPlacementTargetActive(t *testing.T) {
	if placementTargetActive(placementTarget{Status: "deleted"}) {
		t.Fatal("deleted target should be inactive")
	}
	if !placementTargetActive(placementTarget{Status: "active"}) {
		t.Fatal("active target should be active")
	}
}
