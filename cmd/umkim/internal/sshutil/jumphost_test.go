package sshutil

import "testing"

func TestAppendProxyArgs(t *testing.T) {
	args, err := AppendProxyArgs(nil, "linux@jump.example.com:30200", "linux")
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 2 || args[0] != "-o" {
		t.Fatalf("args=%v", args)
	}
	if args[1] != `ProxyCommand=ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o BatchMode=yes -p 30200 -W %h:%p linux@jump.example.com` {
		t.Fatalf("proxy=%q", args[1])
	}
}
