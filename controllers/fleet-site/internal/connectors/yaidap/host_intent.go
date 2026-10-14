package yaidap

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Markers on the placeholder host_vars written by the console/CLI add-host flow.
// PromoteDiscoveryFiles lets a discovery draft replace a file that still carries
// one of these markers (an un-promoted stub), while protecting hand-edited
// host_vars that no longer do.
const (
	discoveryStubMarker = "# Discovery stub for "
	bareMetalStubMarker = "# Bare-metal node "
)

// HostIntent is the minimal, kind-aware description of a host to add to a site's
// YAIDAP inventory. Everything not set is filled from the site catalog.
type HostIntent struct {
	Kind           string           `json:"kind"`
	Name           string           `json:"name"`
	AnsibleHost    string           `json:"ansibleHost,omitempty"`
	MACAddress     string           `json:"macAddress,omitempty"`
	Hypervisor     string           `json:"hypervisor,omitempty"`
	Cores          int              `json:"cores,omitempty"`
	Memory         int              `json:"memory,omitempty"`
	DiskSizeGi     int              `json:"diskSizeGi,omitempty"`
	Disks          []HostDiskIntent `json:"disks,omitempty"`
	NetworkName    string           `json:"networkName,omitempty"`
	Zone           string           `json:"zone,omitempty"`
	Storage        string           `json:"storage,omitempty"`
	OS             string           `json:"os,omitempty"`
	Cluster        string           `json:"cluster,omitempty"`
	GPUPassthrough bool             `json:"gpuPassthrough,omitempty"`
	PXE            bool             `json:"pxe,omitempty"`
}

// HostDiskIntent is an additional (data) disk for a VM guest. The boot disk is
// `diskSizeGi`; these follow it in the rendered `disks:` list, which matters
// because YAIDAP takes `disks[0]` as the boot device.
type HostDiskIntent struct {
	Name    string `json:"name,omitempty"`
	SizeGi  int    `json:"sizeGi,omitempty"`
	Storage string `json:"storage,omitempty"`
	Format  string `json:"format,omitempty"`
}

// maxGuestDisks caps additional disks so the virtio device letters stay sane
// (vdb..vdi).
const maxGuestDisks = 8

func (h *HostIntent) Normalize() {
	h.Kind = strings.ToLower(strings.TrimSpace(h.Kind))
	h.Name = strings.TrimSpace(h.Name)
	h.AnsibleHost = strings.TrimSpace(h.AnsibleHost)
	h.MACAddress = strings.TrimSpace(h.MACAddress)
	h.Hypervisor = strings.TrimSpace(h.Hypervisor)
	h.NetworkName = strings.TrimSpace(h.NetworkName)
	h.Zone = strings.TrimSpace(h.Zone)
	h.Storage = strings.TrimSpace(h.Storage)
	h.OS = strings.TrimSpace(h.OS)
	h.Cluster = strings.TrimSpace(h.Cluster)
	kept := make([]HostDiskIntent, 0, len(h.Disks))
	for _, d := range h.Disks {
		d.Name = strings.TrimSpace(d.Name)
		d.Storage = strings.TrimSpace(d.Storage)
		d.Format = strings.TrimSpace(d.Format)
		kept = append(kept, d)
	}
	h.Disks = kept
}

func (h HostIntent) Validate() error {
	switch h.Kind {
	case "vm", "bare_metal", "hypervisor":
	default:
		return fmt.Errorf("kind must be vm, bare_metal, or hypervisor")
	}
	if h.Name == "" {
		return fmt.Errorf("name is required")
	}
	if h.Kind == "bare_metal" || h.Kind == "hypervisor" {
		if h.AnsibleHost == "" && h.MACAddress == "" {
			return fmt.Errorf("%s requires ansibleHost (SSH) or macAddress (PXE)", h.Kind)
		}
	}
	if len(h.Disks) > 0 {
		if h.Kind != "vm" {
			return fmt.Errorf("additional disks are only supported for vm guests")
		}
		if len(h.Disks) > maxGuestDisks {
			return fmt.Errorf("at most %d additional disks are supported", maxGuestDisks)
		}
		for i, d := range h.Disks {
			if d.SizeGi <= 0 {
				return fmt.Errorf("additional disk %d needs a size greater than 0", i+1)
			}
		}
	}
	return nil
}

func firstPositive(vals ...int) int {
	for _, v := range vals {
		if v > 0 {
			return v
		}
	}
	return 0
}

