package yaidap

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	"github.com/rborso/umkim/controllers/fleet-site/internal/sitehosts"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GuestStatus tracks hub-observed provision/cluster state for one guest.
type GuestStatus struct {
	Phase           string                         `json:"phase,omitempty"`
	Message         string                         `json:"message,omitempty"`
	LastProvisionID string                         `json:"lastProvisionID,omitempty"`
	AnsibleHost     string                         `json:"ansible_host,omitempty"`
	Cluster         *fleetv1alpha1.SiteHostClusterStatus `json:"cluster,omitempty"`
	Kind            string                         `json:"kind,omitempty"`
	VmLocation      string                         `json:"vm_location,omitempty"`
}

// InventoryStatusDoc is _meta/status.json.
type InventoryStatusDoc struct {
	Guests map[string]GuestStatus `json:"guests,omitempty"`
}

// InventoryHostView is a derived host row for API/UI.
type InventoryHostView struct {
	Name        string                              `json:"name"`
	External    bool                                `json:"external,omitempty"`
	AnsibleHost string                              `json:"ansible_host,omitempty"`
	VmLocation  string                              `json:"vm_location,omitempty"`
	Kind        string                              `json:"kind,omitempty"`
	Status      GuestStatus                         `json:"status,omitempty"`
	Clusters    []string                            `json:"clusters,omitempty"`
}

func loadStatusDoc(data map[string]string) InventoryStatusDoc {
	raw := connectors.StatusContent(data)
	var doc InventoryStatusDoc
	_ = json.Unmarshal([]byte(raw), &doc)
	if doc.Guests == nil {
		doc.Guests = map[string]GuestStatus{}
	}
	return doc
}

// LoadStatusDoc reads status from store.
func LoadStatusDoc(ctx context.Context, c client.Client, ns, siteName string) (InventoryStatusDoc, error) {
	data, err := connectors.Load(ctx, c, ns, siteName, provider)
	if err != nil {
		return InventoryStatusDoc{}, err
	}
	if data == nil {
		return InventoryStatusDoc{Guests: map[string]GuestStatus{}}, nil
	}
	return loadStatusDoc(data), nil
}

// UpdateGuestStatus merges one guest status entry.
func UpdateGuestStatus(ctx context.Context, c client.Client, ns, siteName string, guestName string, st GuestStatus) error {
	doc, err := LoadStatusDoc(ctx, c, ns, siteName)
	if err != nil {
		return err
	}
	if doc.Guests == nil {
		doc.Guests = map[string]GuestStatus{}
	}
	if existing, ok := doc.Guests[guestName]; ok {
		if st.Phase == "" {
			st.Phase = existing.Phase
		}
		if st.Message == "" {
			st.Message = existing.Message
		}
		if st.Cluster == nil {
			st.Cluster = existing.Cluster
		}
		if st.VmLocation == "" {
			st.VmLocation = existing.VmLocation
		}
		if st.LastProvisionID == "" {
			st.LastProvisionID = existing.LastProvisionID
		}
		if st.AnsibleHost == "" {
			st.AnsibleHost = existing.AnsibleHost
		}
		if st.Kind == "" {
			st.Kind = existing.Kind
		}
	}
	doc.Guests[guestName] = st
	raw, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	return connectors.PutStatusJSON(ctx, c, ns, siteName, provider, string(raw))
}

