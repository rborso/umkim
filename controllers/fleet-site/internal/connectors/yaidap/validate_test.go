package yaidap

import (
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

func TestValidateGuestSpec(t *testing.T) {
	if err := ValidateGuestSpec("vm01", GuestSpec{AnsibleHost: "203.0.113.10", VmLocation: "least-utilized"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateGuestSpec("vm01", GuestSpec{AnsibleHost: "not-an-ip"}); err == nil {
		t.Fatal("expected invalid IP error")
	}
}

func TestProjectGuestsYAML(t *testing.T) {
	bundle, err := Project(Catalog{}, GuestStore{
		"lab-single-1-0": {AnsibleHost: "172.16.10.161", VmLocation: "least-utilized", Memory: 4096, Cores: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Files["guests.yml"] == "" {
		t.Fatal("missing guests.yml")
	}
	if bundle.Files["host_vars/lab-single-1-0.yml"] == "" {
		t.Fatal("missing host_vars")
	}
}

func TestGuestSpecFromHost(t *testing.T) {
	spec := GuestSpecFromHost(fleetv1alpha1.SiteHostRecord{
		Name:        "h1",
		Kind:        fleetv1alpha1.SiteHostKindVM,
		AnsibleHost: "172.16.10.161",
		Flavor:      map[string]string{"memory": "4096", "cores": "2"},
	})
	if spec.AnsibleHost != "172.16.10.161" {
		t.Fatalf("ansible_host=%q", spec.AnsibleHost)
	}
	if spec.VmLocation != "least-utilized" {
		t.Fatalf("vm_location=%q", spec.VmLocation)
	}
	if spec.Memory != 4096 || spec.Cores != 2 {
		t.Fatalf("flavor memory/cores=%d/%d", spec.Memory, spec.Cores)
	}
}
