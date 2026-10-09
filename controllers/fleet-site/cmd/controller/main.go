package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"time"

	fleetedge "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"github.com/rborso/umkim/controllers/fleet-site/internal/httpapi"
	"github.com/rborso/umkim/controllers/fleet-site/internal/llamaprofiles"
	"github.com/rborso/umkim/controllers/fleet-site/internal/reconcile"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(fleetv1alpha1.AddToScheme(scheme))
	utilruntime.Must(fleetedge.AddToScheme(scheme))
}

func main() {
	var probeAddr string
	var apiAddr string
	var metricsAddr string
	var enableLeaderElection bool
	var resourceMetricsEnabled bool
	var resourceMetricsInterval time.Duration

	flag.StringVar(&probeAddr, "health-probe-bind-address", ":18081", "Health probe bind address.")
	flag.StringVar(&apiAddr, "api-bind-address", ":18080", "HTTP API bind address.")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":18082", "Metrics bind address.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", true, "Enable leader election.")
	flag.BoolVar(&resourceMetricsEnabled, "resource-metrics-enabled", true, "Sync cluster CPU/memory utilization to ManagedCluster labels.")
	flag.DurationVar(&resourceMetricsInterval, "resource-metrics-interval", 60*time.Second, "Interval for resource metrics label sync.")
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	restConfig, err := managerConfig()
	if err != nil {
		setupLog.Error(err, "kubeconfig")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "fleet-site-controller.umkim.com",
	})
	if err != nil {
		setupLog.Error(err, "manager")
		os.Exit(1)
	}

	cfg := config.FromEnv()
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		setupLog.Error(err, "kubernetes clientset")
		os.Exit(1)
	}
	rec := &reconcile.Reconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		Cfg:       cfg,
		Clientset: clientset,
	}
	if err := rec.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "controller")
		os.Exit(1)
	}
	if err := reconcile.SetupResourceMetricsSync(mgr, restConfig, cfg, resourceMetricsEnabled, resourceMetricsInterval); err != nil {
		setupLog.Error(err, "resource metrics sync")
		os.Exit(1)
	}

	if err := llamaprofiles.EnsureSystemProfiles(context.Background(), mgr.GetClient(), cfg.ConsoleNamespace); err != nil {
		setupLog.Error(err, "llama profile seed")
	}

	api := &httpapi.Server{Client: mgr.GetClient(), APIReader: mgr.GetAPIReader(), Cfg: cfg, RESTConfig: restConfig}
	go func() {
		setupLog.Info("starting HTTP API", "addr", apiAddr)
		if err := http.ListenAndServe(apiAddr, api.Handler()); err != nil {
			setupLog.Error(err, "HTTP API")
			os.Exit(1)
		}
	}()

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "healthz")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "readyz")
		os.Exit(1)
	}

	setupLog.Info("starting fleet-site-controller")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "manager stopped")
		os.Exit(1)
	}
}

func managerConfig() (*rest.Config, error) {
	if kc := os.Getenv("KUBECONFIG"); kc != "" {
		return clientcmd.BuildConfigFromFlags("", kc)
	}
	return ctrl.GetConfig()
}
