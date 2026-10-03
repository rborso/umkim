package routesync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/config"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/hostnames"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type catalogList struct {
	Results []catalogInstance `json:"results"`
}

type catalogInstance struct {
	UID         string   `json:"uid"`
	ID          string   `json:"id"`
	DisplayName string   `json:"display_name"`
	ResourceID  string   `json:"resource_id"`
	Spec        instSpec `json:"spec"`
}

type instSpec struct {
	CatalogItemID string      `json:"catalog_item_id"`
	ResourceIDs   []string    `json:"resource_ids"`
	UserValues    []userValue `json:"user_values"`
}

type userValue struct {
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}

type Syncer struct {
	Client client.Client
	Config config.Config
	HTTP   *http.Client
}

func (s *Syncer) RunOnce(ctx context.Context) error {
	if strings.TrimSpace(s.Config.CatalogAPIURL) == "" {
		return nil
	}
	logger := log.FromContext(ctx)
	instances, err := s.fetchInstances(ctx)
	if err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, inst := range instances {
		if instanceResourceID(inst) == "" {
			continue
		}
		id := inst.UID
		if id == "" {
			id = inst.ID
		}
		if id == "" {
			continue
		}
		seen[id] = struct{}{}
		if err := s.syncInstance(ctx, inst); err != nil {
			logger.Error(err, "sync instance", "id", id)
		}
	}
	return s.prune(ctx, seen)
}

func (s *Syncer) fetchInstances(ctx context.Context) ([]catalogInstance, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.Config.CatalogAPIURL+"/catalog-item-instances", nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("catalog API %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var list catalogList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}
	return list.Results, nil
}

func (s *Syncer) syncInstance(ctx context.Context, inst catalogInstance) error {
	uv := map[string]string{}
	for _, v := range inst.Spec.UserValues {
		uv[v.Path] = userValueAsString(v)
	}
	mode := uv["routing.mode"]
	if mode == "" {
		mode = fleetv1alpha1.RouteModeHub
	}
	if mode == fleetv1alpha1.RouteModeNone {
		return s.deleteSyncedRoute(ctx, inst)
	}
	cluster := uv["routing.cluster"]
	if inst.UID != "" {
		if fromPlacement := s.primaryFromPlacementTargets(ctx, inst.UID); fromPlacement != "" {
			cluster = fromPlacement
		}
	}
	if cluster == "" && inst.UID != "" {
		cluster = s.primaryFromPlacementTargets(ctx, inst.UID)
	}
	if cluster == "" {
		log.FromContext(ctx).Info(
			"route sync skipped: no cluster from routing.cluster or placement targets",
			"instanceID", inst.UID,
			"resourceID", instanceResourceID(inst),
		)
		return s.deleteSyncedRoute(ctx, inst)
	}
	standbyCluster := uv["routing.standby_cluster"]
	if inst.UID != "" {
		if fromPlacement := s.standbyFromPlacementTargets(ctx, inst.UID); fromPlacement != "" {
			standbyCluster = fromPlacement
		}
	}
	if standbyCluster == "" && inst.UID != "" {
		standbyCluster = s.standbyFromPlacementTargets(ctx, inst.UID)
	}
	healthPath := uv["routing.health_path"]
	if healthPath == "" {
		healthPath = "/"
	}
	overrideSvcName := strings.TrimSpace(uv["routing.service_name"])
	overrideSvcPort := int32(0)
	if p := strings.TrimSpace(uv["routing.service_port"]); p != "" {
		if n, err := strconv.ParseInt(p, 10, 32); err == nil && n > 0 {
			overrideSvcPort = int32(n)
		}
	}
	failoverMode := uv["routing.failover_mode"]
	if failoverMode == "" {
		failoverMode = uv["failover.mode"]
	}
	if failoverMode == "" && standbyCluster != "" {
		failoverMode = fleetv1alpha1.FailoverModeStandby
	}
	appName := uv["metadata.name"]
	if appName == "" {
		appName = inst.DisplayName
	}
	appName = dnsLabel(appName)
	hubHost := strings.TrimSpace(uv["routing.hub_host"])
	if hubHost == "" {
		hubHost = fmt.Sprintf("%s.%s", appName, s.Config.FleetEdgeDomain)
	}
	siteHost := uv["routing.site_host"]
	svcName := appName
	if lbl := uv["metadata.labels.app"]; lbl != "" {
		svcName = dnsLabel(lbl)
	}
	ns := "default"
	if releaseNS := strings.TrimSpace(uv["release.namespace"]); releaseNS != "" {
		ns = releaseNS
	}
	port := int32(80)
	resourceID := instanceResourceID(inst)
	clusterTargets := s.resolveTargetServices(ctx, inst.UID, resourceID, cluster, standbyCluster, appName, ns, port)
	sortClusterTargets(clusterTargets)
	if len(clusterTargets) > 0 {
		svcName = clusterTargets[0].Service.Name
		ns = clusterTargets[0].Service.Namespace
		port = clusterTargets[0].Service.Port
	} else if resourceID != "" && inst.UID != "" {
		if provider := s.primaryProviderFromPlacementTargets(ctx, inst.UID); provider != "" {
			sprmID := resourceID
			if id := s.primarySprmIDFromPlacementTargets(ctx, inst.UID); id != "" {
				sprmID = id
			}
			if workload, ok := s.workloadServiceFromProvider(ctx, provider, sprmID, appName, ns); ok {
				if workload.Name != "" {
					svcName = workload.Name
				}
				if workload.Namespace != "" {
					ns = workload.Namespace
				}
				port = workload.Port
			}
		}
	}
	if overrideSvcName != "" {
		svcName = overrideSvcName
		if len(clusterTargets) > 0 {
			clusterTargets[0].Service.Name = overrideSvcName
		}
	}
	if overrideSvcPort > 0 {
		port = overrideSvcPort
		if len(clusterTargets) > 0 {
			clusterTargets[0].Service.Port = overrideSvcPort
		}
	}

	route := &fleetv1alpha1.FleetAppRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "route-" + dnsLabel(idShort(inst.UID, inst.ID)),
			Namespace: s.Config.RouteNamespace,
			Labels: map[string]string{
				"edge.umkim.com/instance-id": inst.UID,
				"edge.umkim.com/synced":      "true",
			},
		},
		Spec: fleetv1alpha1.FleetAppRouteSpec{
			InstanceID:     inst.UID,
			CatalogItemID:  inst.Spec.CatalogItemID,
			Cluster:        cluster,
			StandbyCluster: standbyCluster,
			ClusterTargets: clusterTargets,
			Mode:           mode,
			HubHost:        hubHost,
			SiteHost:       siteHost,
			HealthPath:     healthPath,
			FailoverMode:   failoverMode,
			RequireAuth:    parseRequireAuth(uv, mode),
			Service: fleetv1alpha1.ServiceRef{
				Namespace: ns,
				Name:      svcName,
				Port:      port,
			},
		},
	}
	if route.Annotations == nil {
		route.Annotations = map[string]string{}
	}
	if resourceID != "" {
		route.Annotations["fleet.umkim.com/resource-id"] = resourceID
	}
	for _, h := range []string{hubHost, siteHost} {
		if h == "" {
			continue
		}
		if err := hostnames.FindDuplicate(ctx, s.Client, route.Namespace, h, route.Name); err != nil {
			return err
		}
	}

	existing := &fleetv1alpha1.FleetAppRoute{}
	err := s.Client.Get(ctx, types.NamespacedName{Name: route.Name, Namespace: route.Namespace}, existing)
	if client.IgnoreNotFound(err) != nil {
		return err
	}
	if err != nil {
		return s.Client.Create(ctx, route)
	}
	preserveEffectiveMode := existing.Spec.EffectiveMode
	desired := route.Spec
	sortClusterTargets(desired.ClusterTargets)
	if preserveEffectiveMode != "" {
		desired.EffectiveMode = preserveEffectiveMode
	}
	if desired.SiteHost == "" && existing.Spec.SiteHost != "" {
		desired.SiteHost = existing.Spec.SiteHost
	}
	existingSpec := existing.Spec
	sortClusterTargets(existingSpec.ClusterTargets)
	if reflect.DeepEqual(existingSpec, desired) {
		return nil
	}
	existing.Spec = desired
	return s.Client.Update(ctx, existing)
}

