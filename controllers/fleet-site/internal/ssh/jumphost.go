package ssh

import (
	"fmt"
	"strings"
)

// Jumphost is a parsed user@host:port SSH bastion.
type Jumphost struct {
	User string
	Host string
	Port string
}

// ParseJumphost parses user@host:port (user and port optional).
func ParseJumphost(raw, defaultUser string) (Jumphost, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Jumphost{}, false
	}
	user := strings.TrimSpace(defaultUser)
	hostport := raw
	if at := strings.Index(raw, "@"); at >= 0 {
		user = strings.TrimSpace(raw[:at])
		hostport = raw[at+1:]
	}
	hostport = strings.TrimSpace(hostport)
	if user == "" || hostport == "" {
		return Jumphost{}, false
	}
	host := hostport
	port := "22"
	if colon := strings.LastIndex(hostport, ":"); colon >= 0 {
		host = strings.TrimSpace(hostport[:colon])
		port = strings.TrimSpace(hostport[colon+1:])
	}
	if host == "" || port == "" {
		return Jumphost{}, false
	}
	return Jumphost{User: user, Host: host, Port: port}, true
}

// AnsibleSSHCommonArgs appends a ProxyCommand hop when jumphost is set.
func AnsibleSSHCommonArgs(baseArgs, jumphost, defaultSSHUser string) string {
	j, ok := ParseJumphost(jumphost, defaultSSHUser)
	if !ok {
		return baseArgs
	}
	proxy := fmt.Sprintf(
		`-o ProxyCommand="ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o BatchMode=yes -p %s -W %%h:%%p %s@%s"`,
		j.Port, j.User, j.Host,
	)
	if strings.TrimSpace(baseArgs) == "" {
		return proxy
	}
	return baseArgs + " " + proxy
}
