package llamaprofiletest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/appchart"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"github.com/rborso/umkim/controllers/fleet-site/internal/llm"
	"github.com/rborso/umkim/controllers/fleet-site/internal/llamaprofiles"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultAppPollInterval = 15 * time.Second
	defaultVerifyTimeout   = 45 * time.Minute
	maxChatBodyBytes       = 1 << 20
)

// Runner executes profile verify sessions against the hub API.
type Runner struct {
	Client client.Client
	Cfg    config.Config
	HTTP   *http.Client
}

func (r *Runner) Run(ctx context.Context, testNS, testName string) {
	if r.HTTP == nil {
		r.HTTP = &http.Client{Timeout: 120 * time.Second}
	}
	deadline := time.Now().Add(defaultVerifyTimeout)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return
		}
		var test fleetv1alpha1.FleetLlamaProfileTest
		if err := r.Client.Get(ctx, types.NamespacedName{Namespace: testNS, Name: testName}, &test); err != nil {
			return
		}
		phase := test.Status.Phase
		switch phase {
		case fleetv1alpha1.LlamaProfileTestPhaseSucceeded, fleetv1alpha1.LlamaProfileTestPhaseFailed:
			return
		case fleetv1alpha1.LlamaProfileTestPhaseReady:
			return
		case "", fleetv1alpha1.LlamaProfileTestPhasePending:
			if strings.TrimSpace(test.Spec.FleetAppName) != "" {
				if err := r.ensureAPIBase(ctx, &test); err != nil {
					r.fail(ctx, &test, err)
					return
				}
				test.Status.Phase = fleetv1alpha1.LlamaProfileTestPhaseInstalling
				if err := r.patchTestStatus(ctx, &test); err != nil {
					r.fail(ctx, &test, err)
					return
				}
				continue
			}
			if err := r.startInstall(ctx, &test); err != nil {
				r.fail(ctx, &test, err)
				return
			}
			continue
		case fleetv1alpha1.LlamaProfileTestPhaseInstalling:
			if err := r.pollApp(ctx, &test); err != nil {
				if isRetryable(err) {
					time.Sleep(defaultAppPollInterval)
					continue
				}
				r.fail(ctx, &test, err)
				return
			}
		case fleetv1alpha1.LlamaProfileTestPhaseVerifying:
			if err := r.smoke(ctx, &test); err != nil {
				r.cleanupApp(ctx, &test)
				r.fail(ctx, &test, err)
				return
			}
			if test.Spec.Mode == fleetv1alpha1.LlamaProfileTestModeQuick {
				r.cleanupApp(ctx, &test)
				r.setPhase(ctx, &test, fleetv1alpha1.LlamaProfileTestPhaseSucceeded, test.Status.Message)
				return
			}
			r.setPhase(ctx, &test, fleetv1alpha1.LlamaProfileTestPhaseReady, "extended verify ready")
			return
		case fleetv1alpha1.LlamaProfileTestPhaseCleaningUp:
			return
		default:
			time.Sleep(defaultAppPollInterval)
		}
	}
	var test fleetv1alpha1.FleetLlamaProfileTest
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: testNS, Name: testName}, &test); err == nil {
		r.cleanupApp(ctx, &test)
		r.fail(ctx, &test, fmt.Errorf("verify timed out"))
	}
}

func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "not Ready") || strings.Contains(msg, "waiting")
}

func (r *Runner) ensureAPIBase(ctx context.Context, test *fleetv1alpha1.FleetLlamaProfileTest) error {
	if strings.TrimSpace(test.Status.APIBase) != "" {
		return nil
	}
	prof, err := llamaprofiles.ResolveProfile(ctx, r.Client, r.Cfg, test.Spec.ProfileName)
	if err != nil {
		return err
	}
	valuesYAML, err := llamaprofiles.MergeForProfile(prof, r.Cfg.LlamaCppImageRepository())
	if err != nil {
		return err
	}
	apiBase, model, err := llm.APIBaseFromValues(ctx, r.Client, r.Cfg, test.Spec.ClusterName, valuesYAML)
	if err != nil {
		return err
	}
	test.Status.APIBase = apiBase
	test.Status.Model = model
	return r.patchTestStatus(ctx, test)
}

