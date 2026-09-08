package reconcile

import (
	"context"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/sitecluster"
	"github.com/rborso/umkim/controllers/fleet-site/internal/siteprovision"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *Reconciler) reconcileRegistrySiteProvision(ctx context.Context, fs *fleetv1alpha1.FleetSite) (ctrl.Result, error) {
	if fs.Status.Provision == nil || fs.Status.Provision.Current == nil {
		return r.reconcileRegistrySiteHostClusters(ctx, fs)
	}
	phase := fs.Status.Provision.Current.Phase
	if phase != fleetv1alpha1.ProvisionPhaseRunning && phase != fleetv1alpha1.ProvisionPhasePending {
		return r.reconcileRegistrySiteHostClusters(ctx, fs)
	}
	if err := siteprovision.RefreshRunningJob(ctx, r.Client, r.Cfg.Namespace, fs, r.Clientset); err != nil {
		return ctrl.Result{}, err
	}
	if res, err := r.reconcileRegistrySiteHostClusters(ctx, fs); err != nil || res.Requeue || res.RequeueAfter > 0 {
		return res, err
	}
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *Reconciler) reconcileRegistrySiteHostClusters(ctx context.Context, fs *fleetv1alpha1.FleetSite) (ctrl.Result, error) {
	siteName := fs.Spec.EffectiveSiteID(fs.Name)
	if err := sitecluster.ReconcileSite(ctx, r.Client, r.Cfg, siteName); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}
