package reconcile

import (
	"context"
	"time"

	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"github.com/rborso/umkim/controllers/fleet-site/internal/resourcemetrics"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ResourceMetricsSync periodically scrapes spoke utilization and patches ManagedCluster labels.
type ResourceMetricsSync struct {
	Client   client.Client
	HubREST  *rest.Config
	Cfg      config.Config
	Interval time.Duration
	Enabled  bool
}

func (s *ResourceMetricsSync) Start(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("resource-metrics")
	if !s.Enabled {
		logger.Info("resource metrics sync disabled")
		<-ctx.Done()
		return nil
	}
	interval := s.Interval
	if interval <= 0 {
		interval = 60 * time.Second
	}
	scraper := &resourcemetrics.Scraper{
		HubClient:               s.Client,
		HubREST:                 s.HubREST,
		EdgeClustersCMNamespace: s.Cfg.EdgeClustersCMNamespace,
		EdgeClustersCMName:      s.Cfg.EdgeClustersCMName,
	}

	tick := func() {
		names, err := resourcemetrics.ListAvailableManagedClusters(ctx, s.Client)
		if err != nil {
			logger.Error(err, "list managed clusters")
			return
		}
		for _, name := range names {
			if err := resourcemetrics.ApplyLabelsToCluster(ctx, scraper, s.Client, name); err != nil {
				logger.Error(err, "sync cluster resource metrics", "cluster", name)
				continue
			}
			logger.V(1).Info("synced cluster resource metrics", "cluster", name)
		}
	}

	tick()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			tick()
		}
	}
}

// NeedLeaderElection ensures only one replica runs the sync loop.
func (s *ResourceMetricsSync) NeedLeaderElection() bool { return true }

// SetupResourceMetricsSync registers the periodic sync runnable with the manager.
func SetupResourceMetricsSync(mgr ctrl.Manager, hubREST *rest.Config, cfg config.Config, enabled bool, interval time.Duration) error {
	return mgr.Add(&ResourceMetricsSync{
		Client:   mgr.GetClient(),
		HubREST:  hubREST,
		Cfg:      cfg,
		Enabled:  enabled,
		Interval: interval,
	})
}
