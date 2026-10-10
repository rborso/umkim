package exec

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// defaultTimeout bounds a single kubectl invocation so a stuck API server or
// unreachable cluster cannot block the reconcile worker forever.
const defaultTimeout = 60 * time.Second

func Kubectl(ctx context.Context, kubeconfig string, args ...string) (string, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
	}
	var cmdArgs []string
	if kubeconfig != "" {
		cmdArgs = append(cmdArgs, "--kubeconfig="+kubeconfig)
	}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.CommandContext(ctx, "kubectl", cmdArgs...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("kubectl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func ApplyYAML(ctx context.Context, kubeconfig, yaml string) error {
	tmp, err := os.CreateTemp("", "fleet-edge-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(yaml); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	_, err = Kubectl(ctx, kubeconfig, "apply", "-f", tmp.Name())
	return err
}

func DeleteManifestWork(ctx context.Context, kubeconfig, ns, name string) error {
	// --wait=false: a ManifestWork with a stuck finalizer would otherwise block
	// kubectl delete indefinitely and hang the reconcile worker.
	_, err := Kubectl(ctx, kubeconfig, "-n", ns, "delete", "manifestwork", name, "--ignore-not-found", "--wait=false")
	return err
}
