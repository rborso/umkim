package traefik

import "testing"

func TestHostnamesInRoutesYAML(t *testing.T) {
	yaml := []byte("rule: Host(`app.dev.example.com`) && PathPrefix(`/`)")
	hosts := HostnamesInRoutesYAML(yaml)
	if _, ok := hosts["app.dev.example.com"]; !ok {
		t.Fatalf("expected app.dev.example.com in %v", hosts)
	}
}

func TestNewHostnames(t *testing.T) {
	prev := []byte("rule: Host(`a.example.com`)")
	next := []byte("rule: Host(`a.example.com`) && Host(`b.example.com`)")
	added := NewHostnames(prev, next)
	if len(added) != 1 || added[0] != "b.example.com" {
		t.Fatalf("got %v", added)
	}
	if len(NewHostnames(next, next)) != 0 {
		t.Fatal("expected no new hostnames for identical config")
	}
}
