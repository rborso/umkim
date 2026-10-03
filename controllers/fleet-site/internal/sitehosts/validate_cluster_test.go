package sitehosts

import (
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

func TestValidateClusterIntentEnroll(t *testing.T) {
	if err := ValidateClusterIntent(&fleetv1alpha1.SiteHostClusterIntent{
		Mode:        fleetv1alpha1.SiteHostClusterModeEnroll,
		ClusterName: "lab-0",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateClusterIntentHARequiresVIP(t *testing.T) {
	err := ValidateClusterIntent(&fleetv1alpha1.SiteHostClusterIntent{
		Mode:     fleetv1alpha1.SiteHostClusterModeEnroll,
		Topology: fleetv1alpha1.ClusterTopologyHA3Node,
	})
	if err == nil {
		t.Fatal("expected error for missing controlPlaneVip")
	}
}

func TestValidateClusterIntentAddWorker(t *testing.T) {
	err := ValidateClusterIntent(&fleetv1alpha1.SiteHostClusterIntent{
		Mode: fleetv1alpha1.SiteHostClusterModeAddWorker,
	})
	if err == nil {
		t.Fatal("expected error for missing targetCluster")
	}
}
