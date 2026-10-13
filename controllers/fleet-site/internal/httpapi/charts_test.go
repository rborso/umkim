package httpapi

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testChartServer(t *testing.T, objs ...client.Object) *Server {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := fleetv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return &Server{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build(),
		Cfg:    testAppServer(t).Cfg,
	}
}

func TestCreateChartRejectsOffPrefixOrigin(t *testing.T) {
	s := testChartServer(t)
	s.Cfg.HelmOCIPrefixes = "oci://10.10.0.1:30443/charts"
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/charts", strings.NewReader(`{
		"origin":"oci://evil.example/charts/x","version":"1.0.0"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleCharts(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateChartOCI(t *testing.T) {
	s := testChartServer(t)
	s.Cfg.HelmOCIPrefixes = "oci://10.10.0.1:30443/charts,oci://ghcr.io"
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/charts", strings.NewReader(`{
		"origin":"oci://ghcr.io/example/gitea","version":"1.2.3","imagePolicy":"external"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleCharts(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp chartResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Name != "gitea" || resp.Source != "oci" || resp.ImagePolicy != "external" {
		t.Fatalf("resp=%+v", resp)
	}
}

func TestCreateChartHTTP(t *testing.T) {
	s := testChartServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/charts", strings.NewReader(`{
		"origin":"https://example.com/charts/whoami-0.1.0.tgz","version":"0.1.0","imagePolicy":"proxy"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleCharts(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp chartResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Name != "whoami" || resp.Source != "http" || resp.ImagePolicy != "proxy" {
		t.Fatalf("resp=%+v", resp)
	}
}

func TestGetAndDeleteChart(t *testing.T) {
	s := testChartServer(t)
	s.Cfg.HelmOCIPrefixes = "oci://ghcr.io"
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/charts", strings.NewReader(`{
		"origin":"oci://ghcr.io/example/gitea","version":"1.2.3"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleCharts(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	get := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/charts/gitea", nil)
	grec := httptest.NewRecorder()
	s.handleChartByName(grec, get, "gitea")
	if grec.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", grec.Code, grec.Body.String())
	}
	del := httptest.NewRequest(http.MethodDelete, "/api/v1alpha1/charts/gitea", nil)
	drec := httptest.NewRecorder()
	s.handleChartByName(drec, del, "gitea")
	if drec.Code != http.StatusAccepted {
		t.Fatalf("delete status=%d body=%s", drec.Code, drec.Body.String())
	}
	missing := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/charts/gitea", nil)
	mrec := httptest.NewRecorder()
	s.handleChartByName(mrec, missing, "gitea")
	if mrec.Code != http.StatusNotFound {
		t.Fatalf("missing status=%d", mrec.Code)
	}
}

func TestCreateChartRejectsBadImagePolicy(t *testing.T) {
	s := testChartServer(t)
	s.Cfg.HelmOCIPrefixes = "oci://ghcr.io"
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/charts", strings.NewReader(`{
		"origin":"oci://ghcr.io/example/gitea","imagePolicy":"suffix"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleCharts(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPutChartReadyRequiresForce(t *testing.T) {
	existing := &fleetv1alpha1.FleetChart{
		ObjectMeta: metav1.ObjectMeta{Name: "whoami", Namespace: "fleet-console"},
		Spec: fleetv1alpha1.FleetChartSpec{
			Source:  fleetv1alpha1.ChartSourceOCI,
			Origin:  "oci://ghcr.io/example/whoami",
			Version: "0.1.0",
		},
		Status: fleetv1alpha1.FleetChartStatus{Phase: fleetv1alpha1.ChartPhaseReady},
	}
	s := testChartServer(t, existing)
	s.Cfg.HelmOCIPrefixes = "oci://ghcr.io"

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/charts", strings.NewReader(`{
		"name":"whoami","origin":"oci://ghcr.io/example/other","version":"0.2.0","source":"oci"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleCharts(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/charts?force=true", strings.NewReader(`{
		"name":"whoami","origin":"oci://ghcr.io/example/other","version":"0.2.0","source":"oci","force":true
	}`))
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	s.handleCharts(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("force status=%d body=%s", rec2.Code, rec2.Body.String())
	}
}

func TestListChartsEmpty(t *testing.T) {
	s := testChartServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/charts", nil)
	rec := httptest.NewRecorder()
	s.handleCharts(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp chartsListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Charts == nil {
		t.Fatal("charts should be empty list not null")
	}
}

func TestCreateChartUpload(t *testing.T) {
	s := testChartServer(t)
	s.Cfg.ChartInboxDir = t.TempDir()
	pkg := chartTGZ(t, "whoami", "0.1.0")
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("chart", "whoami-0.1.0.tgz")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(pkg); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/charts", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	s.handleCharts(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp chartResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Name != "whoami" || resp.Source != "upload" {
		t.Fatalf("resp=%+v", resp)
	}
	matches, _ := filepath.Glob(filepath.Join(s.Cfg.ChartInboxDir, "*.tgz"))
	if len(matches) != 1 {
		t.Fatalf("inbox files=%v", matches)
	}
	if _, err := os.Stat(matches[0]); err != nil {
		t.Fatal(err)
	}
}

func chartTGZ(t *testing.T, name, version string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte("apiVersion: v2\nname: " + name + "\nversion: " + version + "\n")
	hdr := &tar.Header{Name: name + "/Chart.yaml", Mode: 0644, Size: int64(len(body))}
	if err := tw.WriteHeader(hdr); err != nil {
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
	return buf.Bytes()
}

// uploadChart posts a multipart chart package; force is omitted when empty.
func uploadChart(t *testing.T, s *Server, name, version, force string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("chart", name+"-"+version+".tgz")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(chartTGZ(t, name, version)); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	target := "/api/v1alpha1/charts"
	if force != "" {
		target += "?force=" + force
	}
	req := httptest.NewRequest(http.MethodPost, target, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	s.handleCharts(rec, req)
	return rec
}

func readyUploadChart(name, inboxFile string) *fleetv1alpha1.FleetChart {
	return &fleetv1alpha1.FleetChart{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "fleet-console"},
		Spec: fleetv1alpha1.FleetChartSpec{
			Source:    fleetv1alpha1.ChartSourceUpload,
			Origin:    "upload:" + inboxFile,
			InboxFile: inboxFile,
			Version:   "0.1.0",
		},
		Status: fleetv1alpha1.FleetChartStatus{Phase: fleetv1alpha1.ChartPhaseReady},
	}
}

// A rejected upload must not leave its staged tarball in the inbox.
func TestUploadChartRejectedDropsStagedFile(t *testing.T) {
	s := testChartServer(t, readyUploadChart("whoami", "whoami-aaaaaa.tgz"))
	s.Cfg.ChartInboxDir = t.TempDir()

	rec := uploadChart(t, s, "whoami", "0.2.0", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	matches, _ := filepath.Glob(filepath.Join(s.Cfg.ChartInboxDir, "*.tgz"))
	if len(matches) != 0 {
		t.Fatalf("rejected upload left files in the inbox: %v", matches)
	}
}

// A force-replace must drop the tarball the new spec no longer references.
func TestForceReplaceDropsSupersededInboxFile(t *testing.T) {
	dir := t.TempDir()
	superseded := filepath.Join(dir, "whoami-aaaaaa.tgz")
	if err := os.WriteFile(superseded, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := testChartServer(t, readyUploadChart("whoami", "whoami-aaaaaa.tgz"))
	s.Cfg.ChartInboxDir = dir

	rec := uploadChart(t, s, "whoami", "0.2.0", "true")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(superseded); !os.IsNotExist(err) {
		t.Fatalf("superseded inbox file still present: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*.tgz"))
	if len(matches) != 1 {
		t.Fatalf("want only the new upload in the inbox, got %v", matches)
	}
}
