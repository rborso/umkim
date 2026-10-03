package v1alpha1

import "testing"

func TestEffectiveWanAddressModeExplicit(t *testing.T) {
	spec := FleetSiteSpec{}
	ent := FleetSiteIngressSpec{WanAddressMode: WanAddressModeDNS}
	if got := spec.EffectiveWanAddressMode(ent); got != WanAddressModeDNS {
		t.Fatalf("got %q want dns", got)
	}
}

func TestEffectiveWanAddressModeWanEgress(t *testing.T) {
	spec := FleetSiteSpec{
		Edge: FleetSiteEdgeSpec{
			WanEgress: &FleetSiteWanEgressSpec{Enabled: true},
		},
	}
	ent := FleetSiteIngressSpec{Address: "203.0.113.1"}
	if got := spec.EffectiveWanAddressMode(ent); got != WanAddressModeDNS {
		t.Fatalf("got %q want dns", got)
	}
}

func TestEffectiveWanAddressModeExpectedWanIpFixed(t *testing.T) {
	spec := FleetSiteSpec{
		Edge: FleetSiteEdgeSpec{
			WanEgress: &FleetSiteWanEgressSpec{Enabled: true, ExpectedWanIp: "203.0.113.1"},
		},
	}
	ent := FleetSiteIngressSpec{Address: "203.0.113.1"}
	if got := spec.EffectiveWanAddressMode(ent); got != WanAddressModeFixed {
		t.Fatalf("got %q want fixed", got)
	}
}

func TestEffectiveWanAddressModePrivateAddress(t *testing.T) {
	spec := FleetSiteSpec{}
	ent := FleetSiteIngressSpec{Address: "172.16.10.4"}
	if got := spec.EffectiveWanAddressMode(ent); got != WanAddressModeDNS {
		t.Fatalf("got %q want dns", got)
	}
}

func TestEffectiveWanAddressModePublicDefaultFixed(t *testing.T) {
	spec := FleetSiteSpec{}
	ent := FleetSiteIngressSpec{Address: "203.0.113.1"}
	if got := spec.EffectiveWanAddressMode(ent); got != WanAddressModeFixed {
		t.Fatalf("got %q want fixed", got)
	}
}
