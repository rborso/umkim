package httpapi

import (
	"net/http"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/tenancy"
)

func validateSiteCreate(actor FleetActor, fs *fleetv1alpha1.FleetSite) error {
	if actor.isPlatformAdmin() && fs.Spec.OrgId == "" {
		return nil
	}
	if fs.Spec.OrgId == "" {
		return errBadRequest("orgId is required")
	}
	if !actor.hasOrg(fs.Spec.OrgId) {
		return errForbidden("forbidden org")
	}
	name := fs.Name
	if name == "" {
		name = fs.Spec.SiteName
	}
	return tenancy.ValidateOrgPrefixedName(fs.Spec.OrgId, name)
}

func validateClusterCreate(actor FleetActor, site *fleetv1alpha1.FleetSite, clusterName string, labels map[string]string) (map[string]string, error) {
	orgID := site.Spec.OrgId
	if orgID == "" && !actor.isPlatformAdmin() {
		return nil, errBadRequest("site orgId is required for customer clusters")
	}
	if orgID != "" {
		if !actor.hasOrg(orgID) {
			return nil, errForbidden("forbidden org")
		}
		if err := tenancy.ValidateOrgPrefixedName(orgID, clusterName); err != nil {
			return nil, errBadRequest(err.Error())
		}
	}
	out := labels
	if out == nil {
		out = map[string]string{}
	}
	if !actor.isPlatformAdmin() {
		if out[tenancy.LabelPool] == tenancy.PoolShared {
			return nil, errForbidden("customers cannot create shared pool clusters")
		}
		out = tenancy.EnsureDedicatedPool(out)
	}
	if orgID != "" {
		out = tenancy.SetOrgLabel(out, orgID)
	}
	return out, nil
}

func validateSiteDelete(actor FleetActor, fs *fleetv1alpha1.FleetSite) error {
	if actor.isPlatformAdmin() {
		return nil
	}
	if fs.Spec.OrgId == "" {
		return errForbidden("forbidden")
	}
	if !actor.hasOrg(fs.Spec.OrgId) {
		return errForbidden("forbidden org")
	}
	return nil
}

func validateClusterDelete(actor FleetActor, labels map[string]string) error {
	if actor.isPlatformAdmin() {
		return nil
	}
	if labels == nil {
		return errForbidden("forbidden")
	}
	if labels[tenancy.LabelPool] == tenancy.PoolShared {
		return errForbidden("shared pool clusters cannot be deleted by org users")
	}
	orgID := labels[tenancy.LabelOrg]
	if orgID == "" {
		return errForbidden("forbidden")
	}
	if !actor.hasOrg(orgID) {
		return errForbidden("forbidden org")
	}
	return nil
}

type httpError struct {
	msg  string
	code int
}

func (e *httpError) Error() string { return e.msg }

func errBadRequest(msg string) error {
	return &httpError{msg: msg, code: http.StatusBadRequest}
}

func errForbidden(msg string) error {
	return &httpError{msg: msg, code: http.StatusForbidden}
}

func writeTenancyError(w http.ResponseWriter, err error) bool {
	if he, ok := err.(*httpError); ok {
		http.Error(w, he.msg, he.code)
		return true
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return true
	}
	return false
}
