package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestListLlamaProfilesEmpty(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = fleetv1alpha1.AddToScheme(scheme)
	s := &Server{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Cfg:    config.Config{ConsoleNamespace: "fleet-console"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/llama-profiles", nil)
	w := httptest.NewRecorder()
	s.handleLlamaProfiles(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	var doc struct {
		Profiles []map[string]interface{} `json:"profiles"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
}

func TestCreateLlamaProfileRequiresActor(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = fleetv1alpha1.AddToScheme(scheme)
	s := &Server{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Cfg:    config.Config{ConsoleNamespace: "fleet-console", RequireIdentity: true},
	}
	body := bytes.NewBufferString(`{"id":"custom-prof","profileYaml":"model: x\n"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/llama-profiles", body)
	w := httptest.NewRecorder()
	s.handleLlamaProfiles(w, req)
	if w.Code != http.StatusForbidden && w.Code != http.StatusUnauthorized {
		t.Fatalf("expected auth failure, got %d", w.Code)
	}
}

func TestGetLlamaProfileValuesFromCR(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = fleetv1alpha1.AddToScheme(scheme)
	cr := &fleetv1alpha1.FleetLlamaProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "qwen35-2b-ud-4g", Namespace: "fleet-console"},
		Spec: fleetv1alpha1.FleetLlamaProfileSpec{
			System:      true,
			ProfileYAML: "model: TestModel\nservice:\n  port: 9090\n",
		},
	}
	s := &Server{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr).Build(),
		Cfg:    config.Config{ConsoleNamespace: "fleet-console"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/llama-profiles/qwen35-2b-ud-4g/values", nil)
	w := httptest.NewRecorder()
	s.getLlamaProfileValues(w, req, "qwen35-2b-ud-4g")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("TestModel")) {
		t.Fatalf("expected merged values: %s", w.Body.String())
	}
}
