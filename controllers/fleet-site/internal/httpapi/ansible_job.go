package httpapi

import (
	"context"
	"fmt"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (s *Server) waitAnsibleJob(ctx context.Context, ns, jobName string, pollEvery, timeout time.Duration) (done, ok bool, err error) {
	deadline := time.Now().Add(timeout)
	for {
		if ctx.Err() != nil {
			return false, false, ctx.Err()
		}
		if time.Now().After(deadline) {
			return false, false, fmt.Errorf("job %s timed out after %s", jobName, timeout)
		}
		done, ok, err = s.ansibleJobDone(ctx, ns, jobName)
		if err != nil || done {
			return done, ok, err
		}
		select {
		case <-ctx.Done():
			return false, false, ctx.Err()
		case <-time.After(pollEvery):
		}
	}
}

func (s *Server) ansibleJobDone(ctx context.Context, ns, name string) (done, ok bool, err error) {
	var job batchv1.Job
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &job); err != nil {
		if apierrors.IsNotFound(err) {
			return false, false, nil
		}
		return false, false, err
	}
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
			return true, true, nil
		}
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			msg := c.Message
			if msg == "" {
				msg = "ansible job failed"
			}
			return true, false, nil
		}
	}
	return false, false, nil
}

func (s *Server) ansibleJobPodLogs(ctx context.Context, ns, jobName string) (string, error) {
	cs, err := s.localClientset()
	if err != nil {
		return "", err
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
	podName := pods.Items[0].Name
	raw, err := cs.CoreV1().Pods(ns).GetLogs(podName, &corev1.PodLogOptions{}).DoRaw(ctx)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func clusterAnsibleSSHSecretName(clusterName string) string {
	return "fleet-cluster-" + clusterName + "-ansible-ssh"
}

func storageValidateJobName(clusterName string) string {
	suffix := fmt.Sprintf("%x", time.Now().UnixNano())
	if len(suffix) > 8 {
		suffix = suffix[len(suffix)-8:]
	}
	name := "fleet-cluster-storage-validate-" + clusterName + "-" + suffix
	if len(name) > 63 {
		name = name[:63]
	}
	return strings.TrimRight(name, "-")
}
