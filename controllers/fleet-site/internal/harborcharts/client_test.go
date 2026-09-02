package harborcharts

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListCharts(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2.0/projects/charts/repositories":
			_, _ = w.Write([]byte(`[{"name":"charts/whoami"}]`))
		case "/api/v2.0/projects/charts/repositories/whoami/artifacts":
			_, _ = w.Write([]byte(`[{"tags":[{"name":"0.1.0"}]}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "admin", "x")
	c.HTTP = srv.Client()
	got, err := c.ListCharts("charts")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "whoami" {
		t.Fatalf("got=%+v", got)
	}
	if len(got[0].Versions) != 1 || got[0].Versions[0] != "0.1.0" {
		t.Fatalf("versions=%v", got[0].Versions)
	}
}
