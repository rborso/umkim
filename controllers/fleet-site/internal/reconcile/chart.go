package reconcile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/appchart"
	"github.com/rborso/umkim/controllers/fleet-site/internal/harborcharts"
	"github.com/rborso/umkim/controllers/fleet-site/internal/jobs"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type ChartReconciler struct {
	*Reconciler
}

func (r *ChartReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var ch fleetv1alpha1.FleetChart
	if err := r.Get(ctx, req.NamespacedName, &ch); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !ch.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}
	hash := chartSpecHash(ch.Spec)
	if ch.Spec.EffectiveSource() == fleetv1alpha1.ChartSourceHarbor {
		return r.markHarborReady(ctx, &ch, hash)
	}
	if ch.Status.Phase == fleetv1alpha1.ChartPhasePublishing {
		return r.pollPublish(ctx, &ch, hash)
	}
	specUnchanged := ch.Status.SpecHash == hash && ch.Status.ObservedGeneration == ch.Generation
	if ch.Status.Phase == fleetv1alpha1.ChartPhaseReady && specUnchanged {
		return ctrl.Result{}, nil
	}
	if ch.Status.Phase == fleetv1alpha1.ChartPhaseFailed && specUnchanged {
		return ctrl.Result{}, nil
	}
	jobName := chartJobName(ch.Name)
	resultCM := chartResultCMName(ch.Name)
	job := jobs.ChartPublishJob(jobs.ChartPublishJobOpts{
		Namespace:      ch.Namespace,
		JobName:        jobName,
		ChartName:      ch.Name,
		Version:        ch.Spec.Version,
		Source:         ch.Spec.EffectiveSource(),
		Origin:         ch.Spec.Origin,
		InboxFile:      ch.Spec.InboxFile,
		InboxHostPath:  r.Cfg.InboxDir(),
		ImagePolicy:    ch.Spec.EffectiveImagePolicy(),
		ValuesYAML:     ch.Spec.ValuesYAML,
		DestOCI:        r.Cfg.DefaultChartPrefix(),
		MirrorHost:     r.Cfg.HarborRegistryHost() + "/" + r.Cfg.HarborMirrorProject(),
		RunnerImage:    r.Cfg.AnsibleRunnerImage,
		HubSecretsName: jobs.HubAnsibleSecretsName,
		ResultCM:       resultCM,
	})
	_ = r.Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: resultCM, Namespace: ch.Namespace}})
	if err := controllerutil.SetControllerReference(&ch, job, r.Scheme); err != nil {
		return r.failChart(ctx, &ch, err.Error(), hash)
	}
	if err := r.ensureProviderJob(ctx, job); err != nil {
		if isJobTerminating(err) {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		return r.failChart(ctx, &ch, err.Error(), hash)
	}
	ch.Status.Phase = fleetv1alpha1.ChartPhasePublishing
	ch.Status.Message = "Publishing chart to Harbor"
	ch.Status.LastJobName = jobName
	ch.Status.SpecHash = hash
	ch.Status.ObservedGeneration = ch.Generation
	if err := r.Status().Update(ctx, &ch); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *ChartReconciler) pollPublish(ctx context.Context, ch *fleetv1alpha1.FleetChart, hash string) (ctrl.Result, error) {
	done, ok, msg := r.jobPollState(ctx, ch.Namespace, ch.Status.LastJobName, false)
	if !done {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	if !ok {
		return r.failChart(ctx, ch, "chart publish failed: "+msg, hash)
	}
	r.applyPublishResult(ctx, ch)
	if ch.Status.Chart == "" {
		ch.Status.Chart = strings.TrimRight(r.Cfg.DefaultChartPrefix(), "/") + "/" + ch.Name
	}
	if ch.Spec.Version != "" {
		ch.Status.Versions = addVersion(ch.Status.Versions, ch.Spec.Version)
	}
	ch.Status.Phase = fleetv1alpha1.ChartPhaseReady
	policy := ch.Spec.EffectiveImagePolicy()
	switch policy {
	case fleetv1alpha1.ChartImageCopy:
		if len(ch.Status.ImagesCopied) == 0 {
			ch.Status.Message = "Published to Harbor; image copy recorded none"
		} else {
			ch.Status.Message = "Published to Harbor; images copied"
		}
	case fleetv1alpha1.ChartImageProxy:
		ch.Status.Message = "Published to Harbor; images use pull-through"
	default:
		ch.Status.Message = "Published to Harbor"
	}
	ch.Status.SpecHash = hash
	ch.Status.ObservedGeneration = ch.Generation
	if base := appchart.SafeInboxBase(ch.Spec.InboxFile); base != "" {
		_ = os.Remove(filepath.Join(r.Cfg.InboxDir(), base))
	}
	if err := r.Status().Update(ctx, ch); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ChartReconciler) applyPublishResult(ctx context.Context, ch *fleetv1alpha1.FleetChart) {
	var cm corev1.ConfigMap
	if err := r.Get(ctx, types.NamespacedName{Name: chartResultCMName(ch.Name), Namespace: ch.Namespace}, &cm); err != nil {
		return
	}
	if v := strings.TrimSpace(cm.Data["chart"]); v != "" {
		ch.Status.Chart = v
	}
	if v := strings.TrimSpace(cm.Data["digest"]); v != "" {
		ch.Status.Digest = v
	}
	if v := strings.TrimSpace(cm.Data["version"]); v != "" {
		ch.Status.Versions = addVersion(ch.Status.Versions, v)
	}
	if v := strings.TrimSpace(cm.Data["imageValues.yaml"]); v != "" {
		ch.Status.ImageValues = v
	}
	if raw := strings.TrimSpace(cm.Data["imagesCopied.json"]); raw != "" && raw != "[]" {
		var copied []fleetv1alpha1.FleetChartCopiedImage
		if json.Unmarshal([]byte(raw), &copied) == nil {
			ch.Status.ImagesCopied = copied
		}
	}
}

func (r *ChartReconciler) markHarborReady(ctx context.Context, ch *fleetv1alpha1.FleetChart, hash string) (ctrl.Result, error) {
	ch.Status.Phase = fleetv1alpha1.ChartPhaseReady
	ch.Status.Message = "Imported from Harbor"
	ch.Status.Chart = strings.TrimRight(r.Cfg.DefaultChartPrefix(), "/") + "/" + ch.Name
	if ch.Spec.Version != "" {
		ch.Status.Versions = addVersion(ch.Status.Versions, ch.Spec.Version)
	}
	ch.Status.SpecHash = hash
	ch.Status.ObservedGeneration = ch.Generation
	if err := r.Status().Update(ctx, ch); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ChartReconciler) failChart(ctx context.Context, ch *fleetv1alpha1.FleetChart, msg, hash string) (ctrl.Result, error) {
	ch.Status.Phase = fleetv1alpha1.ChartPhaseFailed
	ch.Status.Message = msg
	if hash != "" {
		ch.Status.SpecHash = hash
		ch.Status.ObservedGeneration = ch.Generation
	}
	if err := r.Status().Update(ctx, ch); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func chartSpecHash(spec fleetv1alpha1.FleetChartSpec) string {
	b, _ := json.Marshal(spec)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:16])
}

