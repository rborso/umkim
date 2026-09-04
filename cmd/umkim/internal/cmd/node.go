package cmd

import (
	"fmt"
	"net/http"

	"github.com/rborso/umkim/cmd/umkim/internal/output"
	"github.com/spf13/cobra"
)

func nodeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "node",
		Short: "Cluster node operations",
	}
	cmd.AddCommand(nodeListCmd())
	cmd.AddCommand(nodeAddCmd())
	cmd.AddCommand(nodeRemoveCmd())
	cmd.AddCommand(nodeReplaceCmd())
	return cmd
}

func nodeListCmd() *cobra.Command {
	var cluster string
	c := &cobra.Command{
		Use:   "list",
		Short: "List nodes in a cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cluster == "" {
				return fmt.Errorf("--cluster required")
			}
			raw, code, err := api.Get("/clusters/" + cluster + "/nodes")
			if err != nil {
				return err
			}
			if err := mustOK(raw, code); err != nil {
				return err
			}
			format, err := outputFormat()
			if err != nil {
				return err
			}
			return output.WriteNodes(raw, format)
		},
	}
	c.Flags().StringVar(&cluster, "cluster", "", "Cluster name")
	return c
}

func nodeAddCmd() *cobra.Command {
	var cluster, address, hostname, role string
	c := &cobra.Command{
		Use:   "add",
		Short: "Add a worker node (POST /clusters/{name}/nodes)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cluster == "" || address == "" {
				return fmt.Errorf("--cluster and --address required")
			}
			body := map[string]string{
				"address": address,
			}
			if hostname != "" {
				body["hostname"] = hostname
			}
			if role != "" {
				body["role"] = role
			}
			raw, code, err := api.Post("/clusters/"+cluster+"/nodes", body)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, http.StatusAccepted); err != nil {
				return err
			}
			printJSON(raw)
			return nil
		},
	}
	c.Flags().StringVar(&cluster, "cluster", "", "Cluster name")
	c.Flags().StringVar(&address, "address", "", "Node management address")
	c.Flags().StringVar(&hostname, "hostname", "", "Optional hostname")
	c.Flags().StringVar(&role, "role", "worker", "Node role (worker only)")
	return c
}

func nodeRemoveCmd() *cobra.Command {
	var cluster string
	var force bool
	c := &cobra.Command{
		Use:   "remove [node]",
		Short: "Remove a worker node",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cluster == "" {
				return fmt.Errorf("--cluster required")
			}
			q := map[string]string{}
			if force {
				q["force"] = "true"
			}
			raw, code, err := api.DeleteQuery("/clusters/"+cluster+"/nodes/"+args[0], q)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, http.StatusAccepted); err != nil {
				return err
			}
			printJSON(raw)
			return nil
		},
	}
	c.Flags().StringVar(&cluster, "cluster", "", "Cluster name")
	c.Flags().BoolVar(&force, "force", false, "Force remove")
	return c
}

func nodeReplaceCmd() *cobra.Command {
	var cluster, oldNode, address, hostname string
	var force bool
	c := &cobra.Command{
		Use:   "replace",
		Short: "Replace a control-plane node (POST /clusters/{name}/control-plane/replace)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cluster == "" || oldNode == "" || address == "" {
				return fmt.Errorf("--cluster, --old-node, and --address required")
			}
			body := map[string]interface{}{
				"oldNode": oldNode,
				"force":   force,
				"newNode": map[string]string{
					"address": address,
				},
			}
			if hostname != "" {
				body["newNode"].(map[string]string)["hostname"] = hostname
			}
			raw, code, err := api.Post("/clusters/"+cluster+"/control-plane/replace", body)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, http.StatusAccepted); err != nil {
				return err
			}
			printJSON(raw)
			return nil
		},
	}
	c.Flags().StringVar(&cluster, "cluster", "", "Cluster name")
	c.Flags().StringVar(&oldNode, "old-node", "", "Control-plane node name to replace (Kubernetes node name)")
	c.Flags().StringVar(&address, "address", "", "New node management address")
	c.Flags().StringVar(&hostname, "hostname", "", "New node hostname (optional)")
	c.Flags().BoolVar(&force, "force", false, "Force replace when old node is offline")
	return c
}
