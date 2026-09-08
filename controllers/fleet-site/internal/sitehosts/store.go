package sitehosts

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const cmLabelApp = "fleet-site-host-inventory"

var hostNamePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

func cmName(siteName string) string {
	return "fleet-site-hosts-" + siteName
}

// List returns all host records for a site.
func List(ctx context.Context, c client.Client, ns, siteName string) ([]fleetv1alpha1.SiteHostRecord, error) {
	var cm corev1.ConfigMap
	err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: cmName(siteName)}, &cm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return decodeHosts(cm.Data["hosts.json"])
}

// Get returns one host record by name.
func Get(ctx context.Context, c client.Client, ns, siteName, hostName string) (fleetv1alpha1.SiteHostRecord, bool, error) {
	hosts, err := List(ctx, c, ns, siteName)
	if err != nil {
		return fleetv1alpha1.SiteHostRecord{}, false, err
	}
	for _, h := range hosts {
		if h.Name == hostName {
			return h, true, nil
		}
	}
	return fleetv1alpha1.SiteHostRecord{}, false, nil
}

// Put creates or replaces a host record.
func Put(ctx context.Context, c client.Client, ns, siteName string, rec fleetv1alpha1.SiteHostRecord) error {
	if err := ValidateRecord(rec, false); err != nil {
		return err
	}
	if err := ValidateClusterIntent(rec.ClusterIntent); err != nil {
		return err
	}
	return patchHosts(ctx, c, ns, siteName, func(hosts []fleetv1alpha1.SiteHostRecord) ([]fleetv1alpha1.SiteHostRecord, error) {
		found := false
		for i := range hosts {
			if hosts[i].Name == rec.Name {
				if rec.Status.Phase == "" && hosts[i].Status.Phase != "" {
					rec.Status = hosts[i].Status
				} else if rec.Status.Cluster == nil && hosts[i].Status.Cluster != nil {
					rec.Status.Cluster = hosts[i].Status.Cluster
				}
				hosts[i] = rec
				found = true
				break
			}
		}
		if !found {
			if rec.Status.Phase == "" {
				rec.Status.Phase = fleetv1alpha1.SiteHostPhaseDeclared
			}
			hosts = append(hosts, rec)
		}
		return hosts, nil
	})
}

// UpdateClusterStatus patches cluster handoff status on a host.
func UpdateClusterStatus(ctx context.Context, c client.Client, ns, siteName, hostName string, st fleetv1alpha1.SiteHostClusterStatus) error {
	return patchHosts(ctx, c, ns, siteName, func(hosts []fleetv1alpha1.SiteHostRecord) ([]fleetv1alpha1.SiteHostRecord, error) {
		for i := range hosts {
			if hosts[i].Name != hostName {
				continue
			}
			if hosts[i].Status.Cluster == nil {
				hosts[i].Status.Cluster = &fleetv1alpha1.SiteHostClusterStatus{}
			}
			if st.Phase != "" {
				hosts[i].Status.Cluster.Phase = st.Phase
			}
			if st.Message != "" {
				hosts[i].Status.Cluster.Message = st.Message
			}
			if st.ClusterName != "" {
				hosts[i].Status.Cluster.ClusterName = st.ClusterName
			}
			if st.FCJName != "" {
				hosts[i].Status.Cluster.FCJName = st.FCJName
			}
			if st.Mode != "" {
				hosts[i].Status.Cluster.Mode = st.Mode
			}
			return hosts, nil
		}
		return hosts, fmt.Errorf("host %q not found", hostName)
	})
}

// Delete removes a host record.
func Delete(ctx context.Context, c client.Client, ns, siteName, hostName string) error {
	return patchHosts(ctx, c, ns, siteName, func(hosts []fleetv1alpha1.SiteHostRecord) ([]fleetv1alpha1.SiteHostRecord, error) {
		out := hosts[:0]
		found := false
		for _, h := range hosts {
			if h.Name == hostName {
				found = true
				continue
			}
			out = append(out, h)
		}
		if !found {
			return hosts, fmt.Errorf("host %q not found", hostName)
		}
		return out, nil
	})
}

