package exec

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func Kubectl(ctx context.Context, kubeconfig string, args ...string) (string, error) {
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
	_, err := Kubectl(ctx, kubeconfig, "-n", ns, "delete", "manifestwork", name, "--ignore-not-found")
	return err
}
