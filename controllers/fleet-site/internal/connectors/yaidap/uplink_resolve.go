package yaidap

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// ResolveHypervisorUplink finds the hypervisor uplink that offers a network,
// from the hypervisor's host_vars, and returns its name and the bridge to
// attach to.
//
// A guest must attach to the uplink that actually carries its network. Assuming
// the first uplink (n0p0) produced guests whose hypervisor_uplink pointed at a
// different physical network; YAIDAP then reported
//
//	guest physical_network='mgmt_access_l1', candidates=[]
//
// and refused to create the VM.
func ResolveHypervisorUplink(hypervisorHostVars, networkName string) (uplink, bridge string) {
	want := strings.TrimSpace(networkName)
	if strings.TrimSpace(hypervisorHostVars) == "" || want == "" {
		return "", ""
	}
	var hv struct {
		Uplinks []struct {
			Name     string `yaml:"name"`
			Networks []struct {
				NetworkName string `yaml:"network_name"`
				BridgeName  string `yaml:"bridge_name"`
			} `yaml:"networks"`
		} `yaml:"uplinks"`
	}
	if err := yaml.Unmarshal([]byte(hypervisorHostVars), &hv); err != nil {
		return "", ""
	}
	for _, u := range hv.Uplinks {
		for _, n := range u.Networks {
			if !strings.EqualFold(strings.TrimSpace(n.NetworkName), want) {
				continue
			}
			b := strings.TrimSpace(n.BridgeName)
			if b == "" {
				b = u.Name + "_" + want
			}
			return u.Name, b
		}
	}
	return "", ""
}
