package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/rborso/umkim/cmd/umkim/internal/config"
	"github.com/spf13/cobra"
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

func TestAppListDevMode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1alpha1/apps" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apps":[{"name":"whoami","clusterName":"lab","chart":"whoami","phase":"Ready","route":{"mode":"none"}}]}`))
	}))
	defer srv.Close()

	setupDevClient(t, srv.URL+"/api/v1alpha1")
	flagOutput = "json"
	c := appListCmd()
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestInfraGetHubDevMode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1alpha1/infrastructure/hub" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"host_name":"hub01","management_site_id":"umkim","hub_zone":"apps.example.com"}`))
	}))
	defer srv.Close()

	setupDevClient(t, srv.URL+"/api/v1alpha1")
	flagOutput = "json"
	c := infraGetHubCmd()
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestAuthUsersForbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1alpha1/users" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		http.Error(w, "forbidden", http.StatusForbidden)
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
	flagAuthURL = srv.URL + "/api/v1alpha1"
	cfgFile = config.Resolve(config.File{}, flagAPIURL, flagAuthURL, "")
	var err error
	api, err = newClient(cfgFile)
	if err != nil {
		t.Fatal(err)
	}

	c := authUsersCmd()
	var listCmd *cobra.Command
	for _, sub := range c.Commands() {
		if sub.Name() == "list" {
			listCmd = sub
			break
		}
	}
	if listCmd == nil {
		t.Fatal("list subcommand not found")
	}
	err = listCmd.RunE(listCmd, nil)
	if err == nil {
		t.Fatal("expected dcAdmin error")
	}
	if !strings.Contains(err.Error(), "dcAdmin") {
		t.Fatalf("error=%q", err.Error())
	}
}

func TestCommandExamples(t *testing.T) {
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		if strings.TrimSpace(c.Example) == "" {
			t.Errorf("missing Example: %s", c.CommandPath())
		}
		for _, line := range strings.Split(c.Example, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			if !strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "   ") {
				t.Errorf("%s: example line must be indented with two spaces: %q", c.CommandPath(), line)
			}
		}
		buf := new(bytes.Buffer)
		c.SetOut(buf)
		c.SetErr(buf)
		if err := c.Help(); err != nil {
			t.Errorf("%s help: %v", c.CommandPath(), err)
		} else if !strings.Contains(buf.String(), "Examples:") {
			t.Errorf("%s help missing Examples section:\n%s", c.CommandPath(), buf.String())
		}
		for _, child := range c.Commands() {
			walk(child)
		}
	}
	walk(rootCmd)
}

func setupDevClient(t *testing.T, apiURL string) {
	t.Helper()
	dir := t.TempDir()
	os.Setenv("UMKIM_CONFIG_DIR", dir)
	os.Setenv("UMKIM_DEV", "1")
	t.Cleanup(func() {
		os.Unsetenv("UMKIM_CONFIG_DIR")
		os.Unsetenv("UMKIM_DEV")
	})
	flagAPIURL = apiURL
	cfgFile = config.Resolve(config.File{}, flagAPIURL, "", "")
	var err error
	api, err = newClient(cfgFile)
	if err != nil {
		t.Fatal(err)
	}
}
