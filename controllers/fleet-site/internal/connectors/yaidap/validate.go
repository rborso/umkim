package yaidap

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

var guestNamePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// ValidateGuestName checks DNS-label guest names.
func ValidateGuestName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("guest name is required")
	}
	if len(name) > 63 || !guestNamePattern.MatchString(name) {
		return fmt.Errorf("invalid guest name %q (lowercase DNS label)", name)
	}
	return nil
}

// ValidateCatalog performs basic structural validation.
func ValidateCatalog(c *Catalog) error {
	if c == nil {
		return nil
	}
	for i, n := range c.Networks {
		if strings.TrimSpace(n.Name) == "" {
			return fmt.Errorf("catalog.networks[%d].name is required", i)
		}
	}
	for i, s := range c.Storages {
		if strings.TrimSpace(s.Name) == "" {
			return fmt.Errorf("catalog.storages[%d].name is required", i)
		}
	}
	return nil
}

// ValidateGuestSpec validates one guest spec.
func ValidateGuestSpec(name string, g GuestSpec) error {
	if err := ValidateGuestName(name); err != nil {
		return err
	}
	if g.Absent {
		return nil
	}
	if ip := strings.TrimSpace(g.AnsibleHost); ip != "" {
		if net.ParseIP(ip) == nil {
			return fmt.Errorf("guest %q: invalid ansible_host %q", name, ip)
		}
	}
	if loc := strings.TrimSpace(g.VmLocation); loc != "" {
		lower := strings.ToLower(loc)
		if lower != "least-utilized" && lower != "least_utilized" && lower != "auto" {
			if err := ValidateGuestName(loc); err != nil {
				return fmt.Errorf("guest %q: invalid vm_location: %w", name, err)
			}
		}
	}
	if g.Memory < 0 || g.Cores < 0 {
		return fmt.Errorf("guest %q: memory and cores must be non-negative", name)
	}
	for i, d := range g.Disks {
		if strings.TrimSpace(d.Name) == "" {
			return fmt.Errorf("guest %q: disks[%d].name is required", name, i)
		}
		if d.Size < 0 {
			return fmt.Errorf("guest %q: disks[%d].size must be non-negative", name, i)
		}
	}
	return nil
}

// ValidateGuestStore validates all guests in the store.
func ValidateGuestStore(store GuestStore) error {
	for name, g := range store {
		if err := ValidateGuestSpec(name, g); err != nil {
			return err
		}
	}
	return nil
}

// DecodeCatalog parses catalog.json bytes.
func DecodeCatalog(raw string) (Catalog, error) {
	if strings.TrimSpace(raw) == "" {
		return Catalog{}, nil
	}
	var c Catalog
	if err := jsonUnmarshal([]byte(raw), &c); err != nil {
		return Catalog{}, fmt.Errorf("invalid catalog.json: %w", err)
	}
	if err := ValidateCatalog(&c); err != nil {
		return Catalog{}, err
	}
	return c, nil
}

// DecodeGuestStore parses guests.json bytes.
func DecodeGuestStore(raw string) (GuestStore, error) {
	if strings.TrimSpace(raw) == "" {
		return GuestStore{}, nil
	}
	var store GuestStore
	if err := jsonUnmarshal([]byte(raw), &store); err != nil {
		return GuestStore{}, fmt.Errorf("invalid guests.json: %w", err)
	}
	if err := ValidateGuestStore(store); err != nil {
		return GuestStore{}, err
	}
	return store, nil
}

func jsonUnmarshal(b []byte, v interface{}) error {
	return jsonDecode(b, v)
}