func sortClusterTargets(targets []fleetv1alpha1.ClusterTarget) {
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].Cluster != targets[j].Cluster {
			return targets[i].Cluster < targets[j].Cluster
		}
		return targets[i].Provider < targets[j].Provider
	})
}

func (s *Syncer) deleteSyncedRoute(ctx context.Context, inst catalogInstance) error {
	id := inst.UID
	if id == "" {
		id = inst.ID
	}
	if id == "" {
		return nil
	}
	var list fleetv1alpha1.FleetAppRouteList
	if err := s.Client.List(ctx, &list); err != nil {
		return err
	}
	for _, item := range list.Items {
		if item.Labels["edge.umkim.com/synced"] != "true" {
			continue
		}
		if item.Spec.InstanceID != id {
			continue
		}
		if err := s.Client.Delete(ctx, &item); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func (s *Syncer) prune(ctx context.Context, seen map[string]struct{}) error {
	var list fleetv1alpha1.FleetAppRouteList
	if err := s.Client.List(ctx, &list); err != nil {
		return err
	}
	for _, item := range list.Items {
		if item.Labels["edge.umkim.com/synced"] != "true" {
			continue
		}
		id := item.Spec.InstanceID
		if _, ok := seen[id]; !ok && id != "" {
			if err := s.Client.Delete(ctx, &item); err != nil {
				return err
			}
		}
	}
	return nil
}

// instanceResourceID returns the SPM resource id from legacy resource_id or spec.resource_ids[0].
func instanceResourceID(inst catalogInstance) string {
	if inst.ResourceID != "" {
		return inst.ResourceID
	}
	if len(inst.Spec.ResourceIDs) > 0 {
		return inst.Spec.ResourceIDs[0]
	}
	return ""
}

func dnsLabel(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else if r == '.' || r == '_' {
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "app"
	}
	return out
}

func idShort(uid, id string) string {
	if uid != "" {
		if len(uid) > 12 {
			return uid[:12]
		}
		return uid
	}
	return id
}

func NewHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

type placementTarget struct {
	Provider string `json:"provider"`
	Role     string `json:"role"`
	SprmID   string `json:"sprm_id"`
	Status   string `json:"status"`
}

type placementTargetsResp struct {
	Targets []placementTarget `json:"targets"`
}

func (s *Syncer) resolveTargetServices(ctx context.Context, instanceID, resourceID, primaryCluster, standbyCluster, appName, defaultNS string, defaultPort int32) []fleetv1alpha1.ClusterTarget {
	targets, ok := s.fetchPlacementTargets(ctx, instanceID)
	if !ok || len(targets) == 0 {
		return nil
	}
	var out []fleetv1alpha1.ClusterTarget
	for _, t := range targets {
		if t.Provider == "" || !placementTargetActive(t) {
			continue
		}
		clusterName := providerToCluster(t.Provider)
		if clusterName == "" {
			continue
		}
		sprmID := t.SprmID
		if sprmID == "" {
			sprmID = resourceID
		}
		svc := fleetv1alpha1.ServiceRef{Namespace: defaultNS, Name: appName, Port: defaultPort}
		if sprmID != "" {
			if workload, ok := s.workloadServiceFromProvider(ctx, t.Provider, sprmID, appName, defaultNS); ok {
				svc = workload
			}
		}
		role := t.Role
		if role == "" {
			switch {
			case clusterName == primaryCluster:
				role = "primary"
			case clusterName == standbyCluster:
				role = "standby"
			}
		}
		out = append(out, fleetv1alpha1.ClusterTarget{
			Cluster:  clusterName,
			Provider: t.Provider,
			Role:     role,
			SprmID:   sprmID,
			Service:  svc,
		})
	}
	return out
}

func (s *Syncer) primaryFromPlacementTargets(ctx context.Context, instanceID string) string {
	targets, ok := s.fetchPlacementTargets(ctx, instanceID)
	if !ok {
		return ""
	}
	for _, t := range targets {
		if t.Role == "primary" && t.Provider != "" && placementTargetActive(t) {
			return providerToCluster(t.Provider)
		}
	}
	for _, t := range targets {
		if t.Provider != "" && placementTargetActive(t) {
			return providerToCluster(t.Provider)
		}
	}
	return ""
}

func (s *Syncer) standbyFromPlacementTargets(ctx context.Context, instanceID string) string {
	targets, ok := s.fetchPlacementTargets(ctx, instanceID)
	if !ok {
		return ""
	}
	for _, t := range targets {
		if t.Role == "standby" && t.Provider != "" && placementTargetActive(t) {
			return providerToCluster(t.Provider)
		}
	}
	return ""
}

func (s *Syncer) fetchPlacementTargets(ctx context.Context, instanceID string) ([]placementTarget, bool) {
	url := s.Config.CatalogAPIURL + "/catalog-item-instances/" + instanceID + "/placement-targets"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false
	}
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	var body placementTargetsResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, false
	}
	return body.Targets, true
}

