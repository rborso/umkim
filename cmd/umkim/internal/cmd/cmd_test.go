package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/rborso/umkim/cmd/umkim/internal/config"
)

func TestClusterListDevMode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Fleet-DC-Admin") != "true" {
			t.Fatalf("missing dev admin header")
		}
		if r.URL.Path != "/api/v1alpha1/clusters" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"clusters":[{"name":"test"}]}`))
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
	c := clusterListCmd()
	c.SetArgs(nil)
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
}