func chartJobName(name string) string {
	n := "fc-p-" + name
	if len(n) <= 63 {
		return n
	}
	sum := sha256.Sum256([]byte(name))
	return "fc-p-" + hex.EncodeToString(sum[:8])
}

func chartResultCMName(name string) string {
	n := "fc-result-" + name
	if len(n) <= 63 {
		return n
	}
	sum := sha256.Sum256([]byte(name))
	return "fc-result-" + hex.EncodeToString(sum[:8])
}

func addVersion(existing []string, v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return existing
	}
	for _, e := range existing {
		if e == v {
			return existing
		}
	}
	return append(existing, v)
}

func (r *Reconciler) SetupChartWithManager(mgr ctrl.Manager) error {
	cr := &ChartReconciler{Reconciler: r}
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&fleetv1alpha1.FleetChart{}).
		Owns(&batchv1.Job{}).
		Complete(cr); err != nil {
		return err
	}
	return mgr.Add(&harborChartImporter{Reconciler: r})
}

type harborChartImporter struct {
	*Reconciler
}

func (h *harborChartImporter) NeedLeaderElection() bool { return true }

func (h *harborChartImporter) Start(ctx context.Context) error {
	_ = h.sync(ctx)
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			_ = h.sync(ctx)
		}
	}
}