// ListInventoryHosts builds derived host views from guests.yml + status + clusters.
func ListInventoryHosts(ctx context.Context, c client.Client, ns, siteName string) ([]InventoryHostView, error) {
	data, err := connectors.Load(ctx, c, ns, siteName, provider)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	data = connectors.NormalizeMetaKeys(data)
	guestsRaw, _ := connectors.GetInventoryFile(data, "guests.yml")
	guests, err := ParseGuestsYAML(guestsRaw)
	if err != nil {
		return nil, err
	}
	bareMetalsRaw, _ := connectors.GetInventoryFile(data, "bare_metals.yml")
	bareMetals, err := ParseBareMetalsYAML(bareMetalsRaw)
	if err != nil {
		return nil, err
	}
	hypervisorsRaw, _ := connectors.GetInventoryFile(data, "hypervisors.yml")
	hypervisors, err := ParseHypervisorsYAML(hypervisorsRaw)
	if err != nil {
		return nil, err
	}
	clustersDoc := LoadClustersFromData(data)
	statusDoc := loadStatusDoc(data)
	memberClusters := map[string][]string{}
	for cname, spec := range clustersDoc.Clusters {
		for _, m := range spec.Members {
			memberClusters[m] = append(memberClusters[m], cname)
		}
	}
	var out []InventoryHostView
	addGuest := func(name string, entry GuestEntry, kind string, external bool) {
		st := statusDoc.Guests[name]
		if st.Phase == "" {
			st.Phase = fleetv1alpha1.SiteHostPhaseDeclared
		}
		ip := strings.TrimSpace(entry.AnsibleHost)
		if ip == "" {
			ip = strings.TrimSpace(st.AnsibleHost)
		}
		vmLoc := strings.TrimSpace(entry.VmLocation)
		if sitehosts.VmLocationIsScheduleToken(vmLoc) {
			if concrete := strings.TrimSpace(st.VmLocation); concrete != "" && !sitehosts.VmLocationIsScheduleToken(concrete) {
				vmLoc = concrete
			}
		}
		out = append(out, InventoryHostView{
			Name:        name,
			External:    external,
			AnsibleHost: ip,
			VmLocation:  vmLoc,
			Kind:        kind,
			Status:      st,
			Clusters:    memberClusters[name],
		})
	}
	for name, entry := range guests.Guests.Hosts {
		addGuest(name, entry, fleetv1alpha1.SiteHostKindVM, false)
	}
	for name, entry := range bareMetals.BareMetals.Hosts {
		addGuest(name, entry, fleetv1alpha1.SiteHostKindBareMetal, true)
	}
	// Read-compat: legacy external.hosts still in hub guests.yml until migrated.
	if len(bareMetals.BareMetals.Hosts) == 0 {
		for name, entry := range guests.External.Hosts {
			addGuest(name, entry, fleetv1alpha1.SiteHostKindBareMetal, true)
		}
	}
	for name, entry := range hypervisors.Hypervisors.Hosts {
		st := statusDoc.Guests[name]
		if st.Phase == "" {
			st.Phase = fleetv1alpha1.SiteHostPhaseDeclared
		}
		ip := strings.TrimSpace(entry.AnsibleHost)
		if ip == "" {
			ip = strings.TrimSpace(st.AnsibleHost)
		}
		out = append(out, InventoryHostView{
			Name:        name,
			AnsibleHost: ip,
			Kind:        fleetv1alpha1.SiteHostKindHypervisor,
			Status:      st,
			Clusters:    memberClusters[name],
		})
	}
	return out, nil
}

// LookupInventoryHost returns one derived host row by name.
func LookupInventoryHost(ctx context.Context, c client.Client, ns, siteName, guestName string) (InventoryHostView, bool, error) {
	hosts, err := ListInventoryHosts(ctx, c, ns, siteName)
	if err != nil {
		return InventoryHostView{}, false, err
	}
	for _, h := range hosts {
		if h.Name == guestName {
			return h, true, nil
		}
	}
	return InventoryHostView{}, false, nil
}

// GuestAddress resolves ansible_host for a guest name.
func GuestAddress(ctx context.Context, c client.Client, ns, siteName, guestName string) (string, error) {
	hosts, err := ListInventoryHosts(ctx, c, ns, siteName)
	if err != nil {
		return "", err
	}
	for _, h := range hosts {
		if h.Name == guestName {
			if ip := strings.TrimSpace(h.AnsibleHost); ip != "" {
				return ip, nil
			}
		}
	}
	return "", fmt.Errorf("guest %q has no ansible_host", guestName)
}

// GuestExists reports whether a host is listed in guests.yml, external, or hypervisors.yml.
func GuestExists(ctx context.Context, c client.Client, ns, siteName, guestName string) (bool, error) {
	data, err := connectors.Load(ctx, c, ns, siteName, provider)
	if err != nil {
		return false, err
	}
	if data == nil {
		return false, nil
	}
	data = connectors.NormalizeMetaKeys(data)
	raw, ok := connectors.GetInventoryFile(data, "guests.yml")
	if ok {
		guests, err := ParseGuestsYAML(raw)
		if err != nil {
			return false, err
		}
		if vmGuestInIndex(guestName, guests) {
			return true, nil
		}
		if guestInIndex(guestName, guests) {
			return true, nil
		}
	}
	bmRaw, ok := connectors.GetInventoryFile(data, "bare_metals.yml")
	if ok {
		bm, err := ParseBareMetalsYAML(bmRaw)
		if err != nil {
			return false, err
		}
		if bareMetalInIndex(guestName, bm) {
			return true, nil
		}
	}
	hvRaw, ok := connectors.GetInventoryFile(data, "hypervisors.yml")
	if ok {
		hvs, err := ParseHypervisorsYAML(hvRaw)
		if err != nil {
			return false, err
		}
		if hypervisorInIndex(guestName, hvs) {
			return true, nil
		}
	}
	return false, nil
}
