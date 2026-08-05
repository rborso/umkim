package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	authv1 "github.com/rborso/umkim/controllers/fleet-console-auth/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/audit"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/bootstrap"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/authorizer"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/catalogacl"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/config"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/httpapi"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/identity"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/rbac"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/session"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/tokens"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(authv1.AddToScheme(scheme))
}

func main() {
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	cfg := config.FromEnv()
	restConfig, err := restConfigFromEnv()
	if err != nil {
		panic(err)
	}
	k8sClient, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		panic(err)
	}

	var providers []identity.IdentityProvider
	if cfg.HtpasswdLine != "" {
		providers = append(providers, identity.NewHtpasswdEnvProvider(cfg.HtpasswdLine, cfg.HtpasswdPath))
	} else if cfg.HtpasswdPath != "" {
		providers = append(providers, identity.NewHtpasswdProvider(cfg.HtpasswdPath))
	}
	if cfg.LDAPURL != "" {
		providers = append(providers, identity.NewLDAPProvider(
			cfg.LDAPURL, cfg.LDAPBaseDN, cfg.LDAPBindDN, cfg.LDAPBindPassword,
			cfg.LDAPUserFilter, cfg.LDAPGroupFilter,
		))
	}
	authn := identity.NewAuthenticator(providers...)
	oidc := identity.NewOIDCStub(cfg.OIDCIssuer, cfg.OIDCClientID, cfg.OIDCClientSecret, cfg.OIDCRedirectURL)
	sess := session.NewManager(cfg.SessionSecret, cfg.SessionTTL, cfg.SessionCookieName, cfg.SessionSecure)
	rbacEngine := &rbac.Engine{Client: k8sClient, Namespace: cfg.Namespace}
	checker := &authorizer.Checker{RBAC: rbacEngine, Client: k8sClient}
	auditRec := &audit.Recorder{Client: k8sClient, Namespace: cfg.Namespace}
	catalog := &catalogacl.Manager{Client: k8sClient, Namespace: cfg.Namespace, RBAC: rbacEngine}
	tokenMgr := &tokens.Manager{Client: k8sClient, Namespace: cfg.Namespace}

	ctx := context.Background()
	if err := bootstrap.Run(ctx, k8sClient, bootstrap.Config{
		Namespace:            cfg.Namespace,
		BootstrapAdminUser:   cfg.BootstrapAdminUser,
		BootstrapLDAPGroup:   cfg.BootstrapLDAPGroup,
		BootstrapLDAPGroupCN: cfg.BootstrapLDAPGroupCN,
	}); err != nil {
		ctrl.Log.WithName("bootstrap").Error(err, "bootstrap failed")
	}

	srv := &httpapi.Server{
		Client:     k8sClient,
		Cfg:        cfg,
		Sessions:   sess,
		Auth:       authn,
		OIDC:       oidc,
		RBAC:       rbacEngine,
		Checker:    checker,
		Audit:      auditRec,
		CatalogACL: catalog,
		Tokens:     tokenMgr,
	}

	httpSrv := &http.Server{
		Addr:              cfg.APIBindAddress,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		ctrl.Log.WithName("auth").Info("listening", "addr", cfg.APIBindAddress)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
}

func restConfigFromEnv() (*rest.Config, error) {
	if kc := os.Getenv("KUBECONFIG"); kc != "" {
		return clientcmd.BuildConfigFromFlags("", kc)
	}
	return rest.InClusterConfig()
}
