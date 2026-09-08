package reconcile

import (
	"context"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors/yaidap"
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
	if err := siteprovision.RefreshRunningJob(ctx, r.Client, r.Cfg.Namespace, fs, r.Clientset, &r.Cfg); err != nil {
		return ctrl.Result{}, err
	}
	if res, err := r.reconcileRegistrySiteHostClusters(ctx, fs); err != nil || res.Requeue || res.RequeueAfter > 0 {
		return res, err
	}
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *Reconciler) reconcileRegistrySiteHostClusters(ctx context.Context, fs *fleetv1alpha1.FleetSite) (ctrl.Result, error) {
	siteName := fs.Spec.EffectiveSiteID(fs.Name)
	if fs.Spec.EffectiveProvisionProvider() == fleetv1alpha1.ProvisionProviderYaidap {
		data, err := connectors.Load(ctx, r.Client, r.Cfg.Namespace, siteName, fleetv1alpha1.ProvisionProviderYaidap)
		if err != nil {
			return ctrl.Result{}, err
		}
		if data != nil {
			if _, ok := connectors.GetInventoryFile(connectors.NormalizeMetaKeys(data), "guests.yml"); ok {
				if err := sitecluster.ReconcileInventorySite(ctx, r.Client, r.Cfg, siteName); err != nil {
					return ctrl.Result{}, err
				}
				return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
			}
		}
		_ = yaidap.MigrateLegacyStore(ctx, r.Client, r.Cfg.Namespace, siteName, "system/reconcile")
		data, err = connectors.Load(ctx, r.Client, r.Cfg.Namespace, siteName, fleetv1alpha1.ProvisionProviderYaidap)
		if err == nil && data != nil {
			if _, ok := connectors.GetInventoryFile(connectors.NormalizeMetaKeys(data), "guests.yml"); ok {
				if err := sitecluster.ReconcileInventorySite(ctx, r.Client, r.Cfg, siteName); err != nil {
					return ctrl.Result{}, err
				}
				return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
			}
		}
	}
	if err := sitecluster.ReconcileSite(ctx, r.Client, r.Cfg, siteName); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}
