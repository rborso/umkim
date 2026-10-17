package yaidap

import (
	"encoding/json"
)

const SchemaVersion = 1

// Catalog holds site-level YAIDAP inventory (networks, storages, defaults).
type Catalog struct {
	Networks    []NetworkRef               `json:"networks,omitempty"`
	Storages    []StorageRef               `json:"storages,omitempty"`
	Hypervisors []string                   `json:"hypervisors,omitempty"`
	Defaults    GuestDefaults              `json:"defaults,omitempty"`
	Extra       map[string]json.RawMessage `json:"extra,omitempty"`
}

type NetworkRef struct {
	// The inventory spells this `network_name` (see group_vars/all/all.yml).
	// yaml.v3 does not honour `json` tags, so without the yaml tag the field
	// stayed empty and every network lookup missed — which silently fell back to
	// the lab physical network and made a VM on any other network unresolvable.
	Name             string `json:"network_name" yaml:"network_name"`
	PhysicalNetwork  string `json:"physical_network,omitempty" yaml:"physical_network,omitempty"`
	Zone             string `json:"zone,omitempty" yaml:"zone,omitempty"`
	BridgeNameSuffix string `json:"bridge_name_suffix,omitempty" yaml:"bridge_name_suffix,omitempty"`
}

type StorageRef struct {
	Name string `json:"network_name" yaml:"network_name"`
	Pool string `json:"pool,omitempty"`
}

// GuestDefaults are applied when rendering host_vars for guests without explicit values.
type GuestDefaults struct {
	OS                   string `json:"os,omitempty"`
	Memory               int    `json:"memory,omitempty"`
	Cores                int    `json:"cores,omitempty"`
	DiskGi               int    `json:"diskGi,omitempty"`
	Storage              string `json:"storage,omitempty"`
	PhysicalNetwork      string `json:"physical_network,omitempty"`
	Zone                 string `json:"zone,omitempty"`
	NetworkName          string `json:"network_name,omitempty"`
	ProvisioningProvider string `json:"provisioning_provider,omitempty"`
}

// GuestStore is guests.json: map guestName → spec.
type GuestStore map[string]GuestSpec

// GuestSpec mirrors YAIDAP guest host_vars + guests.yml host entry fields.
type GuestSpec struct {
	Kind                 string                     `json:"kind,omitempty"`
	AnsibleHost          string                     `json:"ansible_host,omitempty"`
	VmLocation           string                     `json:"vm_location,omitempty"`
	OS                   string                     `json:"os,omitempty"`
	Memory               int                        `json:"memory,omitempty"`
	Cores                int                        `json:"cores,omitempty"`
	Legacy               *bool                      `json:"legacy,omitempty"`
	ConfigureFirewalld   *bool                      `json:"configure_firewalld,omitempty"`
	ProvisioningProvider string                     `json:"provisioning_provider,omitempty"`
	Disks                []DiskSpec                 `json:"disks,omitempty"`
	Uplinks              []UplinkSpec               `json:"uplinks,omitempty"`
	Absent               bool                       `json:"absent,omitempty"`
	Extra                map[string]json.RawMessage `json:"extra,omitempty"`
}

type DiskSpec struct {
	Name     string `json:"network_name" yaml:"network_name"`
	Disk     string `json:"disk,omitempty"`
	Bootdisk bool   `json:"bootdisk,omitempty"`
	Device   int    `json:"device,omitempty"`
	Format   string `json:"format,omitempty"`
	Size     int    `json:"size,omitempty"`
	Storage  string `json:"storage,omitempty"`
}

type UplinkSpec struct {
	Name            string        `json:"network_name" yaml:"network_name"`
	PhysicalNetwork string        `json:"physical_network,omitempty"`
	MACAddress      string        `json:"mac_address,omitempty"`
	Networks        []NetworkSpec `json:"networks,omitempty"`
}

type NetworkSpec struct {
	Zone             string   `json:"zone,omitempty"`
	NetworkName      string   `json:"network_name,omitempty"`
	Model            string   `json:"model,omitempty"`
	CloudInit        bool     `json:"cloudinit,omitempty"`
	PXE              bool     `json:"pxe,omitempty"`
	VMNetwork        bool     `json:"vm_network,omitempty"`
	HypervisorUplink string   `json:"hypervisor_uplink,omitempty"`
	BridgeName       string   `json:"bridge_name,omitempty"`
	IP               string   `json:"ip,omitempty"`
	DefaultRoute     bool     `json:"default_route,omitempty"`
	TCPOpen          []int    `json:"tcp_open,omitempty"`
	UDPOpen          []int    `json:"udp_open,omitempty"`
	ServicesOpen     []string `json:"services_open,omitempty"`
}

// GuestsYAML is the guests.yml top-level structure.
type GuestsYAML struct {
	Guests struct {
		Hosts map[string]GuestEntry `json:"hosts" yaml:"hosts"`
	} `json:"guests" yaml:"guests"`
	External struct {
		Hosts map[string]GuestEntry `json:"hosts" yaml:"hosts"`
	} `json:"external" yaml:"external"`
}

type GuestEntry struct {
	AnsibleHost string `json:"ansible_host,omitempty" yaml:"ansible_host,omitempty"`
	VmLocation  string `json:"vm_location,omitempty" yaml:"vm_location,omitempty"`
}
