package client

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rborso/umkim/cmd/umkim/internal/config"
)

func TestClientDevHeaders(t *testing.T) {
	var gotKind string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKind = r.Header.Get("X-Fleet-Subject-Kind")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c, err := New(ResolveTest(srv.URL+"/api/v1alpha1", true))
	if err != nil {
		t.Fatal(err)
	}
	_, code, err := c.Get("/clusters")
	if err != nil || code != 200 {
		t.Fatalf("get: code=%d err=%v", code, err)
	}
	if gotKind != "User" {
		t.Fatalf("kind=%q", gotKind)
	}
}

func TestClientBearerToken(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c, _ := New(ResolveTest(srv.URL+"/api/v1alpha1", false))
	c.token = "fleet_sa_test"
	_, _, _ = c.Get("/clusters")
	if auth != "Bearer fleet_sa_test" {
		t.Fatalf("auth=%q", auth)
	}
}

func ResolveTest(api string, dev bool) config.File {
	return config.File{APIURL: api, DevMode: dev}
}
