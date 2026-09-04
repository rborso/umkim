package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rborso/umkim/cmd/umkim/internal/output"
	"github.com/spf13/cobra"
)

func clusterPlatformCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "platform",
		Short: "Configure optional spoke platform add-ons (KubeVirt)",
	}
	cmd.AddCommand(clusterPlatformEnableKubevirtCmd())
	cmd.AddCommand(clusterPlatformStatusCmd())
	return cmd
}

func clusterPlatformEnableKubevirtCmd() *cobra.Command {
	var force, lanBridge, keepIP bool
	var wait time.Duration
	var bridgeName, slaveNic string
	c := &cobra.Command{
		Use:   "enable-kubevirt [cluster]",
		Short: "Install KubeVirt platform (POST /clusters/{name}/platform/kubevirt)",
		Long: `Cluster operation: install Multus, CDI, and KubeVirt on a Ready cluster.

Optional --lan-bridge applies the Multus NAD for host-bridge LAN VMs (requires the
Linux bridge already present on nodes, typically created at bootstrap).

Runs as an Ansible job on the hub (mutual exclusion with node/storage ops).
Completed runs appear in umkim node-op history as configureKubevirt.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.TrimSpace(args[0])
			body := map[string]interface{}{
				"lanBridge": lanBridge,
				"force":     force,
			}
			if lanBridge {
				if strings.TrimSpace(bridgeName) == "" {
					bridgeName = "n0p0_mgmt_nw"
				}
				body["bridgeName"] = bridgeName
				body["keepIp"] = keepIP
				if strings.TrimSpace(slaveNic) != "" {
					body["slaveNic"] = strings.TrimSpace(slaveNic)
				}
			}
			raw, code, err := api.Post("/clusters/"+name+"/platform/kubevirt", body)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, http.StatusAccepted, http.StatusOK); err != nil {
				return err
			}
			var resp map[string]interface{}
			_ = json.Unmarshal(raw, &resp)
			phase, _ := resp["phase"].(string)
			msg, _ := resp["message"].(string)
			if msg == "" {
				msg = "KubeVirt platform install accepted"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "OK: %s (phase=%s)\n", msg, phase)
			if wait <= 0 {
				output.WriteJSON(raw)
				return nil
			}
			deadline := time.Now().Add(wait)
			for {
				stRaw, stCode, stErr := api.Get("/clusters/" + name + "/platform/kubevirt")
				if stErr != nil {
					return stErr
				}
				if err := mustOK(stRaw, stCode); err != nil {
					return err
				}
				var st map[string]interface{}
				_ = json.Unmarshal(stRaw, &st)
				p, _ := st["phase"].(string)
				m, _ := st["message"].(string)
				fmt.Fprintf(cmd.OutOrStdout(), "status: phase=%s message=%s\n", p, m)
				if p == "Ready" {
					fmt.Fprintf(cmd.OutOrStdout(), "OK: KubeVirt Ready on %s\n", name)
					return nil
				}
				if p == "Failed" {
					return fmt.Errorf("KubeVirt install failed: %s", m)
				}
				if time.Now().After(deadline) {
					return fmt.Errorf("timed out waiting for KubeVirt Ready (last phase=%s)", p)
				}
				time.Sleep(10 * time.Second)
			}
		},
	}
	c.Flags().BoolVar(&force, "force", false, "Re-run install even if already Ready")
	c.Flags().BoolVar(&lanBridge, "lan-bridge", false, "Also apply Multus NAD for host Linux bridge (LAN VMs)")
	c.Flags().StringVar(&bridgeName, "bridge", "n0p0_mgmt_nw", "Host bridge device name when --lan-bridge")
	c.Flags().StringVar(&slaveNic, "slave", "", "Optional physical NIC for create-time bridge (documented; post-Ready NAD only)")
	c.Flags().BoolVar(&keepIP, "keep-ip", true, "keepIp for bridge intent (bootstrap); NAD path ignores live rewrite")
	c.Flags().DurationVar(&wait, "wait", 0, "Wait for Ready (e.g. 45m); 0 = return after accept")
	return c
}

func clusterPlatformStatusCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "status [cluster]",
		Short: "Show KubeVirt platform status (GET /clusters/{name}/platform/kubevirt)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.TrimSpace(args[0])
			raw, code, err := api.Get("/clusters/" + name + "/platform/kubevirt")
			if err != nil {
				return err
			}
			if err := mustOK(raw, code); err != nil {
				return err
			}
			format, ferr := outputFormat()
			if ferr != nil {
				return ferr
			}
			if format == output.FormatJSON {
				output.WriteJSON(raw)
				return nil
			}
			var st map[string]interface{}
			_ = json.Unmarshal(raw, &st)
			fmt.Fprintf(cmd.OutOrStdout(), "phase=%v install=%v lanBridge=%v message=%v\n",
				st["phase"], st["install"], st["lanBridge"], st["message"])
			return nil
		},
	}
	return c
}
