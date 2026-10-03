package appchart

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
)

var (
	dns1123      = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	chartVersion = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)
)

const maxDNSLabel = 63

func ValidDNS1123(name string) bool {
	name = strings.TrimSpace(name)
	return name != "" && len(name) <= maxDNSLabel && dns1123.MatchString(name)
}

func ValidVersion(v string) bool {
	v = strings.TrimSpace(v)
	return v == "" || (len(v) <= 64 && chartVersion.MatchString(v))
}

func ValidRouteMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "none", "hub", "site":
		return true
	default:
		return false
	}
}

// ExpandChart turns a short name (whoami) into oci://prefix/whoami using the first
// non-wildcard prefix. Full oci:// URLs are returned trimmed.
func ExpandChart(chart, defaultPrefix string) string {
	chart = strings.TrimSpace(chart)
	if chart == "" {
		return ""
	}
	if strings.Contains(chart, "://") {
		return strings.TrimRight(chart, "/")
	}
	prefix := strings.TrimRight(strings.TrimSpace(defaultPrefix), "/")
	if prefix == "" {
		return chart
	}
	return prefix + "/" + strings.TrimPrefix(chart, "/")
}

func RegistryHost(chart string) string {
	chart = strings.TrimSpace(chart)
	if !strings.HasPrefix(chart, "oci://") {
		return ""
	}
	u, err := url.Parse(chart)
	if err != nil {
		rest := strings.TrimPrefix(chart, "oci://")
		host, _, _ := strings.Cut(rest, "/")
		return host
	}
	return u.Host
}

// NeedsRegistryLogin returns the registry host for private Harbor-style registries.
func NeedsRegistryLogin(chart string) string {
	host := RegistryHost(chart)
	if host == "" {
		return ""
	}
	h := strings.ToLower(host)
	public := []string{"docker.io", "registry-1.docker.io", "index.docker.io", "ghcr.io", "quay.io"}
	for _, p := range public {
		if h == p || strings.HasPrefix(h, p+":") || strings.HasSuffix(strings.Split(h, ":")[0], "."+p) {
			return ""
		}
	}
	return host
}

func ChartAllowed(chart string, prefixes []string) error {
	chart = strings.TrimSpace(chart)
	if chart == "" {
		return fmt.Errorf("chart is required")
	}
	if !strings.HasPrefix(chart, "oci://") {
		return fmt.Errorf("chart must be an oci:// URL")
	}
	if strings.ContainsAny(chart, " \t\n\"'`$;&|<>") {
		return fmt.Errorf("chart contains invalid characters")
	}
	allowAny := false
	for _, p := range prefixes {
		p = strings.TrimRight(strings.TrimSpace(p), "/")
		if p == "*" {
			allowAny = true
			break
		}
	}
	if allowAny {
		return nil
	}
	for _, p := range prefixes {
		p = strings.TrimRight(strings.TrimSpace(p), "/")
		if p == "" {
			continue
		}
		if chart == p || strings.HasPrefix(chart, p+"/") {
			return nil
		}
	}
	return fmt.Errorf("chart %q is not on the allowed OCI prefix list", chart)
}

func ValidImagePolicy(p string) bool {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "", "external", "proxy", "copy":
		return true
	default:
		return false
	}
}

func ValidChartSource(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "oci", "http", "upload", "harbor":
		return true
	default:
		return false
	}
}

func InferChartSource(origin string) string {
	o := strings.TrimSpace(origin)
	switch {
	case strings.HasPrefix(o, "oci://"):
		return "oci"
	case strings.HasPrefix(o, "http://"), strings.HasPrefix(o, "https://"):
		return "http"
	default:
		return ""
	}
}

func ValidHTTPOrigin(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("origin is required")
	}
	if strings.ContainsAny(raw, " \t\n\"'`$;&|<>") {
		return fmt.Errorf("origin contains invalid characters")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("origin must be an http(s) URL")
	}
	return nil
}

// NameFromOCI returns the last path element of an oci:// URL.
func NameFromOCI(chart string) string {
	chart = strings.TrimRight(strings.TrimSpace(chart), "/")
	if i := strings.LastIndex(chart, "/"); i >= 0 && i+1 < len(chart) {
		return strings.ToLower(chart[i+1:])
	}
	return strings.ToLower(chart)
}

var httpChartVersionSuffix = regexp.MustCompile(`[-_]v?\d+\.\d+(?:\.\d+)?(?:[-.][0-9A-Za-z]+)*$`)

// NameFromHTTP infers a DNS-1123 chart name from an http(s) .tgz URL.
func NameFromHTTP(origin string) string {
	u, err := url.Parse(strings.TrimSpace(origin))
	if err != nil {
		return ""
	}
	base := strings.ToLower(path.Base(u.Path))
	base = strings.TrimSuffix(base, ".tar.gz")
	base = strings.TrimSuffix(base, ".tgz")
	base = httpChartVersionSuffix.ReplaceAllString(base, "")
	base = strings.Trim(base, "-_.")
	if ValidDNS1123(base) {
		return base
	}
	return ""
}

// LibraryName maps a FleetApp chart field to a FleetChart object name.
func LibraryName(chart, defaultPrefix string) string {
	chart = strings.TrimSpace(chart)
	if chart == "" {
		return ""
	}
	if !strings.Contains(chart, "://") {
		return strings.ToLower(chart)
	}
	expanded := ExpandChart(chart, defaultPrefix)
	prefix := strings.TrimRight(strings.TrimSpace(defaultPrefix), "/")
	if prefix != "" && strings.HasPrefix(expanded, prefix+"/") {
		return strings.ToLower(strings.Trim(expanded[len(prefix)+1:], "/"))
	}
	return NameFromOCI(expanded)
}

func SafeInboxBase(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.ToLower(name)
	if name == "" || name == "." || name == ".." {
		return ""
	}
	ok := regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	if !ok.MatchString(name) || len(name) > 120 {
		return ""
	}
	return name
}
