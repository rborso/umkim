package yaidap

import (
	"context"
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	"github.com/rborso/umkim/controllers/fleet-site/internal/sitehosts"
	"gopkg.in/yaml.v3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NativeInventorySkeleton returns empty native Ansible/YAIDAP tree defaults.
func NativeInventorySkeleton() map[string]string {
	return map[string]string{
		"guests.yml": `guests:
  hosts: {}
`,
		"bare_metals.yml": emptyBareMetalsYAML,
		"hypervisors.yml": `hypervisors:
  hosts: {}
`,
		"group_vars/all/all.yml": `---
memory: 4096
cores: 2
libvirt_dir: /opt/local
storages:
  - name: local
    type: local
    shared: false
    path: /opt/local
    os: [rocky, fedora]
`,
	}
}

// PutInventoryFile validates and writes one YAML file with cross-file checks.
func PutInventoryFile(ctx context.Context, c client.Client, ns, siteName, path, content, actor string) error {
	path = connectors.NormalizeInventoryPath(path)
	if path == "" {
		return connectors.ErrInvalidInventoryPath
	}
	data, err := connectors.Load(ctx, c, ns, siteName, provider)
	if err != nil {
		return err
	}
	guestsYAML := ""
	bareMetalsYAML := ""
	hypervisorsYAML := ""
	if data != nil {
		data = connectors.NormalizeMetaKeys(data)
		guestsYAML, _ = connectors.GetInventoryFile(data, "guests.yml")
		bareMetalsYAML, _ = connectors.GetInventoryFile(data, "bare_metals.yml")
		hypervisorsYAML, _ = connectors.GetInventoryFile(data, "hypervisors.yml")
	}
	if err := ValidateInventoryFileWithContext(path, content, guestsYAML, bareMetalsYAML, hypervisorsYAML); err != nil {
		return err
	}
	return connectors.PutInventoryFile(ctx, c, ns, siteName, provider, path, content, actor, nil)
}

// DeleteInventoryFile removes a file from the store.
func DeleteInventoryFile(ctx context.Context, c client.Client, ns, siteName, path, actor string) error {
	return connectors.DeleteInventoryFile(ctx, c, ns, siteName, provider, path, actor)
}

// ListInventoryFiles lists logical inventory files.
func ListInventoryFiles(ctx context.Context, c client.Client, ns, siteName string) ([]connectors.InventoryFileInfo, error) {
	data, err := connectors.Load(ctx, c, ns, siteName, provider)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	return connectors.ListInventoryFiles(connectors.NormalizeMetaKeys(data)), nil
}

// GetInventoryFileContent returns one file body.
func GetInventoryFileContent(ctx context.Context, c client.Client, ns, siteName, path string) (string, bool, error) {
	data, err := connectors.Load(ctx, c, ns, siteName, provider)
	if err != nil {
		return "", false, err
	}
	if data == nil {
		return "", false, nil
	}
	content, ok := connectors.GetInventoryFile(connectors.NormalizeMetaKeys(data), path)
	return content, ok, nil
}

// MigrateLegacyStore converts legacy JSON/site-hosts inventory into native YAML tree.
func MigrateLegacyStore(ctx context.Context, c client.Client, ns, siteName, actor string) error {
	// Split external.hosts out of guests.yml in its own read-modify-write. Deriving
	// this from the snapshot below and writing it later would clobber a concurrent
	// write to guests.yml, such as a create_vm resolved vm_location (this runs on
	// every provision action via EnsureReadyForProvision).
	if err := migrateBareMetalsLayout(ctx, c, ns, siteName); err != nil {
		return err
	}
	data, err := connectors.Load(ctx, c, ns, siteName, provider)
	if err != nil {
		return err
	}
	put := map[string]string{}
	deletePaths := []string{}
	if data != nil {
		data = connectors.NormalizeMetaKeys(data)
		_, hasGuests := connectors.GetInventoryFile(data, "guests.yml")
		if !hasGuests {
			if data["guests.json"] != "" {
				catalog, guests, err := loadLegacyJSONStore(data)
				if err != nil {
					return err
				}
				bundle, err := Project(catalog, guests)
				if err != nil {
					return err
				}
				for p, content := range bundle.Files {
					put[p] = content
				}
			} else if len(connectors.ListInventoryFiles(data)) == 0 {
				for p, content := range NativeInventorySkeleton() {
					put[p] = content
				}
			}
		}
		// Only project from site-hosts when YAML store does not exist yet.
		if !hasGuests && len(connectors.ListInventoryFiles(data)) == 0 {
			if _, err := clustersFromSiteHosts(ctx, c, ns, siteName, put); err != nil {
				return err
			}
		}
		if catalogYAML, ok := connectors.GetInventoryFile(data, "catalog.yml"); ok {
			gv, err := catalogYAMLToGroupVars(catalogYAML)
			if err != nil {
				return err
			}
			if existing, ok := connectors.GetInventoryFile(data, "group_vars/all/all.yml"); ok && strings.TrimSpace(existing) != "" {
				put["group_vars/all/all.yml"] = mergeGroupVars(existing, gv)
			} else {
				put["group_vars/all/all.yml"] = gv
			}
			deletePaths = append(deletePaths, "catalog.yml")
		}
		if _, ok := connectors.GetInventoryFile(data, "group_vars/all/all.yml"); !ok {
			if _, seeded := put["group_vars/all/all.yml"]; !seeded {
				put["group_vars/all/all.yml"] = NativeInventorySkeleton()["group_vars/all/all.yml"]
			}
		}
		if _, ok := connectors.GetInventoryFile(data, "hypervisors.yml"); !ok {
			if _, seeded := put["hypervisors.yml"]; !seeded {
				if hv := hypervisorsFromCatalog(data); hv != "" {
					put["hypervisors.yml"] = hv
				} else {
					put["hypervisors.yml"] = NativeInventorySkeleton()["hypervisors.yml"]
				}
			}
		}
		if data != nil {
			legacyPut, legacyDelete := MigrateLegacyClusterPaths(data)
			for p, content := range legacyPut {
				put[p] = content
			}
			deletePaths = append(deletePaths, legacyDelete...)
		}
	} else {
		for p, content := range NativeInventorySkeleton() {
			put[p] = content
		}
	}
	if len(put) == 0 && len(deletePaths) == 0 {
		return nil
	}
	for p, content := range put {
		if err := ValidateInventoryFile(p, content); err != nil {
			return err
		}
	}
	if err := connectors.PutInventoryFiles(ctx, c, ns, siteName, provider, actor, put, nil); err != nil {
		return err
	}
	for _, p := range deletePaths {
		if err := connectors.DeleteInventoryFile(ctx, c, ns, siteName, provider, p, actor); err != nil {
			if err != connectors.ErrInventoryFileNotFound {
				return err
			}
		}
	}
	return connectors.PatchInventory(ctx, c, ns, siteName, provider, func(data map[string]string) (connectors.InventoryPatch, error) {
		meta, err := connectors.LoadMetaFromData(data, provider)
		if err != nil {
			return connectors.InventoryPatch{}, err
		}
		if len(put) > 0 {
			meta.SyncPhase = "dirty"
		}
		return connectors.InventoryPatch{Meta: &meta}, nil
	})
}

// migrateBareMetalsLayout writes the bare_metals.yml/guests.yml layout split from the
// current store inside a single read-modify-write. It must not compute the file bodies
// from an earlier snapshot: guests.yml also carries create_vm resolved vm_location
// values, and a stale snapshot would revert them.
func migrateBareMetalsLayout(ctx context.Context, c client.Client, ns, siteName string) error {
	return connectors.PatchInventory(ctx, c, ns, siteName, provider, func(raw map[string]string) (connectors.InventoryPatch, error) {
		data := connectors.NormalizeMetaKeys(raw)
		layoutPut, changed := EnsureBareMetalsLayout(data)
		if !changed {
			return connectors.InventoryPatch{}, nil
		}
		for p, content := range layoutPut {
			if err := ValidateInventoryFile(p, content); err != nil {
				return connectors.InventoryPatch{}, err
			}
		}
		return connectors.InventoryPatch{FilePut: layoutPut}, nil
	})
}

func loadLegacyJSONStore(data map[string]string) (Catalog, GuestStore, error) {
	catalog, err := DecodeCatalog(data["catalog.json"])
	if err != nil {
		return Catalog{}, nil, err
	}
	if len(catalog.Networks) == 0 {
		catalog = CatalogFromInventory(data)
	}
	guests, err := DecodeGuestStore(data["guests.json"])
	if err != nil {
		return Catalog{}, nil, err
	}
	return catalog, guests, nil
}

func catalogYAMLToGroupVars(catalogYAML string) (string, error) {
	var doc Catalog
	if err := yaml.Unmarshal([]byte(catalogYAML), &doc); err != nil {
		return "", fmt.Errorf("catalog.yml: %w", err)
	}
	out := map[string]interface{}{}
	if doc.Defaults.OS != "" {
		out["os"] = doc.Defaults.OS
	}
	if doc.Defaults.Memory > 0 {
		out["memory"] = doc.Defaults.Memory
	}
	if doc.Defaults.Cores > 0 {
		out["cores"] = doc.Defaults.Cores
	}
	if len(doc.Storages) > 0 {
		storages := make([]map[string]interface{}, 0, len(doc.Storages))
		for _, s := range doc.Storages {
			storages = append(storages, map[string]interface{}{
				"name": s.Name,
				"pool": s.Pool,
			})
		}
		out["storages"] = storages
	}
	if len(doc.Networks) > 0 {
		networks := make([]map[string]interface{}, 0, len(doc.Networks))
		for _, n := range doc.Networks {
			networks = append(networks, map[string]interface{}{
				"network_name": n.Name,
				"zone":         n.Zone,
			})
		}
		out["networks"] = networks
	}
	if len(out) == 0 {
		return NativeInventorySkeleton()["group_vars/all/all.yml"], nil
	}
	raw, err := yaml.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func mergeGroupVars(existing, overlay string) string {
	var base, add map[string]interface{}
	_ = yaml.Unmarshal([]byte(existing), &base)
	_ = yaml.Unmarshal([]byte(overlay), &add)
	if base == nil {
		base = map[string]interface{}{}
	}
	for k, v := range add {
		if _, ok := base[k]; !ok {
			base[k] = v
		}
	}
	raw, err := yaml.Marshal(base)
	if err != nil {
		return existing
	}
	return string(raw)
}

func hypervisorsFromCatalog(data map[string]string) string {
	raw, ok := connectors.GetInventoryFile(data, "catalog.yml")
	if !ok {
		return ""
	}
	var doc Catalog
	if err := yaml.Unmarshal([]byte(raw), &doc); err != nil || len(doc.Hypervisors) == 0 {
		return ""
	}
	hosts := map[string]map[string]string{}
	for _, name := range doc.Hypervisors {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		hosts[name] = map[string]string{}
	}
	if len(hosts) == 0 {
		return ""
	}
	out := map[string]interface{}{
		"hypervisors": map[string]interface{}{"hosts": hosts},
	}
	rawYAML, err := yaml.Marshal(out)
	if err != nil {
		return ""
	}
	return string(rawYAML)
}

func clustersFromSiteHosts(ctx context.Context, c client.Client, ns, siteName string, put map[string]string) (string, error) {
	hosts, err := sitehosts.List(ctx, c, ns, siteName)
	if err != nil || len(hosts) == 0 {
		return "", err
	}
	guestsDoc := GuestsYAML{}
	guestsDoc.Guests.Hosts = map[string]GuestEntry{}
	clusters := map[string]ClusterSpec{}
	haGroups := map[string][]fleetv1alpha1.SiteHostRecord{}
	for _, h := range hosts {
		entry := GuestEntry{
			AnsibleHost: strings.TrimSpace(h.AnsibleHost),
			VmLocation:  strings.TrimSpace(h.VmLocation),
		}
		if entry.VmLocation == "" && h.Kind == fleetv1alpha1.SiteHostKindVM {
			entry.VmLocation = "least-utilized"
		}
		guestsDoc.Guests.Hosts[h.Name] = entry
		if hv, ok := put["host_vars/"+h.Name+".yml"]; !ok || strings.TrimSpace(hv) == "" {
			spec := GuestSpecFromHost(h)
			hvYAML, err := renderHostVarsFromSpec(h.Name, spec, Catalog{})
			if err != nil {
				return "", err
			}
			put["host_vars/"+h.Name+".yml"] = hvYAML
		}
		if h.ClusterIntent == nil {
			continue
		}
		intent := h.ClusterIntent
		clusterName := strings.TrimSpace(intent.ClusterName)
		if clusterName == "" {
			clusterName = h.Name + "-0"
		}
		if intent.EffectiveTopology() == fleetv1alpha1.ClusterTopologyHA3Node {
			g := strings.TrimSpace(intent.HAGroup)
			if g == "" {
				g = clusterName
			}
			haGroups[g] = append(haGroups[g], h)
			continue
		}
		clusters[clusterName] = ClusterSpec{
			Topology:        intent.EffectiveTopology(),
			AutoApply:       intent.AutoApply,
			Members:         []string{h.Name},
			ControlPlaneVip: strings.TrimSpace(intent.ControlPlaneVip),
			Jumphost:        strings.TrimSpace(intent.Jumphost),
			IngressId:       strings.TrimSpace(intent.IngressId),
			Mode:            intent.Mode,
			TargetCluster:   strings.TrimSpace(intent.TargetCluster),
			ReplaceNode:     strings.TrimSpace(intent.ReplaceNode),
		}
	}
	for gname, members := range haGroups {
		if len(members) == 0 {
			continue
		}
		intent := members[0].ClusterIntent
		clusterName := strings.TrimSpace(intent.ClusterName)
		if clusterName == "" {
			clusterName = gname
		}
		names := make([]string, 0, len(members))
		for _, m := range members {
			names = append(names, m.Name)
		}
		clusters[clusterName] = ClusterSpec{
			Topology:        fleetv1alpha1.ClusterTopologyHA3Node,
			AutoApply:       intent.AutoApply,
			Members:         names,
			HAGroup:         gname,
			ControlPlaneVip: strings.TrimSpace(intent.ControlPlaneVip),
			Jumphost:        strings.TrimSpace(intent.Jumphost),
			IngressId:       strings.TrimSpace(intent.IngressId),
		}
	}
	if existing, ok := put["guests.yml"]; ok && strings.TrimSpace(existing) != "" {
		// keep projected guests if already set
	} else {
		raw, err := encodeGuestsYAML(guestsDoc)
		if err != nil {
			return "", err
		}
		put["guests.yml"] = raw
	}
	if len(clusters) == 0 {
		return "", nil
	}
	for clusterName, spec := range clusters {
		native, err := clusterSpecsToNativeFiles(map[string]ClusterSpec{clusterName: spec})
		if err != nil {
			return "", err
		}
		for p, content := range native {
			put[p] = content
		}
	}
	return "", nil
}

func encodeGuestsYAML(doc GuestsYAML) (string, error) {
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return "", err
	}
	_ = enc.Close()
	return buf.String(), nil
}

func renderHostVarsFromSpec(name string, spec GuestSpec, catalog Catalog) (string, error) {
	return renderHostVars(name, spec, catalog)
}
