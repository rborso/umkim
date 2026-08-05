package clusterpick

import "testing"

func TestPickSpreadSiteDiversity(t *testing.T) {
	candidates := []ClusterCandidate{
		{Name: "virt01", Site: "homelab", Provider: "k8s-virt01"},
		{Name: "virt02", Site: "homelab", Provider: "k8s-virt02"},
		{Name: "remote1", Site: "lab-remote", Provider: "k8s-remote1"},
	}
	got, err := PickSpread(candidates, 2, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 got %d", len(got))
	}
	if got[0].Site == got[1].Site {
		t.Fatalf("expected distinct sites, got %q and %q", got[0].Site, got[1].Site)
	}
}

func TestPickSpreadInsufficientSites(t *testing.T) {
	candidates := []ClusterCandidate{
		{Name: "virt01", Site: "homelab", Provider: "k8s-virt01"},
		{Name: "virt02", Site: "homelab", Provider: "k8s-virt02"},
	}
	_, err := PickSpread(candidates, 2, true)
	if err == nil {
		t.Fatal("expected error when only one site available")
	}
}