func (r *Runner) startInstall(ctx context.Context, test *fleetv1alpha1.FleetLlamaProfileTest) error {
	prof, err := llamaprofiles.ResolveProfile(ctx, r.Client, r.Cfg, test.Spec.ProfileName)
	if err != nil {
		return err
	}
	valuesYAML, err := llamaprofiles.MergeForProfile(prof, r.Cfg.LlamaCppImageRepository())
	if err != nil {
		return err
	}
	apiBase, model, err := llm.APIBaseFromValues(ctx, r.Client, r.Cfg, test.Spec.ClusterName, valuesYAML)
	if err != nil {
		return err
	}
	chartVer := strings.TrimSpace(test.Spec.ChartVersion)
	if chartVer == "" {
		chartVer = prof.EffectiveChartVersion()
	}
	appName := strings.TrimSpace(test.Spec.FleetAppName)
	if appName == "" {
		appName = "llama-pt-" + shortID(test.Name)
		if len(appName) > 63 {
			appName = appName[:63]
		}
	}
	chart := appchart.ExpandChart(prof.EffectiveChart(), r.Cfg.DefaultChartPrefix())
	app := &fleetv1alpha1.FleetApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: r.Cfg.Namespace,
			Annotations: map[string]string{
				fleetv1alpha1.AnnotationLlamaProfileTest: test.Name,
				fleetv1alpha1.AnnotationLlamaProfileName: test.Spec.ProfileName,
			},
		},
		Spec: fleetv1alpha1.FleetAppSpec{
			ClusterName: test.Spec.ClusterName,
			Chart:       chart,
			Version:     chartVer,
			ReleaseName: appName,
			Namespace:   "llm",
			ValuesYAML:  valuesYAML,
			Route:       fleetv1alpha1.FleetAppRouteIntent{Mode: fleetv1alpha1.AppRouteModeNone},
		},
	}
	if err := r.Client.Create(ctx, app); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	test.Spec.FleetAppName = appName
	test.Status.Phase = fleetv1alpha1.LlamaProfileTestPhaseInstalling
	test.Status.APIBase = apiBase
	test.Status.Model = model
	if err := r.patchTestSpec(ctx, test); err != nil {
		return err
	}
	return r.patchTestStatus(ctx, test)
}

func (r *Runner) pollApp(ctx context.Context, test *fleetv1alpha1.FleetLlamaProfileTest) error {
	appName := strings.TrimSpace(test.Spec.FleetAppName)
	if appName == "" {
		return fmt.Errorf("missing fleetAppName")
	}
	var app fleetv1alpha1.FleetApp
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: r.Cfg.Namespace, Name: appName}, &app); err != nil {
		return err
	}
	switch app.Status.Phase {
	case fleetv1alpha1.AppPhaseReady:
		if err := r.ensureAPIBase(ctx, test); err != nil {
			return err
		}
		test.Status.Phase = fleetv1alpha1.LlamaProfileTestPhaseVerifying
		return r.patchTestStatus(ctx, test)
	case fleetv1alpha1.AppPhaseFailed:
		msg := strings.TrimSpace(app.Status.Message)
		if msg == "" {
			msg = "FleetApp install failed"
		}
		return fmt.Errorf("%s", msg)
	case fleetv1alpha1.AppPhaseDeleting:
		return fmt.Errorf("FleetApp was deleted")
	default:
		msg := strings.TrimSpace(app.Status.Message)
		if msg != "" {
			return fmt.Errorf("waiting: FleetApp phase %s (%s)", app.Status.Phase, msg)
		}
		return fmt.Errorf("waiting: FleetApp phase %s", app.Status.Phase)
	}
}

func (r *Runner) smoke(ctx context.Context, test *fleetv1alpha1.FleetLlamaProfileTest) error {
	deadline := time.Now().Add(45 * time.Minute)
	var lastErr error
	attempt := 0
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		attempt++
		err := r.smokeOnce(ctx, test)
		if err == nil {
			test.Status.Message = "models OK"
			return r.patchTestStatus(ctx, test)
		}
		lastErr = err
		test.Status.Message = fmt.Sprintf("smoke attempt %d: %s", attempt, err.Error())
		_ = r.patchTestStatus(ctx, test)
		if !isRetryableSmoke(err) {
			return fmt.Errorf("smoke failed (non-retryable) at %s: %w", test.Status.APIBase, err)
		}
		time.Sleep(20 * time.Second)
	}
	return fmt.Errorf("smoke timed out after %d attempts at %s: %v", attempt, test.Status.APIBase, lastErr)
}

