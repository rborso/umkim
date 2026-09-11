package yaidap

import (
	"context"
	"fmt"
	"sort"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"gopkg.in/yaml.v3"
)

// ClusterSpec is Fleet cluster intent stored in group_vars/{name}.yml (members come from root {name}.yml).
type ClusterSpec struct {
	Topology        string   `yaml:"topology,omitempty" json:"topology,omitempty"`
	AutoApply       *bool    `yaml:"autoApply,omitempty" json:"autoApply,omitempty"`
	Members         []string `yaml:"members,omitempty" json:"members,omitempty"`
	HAGroup         string   `yaml:"haGroup,omitempty" json:"haGroup,omitempty"`
	ControlPlaneVip string   `yaml:"controlPlaneVip,omitempty" json:"controlPlaneVip,omitempty"`
	Jumphost        string   `yaml:"jumphost,omitempty" json:"jumphost,omitempty"`
	IngressId       string   `yaml:"ingressId,omitempty" json:"ingressId,omitempty"`
	Mode            string   `yaml:"mode,omitempty" json:"mode,omitempty"` // enroll default
	TargetCluster   string   `yaml:"targetCluster,omitempty" json:"targetCluster,omitempty"`
	ReplaceNode     string   `yaml:"replaceNode,omitempty" json:"replaceNode,omitempty"`
}

// ClustersDoc is the aggregated cluster intent (API response shape).
type ClustersDoc struct {
	Clusters map[string]ClusterSpec `yaml:"clusters,omitempty" json:"clusters,omitempty"`
}

func (c *ClusterSpec) EffectiveAutoApply() bool {
	if c == nil || c.AutoApply == nil {
		return true
	}
	return *c.AutoApply
}

func (c *ClusterSpec) EffectiveTopology() string {
	if c == nil || strings.TrimSpace(c.Topology) == "" {
		return fleetv1alpha1.ClusterTopologySingleNode
	}
	return strings.TrimSpace(c.Topology)
}

func (c *ClusterSpec) EffectiveMode() string {
	if c == nil || strings.TrimSpace(c.Mode) == "" {
		return fleetv1alpha1.SiteHostClusterModeEnroll
	}
	return strings.TrimSpace(c.Mode)
}

// ParseClustersYAML parses legacy monolithic clusters.yml.
func ParseClustersYAML(content string) (ClustersDoc, error) {
	var doc ClustersDoc
	if strings.TrimSpace(content) == "" {
		return doc, nil
	}
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return ClustersDoc{}, fmt.Errorf("clusters.yml: %w", err)
	}
	if doc.Clusters == nil {
		doc.Clusters = map[string]ClusterSpec{}
	}
	return doc, nil
}

// ParseClusterFileYAML parses legacy clusters/<name>.yml or group_vars/<name>.yml ClusterSpec body.
func ParseClusterFileYAML(content string) (ClusterSpec, error) {
	var spec ClusterSpec
	if strings.TrimSpace(content) == "" {
		return spec, nil
	}
	if err := yaml.Unmarshal([]byte(content), &spec); err != nil {
		return ClusterSpec{}, fmt.Errorf("cluster intent: %w", err)
	}
	return spec, nil
}

func validateClusterSpec(name string, spec ClusterSpec, guestsYAML, bareMetalsYAML, hypervisorsYAML string) error {
	if err := ValidateGuestName(name); err != nil {
		return fmt.Errorf("cluster %q: invalid name: %w", name, err)
	}
	if len(spec.Members) == 0 {
		return fmt.Errorf("cluster %q: members required", name)
	}
	topology := spec.EffectiveTopology()
	if topology == fleetv1alpha1.ClusterTopologyHA3Node {
		if len(spec.Members) != 3 {
			return fmt.Errorf("cluster %q: ha_3_node requires exactly 3 members", name)
		}
		if strings.TrimSpace(spec.ControlPlaneVip) == "" {
			return fmt.Errorf("cluster %q: controlPlaneVip required for ha_3_node", name)
		}
	}
	for _, m := range spec.Members {
		if err := ValidateGuestName(m); err != nil {
			return fmt.Errorf("cluster %q member %q: %w", name, m, err)
		}
		if (guestsYAML != "" || bareMetalsYAML != "" || hypervisorsYAML != "") && !inventoryHostListed(m, guestsYAML, bareMetalsYAML, hypervisorsYAML) {
			return fmt.Errorf("cluster %q: member %q not listed in guests.yml, bare_metals.yml, or hypervisors.yml", name, m)
		}
	}
	return nil
}

