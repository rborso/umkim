package reconcile

import "testing"

func TestExpandImageMapEntries(t *testing.T) {
	from := "docker.io/traefik/whoami:v1.10.1"
	to := "10.10.0.1:30443/mirror/docker.io/traefik/whoami:v1.10.1"
	got := expandImageMapEntries(from, to)
	want := map[string]string{
		from:                              to,
		"traefik/whoami:v1.10.1":          to,
	}
	for ref, dst := range want {
		if got[ref] != dst {
			t.Fatalf("expandImageMapEntries(%q)[%q] = %q, want %q", from, ref, got[ref], dst)
		}
	}
}
