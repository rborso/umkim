package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/appchart"
	"github.com/rborso/umkim/controllers/fleet-site/internal/llamaprofiletest"
	"github.com/rborso/umkim/controllers/fleet-site/internal/llamaprofiles"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type llamaProfileResponse struct {
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	ProfileYAML  string `json:"profileYaml,omitempty"`
	Chart        string `json:"chart,omitempty"`
	ChartVersion string `json:"chartVersion,omitempty"`
	System       bool   `json:"system,omitempty"`
}

type llamaProfileCreateBody struct {
	ID           string `json:"id"`
	Description  string `json:"description"`
	ProfileYAML  string `json:"profileYaml"`
	ChartVersion string `json:"chartVersion"`
}

type llamaProfilePatchBody struct {
	Description  string `json:"description"`
	ProfileYAML  string `json:"profileYaml"`
	ChartVersion string `json:"chartVersion"`
}

type llamaProfileCRValuesResponse struct {
	Name       string `json:"name"`
	ValuesYAML string `json:"valuesYaml"`
}

type llamaProfileVerifyBody struct {
	ClusterName  string `json:"clusterName"`
	Mode         string `json:"mode"`
	ChartVersion string `json:"chartVersion"`
	Wait         bool   `json:"wait"`
}

type llamaProfileTestResponse struct {
	ID           string `json:"id"`
	ProfileName  string `json:"profileName"`
	ClusterName  string `json:"clusterName"`
	Mode         string `json:"mode"`
	Phase        string `json:"phase"`
	APIBase      string `json:"apiBase,omitempty"`
	Model        string `json:"model,omitempty"`
	Message      string `json:"message,omitempty"`
	FleetAppName string `json:"fleetAppName,omitempty"`
}

func (s *Server) profileNS() string {
	return s.Cfg.ConsoleNamespace
}

func (s *Server) handleLlamaProfiles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listLlamaProfiles(w, r)
	case http.MethodPost:
		s.createLlamaProfile(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleLlamaProfilesSub(w http.ResponseWriter, r *http.Request, sub string) {
	sub = strings.Trim(sub, "/")
	if sub == "" {
		s.handleLlamaProfiles(w, r)
		return
	}
	if sub == "tests" || strings.HasPrefix(sub, "tests/") {
		s.handleLlamaProfileTests(w, r, strings.TrimPrefix(sub, "tests"))
		return
	}
	parts := strings.SplitN(sub, "/", 2)
	name := parts[0]
	rest := ""
	if len(parts) == 2 {
		rest = strings.Trim(parts[1], "/")
	}
	switch rest {
	case "":
		s.handleLlamaProfileByName(w, r, name)
	case "values":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.getLlamaProfileValues(w, r, name)
	case "verify":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.verifyLlamaProfile(w, r, name)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *Server) handleLlamaProfileByName(w http.ResponseWriter, r *http.Request, name string) {
	switch r.Method {
	case http.MethodGet:
		s.getLlamaProfile(w, r, name)
	case http.MethodPatch:
		s.patchLlamaProfile(w, r, name)
	case http.MethodDelete:
		s.deleteLlamaProfile(w, r, name)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listLlamaProfiles(w http.ResponseWriter, r *http.Request) {
	_ = s.ensureLlamaProfileSeed(r.Context())
	var list fleetv1alpha1.FleetLlamaProfileList
	if err := s.Client.List(r.Context(), &list, client.InNamespace(s.profileNS())); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sortByName(list.Items, func(p fleetv1alpha1.FleetLlamaProfile) string { return p.Name })
	out := make([]llamaProfileResponse, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, profileToResponse(&list.Items[i]))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"profiles": out})
}

func (s *Server) getLlamaProfile(w http.ResponseWriter, r *http.Request, name string) {
	p, err := s.loadProfileCR(w, r, name)
	if err != nil {
		return
	}
	writeJSON(w, http.StatusOK, profileToResponse(p))
}

func (s *Server) createLlamaProfile(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireMutatingActor(w, r); !ok {
		return
	}
	var body llamaProfileCreateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	name := strings.TrimSpace(body.ID)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}
	if !appchart.ValidDNS1123(name) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id must be a DNS-1123 label"})
		return
	}
	_ = s.ensureLlamaProfileSeed(r.Context())
	cr := &fleetv1alpha1.FleetLlamaProfile{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.profileNS()},
		Spec: fleetv1alpha1.FleetLlamaProfileSpec{
			Description:  strings.TrimSpace(body.Description),
			ProfileYAML:  body.ProfileYAML,
			Chart:        fleetv1alpha1.DefaultLlamaCppChart,
			ChartVersion: strings.TrimSpace(body.ChartVersion),
			System:       false,
		},
	}
	if cr.Spec.ChartVersion == "" {
		cr.Spec.ChartVersion = fleetv1alpha1.DefaultLlamaCppChartVersion
	}
	if err := s.Client.Create(r.Context(), cr); err != nil {
		if apierrors.IsAlreadyExists(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "profile already exists"})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, profileToResponse(cr))
}

