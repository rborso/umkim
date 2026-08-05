// Package clusterpick — reference implementation for policy-manager fleet-phase2 fork.
// Merge into internal/service/ (or equivalent) when building fleet/policy-manager:fleet-phase2.
//
// Apply: copy functions into placement evaluation after eligible clusters are matched
// by cluster_requirements labels.
package clusterpick

import "sort"

// ClusterCandidate is a placement target with OCM labels and derived provider name.
type ClusterCandidate struct {
	Name     string
	Site     string
	Provider string
	Labels   map[string]string
}

// PickSpread selects up to count clusters using spread strategy.
// When siteDiversity is true, at most one cluster per distinct fleet.site label is chosen.
func PickSpread(candidates []ClusterCandidate, count int, siteDiversity bool) ([]ClusterCandidate, error) {
	if count <= 0 || len(candidates) == 0 {
		return nil, nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Name < candidates[j].Name
	})
	if !siteDiversity {
		if len(candidates) < count {
			return candidates, errInsufficientClusters(count, len(candidates), 0)
		}
		return candidates[:count], nil
	}
	seenSite := map[string]bool{}
	out := make([]ClusterCandidate, 0, count)
	for _, c := range candidates {
		site := c.Site
		if site == "" {
			site = "_unknown"
		}
		if seenSite[site] {
			continue
		}
		seenSite[site] = true
		out = append(out, c)
		if len(out) >= count {
			return out, nil
		}
	}
	return out, errInsufficientClusters(count, len(out), len(seenSite))
}

type insufficientClustersError struct {
	want, got, distinctSites int
}

func (e insufficientClustersError) Error() string {
	return "insufficient distinct sites for placement"
}

func errInsufficientClusters(want, got, distinctSites int) error {
	return insufficientClustersError{want: want, got: got, distinctSites: distinctSites}
}

// SiteFromLabels reads fleet.site from OCM ManagedCluster labels.
func SiteFromLabels(labels map[string]string) string {
	if labels == nil {
		return ""
	}
	return labels["fleet.site"]
}
