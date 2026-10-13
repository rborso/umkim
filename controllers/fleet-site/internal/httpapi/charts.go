package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/appchart"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const maxChartUploadBytes = 32 << 20

type chartCreateBody struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Origin      string `json:"origin"`
	Version     string `json:"version"`
	ImagePolicy string `json:"imagePolicy"`
	ValuesYAML  string `json:"valuesYaml"`
	Force       bool   `json:"force"`
}

type chartResponse struct {
	Name         string                                `json:"name"`
	Source       string                                `json:"source"`
	Origin       string                                `json:"origin,omitempty"`
	Version      string                                `json:"version,omitempty"`
	ImagePolicy  string                                `json:"imagePolicy"`
	Chart        string                                `json:"chart,omitempty"`
	Digest       string                                `json:"digest,omitempty"`
	Versions     []string                              `json:"versions,omitempty"`
	Phase        string                                `json:"phase"`
	Message      string                                `json:"message,omitempty"`
	ImagesCopied []fleetv1alpha1.FleetChartCopiedImage `json:"imagesCopied,omitempty"`
	// OIDCDefaults is the app's recommended Authelia OIDC client (JSON), used to
	// pre-fill the install prompt.
	OIDCDefaults string `json:"oidcDefaults,omitempty"`
}

type chartsListResponse struct {
	Charts []chartResponse `json:"charts"`
}

func (s *Server) handleCharts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listCharts(w, r)
	case http.MethodPost:
		s.createChart(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleChartByName(w http.ResponseWriter, r *http.Request, name string) {
	switch r.Method {
	case http.MethodGet:
		s.getChart(w, r, name)
	case http.MethodDelete:
		s.deleteChart(w, r, name)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listCharts(w http.ResponseWriter, r *http.Request) {
	var list fleetv1alpha1.FleetChartList
	if err := s.Client.List(r.Context(), &list, client.InNamespace(s.Cfg.Namespace)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]chartResponse, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, chartToResponse(&list.Items[i]))
	}
	writeJSON(w, http.StatusOK, chartsListResponse{Charts: out})
}

func (s *Server) getChart(w http.ResponseWriter, r *http.Request, name string) {
	var ch fleetv1alpha1.FleetChart
	if err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: s.Cfg.Namespace, Name: name}, &ch); err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, chartToResponse(&ch))
}

func (s *Server) createChart(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireMutatingActor(w, r); !ok {
		return
	}
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(strings.ToLower(ct), "multipart/form-data") {
		s.createChartUpload(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var body chartCreateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	source := strings.ToLower(strings.TrimSpace(body.Source))
	if source == "" {
		source = appchart.InferChartSource(body.Origin)
	}
	if source == "" {
		source = fleetv1alpha1.ChartSourceHarbor
	}
	name := strings.ToLower(strings.TrimSpace(body.Name))
	if name == "" {
		switch source {
		case fleetv1alpha1.ChartSourceOCI:
			name = appchart.NameFromOCI(body.Origin)
		case fleetv1alpha1.ChartSourceHTTP:
			name = appchart.NameFromHTTP(body.Origin)
		}
	}
	spec := fleetv1alpha1.FleetChartSpec{
		Source:      source,
		Origin:      strings.TrimSpace(body.Origin),
		Version:     strings.TrimSpace(body.Version),
		ImagePolicy: strings.TrimSpace(body.ImagePolicy),
		ValuesYAML:  body.ValuesYAML,
	}
	s.putChart(w, r, name, spec, body.Force || queryParamBool(r, "force"))
}

func (s *Server) createChartUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxChartUploadBytes+1<<20)
	if err := r.ParseMultipartForm(maxChartUploadBytes); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart upload (max 32Mi)"})
		return
	}
	file, hdr, err := r.FormFile("chart")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "multipart field chart (a .tgz) is required"})
		return
	}
	defer file.Close()
	dir := s.Cfg.InboxDir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		http.Error(w, "chart inbox is not writable: "+err.Error(), http.StatusInternalServerError)
		return
	}
	tmp := filepath.Join(dir, "upload-"+randHex(8)+".tgz")
	dst, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	n, copyErr := io.Copy(dst, io.LimitReader(file, maxChartUploadBytes+1))
	_ = dst.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		http.Error(w, copyErr.Error(), http.StatusInternalServerError)
		return
	}
	if n > maxChartUploadBytes {
		_ = os.Remove(tmp)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "chart exceeds 32Mi"})
		return
	}
	metaName, metaVer, err := appchart.MetaFromTGZ(tmp)
	if err != nil {
		_ = os.Remove(tmp)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid helm package: " + err.Error()})
		return
	}
	name := strings.ToLower(strings.TrimSpace(r.FormValue("name")))
	if name == "" {
		name = metaName
	}
	version := strings.TrimSpace(r.FormValue("version"))
	if version == "" {
		version = metaVer
	}
	base := appchart.SafeInboxBase(name + "-" + randHex(6) + ".tgz")
	if base == "" {
		_ = os.Remove(tmp)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not name uploaded chart"})
		return
	}
	final := filepath.Join(dir, base)
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = hdr
	spec := fleetv1alpha1.FleetChartSpec{
		Source:      fleetv1alpha1.ChartSourceUpload,
		Origin:      "upload:" + base,
		Version:     version,
		ImagePolicy: strings.TrimSpace(r.FormValue("imagePolicy")),
		InboxFile:   base,
		ValuesYAML:  r.FormValue("valuesYaml"),
	}
	force := strings.EqualFold(strings.TrimSpace(r.FormValue("force")), "true")
	if !s.putChart(w, r, name, spec, force) {
		// The tarball was staged before validation; drop it when the write is rejected.
		s.removeInboxFile(base, "")
	}
}