func (s *Server) patchLlamaProfile(w http.ResponseWriter, r *http.Request, name string) {
	if _, ok := s.requireMutatingActor(w, r); !ok {
		return
	}
	var p fleetv1alpha1.FleetLlamaProfile
	if err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: s.profileNS(), Name: name}, &p); err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if p.Spec.System {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "system profiles cannot be updated"})
		return
	}
	var body llamaProfilePatchBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if body.Description != "" {
		p.Spec.Description = strings.TrimSpace(body.Description)
	}
	if body.ProfileYAML != "" {
		p.Spec.ProfileYAML = body.ProfileYAML
	}
	if body.ChartVersion != "" {
		p.Spec.ChartVersion = strings.TrimSpace(body.ChartVersion)
	}
	if err := s.Client.Update(r.Context(), &p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, profileToResponse(&p))
}

func (s *Server) deleteLlamaProfile(w http.ResponseWriter, r *http.Request, name string) {
	if _, ok := s.requireMutatingActor(w, r); !ok {
		return
	}
	force := strings.EqualFold(r.URL.Query().Get("force"), "true")
	var p fleetv1alpha1.FleetLlamaProfile
	if err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: s.profileNS(), Name: name}, &p); err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if p.Spec.System && !force {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "system profile; use ?force=true to delete"})
		return
	}
	if err := s.Client.Delete(r.Context(), &p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getLlamaProfileValues(w http.ResponseWriter, r *http.Request, name string) {
	p, err := llamaprofiles.ResolveProfile(r.Context(), s.Client, s.Cfg, name)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	yaml, err := llamaprofiles.MergeForProfile(p, s.Cfg.LlamaCppImageRepository())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, llamaProfileCRValuesResponse{Name: name, ValuesYAML: yaml})
}

func (s *Server) verifyLlamaProfile(w http.ResponseWriter, r *http.Request, profileName string) {
	if _, ok := s.requireMutatingActor(w, r); !ok {
		return
	}
	var body llamaProfileVerifyBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	cluster := strings.TrimSpace(body.ClusterName)
	if cluster == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "clusterName is required"})
		return
	}
	mode := strings.TrimSpace(body.Mode)
	if mode == "" {
		mode = fleetv1alpha1.LlamaProfileTestModeQuick
	}
	switch mode {
	case fleetv1alpha1.LlamaProfileTestModeQuick, fleetv1alpha1.LlamaProfileTestModeExtended:
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "mode must be quick or extended"})
		return
	}
	if _, err := llamaprofiles.ResolveProfile(r.Context(), s.Client, s.Cfg, profileName); err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "profile not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fcj, err := s.findFleetClusterJoin(r.Context(), cluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cluster not found"})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if fcj.Status.Phase != fleetv1alpha1.JoinPhaseReady {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "cluster is not Ready"})
		return
	}
	testID := "test-" + randomHex(8)
	test := &fleetv1alpha1.FleetLlamaProfileTest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testID,
			Namespace: s.profileNS(),
		},
		Spec: fleetv1alpha1.FleetLlamaProfileTestSpec{
			ProfileName:  profileName,
			ClusterName:  cluster,
			Mode:         mode,
			ChartVersion: strings.TrimSpace(body.ChartVersion),
		},
	}
	if err := s.Client.Create(r.Context(), test); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	test.Status.Phase = fleetv1alpha1.LlamaProfileTestPhasePending
	if err := s.Client.Status().Update(r.Context(), test); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	runner := &llamaprofiletest.Runner{Client: s.Client, Cfg: s.Cfg}
	go runner.Run(context.Background(), s.profileNS(), testID)

	resp := testToResponse(test)
	if body.Wait {
		waited, werr := llamaprofiletest.WaitForTerminal(r.Context(), s.Client, s.profileNS(), testID, 10*time.Second, 45*time.Minute)
		if werr != nil && waited != nil {
			resp = testToResponse(waited)
			writeJSON(w, http.StatusOK, resp)
			return
		}
		if waited != nil {
			resp = testToResponse(waited)
		}
		if waited != nil && waited.Status.Phase == fleetv1alpha1.LlamaProfileTestPhaseFailed {
			writeJSON(w, http.StatusConflict, resp)
			return
		}
	}
	writeJSON(w, http.StatusAccepted, resp)
}

