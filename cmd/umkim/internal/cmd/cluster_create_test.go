package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/rborso/umkim/cmd/umkim/internal/config"
)

// captureClusterCreate runs clusterCreateCmd against a stub API and returns the
// posted body. It returns an error when the command fails.
func captureClusterCreate(t *testing.T, args []string) (map[string]interface{}, error) {
	t.Helper()
	var posted map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1alpha1/clusters" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &posted)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"name":"stub"}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	os.Setenv("UMKIM_CONFIG_DIR", dir)
	os.Setenv("UMKIM_DEV", "1")
	t.Cleanup(func() {
		os.Unsetenv("UMKIM_CONFIG_DIR")
		os.Unsetenv("UMKIM_DEV")
	})

	flagAPIURL = srv.URL + "/api/v1alpha1"
	cfgFile = config.Resolve(config.File{}, flagAPIURL, "", "")
	var err error
	api, err = newClient(cfgFile)
	if err != nil {
		t.Fatal(err)
	}
	flagOutput = "json"

	c := clusterCreateCmd()
	c.SetArgs(args)
	// The command prints the response; keep the test output clean.
	c.SetOut(io.Discard)
	runErr := c.Execute()
	return posted, runErr
}

func TestClusterCreateVMsFromFlags(t *testing.T) {
	body, err := captureClusterCreate(t, []string{
		"--name", "lab-ha-1", "--site", "homelab", "--topology", "ha_3_node",
		"--control-addresses", "172.16.10.101,172.16.10.102,172.16.10.103",
		"--worker-addresses", "172.16.10.110",
		"--control-plane-vip", "172.16.10.111",
		"--create-vms", "--auto-provision",
		"--control-cores", "4", "--control-memory", "6144", "--control-disk", "50",
		"--worker-cores", "2", "--worker-memory", "4096", "--worker-disk", "40",
	})
	if err != nil {
		t.Fatal(err)
	}
	if body["clusterName"] != "lab-ha-1" || body["siteId"] != "homelab" {
		t.Fatalf("identity fields: %v", body)
	}
	if body["autoProvision"] != true {
		t.Fatalf("autoProvision: %v", body["autoProvision"])
	}
	// The first control address becomes the bootstrap host.
	if body["bootstrapHost"] != "172.16.10.101" {
		t.Fatalf("bootstrapHost = %v", body["bootstrapHost"])
	}
	nodes, _ := body["nodes"].([]interface{})
	if len(nodes) != 4 {
		t.Fatalf("nodes = %v", nodes)
	}
	first, _ := nodes[0].(map[string]interface{})
	if first["address"] != "172.16.10.101" || first["role"] != "control" {
		t.Fatalf("first node = %v", first)
	}
	// Hostnames stay unset so the server names guests after the cluster.
	if _, ok := first["hostname"]; ok {
		t.Fatalf("hostname should be omitted: %v", first)
	}
	vm, _ := first["vm"].(map[string]interface{})
	if vm["cores"] != float64(4) || vm["memory"] != float64(6144) || vm["diskGi"] != float64(50) {
		t.Fatalf("control vm = %v", vm)
	}
	last, _ := nodes[3].(map[string]interface{})
	if last["role"] != "worker" || last["address"] != "172.16.10.110" {
		t.Fatalf("worker node = %v", last)
	}
	wvm, _ := last["vm"].(map[string]interface{})
	if wvm["cores"] != float64(2) || wvm["diskGi"] != float64(40) {
		t.Fatalf("worker vm = %v", wvm)
	}
}

func TestClusterCreateVMsSingleNode(t *testing.T) {
	body, err := captureClusterCreate(t, []string{
		"--name", "lab-sn", "--site", "homelab",
		"--control-addresses", "172.16.10.120",
		"--create-vms",
	})
	if err != nil {
		t.Fatal(err)
	}
	nodes, _ := body["nodes"].([]interface{})
	if len(nodes) != 1 {
		t.Fatalf("nodes = %v", nodes)
	}
	if body["bootstrapHost"] != "172.16.10.120" {
		t.Fatalf("bootstrapHost = %v", body["bootstrapHost"])
	}
}

func TestClusterCreateVMFlagValidation(t *testing.T) {
	for name, args := range map[string][]string{
		"ha needs three controls": {
			"--name", "x", "--site", "s", "--topology", "ha_3_node",
			"--control-addresses", "172.16.10.1,172.16.10.2", "--create-vms",
		},
		"single needs one control": {
			"--name", "x", "--site", "s",
			"--control-addresses", "172.16.10.1,172.16.10.2", "--create-vms",
		},
		"single rejects workers": {
			"--name", "x", "--site", "s",
			"--control-addresses", "172.16.10.1", "--worker-addresses", "172.16.10.9",
			"--create-vms",
		},
		"duplicate address": {
			"--name", "x", "--site", "s",
			"--control-addresses", "172.16.10.1", "--worker-addresses", "172.16.10.1",
			"--create-vms",
		},
		"missing name": {
			"--site", "s", "--control-addresses", "172.16.10.1", "--create-vms",
		},
		"missing bootstrap host": {
			"--name", "x", "--site", "s",
		},
	} {
		if _, err := captureClusterCreate(t, args); err == nil {
			t.Fatalf("%s: want an error", name)
		} else if !strings.Contains(err.Error(), "required") && !strings.Contains(err.Error(), "needs") &&
			!strings.Contains(err.Error(), "does not support") && !strings.Contains(err.Error(), "duplicate") {
			t.Fatalf("%s: unexpected error %v", name, err)
		}
	}
}

func TestClusterCreateFlagsOverrideFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/body.json"
	if err := os.WriteFile(path, []byte(`{
		"clusterName":"from-file","siteId":"from-file","bootstrapHost":"172.16.10.1",
		"topology":"single_node","nodes":[{"address":"172.16.10.1","role":"control"}]
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	body, err := captureClusterCreate(t, []string{
		"-f", path,
		"--name", "from-flag",
		"--control-addresses", "172.16.10.200",
		"--create-vms",
	})
	if err != nil {
		t.Fatal(err)
	}
	if body["clusterName"] != "from-flag" {
		t.Fatalf("flag did not override the file: %v", body["clusterName"])
	}
	// siteId came from the file.
	if body["siteId"] != "from-file" {
		t.Fatalf("file field lost: %v", body["siteId"])
	}
	// --create-vms replaced the file's nodes.
	nodes, _ := body["nodes"].([]interface{})
	if len(nodes) != 1 {
		t.Fatalf("nodes = %v", nodes)
	}
	node, _ := nodes[0].(map[string]interface{})
	if node["address"] != "172.16.10.200" {
		t.Fatalf("nodes not replaced: %v", node)
	}
	if _, ok := node["vm"]; !ok {
		t.Fatalf("generated node has no vm block: %v", node)
	}
	// bootstrapHost stays as the file set it (explicit).
	if body["bootstrapHost"] != "172.16.10.1" {
		t.Fatalf("bootstrapHost = %v", body["bootstrapHost"])
	}
}
