package reconcile

import (
	"context"
	"fmt"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/clusters"
)

func (r *RouteReconciler) resolveSpokeACMEState(
	ctx context.Context,
	route *fleetv1alpha1.FleetAppRoute,
	clusterName string,
	regCluster clusters.Cluster,
) spokeACMEState {
	if !regCluster.SpokeACMEEnabled(r.Config.ACMEEnabled) {
		return spokeACMEState{}
	}
	mode := route.Spec.EffectiveMode
	if mode == "" {
		mode = route.Spec.Mode
	}
	if mode != fleetv1alpha1.RouteModeSite {
		return spokeACMEState{}
	}
	siteHost := normalizeHostname(route.Spec.SiteHost)
	if siteHost == "" {
		return spokeACMEState{}
	}

	switch route.Status.TLSPhase {
	case fleetv1alpha1.TLSPhaseFailedDNS, fleetv1alpha1.TLSPhaseFailedRateLimit:
		return spokeACMEState{Blocked: true, TLSPhase: route.Status.TLSPhase, TLSMessage: route.Status.TLSMessage}
	}

	if r.Config.ACMEDNSCheckEnabled {
		ok, msg := r.validateSiteDNS(ctx, siteHost, regCluster)
		if !ok {
			_ = r.patchTLSStatus(ctx, route, fleetv1alpha1.TLSPhaseFailedDNS, msg)
			return spokeACMEState{Blocked: true, TLSPhase: fleetv1alpha1.TLSPhaseFailedDNS, TLSMessage: msg}
		}
	}

	ok, msg := validateSpokeConnectivity(ctx, siteHost)
	if !ok {
		_ = r.patchTLSStatus(ctx, route, fleetv1alpha1.TLSPhasePendingPreflight, msg)
		return spokeACMEState{Blocked: true, TLSPhase: fleetv1alpha1.TLSPhasePendingPreflight, TLSMessage: msg}
	}

	if r.spokeACME == nil {
		r.spokeACME = newSpokeACMEQueue()
	}
	if !r.spokeACME.allow(clusterName, siteHost) {
		_ = r.patchTLSStatus(ctx, route, fleetv1alpha1.TLSPhaseQueued, "waiting for spoke ACME queue")
		return spokeACMEState{
			ApplyACME: true,
			SkipApply: true,
			TLSPhase:  fleetv1alpha1.TLSPhaseQueued,
			TLSMessage: "waiting for spoke ACME queue",
		}
	}

	probeHost := spokeProbeHost(ctx, siteHost, regCluster)
	if probeHost == "" {
		msg := fmt.Sprintf("cannot determine public probe address for %s", siteHost)
		_ = r.patchTLSStatus(ctx, route, fleetv1alpha1.TLSPhaseFailed, msg)
		return spokeACMEState{Blocked: true, TLSPhase: fleetv1alpha1.TLSPhaseFailed, TLSMessage: msg}
	}

	if prodOK, _ := probeACMECert(probeHost, siteHost, false); prodOK {
		_ = r.patchTLSStatus(ctx, route, fleetv1alpha1.TLSPhaseReady, "")
		return spokeACMEState{
			ApplyACME:    true,
			CertResolver: certResolverProduction,
			TLSPhase:     fleetv1alpha1.TLSPhaseReady,
		}
	}

	stagingOK := route.Annotations != nil && route.Annotations[annotationSpokeACMEStagingOK] == "true"
	if r.Config.SpokeACMEPreflightEnabled {
		if !stagingOK {
			if stagingReady, _ := probeACMECert(probeHost, siteHost, true); stagingReady {
				_ = r.annotateSpokeStagingOK(ctx, route)
				stagingOK = true
			} else {
				_ = r.patchTLSStatus(ctx, route, fleetv1alpha1.TLSPhasePendingStaging,
					"requesting Let's Encrypt staging certificate (dry-run)")
				return spokeACMEState{
					ApplyACME:    true,
					CertResolver: certResolverStaging,
					TLSPhase:     fleetv1alpha1.TLSPhasePendingStaging,
					TLSMessage:   "requesting Let's Encrypt staging certificate (dry-run)",
				}
			}
		}
	}

	_ = r.patchTLSStatus(ctx, route, fleetv1alpha1.TLSPhaseIssuing, "requesting production Let's Encrypt certificate")
	return spokeACMEState{
		ApplyACME:    true,
		CertResolver: certResolverProduction,
		TLSPhase:     fleetv1alpha1.TLSPhaseIssuing,
		TLSMessage:   "requesting production Let's Encrypt certificate",
	}
}
