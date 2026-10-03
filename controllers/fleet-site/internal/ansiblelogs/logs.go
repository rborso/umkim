package ansiblelogs

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// PodLogs returns stdout/stderr logs from the first pod owned by a batch Job.
func PodLogs(ctx context.Context, cs kubernetes.Interface, ns, jobName string) (string, error) {
	if cs == nil {
		return "", fmt.Errorf("kubernetes client not configured")
	}
	pods, err := cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		LabelSelector: "job-name=" + jobName,
	})
	if err != nil {
		return "", err
	}
	if len(pods.Items) == 0 {
		return "", fmt.Errorf("no pod found for job %s", jobName)
	}
	raw, err := cs.CoreV1().Pods(ns).GetLogs(pods.Items[0].Name, &corev1.PodLogOptions{}).DoRaw(ctx)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// Tail truncates log text to at most maxBytes (UTF-8 safe enough for ansible output).
func Tail(logs string, maxBytes int) string {
	if maxBytes <= 0 || len(logs) <= maxBytes {
		return logs
	}
	start := len(logs) - maxBytes
	for start < len(logs) && logs[start]&0xC0 == 0x80 {
		start++
	}
	return logs[start:]
}
