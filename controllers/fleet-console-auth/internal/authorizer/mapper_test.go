package authorizer

import (
	"net/http"
	"testing"
)

func TestMapFleetApps(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/api/v1alpha1/apps", nil)
	got := MapRequest(req, "fleet")
	if got.Resource != "apps" || got.Verb != "list" {
		t.Fatalf("GET /apps => %+v", got)
	}
	req, _ = http.NewRequest(http.MethodPost, "/api/v1alpha1/apps", nil)
	got = MapRequest(req, "fleet")
	if got.Resource != "apps" || got.Verb != "create" {
		t.Fatalf("POST /apps => %+v", got)
	}
	req, _ = http.NewRequest(http.MethodDelete, "/api/v1alpha1/apps/whoami", nil)
	got = MapRequest(req, "fleet")
	if got.Resource != "apps" || got.Verb != "delete" {
		t.Fatalf("DELETE /apps/whoami => %+v", got)
	}
	req, _ = http.NewRequest(http.MethodPatch, "/api/v1alpha1/apps/whoami", nil)
	got = MapRequest(req, "fleet")
	if got.Resource != "apps" || got.Verb != "update" {
		t.Fatalf("PATCH /apps/whoami => %+v", got)
	}
}

func TestMapFleetLlamaProfiles(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "/api/v1alpha1/llama-profiles/qwen35-2b-ud-4g/verify", nil)
	got := MapRequest(req, "fleet")
	if got.Resource != "llama-profiles" || got.Verb != "create" {
		t.Fatalf("POST verify => %+v", got)
	}
	req, _ = http.NewRequest(http.MethodPost, "/api/v1alpha1/llama-profiles/tests/t1/chat/completions", nil)
	got = MapRequest(req, "fleet")
	if got.Resource != "llama-profiles" || got.Verb != "create" {
		t.Fatalf("POST chat => %+v", got)
	}
}

func TestMapFleetCharts(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/api/v1alpha1/charts", nil)
	got := MapRequest(req, "fleet")
	if got.Resource != "charts" || got.Verb != "list" {
		t.Fatalf("GET /charts => %+v", got)
	}
	req, _ = http.NewRequest(http.MethodPost, "/api/v1alpha1/charts", nil)
	got = MapRequest(req, "fleet")
	if got.Resource != "charts" || got.Verb != "create" {
		t.Fatalf("POST /charts => %+v", got)
	}
	req, _ = http.NewRequest(http.MethodDelete, "/api/v1alpha1/charts/whoami", nil)
	got = MapRequest(req, "fleet")
	if got.Resource != "charts" || got.Verb != "delete" {
		t.Fatalf("DELETE /charts/whoami => %+v", got)
	}
}
