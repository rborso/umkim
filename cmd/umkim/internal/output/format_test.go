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

func TestParseFormat(t *testing.T) {
	f, err := ParseFormat("wide")
	if err != nil || f != FormatWide {
		t.Fatalf("got %v %v", f, err)
	}
	if _, err := ParseFormat("yaml"); err == nil {
		t.Fatal("expected error")
	}
}
