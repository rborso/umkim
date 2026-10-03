package config

import (
	"os"
	"testing"
)

func TestResolveDevDefaults(t *testing.T) {
	t.Setenv("UMKIM_API_URL", "")
	t.Setenv("UMKIM_AUTH_URL", "")
	t.Setenv("UMKIM_TOKEN", "")
	t.Setenv("UMKIM_DEV", "")
	_ = os.Unsetenv("UMKIM_API_URL")
	_ = os.Unsetenv("UMKIM_AUTH_URL")
	_ = os.Unsetenv("UMKIM_TOKEN")
	_ = os.Unsetenv("UMKIM_DEV")
	f := Resolve(File{DevMode: true}, "", "", "")
	if f.APIURL != "http://127.0.0.1:18080/api/v1alpha1" {
		t.Fatalf("api url: %q", f.APIURL)
	}
}

func TestDeriveAuthURL(t *testing.T) {
	got := deriveAuthURL("https://hub.example/fleet/api/v1alpha1")
	want := "https://hub.example/fleet/auth/v1alpha1"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
