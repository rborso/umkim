package ingress

import "testing"

func TestValidateRouteHost(t *testing.T) {
	pf := ClusterIngress{IngressID: "homelab-pf", Method: "port_forward", Wildcard: "*.homelab.example.test"}
	for _, host := range []string{
		"gitea.homelab.example.test",
		"a.b.homelab.example.test",
		"homelab.example.test",
		"GITEA.HOMELAB.EXAMPLE.TEST",
	} {
		if err := ValidateRouteHost(pf, host); err != nil {
			t.Errorf("host %q: unexpected error %v", host, err)
		}
	}
	for _, host := range []string{
		"gitea.other.example.test",
		"homelab.example.test.evil.test",
		"evilhomelab.example.test",
	} {
		if err := ValidateRouteHost(pf, host); err == nil {
			t.Errorf("host %q: want error, got nil", host)
		}
	}
	if err := ValidateRouteHost(pf, "  "); err != nil {
		t.Errorf("empty host must be accepted (no host set), got %v", err)
	}
	rp := ClusterIngress{IngressID: "edge-rp", Method: "reverse_proxy"}
	if err := ValidateRouteHost(rp, "anything.example.test"); err != nil {
		t.Errorf("reverse_proxy without a wildcard must accept any host, got %v", err)
	}
	rpZoned := ClusterIngress{IngressID: "edge-rp", Method: "reverse_proxy", Wildcard: "*.devtest.example.test"}
	if err := ValidateRouteHost(rpZoned, "outside.example.test"); err == nil {
		t.Error("reverse_proxy with a wildcard must reject a host outside it")
	}
}
