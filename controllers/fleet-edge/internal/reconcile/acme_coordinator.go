package reconcile

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/config"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/traefik"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// acmeIssuer drives hub ACME issuance (solver or legacy gateway).
type acmeIssuer interface {
	fleetRoutesYAML(ctx context.Context) []byte
	discoverPending(ctx context.Context, prevRoutesYAML, routesYAML []byte) []string
	issueHostname(ctx context.Context, hostname string, routesYAML []byte, skipStaging bool) error
	validateDNS(ctx context.Context, hostname string) (ok bool, msg string)
	updateTLSPhase(ctx context.Context, hostname, phase, msg string) error
	onCertReady(ctx context.Context, hostname string) error
}

type hostQueueState struct {
	attempts    int
	nextRetry   time.Time
	permanent   bool
	certReady   bool
	lastClass   acmeErrorClass
}

// acmeCoordinator serializes hub ACME issuance with debounced batching.
type acmeCoordinator struct {
	mu               sync.Mutex
	running          bool
	issuing          atomic.Bool
	queue            []string
	hostState        map[string]*hostQueueState
	issuer           acmeIssuer
	cfg              config.Config
	retry            time.Duration
	debounce         time.Duration
	batchPreflightOK bool
	batchPreflightAt time.Time
}

// Issuing reports whether hub ACME issuance is active (gateway must stay up for TLS-ALPN).
func (c *acmeCoordinator) Issuing() bool {
	if c == nil {
		return false
	}
	return c.issuing.Load()
}

func newACMECoordinator(cfg config.Config, issuer acmeIssuer) *acmeCoordinator {
	retry := time.Duration(cfg.ACMEQueueRetryIntervalSec) * time.Second
	if retry < 30*time.Second {
		retry = 30 * time.Second
	}
	debounce := time.Duration(cfg.ACMEDebounceSec) * time.Second
	if debounce < 5*time.Second {
		debounce = 5 * time.Second
	}
	return &acmeCoordinator{
		issuer:    issuer,
		cfg:       cfg,
		retry:     retry,
		debounce:  debounce,
		hostState: map[string]*hostQueueState{},
	}
}

func (c *acmeCoordinator) Start(ctx context.Context) error {
	ticker := time.NewTicker(c.retry)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			c.rediscoverAndKick(ctx)
		}
	}
}

func (c *acmeCoordinator) rediscoverAndKick(ctx context.Context) {
	if !c.cfg.ACMEEnabled {
		return
	}
	routesYAML := c.issuer.fleetRoutesYAML(ctx)
	if len(routesYAML) == 0 {
		return
	}
	pending := c.issuer.discoverPending(ctx, nil, routesYAML)
	if len(pending) > 0 {
		c.enqueue(pending...)
	}
}

func (c *acmeCoordinator) enqueue(hostnames ...string) {
	if !c.cfg.ACMEEnabled || len(hostnames) == 0 {
		return
	}
	c.mu.Lock()
	for _, h := range hostnames {
		h = normalizeHostname(h)
		if h == "" || c.queueContainsLocked(h) {
			continue
		}
		st := c.hostState[h]
		if st != nil && st.permanent {
			continue
		}
		if st != nil && st.certReady {
			continue
		}
		if st != nil && !st.nextRetry.IsZero() && time.Now().Before(st.nextRetry) {
			continue
		}
		c.queue = append(c.queue, h)
		_ = c.issuer.updateTLSPhase(context.Background(), h, fleetv1alpha1.TLSPhaseQueued, "")
	}
	shouldStart := !c.running && len(c.queue) > 0
	if shouldStart {
		c.running = true
	}
	c.mu.Unlock()
	if shouldStart {
		go c.run()
	}
}

func (c *acmeCoordinator) queueContainsLocked(host string) bool {
	for _, q := range c.queue {
		if q == host {
			return true
		}
	}
	return false
}

func (c *acmeCoordinator) popLocked() string {
	if len(c.queue) == 0 {
		return ""
	}
	h := c.queue[0]
	c.queue = c.queue[1:]
	return h
}

func (c *acmeCoordinator) pushBackLocked(host string, delay time.Duration) {
	if host == "" || c.queueContainsLocked(host) {
		return
	}
	st := c.hostState[host]
	if st == nil {
		st = &hostQueueState{}
		c.hostState[host] = st
	}
	if delay > 0 {
		st.nextRetry = time.Now().Add(delay)
	}
	c.queue = append(c.queue, host)
}

func (c *acmeCoordinator) collectBatchLocked() []string {
	if len(c.queue) == 0 {
		return nil
	}
	deadline := time.Now().Add(c.debounce)
	for time.Now().Before(deadline) {
		c.mu.Unlock()
		time.Sleep(500 * time.Millisecond)
		c.mu.Lock()
	}
	var batch []string
	seen := map[string]struct{}{}
	for len(c.queue) > 0 {
		h := c.popLocked()
		if h == "" {
			break
		}
		st := c.hostState[h]
		if st != nil && st.permanent {
			continue
		}
		if st != nil && !st.nextRetry.IsZero() && time.Now().Before(st.nextRetry) {
			c.pushBackLocked(h, 0)
			continue
		}
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		batch = append(batch, h)
	}
	return batch
}

