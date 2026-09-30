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
// sshKeyFile is passed to the bastion hop so hub-runner jobs authenticate with the same
// cluster Ansible key used for spoke nodes (nested ssh does not inherit ansible_ssh_private_key_file).
func AnsibleSSHCommonArgs(baseArgs, jumphost, defaultSSHUser, sshKeyFile string) string {
	j, ok := ParseJumphost(jumphost, defaultSSHUser)
	if !ok {
		return baseArgs
	}
	key := strings.TrimSpace(sshKeyFile)
	if key == "" {
		key = "/root/.ssh/id_rsa"
	}
	proxy := fmt.Sprintf(
		`-o ProxyCommand="ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o BatchMode=yes -p %s -W %%h:%%p %s@%s"`,
		key, j.Port, j.User, j.Host,
	)
	if strings.TrimSpace(baseArgs) == "" {
		return proxy
	}
	return baseArgs + " " + proxy
}
