package llm

import (
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDiscoverBackendsFromApps_sameModelTwoClusters(t *testing.T) {
	apps := []fleetv1alpha1.FleetApp{
		{
			Spec: fleetv1alpha1.FleetAppSpec{
				Chart:       "llama-cpp-helm",
				ClusterName: "lab-gpu-0",
				ValuesYAML:  "model: Qwen3.5-2B-UD\n",
			},
			Status: fleetv1alpha1.FleetAppStatus{Phase: fleetv1alpha1.AppPhaseReady},
		},
		{
			Spec: fleetv1alpha1.FleetAppSpec{
				Chart:       "llama-cpp-helm",
				ClusterName: "lab-single-0",
				ValuesYAML:  "model: Qwen3.5-2B-UD\n",
			},
			Status: fleetv1alpha1.FleetAppStatus{Phase: fleetv1alpha1.AppPhaseReady},
		},
	}
	edge := map[string]string{
		"lab-gpu-0":    "10.10.0.16",
		"lab-single-0": "10.10.0.10",
	}
	got := DiscoverBackendsFromApps(apps, edge)
	if len(got) != 2 {
		t.Fatalf("expected 2 backends, got %d", len(got))
	}
	if got[0].Model != "Qwen3.5-2B-UD" || got[1].Model != "Qwen3.5-2B-UD" {
		t.Fatalf("unexpected models: %+v", got)
	}
}

func TestDiscoverBackendsFromApps_skipsLlmBackendFalse(t *testing.T) {
	apps := []fleetv1alpha1.FleetApp{
		{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					fleetv1alpha1.FleetAppAnnotationLLMBackend: "false",
				},
			},
			Spec: fleetv1alpha1.FleetAppSpec{
				Chart:       "llama-cpp-helm",
				ClusterName: "lab-gpu-0",
				ValuesYAML:  "model: Qwen3.5-2B-UD\n",
			},
			Status: fleetv1alpha1.FleetAppStatus{Phase: fleetv1alpha1.AppPhaseReady},
		},
	}
	edge := map[string]string{"lab-gpu-0": "10.10.0.16"}
	got := DiscoverBackendsFromApps(apps, edge)
	if len(got) != 0 {
		t.Fatalf("expected 0 backends, got %d", len(got))
	}
}

func TestMergeBackends_dropsStaleAliasAtSameAPIBase(t *testing.T) {
	existing := []fleetv1alpha1.FleetLLMBackend{
		{Model: "Qwen3.5-2B-UD-single", APIBase: "http://10.10.0.10:8080/v1"},
		{Model: "Qwen3.5-2B-UD", APIBase: "http://10.10.0.10:8080/v1"},
	}
	auto := []fleetv1alpha1.FleetLLMBackend{
		{Model: "Qwen3.5-2B-UD", APIBase: "http://10.10.0.10:8080/v1"},
		{Model: "Qwen3.5-2B-UD", APIBase: "http://10.10.0.16:8080/v1"},
	}
	got := MergeBackends(existing, auto)
	if len(got) != 2 {
		t.Fatalf("expected 2 backends, got %d: %+v", len(got), got)
	}
}
