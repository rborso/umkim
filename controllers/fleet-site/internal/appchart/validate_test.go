package appchart

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

func TestExpandChart(t *testing.T) {
	got := ExpandChart("whoami", "oci://10.10.0.1:30443/charts")
	want := "oci://10.10.0.1:30443/charts/whoami"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	full := "oci://10.10.0.1:30443/charts/whoami"
	if ExpandChart(full, "oci://other") != full {
		t.Fatalf("full URL should be unchanged")
	}
}

func TestChartAllowed(t *testing.T) {
	prefixes := []string{"oci://10.10.0.1:30443/charts", "oci://ghcr.io"}
	if err := ChartAllowed("oci://10.10.0.1:30443/charts/whoami", prefixes); err != nil {
		t.Fatal(err)
	}
	if err := ChartAllowed("oci://evil.example/charts/whoami", prefixes); err == nil {
		t.Fatal("expected deny")
	}
	if err := ChartAllowed("oci://anything/x", []string{"*"}); err != nil {
		t.Fatal(err)
	}
}

func TestNeedsRegistryLogin(t *testing.T) {
	if h := NeedsRegistryLogin("oci://10.10.0.1:30443/charts/whoami"); h != "10.10.0.1:30443" {
		t.Fatalf("harbor host=%q", h)
	}
	if h := NeedsRegistryLogin("oci://ghcr.io/org/chart"); h != "" {
		t.Fatalf("ghcr should skip login, got %q", h)
	}
}

func TestValidDNS1123(t *testing.T) {
	if !ValidDNS1123("whoami") || !ValidDNS1123("lab-single-0") {
		t.Fatal("expected valid names")
	}
	if ValidDNS1123("WhoAmi") || ValidDNS1123("") || ValidDNS1123("-x") {
		t.Fatal("expected invalid names")
	}
}

func TestLibraryName(t *testing.T) {
	prefix := "oci://10.10.0.1:30443/charts"
	if got := LibraryName("whoami", prefix); got != "whoami" {
		t.Fatalf("short=%q", got)
	}
	if got := LibraryName(prefix+"/whoami", prefix); got != "whoami" {
		t.Fatalf("full=%q", got)
	}
}

func TestNameFromHTTP(t *testing.T) {
	if got := NameFromHTTP("https://example.com/charts/whoami-0.1.0.tgz"); got != "whoami" {
		t.Fatalf("got %q", got)
	}
	if got := NameFromHTTP("https://example.com/pet-clinic-helm-1.2.3.tar.gz"); got != "pet-clinic-helm" {
		t.Fatalf("got %q", got)
	}
	if NameFromHTTP("https://example.com/") != "" {
		t.Fatal("empty path should not invent a name")
	}
}

func TestMergeValuesYAML(t *testing.T) {
	got, err := MergeValuesYAML("a: 1\nb: 2\n", "b: 3\nc: 4\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "a: 1") || !strings.Contains(got, "b: 3") || !strings.Contains(got, "c: 4") {
		t.Fatalf("merged=%q", got)
	}
}

func TestMetaFromTGZ(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte("apiVersion: v2\nname: whoami\nversion: 0.1.0\n")
	if err := tw.WriteHeader(&tar.Header{Name: "whoami/Chart.yaml", Mode: 0644, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	name, ver, err := MetaFromTGZReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if name != "whoami" || ver != "0.1.0" {
		t.Fatalf("name=%q ver=%q", name, ver)
	}
}
