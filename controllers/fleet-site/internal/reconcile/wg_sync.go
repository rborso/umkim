package reconcile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/jobs"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	batchv1 "k8s.io/api/batch/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	annWgPeerHash = "fleet.umkim.com/wg-peer-hash"
	annNodeOpID   = "fleet.umkim.com/node-op-id"
)

func clusterWgSyncJobName(clusterName string) string {
	return "fleet-cluster-wg-" + clusterName
}

func clusterNodeOpJobName(clusterName string) string {
	return "fleet-cluster-nodeop-" + clusterName
}

func siteWgSyncJobName(siteName string) string {
	return "fleet-site-wg-" + siteName
}

func peerSetHash(add, remove []jobs.WGPeer) string {
	parts := make([]string, 0, len(add)+len(remove))
	for _, p := range add {
		parts = append(parts, "a:"+p.PublicKey+":"+p.AllowedIP)
	}
	for _, p := range remove {
		parts = append(parts, "r:"+p.PublicKey+":"+p.AllowedIP)
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:8])
}

func (r *ClusterJoinReconciler) ensureClusterWgSyncJob(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string, wgPeers, removePeers []jobs.WGPeer) error {
	if len(wgPeers) == 0 && len(removePeers) == 0 {
		return nil
	}
	jobName := clusterWgSyncJobName(clusterName)
	hash := peerSetHash(wgPeers, removePeers)

	var existing batchv1.Job
	err := r.Get(ctx, types.NamespacedName{Namespace: fcj.Namespace, Name: jobName}, &existing)
	if err == nil {
		if existing.Annotations != nil && existing.Annotations[annWgPeerHash] == hash {
			if done, ok := jobIsFinished(&existing); done && ok {
				return nil
			}
		}
		if done, _ := jobIsFinished(&existing); !done {
			return nil
		}
		_ = r.Delete(ctx, &existing, client.PropagationPolicy(metav1.DeletePropagationBackground))
	} else if !apierrors.IsNotFound(err) {
		return err
	}

	wgJob := jobs.WgSyncJobMulti(fcj.Namespace, jobName, wgPeers, removePeers)
	if wgJob.Annotations == nil {
		wgJob.Annotations = map[string]string{}
	}
	wgJob.Annotations[annWgPeerHash] = hash
	if err := controllerutil.SetControllerReference(fcj, wgJob, r.Scheme); err != nil {
		return err
	}
	if err := r.Create(ctx, wgJob); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create wg sync job %s: %w", jobName, err)
	}
	return nil
}

func (r *Reconciler) ensureSiteWgSyncJob(ctx context.Context, fs *fleetv1alpha1.FleetSite, siteName string, pub, allowedIP string, removePeers []jobs.WGPeer) error {
	wgPeers := []jobs.WGPeer{{PublicKey: pub, AllowedIP: allowedIP}}
	if pub == "" {
		wgPeers = nil
	}
	if len(wgPeers) == 0 && len(removePeers) == 0 {
		return nil
	}
	jobName := siteWgSyncJobName(siteName)
	hash := peerSetHash(wgPeers, removePeers)

	var existing batchv1.Job
	err := r.Get(ctx, types.NamespacedName{Namespace: fs.Namespace, Name: jobName}, &existing)
	if err == nil {
		if existing.Annotations != nil && existing.Annotations[annWgPeerHash] == hash {
			if done, ok := jobIsFinished(&existing); done && ok {
				return nil
			}
		}
		if done, _ := jobIsFinished(&existing); !done {
			return nil
		}
		_ = r.Delete(ctx, &existing, client.PropagationPolicy(metav1.DeletePropagationBackground))
	} else if !apierrors.IsNotFound(err) {
		return err
	}

	wgJob := jobs.WgSyncJob(fs.Namespace, jobName, pub, allowedIP, removePeers)
	if wgJob.Annotations == nil {
		wgJob.Annotations = map[string]string{}
	}
	wgJob.Annotations[annWgPeerHash] = hash
	if err := controllerutil.SetControllerReference(fs, wgJob, r.Scheme); err != nil {
		return err
	}
	if err := r.Create(ctx, wgJob); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create wg sync job %s: %w", jobName, err)
	}
	return nil
}
