package reconcile

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

const gatewayPreflightACMEPath = "/var/traefik/acme-preflight.json"
const gatewayRunningPodWait = 2 * time.Minute

// clearGatewayPreflightCert removes a hostname from the gateway staging ACME store so
// production TLS-ALPN is not shadowed by a leftover preflight certificate.
func clearGatewayPreflightCert(ctx context.Context, cfg rest.Config, depNS, depName, hostname string) error {
	hostname = strings.ToLower(strings.TrimSpace(hostname))
	if hostname == "" {
		return nil
	}
	clientset, err := kubernetes.NewForConfig(&cfg)
	if err != nil {
		return err
	}
	podName, err := waitGatewayRunningPod(ctx, clientset, depNS, depName, gatewayRunningPodWait)
	if err != nil {
		return err
	}
	raw, err := readGatewayFile(ctx, cfg, depNS, podName, gatewayPreflightACMEPath)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var store map[string]json.RawMessage
	if err := json.Unmarshal(raw, &store); err != nil {
		return fmt.Errorf("parse preflight acme store: %w", err)
	}
	changed := false
	for resolver, body := range store {
		var obj map[string]interface{}
		if err := json.Unmarshal(body, &obj); err != nil {
			continue
		}
		certsRaw, ok := obj["Certificates"].([]interface{})
		if !ok {
			continue
		}
		kept := make([]interface{}, 0, len(certsRaw))
		for _, item := range certsRaw {
			cert, ok := item.(map[string]interface{})
			if !ok {
				kept = append(kept, item)
				continue
			}
			dom, _ := cert["domain"].(map[string]interface{})
			main, _ := dom["main"].(string)
			if strings.EqualFold(strings.TrimSpace(main), hostname) {
				changed = true
				continue
			}
			kept = append(kept, item)
		}
		if len(kept) == len(certsRaw) {
			continue
		}
		obj["Certificates"] = kept
		updated, err := json.Marshal(obj)
		if err != nil {
			return err
		}
		store[resolver] = updated
	}
	if !changed {
		return nil
	}
	out, err := json.Marshal(store)
	if err != nil {
		return err
	}
	return writeGatewayFile(ctx, cfg, depNS, podName, gatewayPreflightACMEPath, out)
}

func chmodGatewayFile(ctx context.Context, cfg rest.Config, ns, pod, path, mode string) error {
	clientset, err := kubernetes.NewForConfig(&cfg)
	if err != nil {
		return err
	}
	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod).
		Namespace(ns).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "traefik",
			Command:   []string{"chmod", mode, path},
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)
	return execCommandWithStdin(ctx, cfg, req, nil)
}

func gatewayRunningPod(ctx context.Context, clientset kubernetes.Interface, ns, depName string) (string, error) {
	return waitGatewayRunningPod(ctx, clientset, ns, depName, 0)
}

func waitGatewayRunningPod(ctx context.Context, clientset kubernetes.Interface, ns, depName string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for {
		pod, err := findGatewayRunningPod(ctx, clientset, ns, depName)
		if err == nil {
			return pod, nil
		}
		if timeout == 0 || time.Now().After(deadline) {
			return "", err
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func findGatewayRunningPod(ctx context.Context, clientset kubernetes.Interface, ns, depName string) (string, error) {
	dep, err := clientset.AppsV1().Deployments(ns).Get(ctx, depName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	selector, err := metav1.LabelSelectorAsSelector(dep.Spec.Selector)
	if err != nil {
		return "", err
	}
	pods, err := clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return "", err
	}
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning && pod.DeletionTimestamp == nil {
			return pod.Name, nil
		}
	}
	return "", fmt.Errorf("no running gateway pod in %s/%s", ns, depName)
}

func readGatewayFile(ctx context.Context, cfg rest.Config, ns, pod, path string) ([]byte, error) {
	clientset, err := kubernetes.NewForConfig(&cfg)
	if err != nil {
		return nil, err
	}
	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod).
		Namespace(ns).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "traefik",
			Command:   []string{"cat", path},
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)
	return execCommand(ctx, cfg, req)
}

func writeGatewayFile(ctx context.Context, cfg rest.Config, ns, pod, path string, data []byte) error {
	clientset, err := kubernetes.NewForConfig(&cfg)
	if err != nil {
		return err
	}
	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod).
		Namespace(ns).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "traefik",
			Command:   []string{"tee", path},
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)
	return execCommandWithStdin(ctx, cfg, req, data)
}

func execCommand(ctx context.Context, cfg rest.Config, req *rest.Request) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	exec, err := remotecommand.NewSPDYExecutor(&cfg, "POST", req.URL())
	if err != nil {
		return nil, err
	}
	if err := exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	}); err != nil {
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("%w: %s", err, stderr.String())
		}
		return nil, err
	}
	return stdout.Bytes(), nil
}

func execCommandWithStdin(ctx context.Context, cfg rest.Config, req *rest.Request, stdin []byte) error {
	var stdout, stderr bytes.Buffer
	exec, err := remotecommand.NewSPDYExecutor(&cfg, "POST", req.URL())
	if err != nil {
		return err
	}
	if err := exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  bytes.NewReader(stdin),
		Stdout: &stdout,
		Stderr: &stderr,
	}); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("%w: %s", err, stderr.String())
		}
		return err
	}
	return nil
}
