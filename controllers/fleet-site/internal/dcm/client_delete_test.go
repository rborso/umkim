package dcm

import "testing"

func TestInstanceTargetsCluster(t *testing.T) {
	inst := CatalogItemInstance{
		Spec: CatalogItemInstanceSpec{
			UserValues: []UserValue{UserValueFromString("routing.cluster", "lab-gpu03")},
		},
	}
	if !InstanceTargetsCluster(inst, "lab-gpu03") {
		t.Fatal("expected match on routing.cluster")
	}
	if InstanceTargetsCluster(inst, "other") {
		t.Fatal("did not expect match")
	}
}

func TestProviderNameTargetsCluster(t *testing.T) {
	if !ProviderNameTargetsCluster("helm-lab-gpu03", "lab-gpu03") {
		t.Fatal("expected helm provider match")
	}
	if ProviderNameTargetsCluster("helm-lab-gpu03", "lab-gpu") {
		t.Fatal("suffix must be exact cluster name")
	}
}
