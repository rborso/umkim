package httpapi

import (
	"context"
	"net/http"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/ingressauth"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

type hubService struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type hubInfrastructureResponse struct {
	HostName         string                 `json:"host_name"`
	ManagementSiteID string                 `json:"management_site_id"`
	WireguardIP      string                 `json:"wireguard_ip,omitempty"`
	PublicEndpoint   string                 `json:"public_endpoint,omitempty"`
	HubGateway       bool                   `json:"hub_gateway,omitempty"`
	HubZone          string                 `json:"hub_zone,omitempty"`
	DnsTargetNote    string                 `json:"dns_target_note,omitempty"`
	Services         []hubService           `json:"services,omitempty"`
	Nodes            []nodeSummary          `json:"nodes,omitempty"`
	IngressAuth      *hubIngressAuthSummary `json:"ingress_auth,omitempty"`
}

type hubIngressAuthSummary struct {
	Enabled    bool   `json:"enabled"`
	Phase      string `json:"phase,omitempty"`
	PortalHost string `json:"portalHost,omitempty"`
	HubEnabled bool   `json:"hubEnabled,omitempty"`
}

func (s *Server) handleInfrastructure(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1alpha1/infrastructure/")
	path = strings.Trim(path, "/")
	if path != "hub" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	s.handleGetHubInfrastructure(r.Context(), w)
}

func (s *Server) handleGetHubInfrastructure(ctx context.Context, w http.ResponseWriter) {
	siteID := s.managementSiteID()
	var fs fleetv1alpha1.FleetSite
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: siteID}, &fs); err != nil {
		if !apierrors.IsNotFound(err) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	fs.Spec.NormalizeIngress()
	resp := hubInfrastructureResponse{
		HostName:         s.hubHostName(),
		ManagementSiteID: siteID,
		WireguardIP:      s.Cfg.HubWireguardIP,
		HubGateway:       fs.Spec.HubGatewayEnabled(),
		HubZone:          fs.Spec.EffectiveHubZone(),
		Services: []hubService{
			{Name: "Harbor", Description: "Fleet image registry"},
			{Name: "Fleet Console", Description: "Sites and clusters"},
			{Name: "OCM hub", Description: "clusteradm / ManifestWorks"},
			{Name: "Hub ingress", Description: "Traefik hostNetwork on :80/:443"},
		},
	}
	if fia, err := ingressauth.EnsureCR(ctx, s.Client, s.Cfg.Namespace, s.Cfg); err == nil {
		portal := fia.Spec.PortalHost
		if portal == "" {
			portal = "auth." + s.Cfg.HubEdgeDomain
		}
		resp.IngressAuth = &hubIngressAuthSummary{
			Enabled:    fia.Status.Phase == fleetv1alpha1.IngressAuthPhaseReady,
			Phase:      fia.Status.Phase,
			PortalHost: portal,
			HubEnabled: fia.Spec.HubEnabled,
		}
		if fia.Spec.HubEnabled || fia.Status.Phase == fleetv1alpha1.IngressAuthPhaseReady {
			resp.Services = append(resp.Services,
				hubService{Name: "Ingress Authelia", Description: "Forward-auth portal at " + portal},
				hubService{Name: "Ingress LLDAP", Description: "Directory for ingress auth (separate from console login)"},
			)
		}
	}
	if we := fs.Spec.EffectiveWanEgress(); we != nil {
		resp.DnsTargetNote = we.DnsTargetNote
	}
	for _, ent := range fs.Spec.Edge.Ingress {
		if ent.Method == fleetv1alpha1.IngressMethodPortForward && ent.Address != "" {
			resp.PublicEndpoint = ent.Address
			break
		}
	}
	if cs, err := s.localClientset(); err == nil {
		resp.Nodes = enrichHubNodes(ctx, cs, s.RESTConfig)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) hubHostName() string {
	if s.Cfg.HubHostInventoryName != "" {
		return s.Cfg.HubHostInventoryName
	}
	return "umkim-hub"
}
