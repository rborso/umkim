package bootstrap

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func inventoryMgmtIPs(iniPath string) ([]string, error) {
	f, err := os.Open(iniPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var ips []string
	sect := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			sect = line == "[fleet_cluster_control]" || line == "[fleet_cluster_workers]"
			continue
		}
		if !sect || line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		for _, field := range strings.Fields(line) {
			if strings.HasPrefix(field, "ansible_host=") {
				ip := strings.TrimPrefix(field, "ansible_host=")
				if ip != "" {
					ips = append(ips, ip)
				}
			}
		}
	}
	return ips, sc.Err()
}

func seedBundleSSHOnPeers(opts Options, jumpSSH []string, jumpTarget string) error {
	tmpDir, err := os.MkdirTemp("", "umkim-bootstrap-seed-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	if err := runCmd("tar", "-xzf", opts.BundleTar, "-C", tmpDir, "ssh/id_rsa.pub", "inventory.ini"); err != nil {
		return nil
	}
	pubBytes, err := os.ReadFile(filepath.Join(tmpDir, "ssh/id_rsa.pub"))
	if err != nil || len(pubBytes) == 0 {
		return nil
	}
	pubkey := strings.TrimSpace(string(pubBytes))
	peers, err := inventoryMgmtIPs(filepath.Join(tmpDir, "inventory.ini"))
	if err != nil || len(peers) == 0 {
		return nil
	}

	sshUser := opts.SSHUser
	if sshUser == "" {
		sshUser = "linux"
	}
	var seedHosts []string
	for _, host := range peers {
		if host != opts.TargetHost {
			seedHosts = append(seedHosts, host)
		}
	}
	if len(seedHosts) == 0 {
		return nil
	}

	var b strings.Builder
	b.WriteString("set -euo pipefail\n")
	quotedPub := shellSingleQuote(pubkey)
	for _, host := range seedHosts {
		target := sshUser + "@" + host
		fmt.Fprintf(&b, `ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o BatchMode=yes %s bash -s <<UMKIM_SEED
mkdir -p ~/.ssh && chmod 700 ~/.ssh
grep -qF %s ~/.ssh/authorized_keys 2>/dev/null || echo %s >> ~/.ssh/authorized_keys
chmod 600 ~/.ssh/authorized_keys
UMKIM_SEED
`, target, quotedPub, quotedPub)
	}
	return runSSHScript(jumpSSH, jumpTarget, b.String())
}

func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
