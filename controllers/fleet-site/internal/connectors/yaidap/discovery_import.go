package yaidap

import (
	"fmt"
	"strings"

	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	"gopkg.in/yaml.v3"
)

const (
	generatedHostVarsPrefix = "generated/host_vars/"
	generatedHostsPrefix    = "generated/hosts/"
)

// PromoteDiscoveryFiles maps jumphost generated/ discovery drafts into canonical
// inventory paths (host_vars/, hypervisors.yml) for import preview and apply.
func PromoteDiscoveryFiles(localData map[string]string, remoteFiles map[string]string) map[string]string {
	if len(remoteFiles) == 0 {
		return remoteFiles
	}
	localData = connectors.NormalizeMetaKeys(localData)
	promoted := map[string]string{}

	for p, content := range remoteFiles {
		if strings.HasPrefix(p, "generated/") {
			continue
		}
		if connectors.HubOnlyInventoryPath(p) {
			continue
		}
		if connectors.InventoryPathAllowed(p) {
			promoted[p] = content
		}
	}

	for p, content := range remoteFiles {
		if !strings.HasPrefix(p, generatedHostVarsPrefix) || !strings.HasSuffix(p, ".yml") {
			continue
		}
		if strings.Contains(p, ".notes.") {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(p, generatedHostVarsPrefix), ".yml")
		if name == "" {
			continue
		}
		canonical := "host_vars/" + name + ".yml"
		promotedContent := stripGeneratedMetadata(content)
		if existing, ok := remoteFiles[canonical]; ok && strings.TrimSpace(existing) != "" {
			if !isDiscoveryStub(existing) {
				// Synced or hand-edited host_vars wins over stale generated drafts.
				continue
			}
			// An un-promoted add-host stub is superseded by the draft, but the
			// discovery draft omits ansible_host — carry the stub's over.
			promotedContent = carryStubAnsibleHost(existing, promotedContent)
		}
		promoted[canonical] = promotedContent
	}

	hvContent := promoted["hypervisors.yml"]
	if strings.TrimSpace(hvContent) == "" {
		hvContent, _ = connectors.GetInventoryFile(localData, "hypervisors.yml")
	}
	for p, content := range remoteFiles {
		if !strings.HasPrefix(p, generatedHostsPrefix) || !strings.HasSuffix(p, ".yml") {
			continue
		}
		merged, err := mergeDiscoveryHostEntry(hvContent, content)
		if err != nil {
			continue
		}
		hvContent = merged
	}
	if strings.TrimSpace(hvContent) != "" {
		promoted["hypervisors.yml"] = hvContent
	}

	return promoted
}

// isDiscoveryStub reports whether host_vars content is the placeholder written by
// the console/CLI add-host flow. Such a stub is meant to be superseded by a
// discovery draft; hand-edited host_vars (which no longer carry the marker) are not.
func isDiscoveryStub(content string) bool {
	return strings.Contains(content, discoveryStubMarker) || strings.Contains(content, bareMetalStubMarker)
}

// carryStubAnsibleHost copies ansible_host from an add-host stub into the promoted
// discovery draft when the draft has none (discovery drafts omit ansible_host, but
// Ansible still needs the address to target the host).
func carryStubAnsibleHost(stub, draft string) string {
	var sd, dd map[string]interface{}
	if yaml.Unmarshal([]byte(stub), &sd) != nil {
		return draft
	}
	host, _ := sd["ansible_host"].(string)
	if strings.TrimSpace(host) == "" {
		return draft
	}
	if yaml.Unmarshal([]byte(draft), &dd) != nil {
		return draft
	}
	if v, _ := dd["ansible_host"].(string); strings.TrimSpace(v) != "" {
		return draft
	}
	dd["ansible_host"] = strings.TrimSpace(host)
	out, err := yaml.Marshal(dd)
	if err != nil {
		return draft
	}
	return string(out)
}

func stripGeneratedMetadata(content string) string {
	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return content
	}
	delete(doc, "generated")
	out, err := yaml.Marshal(doc)
	if err != nil {
		return content
	}
	return string(out)
}

func mergeDiscoveryHostEntry(existingHypervisors, generatedHostEntry string) (string, error) {
	hostName, discoveredHost, err := parseGeneratedHostEntry(generatedHostEntry)
	if err != nil {
		return "", err
	}
	if hostName == "" {
		return existingHypervisors, fmt.Errorf("empty host name in generated host entry")
	}

	doc, err := ParseHypervisorsYAML(existingHypervisors)
	if err != nil {
		return "", err
	}
	if doc.Hypervisors.Hosts == nil {
		doc.Hypervisors.Hosts = map[string]struct {
			AnsibleHost string `yaml:"ansible_host"`
		}{}
	}

	entry, ok := doc.Hypervisors.Hosts[hostName]
	if !ok {
		entry = struct {
			AnsibleHost string `yaml:"ansible_host"`
		}{}
	}
	discoveredIP := strings.TrimSpace(discoveredHost.AnsibleHost)
	if discoveredIP != "" && strings.TrimSpace(entry.AnsibleHost) == "" {
		entry.AnsibleHost = discoveredIP
	}
	doc.Hypervisors.Hosts[hostName] = entry

	out, err := yaml.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func parseGeneratedHostEntry(content string) (string, struct{ AnsibleHost string }, error) {
	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return "", struct{ AnsibleHost string }{}, err
	}
	for _, groupVal := range doc {
		group, ok := groupVal.(map[string]interface{})
		if !ok {
			continue
		}
		hosts, ok := group["hosts"].(map[string]interface{})
		if !ok {
			continue
		}
		for name, hostVal := range hosts {
			hostMap, ok := hostVal.(map[string]interface{})
			if !ok {
				continue
			}
			ip, _ := hostMap["ansible_host"].(string)
			return name, struct{ AnsibleHost string }{AnsibleHost: ip}, nil
		}
	}
	return "", struct{ AnsibleHost string }{}, fmt.Errorf("no host entry found in generated hosts file")
}