func validateClustersYAML(content, guestsYAML, bareMetalsYAML string) error {
	doc, err := ParseClustersYAML(content)
	if err != nil {
		return err
	}
	for name, spec := range doc.Clusters {
		if err := validateClusterSpec(name, spec, guestsYAML, bareMetalsYAML, ""); err != nil {
			return err
		}
	}
	return nil
}

func validateLegacyClusterFile(path, content, guestsYAML, bareMetalsYAML string) error {
	path = connectors.NormalizeInventoryPath(path)
	if !strings.HasPrefix(path, "clusters/") || !strings.HasSuffix(path, ".yml") {
		return fmt.Errorf("invalid legacy cluster file path %q", path)
	}
	name := strings.TrimSuffix(strings.TrimPrefix(path, "clusters/"), ".yml")
	spec, err := ParseClusterFileYAML(content)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return validateClusterSpec(name, spec, guestsYAML, bareMetalsYAML, "")
}

func validateClusterGroupVars(path, content, guestsYAML, bareMetalsYAML string, members []string) error {
	path = connectors.NormalizeInventoryPath(path)
	name := strings.TrimSuffix(strings.TrimPrefix(path, "group_vars/"), ".yml")
	if name == "" || strings.Contains(name, "/") {
		return fmt.Errorf("invalid cluster group_vars path %q", path)
	}
	spec, err := ParseClusterFileYAML(content)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	spec.Members = members
	return validateClusterSpec(name, spec, guestsYAML, bareMetalsYAML, "")
}

