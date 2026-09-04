package bootstrap

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/rborso/umkim/cmd/umkim/internal/sshutil"
)

// Run copies bundle tarball to target and executes bootstrap.sh remotely.
func Run(opts Options) error {
	if opts.BundleTar == "" || opts.TargetHost == "" {
		return fmt.Errorf("bundle and target host required")
	}
	sshUser := opts.SSHUser
	if sshUser == "" {
		sshUser = "linux"
	}
	cluster := opts.ClusterName
	remoteTar := fmt.Sprintf("/tmp/bootstrap-%s.tar.gz", cluster)
	remoteDir := fmt.Sprintf("/tmp/bootstrap-%s", cluster)
	target := sshUser + "@" + opts.TargetHost

	sshBase := []string{"-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null"}
	scpBase := append([]string{}, sshBase...)
	if opts.Jumphost != "" {
		var err error
		sshBase, err = sshutil.AppendProxyArgs(sshBase, opts.Jumphost, sshUser)
		if err != nil {
			return err
		}
		scpBase, err = sshutil.AppendProxyArgs(scpBase, opts.Jumphost, sshUser)
		if err != nil {
			return err
		}
	}

	if err := runCmd("scp", append(scpBase, opts.BundleTar, target+":"+remoteTar)...); err != nil {
		return fmt.Errorf("scp bundle: %w", err)
	}
	script := fmt.Sprintf(`set -euo pipefail
sudo dnf install -y tar gzip
rm -rf %q
mkdir -p %q
tar -xzf %q -C %q
chmod +x %q/bootstrap.sh
cd %q && sudo ./bootstrap.sh
`, remoteDir, remoteDir, remoteTar, remoteDir, remoteDir, remoteDir)
	return runCmd("ssh", append(sshBase, target, script)...)
}

type Options struct {
	ClusterName string
	BundleTar   string
	TargetHost  string
	SSHUser     string
	Jumphost    string
}

func ExtractBundle(tarPath, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	return runCmd("tar", "-xzf", tarPath, "-C", destDir)
}

func DefaultBundleDir(cluster string) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "umkim", "bootstrap", cluster), nil
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