func applyCatalogDefaults(h *HostIntent, catalog Catalog) {
	d := catalog.Defaults
	h.OS = firstNonEmpty(h.OS, d.OS, "rocky-10")
	h.Cores = firstPositive(h.Cores, d.Cores, 2)
	h.Memory = firstPositive(h.Memory, d.Memory, 4096)
	h.DiskSizeGi = firstPositive(h.DiskSizeGi, d.DiskGi, 40)
	h.Storage = firstNonEmpty(h.Storage, d.Storage, "local")
	h.NetworkName = firstNonEmpty(h.NetworkName, d.NetworkName, "lab_nw")
	h.Zone = firstNonEmpty(h.Zone, d.Zone, "lab")
	if h.Hypervisor == "" && h.Kind == "vm" {
		h.Hypervisor = "least-utilized"
	}
	for i := range h.Disks {
		h.Disks[i].Storage = firstNonEmpty(h.Disks[i].Storage, h.Storage, "local")
		h.Disks[i].Format = firstNonEmpty(h.Disks[i].Format, "qcow2")
		if h.Disks[i].Name == "" {
			h.Disks[i].Name = fmt.Sprintf("disk%d", i+1)
		}
	}
}

func networkFor(catalog Catalog, name string) (NetworkRef, bool) {
	for _, n := range catalog.Networks {
		if strings.EqualFold(strings.TrimSpace(n.Name), strings.TrimSpace(name)) {
			return n, true
		}
	}
	return NetworkRef{}, false
}

func physicalNetworkFor(catalog Catalog, name string) string {
	if n, ok := networkFor(catalog, name); ok && strings.TrimSpace(n.PhysicalNetwork) != "" {
		return strings.TrimSpace(n.PhysicalNetwork)
	}
	return "lab_access_l1"
}

// deriveMAC builds a deterministic, collision-resistant MAC from an IPv4 address
// (52:54:00 + last three octets) so VMs do not all default to the same MAC.
func deriveMAC(ip string) string {
	parts := strings.Split(strings.TrimSpace(ip), ".")
	if len(parts) != 4 {
		return ""
	}
	var b [3]int
	for i := 0; i < 3; i++ {
		n, err := strconv.Atoi(parts[i+1])
		if err != nil || n < 0 || n > 255 {
			return ""
		}
		b[i] = n
	}
	return fmt.Sprintf("52:54:00:%02x:%02x:%02x", b[0], b[1], b[2])
}

func macOrDerive(h HostIntent) string {
	if m := strings.TrimSpace(h.MACAddress); m != "" {
		return m
	}
	if m := deriveMAC(h.AnsibleHost); m != "" {
		return m
	}
	return "52:54:00:00:00:01"
}

func yamlScalar(v string) string {
	// Keep values readable; quote only when needed.
	if v == "" {
		return `""`
	}
	if strings.ContainsAny(v, ":#{}[],&*!|>'\"%@`") || strings.HasPrefix(v, " ") || strings.HasSuffix(v, " ") {
		return fmt.Sprintf("%q", v)
	}
	return v
}

// BuildHostFiles merges a host intent into the site's inventory files and returns
// the files to write (path -> content). `current` holds the present content of
// the relevant files (missing entries are treated as empty).
func BuildHostFiles(current map[string]string, intent HostIntent, catalog Catalog) (map[string]string, error) {
	intent.Normalize()
	if err := intent.Validate(); err != nil {
		return nil, err
	}
	applyCatalogDefaults(&intent, catalog)

	out := map[string]string{}
	hostVarsPath := fmt.Sprintf("host_vars/%s.yml", intent.Name)

	switch intent.Kind {
	case "vm":
		entry := map[string]interface{}{"vm_location": intent.Hypervisor}
		if intent.AnsibleHost != "" {
			entry["ansible_host"] = intent.AnsibleHost
		}
		merged, err := mergeHostsYAML(current["guests.yml"], "guests", intent.Name, entry)
		if err != nil {
			return nil, err
		}
		out["guests.yml"] = merged
		out[hostVarsPath] = renderGuestHostVars(intent, catalog)
	case "bare_metal":
		entry := map[string]interface{}{}
		if intent.AnsibleHost != "" {
			entry["ansible_host"] = intent.AnsibleHost
		}
		merged, err := mergeHostsYAML(current["bare_metals.yml"], "bare_metals", intent.Name, entry)
		if err != nil {
			return nil, err
		}
		out["bare_metals.yml"] = merged
		out[hostVarsPath] = renderBareMetalHostVars(intent)
	case "hypervisor":
		entry := map[string]interface{}{}
		if intent.AnsibleHost != "" {
			entry["ansible_host"] = intent.AnsibleHost
		}
		merged, err := mergeHostsYAML(current["hypervisors.yml"], "hypervisors", intent.Name, entry)
		if err != nil {
			return nil, err
		}
		out["hypervisors.yml"] = merged
		out[hostVarsPath] = renderHypervisorHostVars(intent, catalog)
	}

	if intent.Cluster != "" {
		merged, err := mergeClusterGroup(current[fmt.Sprintf("%s.yml", intent.Cluster)], intent.Cluster, intent.Name)
		if err != nil {
			return nil, err
		}
		out[fmt.Sprintf("%s.yml", intent.Cluster)] = merged
	}
	return out, nil
}

