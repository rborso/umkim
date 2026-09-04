package output

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func capture(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func TestWriteClustersTable(t *testing.T) {
	raw := []byte(`{"clusters":[{"name":"lab-ha-0","site":"homelab","provisioning_phase":"Ready","available":true}]}`)
	out := capture(func() {
		if err := WriteClusters(raw, FormatTable); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "lab-ha-0") || !strings.Contains(out, "yes") {
		t.Fatalf("table output: %q", out)
	}
}

func TestWriteChartsTable(t *testing.T) {
	raw := []byte(`{"charts":[{"name":"whoami","source":"harbor","version":"0.1.0","phase":"Ready","imagePolicy":"external"}]}`)
	out := capture(func() {
		if err := WriteCharts(raw, FormatTable); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "whoami") || !strings.Contains(out, "Ready") {
		t.Fatalf("table output: %q", out)
	}
}

func TestClusterTopologyFromLabels(t *testing.T) {
	c := map[string]interface{}{
		"labels": map[string]interface{}{
			"fleet.topology/topology": "ha_3_node",
		},
		"topology": map[string]interface{}{"ha": "true"},
	}
	if got := clusterTopology(c); got != "ha_3_node" {
		t.Fatalf("got %q", got)
	}
}

func TestWriteSiteIngressTable(t *testing.T) {
	rows := []map[string]interface{}{
		{"id": "site-pf", "method": "port_forward", "address": "203.0.113.50", "zone": "site.example.com", "assignedClusters": []interface{}{"lab-ha-0"}},
		{"id": "edge-rp", "method": "reverse_proxy", "description": "nginx", "assignedClusters": []string{"lab-single-0"}},
	}
	out := capture(func() {
		if err := WriteSiteIngress("homelab", true, "hub.example.com", rows, FormatTable); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "site-pf") || !strings.Contains(out, "port_forward") || !strings.Contains(out, "HUB_GATEWAY") {
		t.Fatalf("table output: %q", out)
	}
	if !strings.Contains(out, "lab-ha-0") || !strings.Contains(out, "lab-single-0") {
		t.Fatalf("assigned clusters missing: %q", out)
	}
}

func TestSiteIngressSummary(t *testing.T) {
	spec := map[string]interface{}{
		"edge": map[string]interface{}{
			"hubGateway": true,
			"ingress": []interface{}{
				map[string]interface{}{"id": "site-pf", "method": "port_forward"},
			},
		},
	}
	got := siteIngressSummary(spec)
	if !strings.Contains(got, "hub_gateway") || !strings.Contains(got, "site-pf(port_forward)") {
		t.Fatalf("got %q", got)
	}
}
