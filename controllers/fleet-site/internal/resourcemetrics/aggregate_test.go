package resourcemetrics

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func readyNode(name string, cpu, mem string) corev1.Node {
	return corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(cpu),
				corev1.ResourceMemory: resource.MustParse(mem),
			},
		},
	}
}

func TestAggregateCluster_utilization(t *testing.T) {
	nodes := []corev1.Node{
		readyNode("a", "4", "8Gi"),
		readyNode("b", "4", "8Gi"),
	}
	usage := map[string]corev1.ResourceList{
		"a": {
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
		"b": {
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
	}
	u := AggregateCluster(nodes, usage, nil, nil)
	if !u.MetricsOK {
		t.Fatalf("expected metrics ok")
	}
	if u.CPUUtil != 25 {
		t.Fatalf("cpu util: got %d want 25", u.CPUUtil)
	}
	if u.MemoryUtil != 25 {
		t.Fatalf("mem util: got %d want 25", u.MemoryUtil)
	}
	if !u.CapacityOK {
		t.Fatalf("expected capacity ok")
	}
	if u.AllocCPUMilli != 8000 {
		t.Fatalf("alloc cpu: got %d want 8000", u.AllocCPUMilli)
	}
	if u.MaxNodeCPUMilli != 4000 {
		t.Fatalf("max node cpu: got %d want 4000", u.MaxNodeCPUMilli)
	}
}

func TestAggregateCluster_missingUsage(t *testing.T) {
	nodes := []corev1.Node{readyNode("a", "2", "4Gi")}
	u := AggregateCluster(nodes, nil, nil, nil)
	if u.MetricsOK {
		t.Fatalf("expected metrics not ok without usage")
	}
	if !u.CapacityOK {
		t.Fatalf("expected capacity ok from node allocatable")
	}
	if u.MaxNodeMemBytes != 4*1024*1024*1024 {
		t.Fatalf("max node mem: got %d", u.MaxNodeMemBytes)
	}
}