// UpdateStatus patches status fields on an existing host.
func UpdateStatus(ctx context.Context, c client.Client, ns, siteName, hostName string, st fleetv1alpha1.SiteHostStatus) error {
	return patchHosts(ctx, c, ns, siteName, func(hosts []fleetv1alpha1.SiteHostRecord) ([]fleetv1alpha1.SiteHostRecord, error) {
		for i := range hosts {
			if hosts[i].Name != hostName {
				continue
			}
			if st.Phase != "" {
				hosts[i].Status.Phase = st.Phase
			}
			if st.Message != "" {
				hosts[i].Status.Message = st.Message
			}
			if st.LastProvisionID != "" {
				hosts[i].Status.LastProvisionID = st.LastProvisionID
			}
			if st.AnsibleHost != "" {
				hosts[i].AnsibleHost = st.AnsibleHost
			}
			if st.Cluster != nil {
				hosts[i].Status.Cluster = st.Cluster
			}
			return hosts, nil
		}
		return hosts, fmt.Errorf("host %q not found", hostName)
	})
}

// UpdateVmLocation patches vm_location on an existing VM host record.
func UpdateVmLocation(ctx context.Context, c client.Client, ns, siteName, hostName, vmLocation string) error {
	vmLocation = strings.TrimSpace(vmLocation)
	if vmLocation == "" {
		return nil
	}
	if err := ValidateVmLocation(fleetv1alpha1.SiteHostKindVM, vmLocation); err != nil {
		return err
	}
	return patchHosts(ctx, c, ns, siteName, func(hosts []fleetv1alpha1.SiteHostRecord) ([]fleetv1alpha1.SiteHostRecord, error) {
		for i := range hosts {
			if hosts[i].Name != hostName {
				continue
			}
			if hosts[i].Kind != fleetv1alpha1.SiteHostKindVM {
				return hosts, fmt.Errorf("host %q is not a VM", hostName)
			}
			hosts[i].VmLocation = vmLocation
			return hosts, nil
		}
		return hosts, fmt.Errorf("host %q not found", hostName)
	})
}

func patchHosts(ctx context.Context, c client.Client, ns, siteName string, mutate func([]fleetv1alpha1.SiteHostRecord) ([]fleetv1alpha1.SiteHostRecord, error)) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		hosts, err := List(ctx, c, ns, siteName)
		if err != nil {
			return err
		}
		updated, err := mutate(hosts)
		if err != nil {
			return err
		}
		return writeHostsOnce(ctx, c, ns, siteName, updated)
	})
}

func writeHostsOnce(ctx context.Context, c client.Client, ns, siteName string, hosts []fleetv1alpha1.SiteHostRecord) error {
	raw, err := json.Marshal(hosts)
	if err != nil {
		return err
	}
	name := cmName(siteName)
	var existing corev1.ConfigMap
	err = c.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &existing)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
				Labels: map[string]string{
					"app":        cmLabelApp,
					"fleet-site": siteName,
				},
			},
			Data: map[string]string{"hosts.json": string(raw)},
		}
		return c.Create(ctx, cm)
	}
	existing.Data = map[string]string{"hosts.json": string(raw)}
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	existing.Labels["app"] = cmLabelApp
	existing.Labels["fleet-site"] = siteName
	return c.Update(ctx, &existing)
}

func decodeHosts(raw string) ([]fleetv1alpha1.SiteHostRecord, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var hosts []fleetv1alpha1.SiteHostRecord
	if err := json.Unmarshal([]byte(raw), &hosts); err != nil {
		return nil, fmt.Errorf("invalid hosts inventory: %w", err)
	}
	return hosts, nil
}

// ValidateRecord checks a host record before write.
func ValidateRecord(rec fleetv1alpha1.SiteHostRecord, partial bool) error {
	name := strings.TrimSpace(rec.Name)
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if len(name) > 63 || !hostNamePattern.MatchString(name) {
		return fmt.Errorf("invalid host name %q (lowercase DNS label)", name)
	}
	if !partial {
		kind := strings.TrimSpace(rec.Kind)
		if kind == "" {
			return fmt.Errorf("kind is required")
		}
		if kind != fleetv1alpha1.SiteHostKindVM && kind != fleetv1alpha1.SiteHostKindBareMetal {
			return fmt.Errorf("kind must be %q or %q", fleetv1alpha1.SiteHostKindVM, fleetv1alpha1.SiteHostKindBareMetal)
		}
	}
	if k := strings.TrimSpace(rec.Kind); k != "" && k != fleetv1alpha1.SiteHostKindVM && k != fleetv1alpha1.SiteHostKindBareMetal {
		return fmt.Errorf("kind must be %q or %q", fleetv1alpha1.SiteHostKindVM, fleetv1alpha1.SiteHostKindBareMetal)
	}
	return ValidateVmLocation(rec.Kind, rec.VmLocation)
}
