package sshutil

import (
	"fmt"
	"strings"
)

// AppendProxyArgs adds -o ProxyCommand=... to ssh/scp argv (quoted inner ssh command).
func AppendProxyArgs(base []string, jumphost, defaultUser string) ([]string, error) {
	j, ok := parseJumphost(jumphost, defaultUser)
	if !ok {
		return nil, fmt.Errorf("invalid jumphost %q", jumphost)
	}
	inner := fmt.Sprintf(
		"ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o BatchMode=yes -p %s -W %%h:%%p %s@%s",
		j.port, j.user, j.host,
	)
	return append(base, "-o", "ProxyCommand="+inner), nil
}

// ProxySSHArgs returns a single -o flag value for inventory-style ansible_ssh_common_args.
func ProxySSHArgs(jumphost, defaultUser string) (string, error) {
	args, err := AppendProxyArgs(nil, jumphost, defaultUser)
	if err != nil {
		return "", err
	}
	if len(args) != 2 {
		return "", fmt.Errorf("unexpected proxy args")
	}
	return args[0] + " " + args[1], nil
}

type jump struct {
	user, host, port string
}

func parseJumphost(raw, defaultUser string) (jump, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return jump{}, false
	}
	user := strings.TrimSpace(defaultUser)
	hostport := raw
	if at := strings.Index(raw, "@"); at >= 0 {
		user = strings.TrimSpace(raw[:at])
		hostport = raw[at+1:]
	}
	host := strings.TrimSpace(hostport)
	port := "22"
	if colon := strings.LastIndex(hostport, ":"); colon >= 0 {
		host = strings.TrimSpace(hostport[:colon])
		port = strings.TrimSpace(hostport[colon+1:])
	}
	if user == "" || host == "" || port == "" {
		return jump{}, false
	}
	return jump{user: user, host: host, port: port}, true
}
