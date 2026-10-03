package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/llmkeys"
)

func (s *Server) handleProxyLLM(w http.ResponseWriter, r *http.Request) {
	var keyInfo llmkeys.KeyInfo
	authorized := false
	if actor, ok := s.resolveActor(r); ok {
		if isDC, _ := s.RBAC.IsDCAdmin(r.Context(), actor); isDC {
			authorized = true
		}
	}
	if !authorized {
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			token := strings.TrimSpace(auth[7:])
			if info, ok := llmkeys.ValidateBearer(r.Context(), s.Client, s.Cfg.Namespace, token); ok {
				keyInfo = info
				authorized = true
			}
		}
	}
	if !authorized {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if len(keyInfo.Models) > 0 && r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/chat/completions") {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		_ = r.Body.Close()
		var req struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if !llmkeys.ModelAllowed(keyInfo.Models, req.Model) {
			http.Error(w, "model not allowed for this API key", http.StatusForbidden)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
	}

	target, err := url.Parse(s.Cfg.LLMUpstream)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	masterKey, err := llmkeys.LiteLLMMasterKey(r.Context(), s.Client, llmkeys.MasterKeyConfig{
		DirectKey:  s.Cfg.LLMMasterKey,
		SecretNS:   s.Cfg.LLMMasterKeySecretNS,
		SecretName: s.Cfg.LLMMasterKeySecret,
		SecretKey:  s.Cfg.LLMMasterKeySecretKey,
	})
	if err != nil {
		log.Printf("llm proxy: %v", err)
		http.Error(w, "llm upstream misconfigured", http.StatusInternalServerError)
		return
	}
	if masterKey != "" {
		r.Header.Set("Authorization", "Bearer "+masterKey)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/proxy/llm")
	if r.URL.Path == "" {
		r.URL.Path = "/"
	}
	r.Host = target.Host
	proxy.ServeHTTP(w, r)
}