func (s *Server) handleLlamaProfileTests(w http.ResponseWriter, r *http.Request, sub string) {
	sub = strings.Trim(sub, "/")
	if sub == "" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.listLlamaProfileTests(w, r)
		return
	}
	parts := strings.SplitN(sub, "/", 2)
	id := parts[0]
	rest := ""
	if len(parts) == 2 {
		rest = strings.Trim(parts[1], "/")
	}
	switch {
	case rest == "":
		s.handleLlamaProfileTestByID(w, r, id)
	case rest == "chat/completions" && r.Method == http.MethodPost:
		s.handleLlamaProfileTestChat(w, r, id)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *Server) listLlamaProfileTests(w http.ResponseWriter, r *http.Request) {
	var list fleetv1alpha1.FleetLlamaProfileTestList
	if err := s.Client.List(r.Context(), &list, client.InNamespace(s.profileNS())); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Tests are shown newest first; cached List order is not meaningful.
	sort.SliceStable(list.Items, func(i, j int) bool {
		ti, tj := list.Items[i].CreationTimestamp, list.Items[j].CreationTimestamp
		if ti.Equal(&tj) {
			return list.Items[i].Name < list.Items[j].Name
		}
		return tj.Before(&ti)
	})
	extendedOnly := strings.EqualFold(r.URL.Query().Get("extended"), "true")
	activeOnly := strings.EqualFold(r.URL.Query().Get("active"), "true")
	out := make([]llamaProfileTestResponse, 0)
	for i := range list.Items {
		t := &list.Items[i]
		if extendedOnly && t.Spec.Mode != fleetv1alpha1.LlamaProfileTestModeExtended {
			continue
		}
		if activeOnly && isTerminalTestPhase(t.Status.Phase) {
			continue
		}
		out = append(out, testToResponse(t))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"tests": out})
}

func (s *Server) handleLlamaProfileTestByID(w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case http.MethodGet:
		var test fleetv1alpha1.FleetLlamaProfileTest
		if err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: s.profileNS(), Name: id}, &test); err != nil {
			if apierrors.IsNotFound(err) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, testToResponse(&test))
	case http.MethodDelete:
		if _, ok := s.requireMutatingActor(w, r); !ok {
			return
		}
		var test fleetv1alpha1.FleetLlamaProfileTest
		if err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: s.profileNS(), Name: id}, &test); err != nil {
			if apierrors.IsNotFound(err) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		runner := &llamaprofiletest.Runner{Client: s.Client, Cfg: s.Cfg}
		if err := runner.EndTest(r.Context(), s.profileNS(), &test); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleLlamaProfileTestChat(w http.ResponseWriter, r *http.Request, id string) {
	if _, ok := s.requireMutatingActor(w, r); !ok {
		return
	}
	var test fleetv1alpha1.FleetLlamaProfileTest
	if err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: s.profileNS(), Name: id}, &test); err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	runner := &llamaprofiletest.Runner{Client: s.Client, Cfg: s.Cfg}
	out, code, err := runner.ProxyChat(r.Context(), &test, body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(out)
}

func (s *Server) loadProfileCR(w http.ResponseWriter, r *http.Request, name string) (*fleetv1alpha1.FleetLlamaProfile, error) {
	var p fleetv1alpha1.FleetLlamaProfile
	if err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: s.profileNS(), Name: name}, &p); err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return nil, err
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil, err
	}
	return &p, nil
}

func (s *Server) ensureLlamaProfileSeed(ctx context.Context) error {
	return llamaprofiles.EnsureSystemProfiles(ctx, s.Client, s.profileNS())
}

func profileToResponse(p *fleetv1alpha1.FleetLlamaProfile) llamaProfileResponse {
	return llamaProfileResponse{
		Name:         p.Name,
		Description:  p.Spec.Description,
		ProfileYAML:  p.Spec.ProfileYAML,
		Chart:        p.EffectiveChart(),
		ChartVersion: p.EffectiveChartVersion(),
		System:       p.Spec.System,
	}
}

func testToResponse(t *fleetv1alpha1.FleetLlamaProfileTest) llamaProfileTestResponse {
	return llamaProfileTestResponse{
		ID:           t.Name,
		ProfileName:  t.Spec.ProfileName,
		ClusterName:  t.Spec.ClusterName,
		Mode:         t.Spec.Mode,
		Phase:        t.Status.Phase,
		APIBase:      t.Status.APIBase,
		Model:        t.Status.Model,
		Message:      t.Status.Message,
		FleetAppName: t.Spec.FleetAppName,
	}
}

func isTerminalTestPhase(phase string) bool {
	switch phase {
	case fleetv1alpha1.LlamaProfileTestPhaseSucceeded, fleetv1alpha1.LlamaProfileTestPhaseFailed:
		return true
	default:
		return false
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(b)
}