// marshalInventoryYAML renders inventory YAML with the repo's 2-space indent so
// generated files match the hand-written templates and examples.
func marshalInventoryYAML(doc interface{}) (string, error) {
	var b strings.Builder
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return b.String(), nil
}

func mergeHostsYAML(current, topKey, name string, entry map[string]interface{}) (string, error) {
	doc := map[string]interface{}{}
	if strings.TrimSpace(current) != "" {
		if err := yaml.Unmarshal([]byte(current), &doc); err != nil {
			return "", fmt.Errorf("%s is not valid YAML: %w", topKey, err)
		}
	}
	top, _ := doc[topKey].(map[string]interface{})
	if top == nil {
		top = map[string]interface{}{}
		doc[topKey] = top
	}
	hosts, _ := top["hosts"].(map[string]interface{})
	if hosts == nil {
		hosts = map[string]interface{}{}
		top["hosts"] = hosts
	}
	prev, _ := hosts[name].(map[string]interface{})
	if prev == nil {
		prev = map[string]interface{}{}
	}
	for k, v := range entry {
		if v != nil && v != "" {
			prev[k] = v
		}
	}
	hosts[name] = prev
	return marshalInventoryYAML(doc)
}

// mergeClusterGroup adds a host to a root group file `<cluster>.yml` hosts list.
func mergeClusterGroup(current, cluster, host string) (string, error) {
	doc := map[string]interface{}{}
	if strings.TrimSpace(current) != "" {
		if err := yaml.Unmarshal([]byte(current), &doc); err != nil {
			return "", fmt.Errorf("%s.yml is not valid YAML: %w", cluster, err)
		}
	}
	group, _ := doc[cluster].(map[string]interface{})
	if group == nil {
		group = map[string]interface{}{}
		doc[cluster] = group
	}
	// Ansible (and ParseInventoryGroupMembers) expect the map form:
	//   cluster: {hosts: {node: {}}}
	// Accept the legacy list form on read so an older file is healed rather than
	// losing its members.
	hosts := map[string]interface{}{}
	switch raw := group["hosts"].(type) {
	case map[string]interface{}:
		for name, body := range raw {
			hosts[name] = body
		}
	case []interface{}:
		for _, h := range raw {
			if s, ok := h.(string); ok && strings.TrimSpace(s) != "" {
				hosts[s] = map[string]interface{}{}
			}
		}
	}
	if _, ok := hosts[host]; !ok {
		hosts[host] = map[string]interface{}{}
	}
	group["hosts"] = hosts
	return marshalInventoryYAML(doc)
}

