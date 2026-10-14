package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	fleetedge "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/ingress"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Verify check statuses.
const (
	ingressVerifyOK   = "ok"
	ingressVerifyWarn = "warn"
	ingressVerifyFail = "fail"
	ingressVerifySkip = "skip"
)

const (
	ingressVerifyProbeTimeout = 5 * time.Second
	// Default spoke Traefik NodePorts for a port_forward / reverse_proxy entry.
	ingressVerifyHTTPPort  = 30080
	ingressVerifyHTTPSPort = 30443
)

// Probe indirection so tests do not depend on live DNS or network reachability.
var (
	ingressVerifyLookupHost = func(ctx context.Context, host string) ([]string, error) {
		var resolver net.Resolver
		return resolver.LookupHost(ctx, host)
	}
	ingressVerifyDial = func(ctx context.Context, address string) error {
		var d net.Dialer
		conn, err := d.DialContext(ctx, "tcp", address)
		if err != nil {
			return err
		}
		return conn.Close()
	}
)

type ingressVerifyCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type ingressVerifyRow struct {
	ID               string               `json:"id"`
	Method           string               `json:"method"`
	Address          string               `json:"address,omitempty"`
	WanAddressMode   string               `json:"wanAddressMode,omitempty"`
	Zone             string               `json:"zone,omitempty"`
	Wildcard         string               `json:"wildcard,omitempty"`
	AssignedClusters []string             `json:"assignedClusters,omitempty"`
	OK               bool                 `json:"ok"`
	Checks           []ingressVerifyCheck `json:"checks"`
}

type ingressVerifyResponse struct {
	Site string             `json:"site"`
	OK   bool               `json:"ok"`
	Rows []ingressVerifyRow `json:"ingress"`
}

