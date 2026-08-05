package traefik

import (
	"regexp"
	"strings"
)

var hostRulePattern = regexp.MustCompile(`Host\(\x60([^\x60]+)\x60\)`)

// HostnamesInRoutesYAML returns hostnames referenced in Traefik router rules.
func HostnamesInRoutesYAML(data []byte) map[string]struct{} {
	hosts := map[string]struct{}{}
	if len(data) == 0 {
		return hosts
	}
	for _, match := range hostRulePattern.FindAllStringSubmatch(string(data), -1) {
		if len(match) < 2 {
			continue
		}
		h := strings.ToLower(strings.TrimSpace(match[1]))
		if h != "" {
			hosts[h] = struct{}{}
		}
	}
	return hosts
}

// NewHostnames returns hostnames present in next but not in prev.
func NewHostnames(prev, next []byte) []string {
	oldHosts := HostnamesInRoutesYAML(prev)
	var added []string
	for h := range HostnamesInRoutesYAML(next) {
		if _, ok := oldHosts[h]; !ok {
			added = append(added, h)
		}
	}
	return added
}
