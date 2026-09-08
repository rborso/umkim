package tofu

import (
	"encoding/json"
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

const SchemaVersion = 1

// Catalog holds OpenTofu workspace-level inputs (skeleton).
type Catalog struct {
	Backend   map[string]interface{} `json:"backend,omitempty"`
	Providers map[string]interface{} `json:"providers,omitempty"`
	Defaults  map[string]interface{} `json:"defaults,omitempty"`
}

// GuestStore maps host name to tfvars fragment.
type GuestStore map[string]GuestSpec

// GuestSpec is per-host OpenTofu variables (skeleton).
type GuestSpec struct {
	Module     string                 `json:"module,omitempty"`
	Variables  map[string]interface{} `json:"variables,omitempty"`
	Absent     bool                   `json:"absent,omitempty"`
}

const provider = fleetv1alpha1.ProvisionProviderOpenTofu

// DecodeCatalog parses catalog.json for tofu connector.
func DecodeCatalog(raw string) (Catalog, error) {
	if strings.TrimSpace(raw) == "" {
		return Catalog{}, nil
	}
	var c Catalog
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return Catalog{}, fmt.Errorf("invalid catalog.json: %w", err)
	}
	return c, nil
}

// DecodeGuestStore parses guests.json for tofu connector.
func DecodeGuestStore(raw string) (GuestStore, error) {
	if strings.TrimSpace(raw) == "" {
		return GuestStore{}, nil
	}
	var store GuestStore
	if err := json.Unmarshal([]byte(raw), &store); err != nil {
		return GuestStore{}, fmt.Errorf("invalid guests.json: %w", err)
	}
	return store, nil
}