// verifySiteIngress is a read-only check of a site's ingress declarations: it
// reports what is well-formed, what is bound, whether the published hosts
// resolve, and whether the bound spoke Traefik answers over WireGuard. The
// operator-managed WAN port-forward and DNS zone cannot be checked from the hub
// and are reported as such.
func (s *Server) verifySiteIngress(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName string) {
	var body struct {
		ID string `json:"id"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	wantID := strings.TrimSpace(body.ID)

	var fs fleetv1alpha1.FleetSite
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: siteName}, &fs); err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "site not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fs.Spec.NormalizeIngress()

	if wantID != "" {
		if _, ok := fs.Spec.IngressByID(wantID); !ok {
			http.Error(w, fmt.Sprintf("ingress %q not found on site %s", wantID, siteName), http.StatusNotFound)
			return
		}
	}

	edge, err := loadEdgeClusters(ctx, s.Client, s.Cfg.EdgeClustersCMNamespace, s.Cfg.EdgeClustersCMName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	routes := s.loadSiteRoutes(ctx)
	assignedByIngress := assignedClustersByIngress(&fs, edge)

	resp := ingressVerifyResponse{Site: siteName, OK: true}
	for _, ent := range fs.Spec.Edge.Ingress {
		if wantID != "" && ent.ID != wantID {
			continue
		}
		row := s.verifyIngressRow(ctx, &fs, ent, assignedByIngress[ent.ID], edge, routes)
		if !row.OK {
			resp.OK = false
		}
		resp.Rows = append(resp.Rows, row)
	}
	if len(resp.Rows) == 0 {
		http.Error(w, "site has no ingress entries", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// assignedClustersByIngress maps an ingress id to the clusters bound to it,
// resolving a cluster that left ingressId unset to the site's sole port_forward
// entry (the same implicit binding the route and edge-cluster code use).
func assignedClustersByIngress(fs *fleetv1alpha1.FleetSite, edge edgeClustersFile) map[string][]string {
	out := map[string][]string{}
	siteID := fs.Spec.EffectiveSiteID(fs.Name)
	auto := fs.Spec.AutoIngressID()
	for name, ent := range edge.Clusters {
		if ent.Management || ent.ReadOnly {
			continue
		}
		if ent.Site != "" && ent.Site != siteID {
			continue
		}
		id := strings.TrimSpace(ent.IngressID)
		if id == "" {
			id = auto
		}
		if id == "" {
			continue
		}
		out[id] = append(out[id], name)
	}
	for id := range out {
		sort.Strings(out[id])
	}
	return out
}

func (s *Server) loadSiteRoutes(ctx context.Context) []fleetedge.FleetAppRoute {
	ns := s.Cfg.EdgeRoutesNamespace
	if ns == "" {
		ns = "fleet-system"
	}
	var list fleetedge.FleetAppRouteList
	if err := s.Client.List(ctx, &list, client.InNamespace(ns)); err != nil {
		return nil
	}
	return list.Items
}

func (s *Server) verifyIngressRow(ctx context.Context, fs *fleetv1alpha1.FleetSite, ent fleetv1alpha1.FleetSiteIngressSpec, assigned []string, edge edgeClustersFile, routes []fleetedge.FleetAppRoute) ingressVerifyRow {
	mode := fs.Spec.EffectiveWanAddressMode(ent)
	row := ingressVerifyRow{
		ID:               ent.ID,
		Method:           ent.Method,
		Address:          strings.TrimSpace(ent.Address),
		WanAddressMode:   mode,
		Zone:             strings.TrimSpace(ent.Zone),
		Wildcard:         strings.TrimSpace(ent.EffectiveWildcard()),
		AssignedClusters: assigned,
	}
	row.Checks = append(row.Checks,
		verifyIngressShape(ent, mode),
		verifyIngressWildcard(ent, fs.Spec.Edge.Ingress),
		verifyIngressBinding(ent, assigned),
		s.verifyIngressRoutes(ctx, ent, mode, assigned, routes),
		verifyIngressUpstream(ctx, assigned, edge),
		ingressVerifyCheck{
			Name:   "operator",
			Status: ingressVerifySkip,
			Detail: "WAN DNS and the router port-forward (80→" + fmt.Sprint(ingressVerifyHTTPPort) +
				", 443→" + fmt.Sprint(ingressVerifyHTTPSPort) + ") are operator-managed and cannot be checked from the hub",
		},
	)
	row.OK = true
	for _, c := range row.Checks {
		if c.Status == ingressVerifyFail {
			row.OK = false
		}
	}
	return row
}

func verifyIngressShape(ent fleetv1alpha1.FleetSiteIngressSpec, mode string) ingressVerifyCheck {
	if ent.Method != fleetv1alpha1.IngressMethodPortForward {
		return ingressVerifyCheck{Name: "shape", Status: ingressVerifyOK, Detail: "reverse_proxy entry (external TLS, operator-managed vhosts)"}
	}
	if strings.TrimSpace(ent.Zone) == "" {
		return ingressVerifyCheck{Name: "shape", Status: ingressVerifyFail, Detail: "zone is required for port_forward"}
	}
	if mode == fleetv1alpha1.WanAddressModeFixed && strings.TrimSpace(ent.Address) == "" {
		return ingressVerifyCheck{Name: "shape", Status: ingressVerifyFail, Detail: "address is required for port_forward with wanAddressMode fixed"}
	}
	if mode == fleetv1alpha1.WanAddressModeDNS {
		return ingressVerifyCheck{Name: "shape", Status: ingressVerifyOK, Detail: "port_forward, wanAddressMode dns (DNS-only WAN validation)"}
	}
	return ingressVerifyCheck{Name: "shape", Status: ingressVerifyOK, Detail: "port_forward, wanAddressMode fixed"}
}

func verifyIngressWildcard(ent fleetv1alpha1.FleetSiteIngressSpec, all []fleetv1alpha1.FleetSiteIngressSpec) ingressVerifyCheck {
	wildcard := strings.ToLower(strings.TrimSpace(ent.EffectiveWildcard()))
	if ent.Method != fleetv1alpha1.IngressMethodPortForward {
		if wildcard == "" {
			return ingressVerifyCheck{Name: "wildcard", Status: ingressVerifySkip, Detail: "reverse_proxy has no zone or wildcard; route hosts are not constrained"}
		}
		return ingressVerifyCheck{Name: "wildcard", Status: ingressVerifyOK, Detail: "route hosts must fall under " + wildcard}
	}
	if wildcard == "" {
		return ingressVerifyCheck{Name: "wildcard", Status: ingressVerifyFail, Detail: "port_forward has no effective wildcard (set zone or wildcard)"}
	}
	for _, other := range all {
		if other.ID == ent.ID || other.Method != fleetv1alpha1.IngressMethodPortForward {
			continue
		}
		if strings.ToLower(strings.TrimSpace(other.EffectiveWildcard())) == wildcard {
			return ingressVerifyCheck{Name: "wildcard", Status: ingressVerifyFail,
				Detail: fmt.Sprintf("wildcard %s is also claimed by ingress %q", wildcard, other.ID)}
		}
	}
	return ingressVerifyCheck{Name: "wildcard", Status: ingressVerifyOK, Detail: "unique wildcard " + wildcard}
}

func verifyIngressBinding(ent fleetv1alpha1.FleetSiteIngressSpec, assigned []string) ingressVerifyCheck {
	if ent.Method == fleetv1alpha1.IngressMethodPortForward {
		switch len(assigned) {
		case 0:
			return ingressVerifyCheck{Name: "binding", Status: ingressVerifyWarn, Detail: "no cluster bound; the entry is declared but unused"}
		case 1:
			return ingressVerifyCheck{Name: "binding", Status: ingressVerifyOK, Detail: "bound to " + assigned[0]}
		default:
			return ingressVerifyCheck{Name: "binding", Status: ingressVerifyFail,
				Detail: "port_forward allows one cluster per entry; bound to " + strings.Join(assigned, ", ")}
		}
	}
	if len(assigned) == 0 {
		return ingressVerifyCheck{Name: "binding", Status: ingressVerifyWarn, Detail: "no cluster bound"}
	}
	return ingressVerifyCheck{Name: "binding", Status: ingressVerifyOK, Detail: "bound to " + strings.Join(assigned, ", ")}
}

// verifyIngressRoutes checks the site-mode routes published through this entry:
// every host must fall under the wildcard, must resolve, and (in fixed mode)
// must resolve to the entry address.
func (s *Server) verifyIngressRoutes(ctx context.Context, ent fleetv1alpha1.FleetSiteIngressSpec, mode string, assigned []string, routes []fleetedge.FleetAppRoute) ingressVerifyCheck {
	bound := map[string]struct{}{}
	for _, name := range assigned {
		bound[name] = struct{}{}
	}
	hosts := []string{}
	for i := range routes {
		rt := &routes[i]
		if !strings.EqualFold(strings.TrimSpace(rt.Spec.Mode), "site") {
			continue
		}
		if _, ok := bound[rt.Spec.Cluster]; !ok {
			continue
		}
		if h := strings.TrimSpace(rt.Spec.SiteHost); h != "" {
			hosts = append(hosts, h)
		}
	}
	if len(hosts) == 0 {
		return ingressVerifyCheck{Name: "routes", Status: ingressVerifySkip, Detail: "no site routes published through this entry"}
	}
	sort.Strings(hosts)
	wildcard := strings.TrimSpace(ent.EffectiveWildcard())
	details := []string{}
	status := ingressVerifyOK
	for _, h := range hosts {
		if wildcard != "" && !ingress.HostMatchesWildcard(h, wildcard) {
			status = ingressVerifyFail
			details = append(details, fmt.Sprintf("%s: outside wildcard %s", h, wildcard))
			continue
		}
		probeCtx, cancel := context.WithTimeout(ctx, ingressVerifyProbeTimeout)
		ips, err := ingressVerifyLookupHost(probeCtx, h)
		cancel()
		if err != nil {
			if status == ingressVerifyOK {
				status = ingressVerifyWarn
			}
			details = append(details, fmt.Sprintf("%s: DNS %v", h, err))
			continue
		}
		if mode == fleetv1alpha1.WanAddressModeFixed {
			addr := strings.TrimSpace(ent.Address)
			if addr != "" && !containsString(ips, addr) {
				status = ingressVerifyFail
				details = append(details, fmt.Sprintf("%s: resolves to %s, expected %s", h, strings.Join(ips, ","), addr))
				continue
			}
		}
		details = append(details, fmt.Sprintf("%s → %s", h, strings.Join(ips, ",")))
	}
	return ingressVerifyCheck{Name: "routes", Status: status, Detail: strings.Join(details, "; ")}
}

// verifyIngressUpstream dials the bound clusters' spoke Traefik NodePort over
// the WireGuard address, which is the part of the path the hub can reach.
func verifyIngressUpstream(ctx context.Context, assigned []string, edge edgeClustersFile) ingressVerifyCheck {
	if len(assigned) == 0 {
		return ingressVerifyCheck{Name: "upstream", Status: ingressVerifySkip, Detail: "no cluster bound"}
	}
	details := []string{}
	status := ingressVerifyOK
	for _, name := range assigned {
		ent, ok := edge.Clusters[name]
		if !ok {
			status = ingressVerifyFail
			details = append(details, name+": not in the edge cluster registry")
			continue
		}
		host := strings.TrimSpace(ent.UpstreamHost)
		if host == "" {
			status = ingressVerifyFail
			details = append(details, name+": registry has no upstream_host")
			continue
		}
		port := ent.TraefikHTTPPort
		if port <= 0 {
			port = ingressVerifyHTTPPort
		}
		addr := net.JoinHostPort(host, fmt.Sprint(port))
		probeCtx, cancel := context.WithTimeout(ctx, ingressVerifyProbeTimeout)
		err := ingressVerifyDial(probeCtx, addr)
		cancel()
		if err != nil {
			status = ingressVerifyFail
			details = append(details, fmt.Sprintf("%s: spoke Traefik %s unreachable (%v)", name, addr, err))
			continue
		}
		details = append(details, fmt.Sprintf("%s: spoke Traefik %s reachable", name, addr))
	}
	return ingressVerifyCheck{Name: "upstream", Status: status, Detail: strings.Join(details, "; ")}
}

func containsString(list []string, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	for _, v := range list {
		if strings.ToLower(strings.TrimSpace(v)) == want {
			return true
		}
	}
	return false
}
