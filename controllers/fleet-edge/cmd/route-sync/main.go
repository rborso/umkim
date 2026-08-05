package main

import (
	"flag"
	"os"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/config"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/routesync"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(fleetv1alpha1.AddToScheme(scheme))
}

func main() {
	var once bool
	flag.BoolVar(&once, "once", false, "Run one sync cycle and exit")
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	cfg := config.FromEnv()
	k8sClient, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		panic(err)
	}
	syncer := &routesync.Syncer{
		Client: k8sClient,
		Config: cfg,
		HTTP:   routesync.NewHTTPClient(),
	}
	ctx := ctrl.SetupSignalHandler()
	interval := time.Duration(cfg.RouteSyncIntervalSec) * time.Second
	if once {
		if err := syncer.RunOnce(ctx); err != nil {
			os.Exit(1)
		}
		return
	}
	for {
		if err := syncer.RunOnce(ctx); err != nil {
			ctrl.Log.WithName("route-sync").Error(err, "sync failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}