func providerToCluster(provider string) string {
	for _, prefix := range []string{"k8s-", "helm-", "olm-"} {
		if strings.HasPrefix(provider, prefix) {
			return strings.TrimPrefix(provider, prefix)
		}
	}
	return provider
}

func (s *Syncer) primaryProviderFromPlacementTargets(ctx context.Context, instanceID string) string {
	targets, ok := s.fetchPlacementTargets(ctx, instanceID)
	if !ok {
		return ""
	}
	for _, t := range targets {
		if t.Role == "primary" && t.Provider != "" && placementTargetActive(t) {
			return t.Provider
		}
	}
	for _, t := range targets {
		if t.Provider != "" && placementTargetActive(t) {
			return t.Provider
		}
	}
	return ""
}

func (s *Syncer) primarySprmIDFromPlacementTargets(ctx context.Context, instanceID string) string {
	targets, ok := s.fetchPlacementTargets(ctx, instanceID)
	if !ok {
		return ""
	}
	for _, t := range targets {
		if t.Role == "primary" && t.SprmID != "" && placementTargetActive(t) {
			return t.SprmID
		}
	}
	for _, t := range targets {
		if t.SprmID != "" && placementTargetActive(t) {
			return t.SprmID
		}
	}
	return ""
}

type providersListResp struct {
	Providers []struct {
		Name     string `json:"name"`
		Endpoint string `json:"endpoint"`
	} `json:"providers"`
}

type containerWorkloadResp struct {
	Service *struct {
		Name string `json:"name"`
	} `json:"service"`
	Spec struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
	} `json:"spec"`
}

func (s *Syncer) providerEndpoint(ctx context.Context, providerName string) (string, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.Config.CatalogAPIURL+"/providers", nil)
	if err != nil {
		return "", false
	}
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false
	}
	var body providersListResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", false
	}
	for _, p := range body.Providers {
		if p.Name == providerName && p.Endpoint != "" {
			return p.Endpoint, true
		}
	}
	return "", false
}