func chartUpdateNeedsForce(existing *fleetv1alpha1.FleetChart, spec fleetv1alpha1.FleetChartSpec) bool {
	if existing.Status.Phase == fleetv1alpha1.ChartPhaseReady {
		if !strings.EqualFold(strings.TrimSpace(existing.Spec.Source), strings.TrimSpace(spec.Source)) {
			return true
		}
		if strings.TrimSpace(existing.Spec.Origin) != strings.TrimSpace(spec.Origin) {
			return true
		}
	}
	return false
}

// putChart writes or replaces a library chart. It reports whether the write
// succeeded so the upload handler can drop a tarball the request rejected.
func (s *Server) putChart(w http.ResponseWriter, r *http.Request, name string, spec fleetv1alpha1.FleetChartSpec, force bool) bool {
	if !appchart.ValidDNS1123(name) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name must be a DNS-1123 label (lowercase)"})
		return false
	}
	if !appchart.ValidChartSource(spec.Source) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source must be oci, http, upload, or harbor"})
		return false
	}
	if !appchart.ValidImagePolicy(spec.ImagePolicy) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "imagePolicy must be external, proxy, or copy"})
		return false
	}
	if spec.ImagePolicy == "" {
		spec.ImagePolicy = fleetv1alpha1.ChartImageExternal
	}
	if !appchart.ValidVersion(spec.Version) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid chart version"})
		return false
	}
	switch spec.Source {
	case fleetv1alpha1.ChartSourceOCI:
		origin := spec.Origin
		if !strings.HasPrefix(origin, "oci://") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "origin must be an oci:// URL"})
			return false
		}
		if err := appchart.ChartAllowed(origin, s.Cfg.ChartOCIPrefixes()); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return false
		}
	case fleetv1alpha1.ChartSourceHTTP:
		if err := appchart.ValidHTTPOrigin(spec.Origin); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return false
		}
	case fleetv1alpha1.ChartSourceUpload:
		if spec.InboxFile == "" || appchart.SafeInboxBase(spec.InboxFile) != spec.InboxFile {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "upload is missing a chart file"})
			return false
		}
	}
	var existing fleetv1alpha1.FleetChart
	err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: s.Cfg.Namespace, Name: name}, &existing)
	if err == nil {
		if !force && chartUpdateNeedsForce(&existing, spec) {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "chart " + name + " is Ready with a different source/origin; set force=true to replace",
			})
			return false
		}
		superseded := existing.Spec.InboxFile
		existing.Spec = spec
		if err := s.Client.Update(r.Context(), &existing); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return false
		}
		s.removeInboxFile(superseded, spec.InboxFile)
		writeJSON(w, http.StatusOK, chartToResponse(&existing))
		return true
	}
	if !apierrors.IsNotFound(err) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return false
	}
	ch := &fleetv1alpha1.FleetChart{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.Cfg.Namespace},
		Spec:       spec,
	}
	if err := s.Client.Create(r.Context(), ch); err != nil {
		if apierrors.IsAlreadyExists(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "chart " + name + " already exists"})
			return false
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return false
	}
	writeJSON(w, http.StatusCreated, chartToResponse(ch))
	return true
}

// removeInboxFile drops a staged chart tarball that is no longer referenced by
// any FleetChart. The upload handler stages the file before the request is
// validated, so rejected and force-replaced uploads would otherwise leak it.
func (s *Server) removeInboxFile(base, keep string) {
	if base == "" || base == keep {
		return
	}
	safe := appchart.SafeInboxBase(base)
	if safe == "" {
		return
	}
	_ = os.Remove(filepath.Join(s.Cfg.InboxDir(), safe))
}

func (s *Server) deleteChart(w http.ResponseWriter, r *http.Request, name string) {
	if _, ok := s.requireMutatingActor(w, r); !ok {
		return
	}
	var ch fleetv1alpha1.FleetChart
	if err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: s.Cfg.Namespace, Name: name}, &ch); err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.Client.Delete(r.Context(), &ch); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.removeInboxFile(ch.Spec.InboxFile, "")
	writeJSON(w, http.StatusAccepted, map[string]string{"name": name})
}

func chartToResponse(ch *fleetv1alpha1.FleetChart) chartResponse {
	versions := ch.Status.Versions
	if versions == nil {
		versions = []string{}
	}
	copied := ch.Status.ImagesCopied
	if copied == nil {
		copied = []fleetv1alpha1.FleetChartCopiedImage{}
	}
	return chartResponse{
		Name:         ch.Name,
		Source:       ch.Spec.Source,
		Origin:       ch.Spec.Origin,
		Version:      firstNonEmpty(ch.Spec.Version, firstVersion(ch.Status.Versions)),
		ImagePolicy:  ch.Spec.EffectiveImagePolicy(),
		Chart:        ch.Status.Chart,
		Digest:       ch.Status.Digest,
		Versions:     versions,
		Phase:        ch.Status.Phase,
		Message:      ch.Status.Message,
		ImagesCopied: copied,
		OIDCDefaults: ch.Status.OIDCDefaults,
	}
}

func firstVersion(vs []string) string {
	if len(vs) == 0 {
		return ""
	}
	return vs[0]
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:n]
}
