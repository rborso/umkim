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

func TestChartListDevMode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1alpha1/charts" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"charts":[{"name":"whoami","source":"harbor","phase":"Ready","imagePolicy":"external"}]}`))
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
	c := chartListCmd()
	c.SetArgs(nil)
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestChartAddOrigin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1alpha1/charts" {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"name":"whoami","source":"oci","phase":"Pending","imagePolicy":"external"}`))
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

	c := chartAddCmd()
	c.SetArgs([]string{"--origin", "oci://ghcr.io/example/charts/whoami", "--version", "0.1.0"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestSiteIngressList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1alpha1/sites/homelab" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"metadata":{"name":"homelab"},
			"spec":{"siteName":"homelab","edge":{"hubGateway":false,"ingress":[
				{"id":"site-pf","method":"port_forward","address":"203.0.113.50","zone":"site.example.com"}
			]}}
		}`))
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
	c := siteIngressListCmd()
	c.SetArgs([]string{"homelab"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
}