func (c *acmeCoordinator) run() {
	defer func() {
		if rec := recover(); rec != nil {
			log.Log.WithName("acme-queue").Error(fmt.Errorf("panic: %v", rec), "ACME worker panicked; will retry on next enqueue")
		}
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
	}()

	ctx := context.Background()
	logger := log.Log.WithName("acme-queue")

	for {
		c.mu.Lock()
		if len(c.queue) == 0 {
			c.mu.Unlock()
			return
		}
		batch := c.collectBatchLocked()
		c.mu.Unlock()
		if len(batch) == 0 {
			time.Sleep(c.retry)
			continue
		}

		skipStaging := false
		if c.batchPreflightOK && time.Since(c.batchPreflightAt) < 15*time.Minute {
			skipStaging = true
		}

		for _, host := range batch {
			if !hostnameNeedsProduction(ctx, effectiveGatewayProbeHost(c.cfg), host) {
				logger.Info("hostname already has production cert", "host", host)
				c.markCertReady(host)
				_ = c.issuer.updateTLSPhase(ctx, host, fleetv1alpha1.TLSPhaseReady, "")
				continue
			}

			if c.cfg.ACMEDNSCheckEnabled {
				ok, msg := c.issuer.validateDNS(ctx, host)
				if !ok {
					logger.Info("DNS pre-check failed; permanent skip", "host", host, "msg", msg)
					_ = c.issuer.updateTLSPhase(ctx, host, fleetv1alpha1.TLSPhaseFailedDNS, msg)
					c.mu.Lock()
					st := c.hostState[host]
					if st == nil {
						st = &hostQueueState{}
						c.hostState[host] = st
					}
					st.permanent = true
					st.lastClass = acmeErrorDNS
					c.mu.Unlock()
					continue
				}
			}

			c.issuing.Store(true)
			func() {
				defer c.issuing.Store(false)
				_ = c.issuer.updateTLSPhase(ctx, host, fleetv1alpha1.TLSPhaseIssuing, "")
				logger.Info("issuing ACME cert", "host", host)
				routesYAML := c.issuer.fleetRoutesYAML(ctx)
				issueCtx, cancel := context.WithTimeout(ctx, 25*time.Minute)
				err := c.issuer.issueHostname(issueCtx, host, routesYAML, skipStaging)
				cancel()
				if err != nil {
					class := classifyACMEError(err)
					logger.Error(err, "ACME issuance failed", "host", host, "class", class)
					c.mu.Lock()
					st := c.hostState[host]
					if st == nil {
						st = &hostQueueState{}
						c.hostState[host] = st
					}
					st.attempts++
					st.lastClass = class
					phase := tlsPhaseForClass(class)
					if class == acmeErrorDNS || st.attempts >= 5 {
						st.permanent = true
					}
					if phase == fleetv1alpha1.TLSPhaseFailedRateLimit && st.attempts >= 3 {
						st.permanent = true
					}
					delay := backoffForAttempt(class, st.attempts)
					st.nextRetry = time.Now().Add(delay)
					c.pushBackLocked(host, 0)
					c.mu.Unlock()
					_ = c.issuer.updateTLSPhase(ctx, host, phase, err.Error())
					return
				}
				if !skipStaging && c.cfg.ACMEPreflightEnabled {
					c.batchPreflightOK = true
					c.batchPreflightAt = time.Now()
					skipStaging = true
				}
				logger.Info("ACME cert ready", "host", host)
				c.markCertReady(host)
				_ = c.issuer.onCertReady(ctx, host)
			}()
			continue
		}
	}
}

func (c *acmeCoordinator) markCertReady(host string) {
	host = normalizeHostname(host)
	if host == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	st := c.hostState[host]
	if st == nil {
		st = &hostQueueState{}
		c.hostState[host] = st
	}
	st.certReady = true
	st.nextRetry = time.Time{}
}

// TLS phase helpers use fleetv1alpha1 constants.

func hostnameNeedsProduction(ctx context.Context, probeHost, host string) bool {
	ok, err := probeACMECert(probeHost, host, false)
	if err != nil {
		// Transient probe errors (gateway rollout, connection refused) must not re-queue issuance.
		return false
	}
	return !ok
}

func discoverPendingHostnames(ctx context.Context, probeHost string, prevRoutesYAML, routesYAML []byte) []string {
	if len(routesYAML) == 0 {
		return nil
	}
	hosts, err := traefik.HostnamesForResolver(routesYAML, traefik.CertResolverProduction)
	if err != nil {
		return traefik.NewHostnames(prevRoutesYAML, routesYAML)
	}
	seen := map[string]struct{}{}
	var pending []string
	add := func(h string) {
		h = normalizeHostname(h)
		if h == "" {
			return
		}
		if _, ok := seen[h]; ok {
			return
		}
		seen[h] = struct{}{}
		if hostnameNeedsProduction(ctx, probeHost, h) {
			pending = append(pending, h)
		}
	}
	for _, h := range traefik.NewHostnames(prevRoutesYAML, routesYAML) {
		add(h)
	}
	for _, h := range hosts {
		add(h)
	}
	return pending
}

func normalizeHostname(h string) string {
	return strings.ToLower(strings.TrimSpace(h))
}
