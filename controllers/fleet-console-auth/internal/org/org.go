package org

import (
	"fmt"
	"regexp"
	"strings"
)

// SlugPattern matches admin-assigned org slugs (DNS-label segment).
var SlugPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,30}[a-z0-9]$`)

var reservedSlugs = map[string]struct{}{
	"management": {}, "umkim": {}, "fleet": {}, "kube-system": {},
	"default": {}, "system": {}, "admin": {}, "platform": {},
}

// ValidateSlug returns an error when slug is invalid or reserved.
func ValidateSlug(slug string) error {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return fmt.Errorf("org slug is required")
	}
	if !SlugPattern.MatchString(slug) {
		return fmt.Errorf("org slug must match %s", SlugPattern.String())
	}
	if _, ok := reservedSlugs[slug]; ok {
		return fmt.Errorf("org slug %q is reserved", slug)
	}
	return nil
}

// NameHasOrgPrefix reports whether name starts with orgSlug followed by '-'.
func NameHasOrgPrefix(orgSlug, name string) bool {
	orgSlug = strings.TrimSpace(orgSlug)
	name = strings.TrimSpace(name)
	if orgSlug == "" || name == "" {
		return false
	}
	prefix := orgSlug + "-"
	return strings.HasPrefix(name, prefix) && len(name) > len(prefix)
}

// ValidateOrgPrefixedName ensures a site or cluster name uses the org slug prefix.
func ValidateOrgPrefixedName(orgSlug, name string) error {
	if err := ValidateSlug(orgSlug); err != nil {
		return err
	}
	if !NameHasOrgPrefix(orgSlug, name) {
		return fmt.Errorf("name %q must start with %q", name, orgSlug+"-")
	}
	rest := strings.TrimPrefix(name, orgSlug+"-")
	if rest == "" {
		return fmt.Errorf("name %q must have a suffix after org prefix", name)
	}
	namePattern := regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	if !namePattern.MatchString(rest) {
		return fmt.Errorf("name suffix %q must be DNS-1123 compliant", rest)
	}
	return nil
}
