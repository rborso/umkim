package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func clusterGpuCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gpu",
		Short: "GPU host driver and runtime on a cluster",
	}
	cmd.AddCommand(clusterGpuStatusCmd())
	cmd.AddCommand(clusterGpuHostPrepCmd())
	cmd.AddCommand(clusterGpuEnableCmd())
	cmd.AddCommand(clusterGpuDiscoverCmd())
	return cmd
}

func clusterGpuStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status NAME",
		Short: "GPU hardware and cluster GPU capability (GET /clusters/NAME/gpu)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/clusters/" + args[0] + "/gpu")
			if err != nil {
				return err
			}
			if err := mustOK(raw, code); err != nil {
				return err
			}
			printJSON(raw)
			return nil
		},
	}
}

func clusterGpuHostPrepCmd() *cobra.Command {
	var force bool
	var wait string
	c := &cobra.Command{
		Use:   "host-prep NAME",
		Short: "Install NVIDIA host driver on cluster nodes (POST /clusters/NAME/gpu/host-prep)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]interface{}{}
			if force {
				body["force"] = true
			}
			raw, code, err := api.Post("/clusters/"+args[0]+"/gpu/host-prep", body)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code); err != nil {
				return err
			}
			if wait == "" {
				return printPatchOK("gpu host-prep", raw)
			}
			d, err := time.ParseDuration(wait)
			if err != nil {
				return fmt.Errorf("invalid --wait: %w", err)
			}
			deadline := time.Now().Add(d)
			sawApplying := false
			for time.Now().Before(deadline) {
				st, code, err := api.Get("/clusters/" + args[0] + "/gpu")
				if err != nil {
					return err
				}
				if code < 200 || code >= 300 {
					return fmt.Errorf("gpu status HTTP %d", code)
				}
				var doc map[string]interface{}
				if err := json.Unmarshal(st, &doc); err != nil {
					return err
				}
				phase, _ := doc["hostPhase"].(string)
				if phase == "Applying" {
					sawApplying = true
				}
				if phase == "Ready" {
					printJSON(st)
					return nil
				}
				if phase == "Failed" && sawApplying {
					return fmt.Errorf("GPU host prep failed: %v", doc["hostMessage"])
				}
				time.Sleep(15 * time.Second)
			}
			return fmt.Errorf("timeout waiting for GPU host prep")
		},
	}
	c.Flags().BoolVar(&force, "force", false, "Allow host prep on a Ready cluster (post-bootstrap repair)")
	c.Flags().StringVar(&wait, "wait", "", "Wait for host prep to finish (e.g. 45m)")
	return c
}

func clusterGpuEnableCmd() *cobra.Command {
	var wait string
	c := &cobra.Command{
		Use:   "enable NAME",
		Short: "Enable NVIDIA container runtime and device plugin (POST /clusters/NAME/gpu/enable)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Post("/clusters/"+args[0]+"/gpu/enable", map[string]interface{}{})
			if err != nil {
				return err
			}
			if err := mustOK(raw, code); err != nil {
				return err
			}
			if wait == "" {
				return printPatchOK("gpu enable", raw)
			}
			d, err := time.ParseDuration(wait)
			if err != nil {
				return fmt.Errorf("invalid --wait: %w", err)
			}
			deadline := time.Now().Add(d)
			sawApplying := false
			for time.Now().Before(deadline) {
				st, code, err := api.Get("/clusters/" + args[0] + "/gpu")
				if err != nil {
					return err
				}
				if code < 200 || code >= 300 {
					return fmt.Errorf("gpu status HTTP %d", code)
				}
				var doc map[string]interface{}
				if err := json.Unmarshal(st, &doc); err != nil {
					return err
				}
				phase, _ := doc["runtimePhase"].(string)
				if phase == "Applying" {
					sawApplying = true
				}
				if phase == "Ready" {
					printJSON(st)
					return nil
				}
				if phase == "Failed" && sawApplying {
					return fmt.Errorf("GPU runtime enable failed: %v", doc["runtimeMessage"])
				}
				time.Sleep(15 * time.Second)
			}
			return fmt.Errorf("timeout waiting for GPU runtime enable")
		},
	}
	c.Flags().StringVar(&wait, "wait", "", "Wait for runtime enable to finish (e.g. 30m)")
	return c
}

func clusterGpuDiscoverCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "discover NAME",
		Short: "Re-run capability discovery (POST /clusters/NAME/gpu/discover)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Post("/clusters/"+args[0]+"/gpu/discover", map[string]interface{}{})
			if err != nil {
				return err
			}
			if err := mustOK(raw, code); err != nil {
				return err
			}
			printJSON(raw)
			return nil
		},
	}
}
