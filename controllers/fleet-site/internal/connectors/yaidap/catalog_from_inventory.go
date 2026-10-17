package yaidap

import (
	"gopkg.in/yaml.v3"

	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
)

// CatalogFromInventory builds a Catalog from the modern inventory files:
// group_vars/all/all.yml (networks, storages, sizing) and hypervisors.yml
// (the hypervisor pool).
//
// The controller's catalog used to come only from catalog.json, which is a
// legacy key that a modern inventory does not have. DecodeCatalog returns an
// empty Catalog for blank input rather than an error, so an inventory without
// catalog.json produced a catalog with no networks at all — every network
// lookup missed, and physicalNetworkFor silently fell back to the lab physical
// network. A VM on any other network then failed to resolve a hypervisor
// uplink at create time.
func CatalogFromInventory(data map[string]string) Catalog {
	var cat Catalog

	var gv struct {
		Memory   int          `yaml:"memory"`
		Cores    int          `yaml:"cores"`
		Networks []NetworkRef `yaml:"networks"`
		Storages []StorageRef `yaml:"storages"`
	}
	// The ConfigMap is keyed by connectors.FileKey(path), not by the logical
	// path, so read through the accessor the rest of the code uses.
	if raw, ok := connectors.GetInventoryFile(data, "group_vars/all/all.yml"); ok {
		if err := yaml.Unmarshal([]byte(raw), &gv); err == nil {
			cat.Networks = gv.Networks
			cat.Storages = gv.Storages
			cat.Defaults.Memory = gv.Memory
			cat.Defaults.Cores = gv.Cores
		}
	}

	// hypervisors.yml: hypervisors.hosts.<name>
	var hv struct {
		Hypervisors struct {
			Hosts map[string]map[string]any `yaml:"hosts"`
		} `yaml:"hypervisors"`
	}
	if raw, ok := connectors.GetInventoryFile(data, "hypervisors.yml"); ok {
		if err := yaml.Unmarshal([]byte(raw), &hv); err == nil {
			for name := range hv.Hypervisors.Hosts {
				cat.Hypervisors = append(cat.Hypervisors, name)
			}
		}
	}

	return cat
}
