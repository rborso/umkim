package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/jobs"
	"github.com/rborso/umkim/controllers/fleet-site/internal/storage"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (s *Server) handleValidateStorage(ctx context.Context, w http.ResponseWriter, r *http.Request, clusterName string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req clusterStorageBody
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Backend != "" && req.Backend != fleetv1alpha1.StorageBackendNFSExternal {
		http.Error(w, "only nfs_external is supported for validate-storage", http.StatusBadRequest)
		return
	}
	req.Backend = fleetv1alpha1.StorageBackendNFSExternal

	fcj, err := s.findFleetClusterJoin(ctx, clusterName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "cluster join not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if fcj.Status.Phase != fleetv1alpha1.JoinPhaseReady {
		http.Error(w, "cluster must be Ready", http.StatusConflict)
		return
	}
	edge, _ := loadEdgeClusters(ctx, s.Client, s.Cfg.EdgeClustersCMNamespace, s.Cfg.EdgeClustersCMName)
	if s.clusterReadOnly(clusterName, edge) {
		http.Error(w, "cluster is read-only", http.StatusForbidden)
		return
	}

	next := storageSpecFromBody(&req)
	if next == nil || next.NFS == nil {
		http.Error(w, "storage.nfs is required", http.StatusBadRequest)
		return
	}
	if next.NFS.ClusterPath == "" {
		next.NFS.ClusterPath = fcj.Spec.EffectiveClusterName(fcj.Name)
	}
	if err := storage.ValidateNFSSpecProbe(next, fcj); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	invName := clusterInventoryCMName(clusterName)
	if err := s.ensureClusterInventoryForJob(ctx, fcj.Namespace, invName); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err := s.ensureClusterSSHForJob(ctx, fcj.Namespace, clusterName); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jobName := storageValidateJobName(clusterName)
	extra := storage.NFSSpecExtraArgs(next, clusterName)
	j := jobs.ValidateStorageJob(fcj.Namespace, jobName, clusterName, fcj.Spec.SiteId,
		s.Cfg.AnsibleRunnerImage, clusterAnsibleSSHSecretName(clusterName), invName, extra)
	if j.Labels == nil {
		j.Labels = map[string]string{}
	}
	j.Labels["fleet.umkim.com/validate-storage"] = "true"
	if err := s.Client.Create(ctx, j); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() {
		bg := context.Background()
		_ = s.Client.Delete(bg, &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: fcj.Namespace},
		}, client.PropagationPolicy(metav1.DeletePropagationForeground))
	}()

	runCtx, cancel := context.WithTimeout(ctx, 100*time.Second)
	defer cancel()
	done, ok, err := s.waitAnsibleJob(runCtx, fcj.Namespace, jobName, 2*time.Second, 95*time.Second)
	if err != nil {
		http.Error(w, err.Error(), http.StatusGatewayTimeout)
		return
	}
	if !done {
		http.Error(w, "storage validation did not complete", http.StatusGatewayTimeout)
		return
	}

	logs, logErr := s.ansibleJobPodLogs(runCtx, fcj.Namespace, jobName)
	if parsed, found := storage.ParseNFSValidateJobLogs(logs); found {
		status := http.StatusOK
		if !parsed.OK {
			status = http.StatusUnprocessableEntity
		}
		writeJSON(w, status, parsed)
		return
	}

	msg := "NFS validation failed"
	if logErr == nil {
		if tail := ansibleLogTail(logs, 12); tail != "" {
			msg = msg + ": " + tail
		}
	} else if !ok {
		msg = "NFS validation job failed"
	}
	if ok {
		writeJSON(w, http.StatusOK, &storage.NFSValidateResult{
			OK: true,
			Checks: []storage.NFSValidateCheck{{
				Name:    "ansible",
				OK:      true,
				Message: "Validation job completed (no structured result in logs)",
			}},
		})
		return
	}
	writeJSON(w, http.StatusUnprocessableEntity, &storage.NFSValidateResult{
		OK: false,
		Checks: []storage.NFSValidateCheck{{
			Name:    "ansible",
			OK:      false,
			Message: msg,
		}},
	})
}

func (s *Server) ensureClusterInventoryForJob(ctx context.Context, ns, name string) error {
	var cm corev1.ConfigMap
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("cluster ansible inventory not found (provision or adopt cluster first)")
		}
		return err
	}
	if strings.TrimSpace(cm.Data["inventory.ini"]) == "" {
		return fmt.Errorf("cluster ansible inventory is empty")
	}
	return nil
}

func (s *Server) ensureClusterSSHForJob(ctx context.Context, ns, clusterName string) error {
	name := clusterAnsibleSSHSecretName(clusterName)
	var sec corev1.Secret
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &sec); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("cluster ansible SSH secret not found")
		}
		return err
	}
	if len(sec.Data["id_rsa"]) == 0 {
		return fmt.Errorf("cluster ansible SSH secret missing id_rsa")
	}
	return nil
}

func ansibleLogTail(logs string, maxLines int) string {
	lines := strings.Split(strings.TrimSpace(logs), "\n")
	if len(lines) <= maxLines {
		return strings.TrimSpace(logs)
	}
	return strings.Join(lines[len(lines)-maxLines:], "\n")
}
