package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var errNoPatchFields = fmt.Errorf("no patch fields: specify at least one flag")

func clusterEdgeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ingress-auth",
		Short: "Per-cluster ingress authentication",
	}
	enable := clusterIngressAuthEnableCmd()
	cmd.AddCommand(enable)
	cmd.AddCommand(clusterIngressAuthDisableCmd())
	return cmd
}

func clusterAcmeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "acme",
		Short: "Per-cluster Let's Encrypt",
	}
	cmd.AddCommand(clusterAcmeEnableCmd())
	cmd.AddCommand(clusterAcmeDisableCmd())
	return cmd
}

func clusterIngressAuthEnableCmd() *cobra.Command {
	var password, passwordFile string
	var exposeWebUI bool
	c := &cobra.Command{
		Use:   "enable [cluster]",
		Short: "Enable ingress auth on a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pass := strings.TrimSpace(password)
			if passwordFile != "" {
				raw, err := os.ReadFile(passwordFile)
				if err != nil {
					return err
				}
				pass = strings.TrimSpace(string(raw))
			}
			if pass == "" {
				return fmt.Errorf("--lldap-password or --lldap-password-file required")
			}
			body := map[string]interface{}{
				"lldapAdminPassword": pass,
				"exposeLldapWebUI":   exposeWebUI,
			}
			raw, code, err := api.Post("/clusters/"+args[0]+"/ingress-auth/enable", body)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code); err != nil {
				return err
			}
			return printPatchOK("cluster "+args[0]+" ingress-auth", raw)
		},
	}
	c.Flags().StringVar(&password, "lldap-password", "", "LLDAP admin password (prefer --lldap-password-file)")
	c.Flags().StringVar(&passwordFile, "lldap-password-file", "", "File containing LLDAP admin password")
	c.Flags().BoolVar(&exposeWebUI, "expose-lldap-webui", false, "Expose LLDAP Web UI on cluster ingress")
	return c
}

func clusterIngressAuthDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable [cluster]",
		Short: "Disable ingress auth on a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Post("/clusters/"+args[0]+"/ingress-auth/disable", nil)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code); err != nil {
				return err
			}
			return printPatchOK("cluster "+args[0]+" ingress-auth", raw)
		},
	}
}

func clusterAcmeEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable [cluster]",
		Short: "Enable ACME on a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Post("/clusters/"+args[0]+"/acme/enable", nil)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code); err != nil {
				return err
			}
			return printPatchOK("cluster "+args[0]+" acme", raw)
		},
	}
}

func clusterAcmeDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable [cluster]",
		Short: "Disable ACME on a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Post("/clusters/"+args[0]+"/acme/disable", nil)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code); err != nil {
				return err
			}
			return printPatchOK("cluster "+args[0]+" acme", raw)
		},
	}
}

func clusterValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate [name]",
		Short: "Validate cluster intent (POST /clusters/{name}/validate)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Post("/clusters/"+args[0]+"/validate", nil)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code); err != nil {
				return err
			}
			if len(raw) > 0 {
				printJSON(raw)
			} else {
				fmt.Printf("OK: cluster %q validation passed\n", args[0])
			}
			return nil
		},
	}
}

func clusterDiscoverCmd() *cobra.Command {
	var wait time.Duration
	var interval time.Duration
	c := &cobra.Command{
		Use:   "discover [name]",
		Short: "Discover cluster capabilities (POST /discover-capabilities)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			raw, code, err := api.Post("/clusters/"+name+"/discover-capabilities", nil)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, http.StatusAccepted, http.StatusOK); err != nil {
				return err
			}
			if wait <= 0 {
				if len(raw) > 0 {
					printJSON(raw)
				} else {
					fmt.Printf("OK: capability discovery started for cluster %q\n", name)
				}
				return nil
			}
			deadline := time.Now().Add(wait)
			for time.Now().Before(deadline) {
				got, gerr := fetchClusterDiscoveryPhase(name)
				if gerr != nil {
					return gerr
				}
				switch got {
				case "Succeeded":
					raw, code, err := api.Get("/clusters/" + name)
					if err != nil {
						return err
					}
					if err := mustOK(raw, code); err != nil {
						return err
					}
					printJSON(raw)
					return nil
				case "Failed":
					return fmt.Errorf("capability discovery failed for cluster %q", name)
				}
				time.Sleep(interval)
			}
			return fmt.Errorf("timed out waiting for capability discovery on cluster %q", name)
		},
	}
	c.Flags().DurationVar(&wait, "wait", 0, "Wait for discovery to finish (e.g. 5m)")
	c.Flags().DurationVar(&interval, "interval", 5*time.Second, "Poll interval when --wait is set")
	return c
}

func fetchClusterDiscoveryPhase(name string) (string, error) {
	raw, code, err := api.Get("/clusters/" + name)
	if err != nil {
		return "", err
	}
	if err := mustOK(raw, code); err != nil {
		return "", err
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", err
	}
	disc, _ := doc["capability_discovery"].(map[string]interface{})
	if disc == nil {
		return "", nil
	}
	if p, ok := disc["phase"].(string); ok {
		return p, nil
	}
	return "", nil
}
