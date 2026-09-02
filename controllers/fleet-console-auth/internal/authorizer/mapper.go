package authorizer

import (
	"net/http"
	"strings"
)

type MappedRequest struct {
	Resource    string
	Verb        string
	ScopeKind   string
	ScopeName   string
	AccessLevel string
	Path        string
}

func MapRequest(r *http.Request, backend string) MappedRequest {
	path := r.URL.Path
	method := r.Method

	switch backend {
	case "fleet":
		return mapFleet(path, method)
	default:
		return MappedRequest{Resource: "unknown", Verb: methodToVerb(method)}
	}
}

func mapFleet(path, method string) MappedRequest {
	path = strings.TrimPrefix(path, "/proxy/fleet/")
	path = strings.TrimPrefix(path, "/api/v1alpha1/")
	path = strings.TrimPrefix(path, "api/v1alpha1/")
	path = strings.TrimPrefix(path, "v1alpha1/")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	verb := methodToVerb(method)

	if len(parts) == 0 || parts[0] == "" {
		return MappedRequest{Resource: "health", Verb: verb}
	}
	switch parts[0] {
	case "sites":
		if len(parts) == 1 {
			return MappedRequest{Resource: "sites", Verb: listVerb(method)}
		}
		site := parts[1]
		if len(parts) >= 3 {
			return MappedRequest{Resource: "sites", Verb: verb, ScopeKind: "Site", ScopeName: site}
		}
		return MappedRequest{Resource: "sites", Verb: verb, ScopeKind: "Site", ScopeName: site}
	case "clusters":
		if len(parts) == 1 {
			return MappedRequest{Resource: "clusters", Verb: listVerb(method)}
		}
		cluster := parts[1]
		return MappedRequest{Resource: "clusters", Verb: verb, ScopeKind: "Cluster", ScopeName: cluster}
	case "infrastructure":
		return MappedRequest{Resource: "infrastructure", Verb: listVerb(method)}
	case "cluster-enrollments":
		return MappedRequest{Resource: "clusters", Verb: "update"}
	case "edge":
		return MappedRequest{Resource: "edge", Verb: listVerb(method)}
	case "apps":
		if len(parts) == 1 {
			return MappedRequest{Resource: "apps", Verb: listVerb(method)}
		}
		return MappedRequest{Resource: "apps", Verb: verb}
	case "charts":
		if len(parts) == 1 {
			return MappedRequest{Resource: "charts", Verb: listVerb(method)}
		}
		return MappedRequest{Resource: "charts", Verb: verb}
	default:
		return MappedRequest{Resource: parts[0], Verb: verb}
	}
}

func methodToVerb(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead:
		return "get"
	case http.MethodPost:
		return "create"
	case http.MethodPut, http.MethodPatch:
		return "update"
	case http.MethodDelete:
		return "delete"
	default:
		return strings.ToLower(method)
	}
}

func listVerb(method string) string {
	if method == http.MethodGet || method == http.MethodHead {
		return "list"
	}
	return methodToVerb(method)
}
