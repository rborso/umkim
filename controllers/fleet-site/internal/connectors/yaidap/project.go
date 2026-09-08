package yaidap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	"gopkg.in/yaml.v3"
)

// Project renders YAIDAP inventory files from hub connector store content.
func Project(catalog Catalog, guests GuestStore) (connectors.ProjectBundle, error) {
	if err := ValidateCatalog(&catalog); err != nil {
		return connectors.ProjectBundle{}, err
	}
	if err := ValidateGuestStore(guests); err != nil {
		return connectors.ProjectBundle{}, err
	}
	files := map[string]string{}
	guestsYAML, err := renderGuestsYAML(guests)
	if err != nil {
		return connectors.ProjectBundle{}, err
	}
	files["guests.yml"] = guestsYAML
	for name, spec := range guests {
		if spec.Absent {
			continue
		}
		hostVars, err := renderHostVars(name, spec, catalog)
		if err != nil {
			return connectors.ProjectBundle{}, err
		}
		files["host_vars/"+name+".yml"] = hostVars
	}
	return connectors.ProjectBundle{Files: files}, nil
}

func renderGuestsYAML(guests GuestStore) (string, error) {
	doc := GuestsYAML{}
	doc.Guests.Hosts = map[string]GuestEntry{}
	doc.External.Hosts = map[string]GuestEntry{}
	for name, spec := range guests {
		if spec.Absent {
			continue
		}
		entry := GuestEntry{
			AnsibleHost: strings.TrimSpace(spec.AnsibleHost),
		}
		if spec.Kind != fleetv1alpha1.SiteHostKindBareMetal {
			entry.VmLocation = strings.TrimSpace(spec.VmLocation)
			if entry.VmLocation == "" {
				entry.VmLocation = "least-utilized"
			}
		} else if loc := strings.TrimSpace(spec.VmLocation); loc != "" {
			entry.VmLocation = loc
		}
		doc.Guests.Hosts[name] = entry
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return "", err
	}
	_ = enc.Close()
	return buf.String(), nil
}

func renderHostVars(name string, spec GuestSpec, catalog Catalog) (string, error) {
	def := catalog.Defaults
	out := map[string]interface{}{}
	out["# Managed by UMKIM fleet-site connector inventory"] = nil
	delete(out, "# Managed by UMKIM fleet-site connector inventory")

	doc := map[string]interface{}{}
	if spec.Kind == fleetv1alpha1.SiteHostKindBareMetal {
		if ip := strings.TrimSpace(spec.AnsibleHost); ip != "" {
			doc["ansible_host"] = ip
		}
		doc["os"] = firstNonEmpty(spec.OS, def.OS, "rocky-10")
		return encodeHostVars(doc)
	}
	if loc := strings.TrimSpace(spec.VmLocation); loc != "" {
		doc["vm_location"] = loc
	} else {
		doc["vm_location"] = "least-utilized"
	}
	if ip := strings.TrimSpace(spec.AnsibleHost); ip != "" {
		doc["ansible_host"] = ip
	}
	doc["os"] = firstNonEmpty(spec.OS, def.OS, "rocky-10")
	doc["legacy"] = boolOrDefault(spec.Legacy, false)
	doc["configure_firewalld"] = boolOrDefault(spec.ConfigureFirewalld, false)
	doc["provisioning_provider"] = firstNonEmpty(spec.ProvisioningProvider, def.ProvisioningProvider, "libvirt")
	doc["memory"] = intOrDefault(spec.Memory, def.Memory, 4096)
	doc["cores"] = intOrDefault(spec.Cores, def.Cores, 2)

	disks := spec.Disks
	if len(disks) == 0 {
		size := intOrDefault(0, def.DiskGi, 40)
		storage := firstNonEmpty(def.Storage, "local")
		disks = []DiskSpec{{
			Name: "bootdisk", Disk: "vda", Bootdisk: true, Device: 0,
			Format: "qcow2", Size: size, Storage: storage,
		}}
	}
	doc["disks"] = disks

	uplinks := spec.Uplinks
	if len(uplinks) == 0 {
		uplinks = defaultUplinks(spec, catalog)
	}
	doc["uplinks"] = uplinks

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.WriteString("# Managed by UMKIM fleet-site connector inventory.\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return "", err
	}
	_ = enc.Close()
	_ = name
	return buf.String(), nil
}

func encodeHostVars(doc map[string]interface{}) (string, error) {
	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.WriteString("# Managed by UMKIM fleet-site connector inventory.\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return "", err
	}
	_ = enc.Close()
	return buf.String(), nil
}

func defaultUplinks(spec GuestSpec, catalog Catalog) []UplinkSpec {
	def := catalog.Defaults
	phys := firstNonEmpty(def.PhysicalNetwork, "lab_access_l1")
	zone := firstNonEmpty(def.Zone, "lab")
	netName := firstNonEmpty(def.NetworkName, "lab_nw")
	ip := strings.TrimSpace(spec.AnsibleHost)
	mac := macFromIP(ip)
	return []UplinkSpec{{
		Name:            "n0p0",
		PhysicalNetwork: phys,
		MACAddress:      mac,
		Networks: []NetworkSpec{{
			Zone:             zone,
			NetworkName:      netName,
			Model:            "virtio",
			CloudInit:        true,
			HypervisorUplink: "n0p0",
			BridgeName:       "n0p0_" + netName,
			IP:               ip,
			DefaultRoute:     true,
			TCPOpen:          []int{22},
		}},
	}}
}

func macFromIP(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return "52:54:00:01:00:01"
	}
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.To4() == nil {
		return "52:54:00:01:00:01"
	}
	octets := parsed.To4()
	return fmt.Sprintf("52:54:00:%02x:%02x:01", octets[2], octets[3])
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}

func intOrDefault(specVal, defVal, fallback int) int {
	if specVal > 0 {
		return specVal
	}
	if defVal > 0 {
		return defVal
	}
	return fallback
}

func boolOrDefault(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}

// EncodeBundleJSON serializes a project bundle for connector extra vars.
func EncodeBundleJSON(bundle connectors.ProjectBundle) (string, error) {
	raw, err := json.Marshal(bundle)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
