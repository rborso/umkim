package providers

import (
	"context"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type NodePortAlloc struct {
	K8s      int
	Helm     int
	Kubevirt int
}

func AllocateNodePorts(ctx context.Context, c client.Client, ns string, install fleetv1alpha1.ProviderInstallSpec) (NodePortAlloc, error) {
	usedK8s := map[int]bool{}
	usedHelm := map[int]bool{}
	usedKubevirt := map[int]bool{}
	var list fleetv1alpha1.FleetClusterJoinList
	if err := c.List(ctx, &list, client.InNamespace(ns)); err != nil {
		return NodePortAlloc{}, err
	}
	for i := range list.Items {
		p := list.Items[i].Spec.Providers
		if p.K8sNodePort > 0 {
			usedK8s[p.K8sNodePort] = true
		}
		if p.HelmNodePort > 0 {
			usedHelm[p.HelmNodePort] = true
		}
		if p.KubevirtSPNodeport > 0 {
			usedKubevirt[p.KubevirtSPNodeport] = true
		}
	}
	out := NodePortAlloc{
		K8s:  nextFreePort(usedK8s, fleetv1alpha1.ProviderK8sNodePortBase, 30209),
		Helm: nextFreePort(usedHelm, fleetv1alpha1.ProviderHelmNodePortBase, 30209),
	}
	if install.Kubevirt {
		out.Kubevirt = nextFreePort(usedKubevirt, fleetv1alpha1.ProviderKubevirtSPNodePortBase, 30239)
	}
	return out, nil
}

func nextFreePort(used map[int]bool, base, max int) int {
	for p := base; p <= max; p++ {
		if !used[p] {
			return p
		}
	}
	return base
}
