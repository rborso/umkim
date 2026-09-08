package bootstrap

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rborso/umkim/cmd/umkim/internal/sshutil"
)

// Run copies bundle tarball to target and executes bootstrap.sh remotely.
func Run(opts Options) error {
	if opts.BundleTar == "" || opts.TargetHost == "" {
		return fmt.Errorf("bundle and target host required")
	}
	if opts.Jumphost != "" {
		return runViaJumphost(opts)
	}
	return runDirect(opts)
}

func runDirect(opts Options) error {
	sshUser := opts.SSHUser
	if sshUser == "" {
		sshUser = "linux"
	}
	cluster := opts.ClusterName
	remoteTar := fmt.Sprintf("/tmp/bootstrap-%s.tar.gz", cluster)
	remoteDir := fmt.Sprintf("/tmp/bootstrap-%s", cluster)
	target := sshUser + "@" + opts.TargetHost

	sshBase := []string{"-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null", "-o", "BatchMode=yes"}
	scpBase := append([]string{}, sshBase...)

	if err := runCmd("scp", append(scpBase, opts.BundleTar, target+":"+remoteTar)...); err != nil {
		return fmt.Errorf("scp bundle: %w", err)
	}
	return runSSHScript(sshBase, target, remoteBootstrapScript(remoteTar, remoteDir))
}

func runViaJumphost(opts Options) error {
	sshUser := opts.SSHUser
	if sshUser == "" {
		sshUser = "linux"
	}
	jUser, jHost, jPort, ok := sshutil.ParseJumphost(opts.Jumphost, sshUser)
	if !ok {
		return fmt.Errorf("invalid jumphost %q", opts.Jumphost)
	}
	cluster := opts.ClusterName
	jumpTarget := jUser + "@" + jHost
	jumpTar := fmt.Sprintf("/tmp/bootstrap-%s.tar.gz", cluster)
	target := sshUser + "@" + opts.TargetHost
	targetTar := jumpTar
	remoteDir := fmt.Sprintf("/tmp/bootstrap-%s", cluster)

	jumpSSH := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "BatchMode=yes",
		"-p", jPort,
	}
	jumpSCP := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "BatchMode=yes",
		"-P", jPort,
	}
	if err := seedBundleSSHOnPeers(opts, jumpSSH, jumpTarget); err != nil {
		return fmt.Errorf("seed bundle SSH on peers: %w", err)
	}
	if err := runCmd("scp", append(jumpSCP, opts.BundleTar, jumpTarget+":"+jumpTar)...); err != nil {
		return fmt.Errorf("scp bundle to jumphost: %w", err)
	}

	inner := fmt.Sprintf(`set -euo pipefail
scp -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o BatchMode=yes %q %s:%q
ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o BatchMode=yes %s bash -s <<'UMKIM_BOOT'
%s
UMKIM_BOOT
`, jumpTar, target, targetTar, target, remoteBootstrapScript(targetTar, remoteDir))

	return runSSHScript(jumpSSH, jumpTarget, inner)
}

func remoteBootstrapScript(remoteTar, remoteDir string) string {
	return fmt.Sprintf(`set -euo pipefail
sudo dnf install -y tar gzip
rm -rf %q
mkdir -p %q
tar -xzf %q -C %q
chmod +x %q/bootstrap.sh
cd %q && sudo ./bootstrap.sh
`, remoteDir, remoteDir, remoteTar, remoteDir, remoteDir, remoteDir)
}

func runSSHScript(sshBase []string, target, script string) error {
	args := append([]string{}, sshBase...)
	args = append(args, target, "bash", "-s")
	cmd := exec.Command("ssh", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = strings.NewReader(script)
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
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
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}