// ParseInventoryGroupMembers reads host names from a root Ansible group file.
func ParseInventoryGroupMembers(content, groupName string) ([]string, error) {
	groupName = strings.TrimSpace(groupName)
	if groupName == "" {
		return nil, fmt.Errorf("group name required")
	}
	var doc map[string]inventoryGroupBody
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("%s.yml: empty group file", groupName)
	}
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return nil, fmt.Errorf("%s.yml: %w", groupName, err)
	}
	body, ok := doc[groupName]
	if !ok {
		return nil, fmt.Errorf("%s.yml: top-level key %q required", groupName, groupName)
	}
	if len(body.Hosts) == 0 && len(body.Children) == 0 {
		return nil, fmt.Errorf("%s.yml: %q.hosts or children required", groupName, groupName)
	}
	if len(body.Children) > 0 {
		return nil, fmt.Errorf("%s.yml: nested children groups are not supported yet", groupName)
	}
	names := make([]string, 0, len(body.Hosts))
	for name := range body.Hosts {
		if err := ValidateGuestName(name); err != nil {
			return nil, fmt.Errorf("%s.yml host %q: %w", groupName, name, err)
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

type inventoryGroupBody struct {
	Hosts    map[string]interface{} `yaml:"hosts"`
	Children map[string]interface{} `yaml:"children"`
}

func validateRootInventoryGroupFile(path, content, guestsYAML, bareMetalsYAML, hypervisorsYAML string) error {
	groupName := connectors.RootInventoryGroupName(path)
	if groupName == "" {
		return fmt.Errorf("invalid root group file %q", path)
	}
	members, err := ParseInventoryGroupMembers(content, groupName)
	if err != nil {
		return err
	}
	if guestsYAML != "" || bareMetalsYAML != "" || hypervisorsYAML != "" {
		for _, m := range members {
			if !inventoryHostListed(m, guestsYAML, bareMetalsYAML, hypervisorsYAML) {
				return fmt.Errorf("%s: member %q not listed in guests.yml, bare_metals.yml, or hypervisors.yml", path, m)
			}
		}
	}
	return nil
}

// LoadClustersFile reads cluster intent from root group files + group_vars (and legacy clusters/*).
func LoadClustersFile(ctx context.Context, c client.Client, ns, siteName string) (ClustersDoc, error) {
	data, err := connectors.Load(ctx, c, ns, siteName, provider)
	if err != nil {
		return ClustersDoc{}, err
	}
	if data == nil {
		return ClustersDoc{}, nil
	}
	return LoadClustersFromData(connectors.NormalizeMetaKeys(data)), nil
}

// LoadClustersFromData aggregates cluster intent from the native Ansible tree.
func LoadClustersFromData(data map[string]string) ClustersDoc {
	doc := ClustersDoc{Clusters: map[string]ClusterSpec{}}
	guestsYAML, _ := connectors.GetInventoryFile(data, "guests.yml")
	bareMetalsYAML, _ := connectors.GetInventoryFile(data, "bare_metals.yml")
	hypervisorsYAML, _ := connectors.GetInventoryFile(data, "hypervisors.yml")
	for _, info := range connectors.ListInventoryFiles(data) {
		if !connectors.IsRootInventoryGroupFile(info.Path) {
			continue
		}
		name := connectors.RootInventoryGroupName(info.Path)
		rootRaw, ok := connectors.GetInventoryFile(data, info.Path)
		if !ok {
			continue
		}
		members, err := ParseInventoryGroupMembers(rootRaw, name)
		if err != nil {
			continue
		}
		gvPath := "group_vars/" + name + ".yml"
		gvRaw, _ := connectors.GetInventoryFile(data, gvPath)
		spec, err := ParseClusterFileYAML(gvRaw)
		if err != nil {
			continue
		}
		spec.Members = members
		if err := validateClusterSpec(name, spec, guestsYAML, bareMetalsYAML, hypervisorsYAML); err != nil {
			continue
		}
		doc.Clusters[name] = spec
	}
	if len(doc.Clusters) > 0 {
		return doc
	}
	return loadLegacyClustersFromData(data, guestsYAML)
}

func loadLegacyClustersFromData(data map[string]string, guestsYAML string) ClustersDoc {
	doc := ClustersDoc{Clusters: map[string]ClusterSpec{}}
	for _, info := range connectors.ListInventoryFiles(data) {
		if !strings.HasPrefix(info.Path, "clusters/") || !strings.HasSuffix(info.Path, ".yml") {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(info.Path, "clusters/"), ".yml")
		raw, ok := connectors.GetInventoryFile(data, info.Path)
		if !ok {
			continue
		}
		spec, err := ParseClusterFileYAML(raw)
		if err != nil {
			continue
		}
		if err := validateClusterSpec(name, spec, guestsYAML, "", ""); err != nil {
			continue
		}
		doc.Clusters[name] = spec
	}
	if len(doc.Clusters) > 0 {
		return doc
	}
	raw, ok := connectors.GetInventoryFile(data, "clusters.yml")
	if !ok {
		return doc
	}
	legacy, err := ParseClustersYAML(raw)
	if err != nil {
		return doc
	}
	return legacy
}

// ClusterIntentFromSpec maps ClusterSpec to SiteHostClusterIntent for a member.
func ClusterIntentFromSpec(clusterName string, spec ClusterSpec) *fleetv1alpha1.SiteHostClusterIntent {
	intent := &fleetv1alpha1.SiteHostClusterIntent{
		Mode:            spec.EffectiveMode(),
		ClusterName:     clusterName,
		TargetCluster:   strings.TrimSpace(spec.TargetCluster),
		Topology:        spec.EffectiveTopology(),
		HAGroup:         strings.TrimSpace(spec.HAGroup),
		ReplaceNode:     strings.TrimSpace(spec.ReplaceNode),
		ControlPlaneVip: strings.TrimSpace(spec.ControlPlaneVip),
		Jumphost:        strings.TrimSpace(spec.Jumphost),
		IngressId:       strings.TrimSpace(spec.IngressId),
		AutoApply:       spec.AutoApply,
	}
	if intent.HAGroup == "" && intent.EffectiveTopology() == fleetv1alpha1.ClusterTopologyHA3Node {
		intent.HAGroup = clusterName
	}
	if intent.Mode == fleetv1alpha1.SiteHostClusterModeEnroll {
		intent.ClusterName = clusterName
	}
	return intent
}

// EncodeInventoryGroupFile renders root {name}.yml Ansible group membership.
func EncodeInventoryGroupFile(groupName string, members []string) (string, error) {
	groupName = strings.TrimSpace(groupName)
	if groupName == "" {
		return "", fmt.Errorf("group name required")
	}
	hosts := map[string]interface{}{}
	for _, m := range members {
		hosts[m] = nil
	}
	doc := map[string]interface{}{
		groupName: map[string]interface{}{"hosts": hosts},
	}
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return "", err
	}
	_ = enc.Close()
	return buf.String(), nil
}

// EncodeClusterGroupVarsYAML renders group_vars/{name}.yml Fleet intent (without members).
func EncodeClusterGroupVarsYAML(spec ClusterSpec) (string, error) {
	out := spec
	out.Members = nil
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(out); err != nil {
		return "", err
	}
	_ = enc.Close()
	return buf.String(), nil
}

// SplitClustersYAML converts legacy clusters.yml into root group + group_vars files.
func SplitClustersYAML(content string) (map[string]string, error) {
	doc, err := ParseClustersYAML(content)
	if err != nil {
		return nil, err
	}
	return clusterSpecsToNativeFiles(doc.Clusters)
}

// SplitLegacyClusterFile converts one legacy clusters/<name>.yml into native files.
func SplitLegacyClusterFile(name, content string) (map[string]string, error) {
	spec, err := ParseClusterFileYAML(content)
	if err != nil {
		return nil, err
	}
	return clusterSpecsToNativeFiles(map[string]ClusterSpec{name: spec})
}

func clusterSpecsToNativeFiles(clusters map[string]ClusterSpec) (map[string]string, error) {
	out := map[string]string{}
	for name, spec := range clusters {
		root, err := EncodeInventoryGroupFile(name, spec.Members)
		if err != nil {
			return nil, err
		}
		gv, err := EncodeClusterGroupVarsYAML(spec)
		if err != nil {
			return nil, err
		}
		out[name+".yml"] = root
		out["group_vars/"+name+".yml"] = gv
	}
	return out, nil
}

// HasLegacyClusterDirFiles reports whether deprecated clusters/*.yml exists in store data.
func HasLegacyClusterDirFiles(data map[string]string) bool {
	for _, info := range connectors.ListInventoryFiles(data) {
		if connectors.LegacyClusterInventoryPath(info.Path) {
			return true
		}
	}
	return false
}

// MigrateLegacyClusterPaths rewrites clusters/* and clusters.yml into root group + group_vars files.
func MigrateLegacyClusterPaths(data map[string]string) (put map[string]string, deletePaths []string) {
	put = map[string]string{}
	deletePaths = []string{}
	if raw, ok := connectors.GetInventoryFile(data, "clusters.yml"); ok {
		split, err := SplitClustersYAML(raw)
		if err == nil {
			for p, content := range split {
				put[p] = content
			}
			deletePaths = append(deletePaths, "clusters.yml")
		}
	}
	for _, info := range connectors.ListInventoryFiles(data) {
		if !strings.HasPrefix(info.Path, "clusters/") || !strings.HasSuffix(info.Path, ".yml") {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(info.Path, "clusters/"), ".yml")
		raw, ok := connectors.GetInventoryFile(data, info.Path)
		if !ok {
			continue
		}
		split, err := SplitLegacyClusterFile(name, raw)
		if err != nil {
			continue
		}
		for p, content := range split {
			put[p] = content
		}
		deletePaths = append(deletePaths, info.Path)
	}
	return put, deletePaths
}