func (h *harborChartImporter) sync(ctx context.Context) error {
	logger := log.FromContext(ctx)
	if h.Client == nil {
		return nil
	}
	var secret corev1.Secret
	if err := h.Get(ctx, types.NamespacedName{Name: jobs.HubAnsibleSecretsName, Namespace: h.Cfg.Namespace}, &secret); err != nil {
		return nil
	}
	pw := harborPasswordFromSecret(&secret)
	if pw == "" {
		return nil
	}
	c := harborcharts.NewClient("https://"+h.Cfg.HarborRegistryHost(), "admin", pw)
	repos, err := c.ListCharts(h.Cfg.HarborChartsProject())
	if err != nil {
		logger.Info("harbor chart import skipped", "err", err.Error())
		return err
	}
	ns := h.Cfg.Namespace
	prefix := strings.TrimRight(h.Cfg.DefaultChartPrefix(), "/")
	for _, repo := range repos {
		if !appchart.ValidDNS1123(repo.Name) {
			continue
		}
		var existing fleetv1alpha1.FleetChart
		err := h.Get(ctx, types.NamespacedName{Name: repo.Name, Namespace: ns}, &existing)
		if err == nil {
			if existing.Status.Phase == "" || existing.Status.Chart == "" {
				existing.Status.Phase = fleetv1alpha1.ChartPhaseReady
				existing.Status.Chart = prefix + "/" + repo.Name
				existing.Status.Versions = mergeVersions(existing.Status.Versions, repo.Versions)
				existing.Status.Message = "Imported from Harbor"
				_ = h.Status().Update(ctx, &existing)
			} else if len(repo.Versions) > 0 {
				merged := mergeVersions(existing.Status.Versions, repo.Versions)
				if len(merged) != len(existing.Status.Versions) {
					existing.Status.Versions = merged
					_ = h.Status().Update(ctx, &existing)
				}
			}
			continue
		}
		if !apierrors.IsNotFound(err) {
			continue
		}
		ver := ""
		if len(repo.Versions) > 0 {
			ver = repo.Versions[0]
		}
		ch := &fleetv1alpha1.FleetChart{
			ObjectMeta: metav1.ObjectMeta{Name: repo.Name, Namespace: ns},
			Spec: fleetv1alpha1.FleetChartSpec{
				Source:      fleetv1alpha1.ChartSourceHarbor,
				ImagePolicy: fleetv1alpha1.ChartImageExternal,
				Version:     ver,
			},
		}
		if err := h.Create(ctx, ch); err != nil && !apierrors.IsAlreadyExists(err) {
			logger.Info("create FleetChart", "name", repo.Name, "err", err.Error())
		}
	}
	return nil
}

func mergeVersions(a, b []string) []string {
	out := append([]string(nil), a...)
	for _, v := range b {
		out = addVersion(out, v)
	}
	return out
}

func harborPasswordFromSecret(secret *corev1.Secret) string {
	raw, ok := secret.Data["ansible_vars.yml"]
	if !ok {
		return ""
	}
	// keep this dependency-light: scan for harbor_admin_password: value
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "harbor_admin_password:") {
			continue
		}
		v := strings.TrimSpace(strings.TrimPrefix(line, "harbor_admin_password:"))
		return strings.Trim(v, `"'`)
	}
	return ""
}