func (r *Runner) smokeOnce(ctx context.Context, test *fleetv1alpha1.FleetLlamaProfileTest) error {
	base := strings.TrimSuffix(strings.TrimSpace(test.Status.APIBase), "/")
	if base == "" {
		return fmt.Errorf("apiBase not set")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", nil)
	if err != nil {
		return err
	}
	url := base + "/models"
	resp, err := r.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("models GET %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return nil
}

func isRetryableSmoke(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "no route to host") ||
		strings.Contains(msg, "502") ||
		strings.Contains(msg, "503")
}

func (r *Runner) cleanupApp(ctx context.Context, test *fleetv1alpha1.FleetLlamaProfileTest) {
	appName := strings.TrimSpace(test.Spec.FleetAppName)
	if appName == "" {
		return
	}
	var app fleetv1alpha1.FleetApp
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: r.Cfg.Namespace, Name: appName}, &app); err != nil {
		return
	}
	_ = r.Client.Delete(ctx, &app)
}

func (r *Runner) fail(ctx context.Context, test *fleetv1alpha1.FleetLlamaProfileTest, err error) {
	test.Status.Phase = fleetv1alpha1.LlamaProfileTestPhaseFailed
	test.Status.Message = err.Error()
	_ = r.patchTestStatus(ctx, test)
}

func (r *Runner) setPhase(ctx context.Context, test *fleetv1alpha1.FleetLlamaProfileTest, phase, msg string) {
	test.Status.Phase = phase
	if msg != "" {
		test.Status.Message = msg
	}
	_ = r.patchTestStatus(ctx, test)
}

func (r *Runner) patchTestSpec(ctx context.Context, test *fleetv1alpha1.FleetLlamaProfileTest) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		latest := &fleetv1alpha1.FleetLlamaProfileTest{}
		if err := r.Client.Get(ctx, types.NamespacedName{Namespace: test.Namespace, Name: test.Name}, latest); err != nil {
			return err
		}
		latest.Spec = test.Spec
		return r.Client.Update(ctx, latest)
	})
}

func (r *Runner) patchTestStatus(ctx context.Context, test *fleetv1alpha1.FleetLlamaProfileTest) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		latest := &fleetv1alpha1.FleetLlamaProfileTest{}
		if err := r.Client.Get(ctx, types.NamespacedName{Namespace: test.Namespace, Name: test.Name}, latest); err != nil {
			return err
		}
		latest.Status = test.Status
		return r.Client.Status().Update(ctx, latest)
	})
}

// ProxyChat forwards an OpenAI chat completion request to the spoke llama-server.
func (r *Runner) ProxyChat(ctx context.Context, test *fleetv1alpha1.FleetLlamaProfileTest, body []byte) ([]byte, int, error) {
	if test.Spec.Mode != fleetv1alpha1.LlamaProfileTestModeExtended {
		return nil, 0, fmt.Errorf("chat is only available for extended tests")
	}
	if test.Status.Phase != fleetv1alpha1.LlamaProfileTestPhaseReady {
		return nil, 0, fmt.Errorf("test is not Ready (phase %s)", test.Status.Phase)
	}
	if len(body) > maxChatBodyBytes {
		return nil, 0, fmt.Errorf("request body too large")
	}
	base := strings.TrimSuffix(strings.TrimSpace(test.Status.APIBase), "/")
	url := base + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if r.HTTP == nil {
		r.HTTP = &http.Client{Timeout: 120 * time.Second}
	}
	resp, err := r.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(io.LimitReader(resp.Body, maxChatBodyBytes))
	if err != nil {
		return nil, 0, err
	}
	return out, resp.StatusCode, nil
}

// EndTest deletes the FleetApp and test CR.
func (r *Runner) EndTest(ctx context.Context, testNS string, test *fleetv1alpha1.FleetLlamaProfileTest) error {
	r.cleanupApp(ctx, test)
	if err := r.Client.Delete(ctx, test); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func shortID(name string) string {
	name = strings.TrimSpace(name)
	if len(name) <= 8 {
		return name
	}
	return name[len(name)-8:]
}

// WaitForTerminal polls test status until phase is terminal or timeout.
func WaitForTerminal(ctx context.Context, c client.Client, ns, name string, interval, timeout time.Duration) (*fleetv1alpha1.FleetLlamaProfileTest, error) {
	deadline := time.Now().Add(timeout)
	for {
		var test fleetv1alpha1.FleetLlamaProfileTest
		if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &test); err != nil {
			return nil, err
		}
		switch test.Status.Phase {
		case fleetv1alpha1.LlamaProfileTestPhaseSucceeded, fleetv1alpha1.LlamaProfileTestPhaseFailed, fleetv1alpha1.LlamaProfileTestPhaseReady:
			return &test, nil
		}
		if time.Now().After(deadline) {
			return &test, fmt.Errorf("timeout waiting for test phase")
		}
		time.Sleep(interval)
	}
}

// DecodeChatBody validates JSON for chat proxy.
func DecodeChatBody(raw []byte) (map[string]interface{}, error) {
	var doc map[string]interface{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}
