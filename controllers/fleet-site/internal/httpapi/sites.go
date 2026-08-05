package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/registry"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type siteResponse struct {
	fleetv1alpha1.FleetSite `json:",inline"`
	Clusters                []string `json:"clusters,omitempty"`
	ClusterCount            int      `json:"clusterCount,omitempty"`
}

type sitePatchBody struct {
	DisplayName           *string                                    `json:"displayName,omitempty"`
	SiteID                *string                                    `json:"siteId,omitempty"`
	Region                *string                                    `json:"region,omitempty"`
	FailureDomain         *string                                    `json:"failureDomain,omitempty"`
	EdgeDomain            *string                                    `json:"edgeDomain,omitempty"`
	Wildcard              *string                                    `json:"wildcard,omitempty"`
	ExposureMode          *string                                    `json:"exposureMode,omitempty"`
	DefaultExposureMode   *string                                    `json:"defaultExposureMode,omitempty"`
	ExternalEndpoints     *[]fleetv1alpha1.FleetSiteExternalEndpointSpec `json:"externalEndpoints,omitempty"`
	Domains               *[]fleetv1alpha1.FleetSiteDomainSpec       `json:"domains,omitempty"`
	WanEgress             *fleetv1alpha1.FleetSiteWanEgressSpec      `json:"wanEgress,omitempty"`
	ExternalAccess        *fleetv1alpha1.FleetSiteExternalAccessSpec `json:"externalAccess,omitempty"`
	ProvisioningMode      *string                                    `json:"provisioningMode,omitempty"`
}

func (s *Server) enrichSite(ctx context.Context, fs *fleetv1alpha1.FleetSite) siteResponse {
	siteID := fs.Spec.EffectiveSiteID(fs.Name)
	mcMap, _ := registry.ClustersBySiteID(ctx, s.Client)
	clusters := mcMap[siteID]
	if clusters == nil {
		clusters = []string{}
	}
	return siteResponse{
		FleetSite:    *fs,
		Clusters:     clusters,
		ClusterCount: len(clusters),
	}
}

func (s *Server) listSitesEnriched(ctx context.Context) ([]siteResponse, error) {
	var list fleetv1alpha1.FleetSiteList
	if err := s.Client.List(ctx, &list, client.InNamespace(s.Cfg.Namespace)); err != nil {
		return nil, err
	}
	mcMap, _ := registry.ClustersBySiteID(ctx, s.Client)
	out := make([]siteResponse, 0, len(list.Items))
	for i := range list.Items {
		fs := list.Items[i]
		siteID := fs.Spec.EffectiveSiteID(fs.Name)
		clusters := mcMap[siteID]
		if clusters == nil {
			clusters = []string{}
		}
		out = append(out, siteResponse{FleetSite: fs, Clusters: clusters, ClusterCount: len(clusters)})
	}
	return out, nil
}

func (s *Server) handlePatchSite(ctx context.Context, w http.ResponseWriter, r *http.Request, name string) {
	if _, ok := s.requireMutatingActor(w, r); !ok {
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var patch sitePatchBody
	if err := json.Unmarshal(body, &patch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var fs fleetv1alpha1.FleetSite
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: name}, &fs); err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	siteID := fs.Spec.EffectiveSiteID(fs.Name)
	mgmtSite := isManagementFleetSite(&fs, siteID, s.managementSiteID())

	if mgmtSite {
		if patch.SiteID != nil || patch.ProvisioningMode != nil || patch.ExposureMode != nil ||
			patch.EdgeDomain != nil || patch.Wildcard != nil {
			http.Error(w, "management site: only display name, domains, external endpoints, and DNS notes may be changed", http.StatusForbidden)
			return
		}
	}

	if patch.SiteID != nil && *patch.SiteID != siteID {
		mcMap, _ := registry.ClustersBySiteID(ctx, s.Client)
		if len(mcMap[siteID]) > 0 {
			http.Error(w, "cannot change siteId while clusters are attached", http.StatusConflict)
			return
		}
		fs.Spec.SiteId = *patch.SiteID
	}
	if patch.DisplayName != nil {
		fs.Spec.DisplayName = *patch.DisplayName
	}
	if patch.Region != nil {
		fs.Spec.Region = *patch.Region
	}
	if patch.FailureDomain != nil {
		fs.Spec.FailureDomain = *patch.FailureDomain
	}
	if !mgmtSite {
		if patch.EdgeDomain != nil {
			fs.Spec.Edge.EdgeDomain = *patch.EdgeDomain
			fs.Spec.EdgeDomain = *patch.EdgeDomain
		}
		if patch.Wildcard != nil {
			fs.Spec.Edge.Wildcard = *patch.Wildcard
		}
		if patch.ExposureMode != nil {
			fs.Spec.Edge.ExposureMode = *patch.ExposureMode
		}
		if patch.DefaultExposureMode != nil {
			fs.Spec.Edge.DefaultExposureMode = *patch.DefaultExposureMode
		}
	}
	if patch.ExternalEndpoints != nil {
		fs.Spec.Edge.ExternalEndpoints = *patch.ExternalEndpoints
	}
	if patch.Domains != nil {
		domains := *patch.Domains
		for i := range domains {
			domains[i].AssignedClusters = nil
		}
		fs.Spec.Edge.Domains = domains
	}
	if patch.WanEgress != nil {
		fs.Spec.Edge.WanEgress = patch.WanEgress
	}
	if patch.ExternalAccess != nil {
		fs.Spec.Edge.ExternalAccess = patch.ExternalAccess
		if fs.Spec.Edge.WanEgress == nil {
			fs.Spec.Edge.WanEgress = &fleetv1alpha1.FleetSiteWanEgressSpec{
				Enabled:       patch.ExternalAccess.Enabled,
				DnsTargetNote: patch.ExternalAccess.DnsTargetNote,
				ExpectedWanIp: patch.ExternalAccess.ExpectedWanIp,
			}
		}
	}
	if patch.ProvisioningMode != nil {
		fs.Spec.ProvisioningMode = *patch.ProvisioningMode
	}
	fs.Spec.NormalizeEdge()

	if err := s.Client.Update(ctx, &fs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = registry.SyncSiteAssignedClusters(ctx, s.Client, s.domainRegistryConfig())
	_ = registry.ReconcileEdgeDomains(ctx, s.Client, s.domainRegistryConfig())
	writeJSON(w, http.StatusOK, s.enrichSite(ctx, &fs))
}
