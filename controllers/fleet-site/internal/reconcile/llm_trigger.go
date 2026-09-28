package reconcile

import (
	"context"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/fleetllm"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *Reconciler) requestFleetLLMReconcile(ctx context.Context) error {
	ns := r.Cfg.Namespace
	var fl fleetv1alpha1.FleetLLM
	err := r.Get(ctx, types.NamespacedName{Namespace: ns, Name: fleetv1alpha1.FleetLLMName}, &fl)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			_, err = fleetllm.EnsureCR(ctx, r.Client, ns, r.Cfg)
			return err
		}
		return err
	}
	if fl.Annotations == nil {
		fl.Annotations = map[string]string{}
	}
	fl.Annotations[fleetllm.ReconcileAnnotation] = time.Now().UTC().Format(time.RFC3339)
	return r.Update(ctx, &fl)
}
