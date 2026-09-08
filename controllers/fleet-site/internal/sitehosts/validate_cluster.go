package sitehosts

import (
	"fmt"
	"regexp"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

var clusterNamePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// ValidateClusterIntent checks cluster automation intent on a host record.
func ValidateClusterIntent(intent *fleetv1alpha1.SiteHostClusterIntent) error {
	if intent == nil {
		return nil
	}
	mode := strings.TrimSpace(intent.Mode)
	if mode == "" {
		return nil
	}
	switch mode {
	case fleetv1alpha1.SiteHostClusterModeEnroll,
		fleetv1alpha1.SiteHostClusterModeAddWorker,
		fleetv1alpha1.SiteHostClusterModeReplaceControl:
	default:
		return fmt.Errorf("clusterIntent.mode must be enroll, add_worker, or replace_control")
	}
	topology := intent.EffectiveTopology()
	if topology != fleetv1alpha1.ClusterTopologySingleNode && topology != fleetv1alpha1.ClusterTopologyHA3Node {
		return fmt.Errorf("clusterIntent.topology must be single_node or ha_3_node")
	}
	switch mode {
	case fleetv1alpha1.SiteHostClusterModeEnroll:
		name := strings.TrimSpace(intent.ClusterName)
		if name != "" && (len(name) > 63 || !clusterNamePattern.MatchString(name)) {
			return fmt.Errorf("invalid clusterIntent.clusterName %q", name)
		}
		if topology == fleetv1alpha1.ClusterTopologyHA3Node && strings.TrimSpace(intent.ControlPlaneVip) == "" {
			return fmt.Errorf("clusterIntent.controlPlaneVip is required for ha_3_node enroll")
		}
	case fleetv1alpha1.SiteHostClusterModeAddWorker:
		if strings.TrimSpace(intent.TargetCluster) == "" {
			return fmt.Errorf("clusterIntent.targetCluster is required for add_worker")
		}
	case fleetv1alpha1.SiteHostClusterModeReplaceControl:
		if strings.TrimSpace(intent.TargetCluster) == "" {
			return fmt.Errorf("clusterIntent.targetCluster is required for replace_control")
		}
		if strings.TrimSpace(intent.ReplaceNode) == "" {
			return fmt.Errorf("clusterIntent.replaceNode is required for replace_control")
		}
	}
	return nil
}