func renderGuestHostVars(h HostIntent, catalog Catalog) string {
	physical := physicalNetworkFor(catalog, h.NetworkName)
	mac := macOrDerive(h)
	bridge := fmt.Sprintf("n0p0_%s", h.NetworkName)
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "# YAIDAP guest profile for %s. Generated by the console/CLI add-host flow.\n", h.Name)
	if h.AnsibleHost != "" {
		fmt.Fprintf(&b, "ansible_host: %s\n", h.AnsibleHost)
	}
	fmt.Fprintf(&b, "memory: %d\n", h.Memory)
	fmt.Fprintf(&b, "cores: %d\n", h.Cores)
	fmt.Fprintf(&b, "os: %s\n", h.OS)
	b.WriteString("provisioning_provider: libvirt\n")
	b.WriteString("configure_firewalld: false\n")
	if h.GPUPassthrough {
		fmt.Fprintf(&b, "vm_location: %s\n", h.Hypervisor)
		b.WriteString("gpu_passthrough:\n  enabled: true\n  auto_detect: false\n  include_audio_function: true\n  pci_devices:\n    - \"0000:01:00.0\"\n    - \"0000:01:00.1\"\n")
	}
	b.WriteString("disks:\n")
	b.WriteString("  - name: bootdisk\n    disk: vda\n    bootdisk: true\n    device: 0\n    format: qcow2\n")
	fmt.Fprintf(&b, "    size: %d\n", h.DiskSizeGi)
	fmt.Fprintf(&b, "    storage: %s\n", h.Storage)
	// Additional disks follow the boot disk: YAIDAP derives the target device
	// from the list position (disks[0] is boot), so order is significant.
	for i, d := range h.Disks {
		fmt.Fprintf(&b, "  - name: %s\n", yamlScalar(d.Name))
		fmt.Fprintf(&b, "    disk: vd%c\n", 'b'+rune(i))
		fmt.Fprintf(&b, "    device: %d\n", i+1)
		fmt.Fprintf(&b, "    format: %s\n", yamlScalar(d.Format))
		fmt.Fprintf(&b, "    size: %d\n", d.SizeGi)
		fmt.Fprintf(&b, "    storage: %s\n", yamlScalar(d.Storage))
	}
	b.WriteString("uplinks:\n")
	b.WriteString("  - name: n0p0\n")
	fmt.Fprintf(&b, "    physical_network: %s\n", yamlScalar(physical))
	fmt.Fprintf(&b, "    mac_address: %q\n", mac)
	b.WriteString("    networks:\n")
	fmt.Fprintf(&b, "      - zone: %s\n", yamlScalar(h.Zone))
	fmt.Fprintf(&b, "        network_name: %s\n", yamlScalar(h.NetworkName))
	b.WriteString("        model: virtio\n        cloudinit: true\n        pxe: false\n        vm_network: false\n")
	b.WriteString("        hypervisor_uplink: n0p0\n")
	fmt.Fprintf(&b, "        bridge_name: %s\n", yamlScalar(bridge))
	if h.AnsibleHost != "" {
		fmt.Fprintf(&b, "        ip: %q\n", h.AnsibleHost)
	} else {
		b.WriteString("        ip: \"\"\n")
	}
	b.WriteString("        default_route: true\n        tcp_open:\n          - 22\n        udp_open: []\n        services_open: []\n")
	return b.String()
}

func renderBareMetalHostVars(h HostIntent) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "%s%s (bare_metals.yml — not a libvirt guest).\n", bareMetalStubMarker, h.Name)
	if h.AnsibleHost != "" {
		fmt.Fprintf(&b, "ansible_host: %s\n", h.AnsibleHost)
	}
	fmt.Fprintf(&b, "os: %s\n", h.OS)
	b.WriteString("legacy: false\ninstall_os: false\nconfigure_firewalld: false\n")
	if h.MACAddress != "" || h.PXE {
		b.WriteString("tempxe_discovery:\n")
		fmt.Fprintf(&b, "  enabled: %t\n", h.PXE)
		b.WriteString("uplinks:\n  - name: n0p0\n    physical_network: CHANGE_ME\n")
		fmt.Fprintf(&b, "    mac_address: %q\n", macOrDerive(h))
		b.WriteString("    networks:\n      - zone: lab\n        network_name: CHANGE_ME\n        cloudinit: false\n        pxe: true\n        vm_network: false\n")
		if h.AnsibleHost != "" {
			fmt.Fprintf(&b, "        ip: %q\n", h.AnsibleHost)
		}
		b.WriteString("        default_route: true\n        tcp_open:\n          - 22\n        udp_open: []\n        services_open: []\n")
	}
	return b.String()
}

func renderHypervisorHostVars(h HostIntent, catalog Catalog) string {
	mac := macOrDerive(h)
	physical := physicalNetworkFor(catalog, h.NetworkName)
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "%s%s. Run Discover (SSH or PXE), then import preview to promote drafts.\n", discoveryStubMarker, h.Name)
	if h.AnsibleHost != "" {
		fmt.Fprintf(&b, "ansible_host: %s\n", h.AnsibleHost)
	}
	b.WriteString("configure_firewalld: false\n")
	b.WriteString("tempxe_discovery:\n")
	fmt.Fprintf(&b, "  enabled: %t\n", h.PXE)
	b.WriteString("uplinks:\n  - name: n0p0\n")
	fmt.Fprintf(&b, "    physical_network: %s\n", yamlScalar(physical))
	fmt.Fprintf(&b, "    mac_address: %q\n", mac)
	b.WriteString("    networks:\n")
	fmt.Fprintf(&b, "      - zone: %s\n", yamlScalar(h.Zone))
	fmt.Fprintf(&b, "        network_name: %s\n", yamlScalar(h.NetworkName))
	fmt.Fprintf(&b, "        pxe: %t\n", h.PXE)
	b.WriteString("        vm_network: true\n")
	return b.String()
}
