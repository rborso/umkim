package ssh

import (
	"strings"
	"testing"
)

func TestParseJumphost(t *testing.T) {
	j, ok := ParseJumphost("linux@jump.example.com:30200", "admin")
	if !ok || j.User != "linux" || j.Host != "jump.example.com" || j.Port != "30200" {
		t.Fatalf("unexpected parse: %+v ok=%v", j, ok)
	}
	j, ok = ParseJumphost("jump.example.com", "linux")
	if !ok || j.User != "linux" || j.Port != "22" {
		t.Fatalf("default user/port: %+v ok=%v", j, ok)
	}
}

func TestAnsibleSSHCommonArgs(t *testing.T) {
	got := AnsibleSSHCommonArgs("-o StrictHostKeyChecking=no", "linux@jump.example.com:30200", "admin", "/root/.ssh/id_rsa")
	if got == "-o StrictHostKeyChecking=no" {
		t.Fatal("expected proxy args")
	}
	for _, part := range []string{"ProxyCommand", "-i /root/.ssh/id_rsa", "jump.example.com", "30200", "linux@"} {
		if !strings.Contains(got, part) {
			t.Fatalf("missing %q in %q", part, got)
		}
	}
}
