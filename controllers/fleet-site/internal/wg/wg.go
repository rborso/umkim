package wg

import (
	"fmt"
	"os/exec"
	"strings"
)

func GenKeyPair() (priv, pub string, err error) {
	out, err := exec.Command("wg", "genkey").Output()
	if err != nil {
		return "", "", fmt.Errorf("wg genkey: %w", err)
	}
	priv = strings.TrimSpace(string(out))
	pubOut, err := exec.Command("bash", "-c", fmt.Sprintf("printf '%%s\\n' %q | wg pubkey", priv)).Output()
	if err != nil {
		return "", "", fmt.Errorf("wg pubkey: %w", err)
	}
	pub = strings.TrimSpace(string(pubOut))
	return priv, pub, nil
}

func AllocateAddress(used map[string]bool, start, end int) (string, error) {
	for o := start; o <= end; o++ {
		ip := fmt.Sprintf("10.10.0.%d", o)
		if !used[ip] {
			return ip + "/32", nil
		}
	}
	return "", fmt.Errorf("no free address in pool %d-%d", start, end)
}

func HostIP(cidr string) string {
	return strings.SplitN(cidr, "/", 2)[0]
}
