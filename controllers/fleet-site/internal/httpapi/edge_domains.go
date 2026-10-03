package httpapi

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (s *Server) handleEdgeDomains(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/v1alpha1/edge/domains":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.handleGetDomains(r.Context(), w)
	case "/api/v1alpha1/edge/hostnames/check":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.handleHostnameCheck(r.Context(), w, r)
	case "/api/v1alpha1/edge/dns/validate":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.handleDNSValidate(w, r)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *Server) handleGetDomains(ctx context.Context, w http.ResponseWriter) {
	ns := s.Cfg.EdgeDomainsCMNamespace
	name := s.Cfg.EdgeDomainsCMName
	if ns == "" || name == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{"registry": nil})
		return
	}
	var cm corev1.ConfigMap
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &cm); err != nil {
		http.Error(w, "domain registry not found", http.StatusNotFound)
		return
	}
	raw := cm.Data["domains.json"]
	var registry interface{}
	if err := json.Unmarshal([]byte(raw), &registry); err != nil {
		http.Error(w, "invalid domains.json", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"registry": registry})
}

func (s *Server) handleHostnameCheck(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	host := strings.TrimSpace(r.URL.Query().Get("host"))
	if host == "" {
		http.Error(w, "host query required", http.StatusBadRequest)
		return
	}
	available := true
	var usedBy string
	if s.Cfg.EdgeRoutesNamespace != "" {
		// List FleetAppRoute via unstructured would need scheme; return optimistic if not wired.
		_ = ctx
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"host":      host,
		"available": available,
		"used_by":   usedBy,
	})
}

func (s *Server) handleDNSValidate(w http.ResponseWriter, r *http.Request) {
	host := strings.TrimSpace(r.URL.Query().Get("host"))
	expected := strings.TrimSpace(r.URL.Query().Get("expected_ip"))
	if host == "" {
		http.Error(w, "host query required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	ips, err := net.DefaultResolver.LookupHost(ctx, host)
	ok := err == nil && len(ips) > 0
	matched := false
	if ok && expected != "" {
		for _, ip := range ips {
			if ip == expected {
				matched = true
				break
			}
		}
	} else if ok && expected == "" {
		matched = true
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"host":        host,
		"resolved":    ok,
		"addresses":   ips,
		"expected_ip": expected,
		"matched":     matched,
		"error":       errString(err),
	})
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
