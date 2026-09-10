package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/rborso/umkim/cmd/umkim/internal/output"
	"github.com/spf13/cobra"
)

func siteHostCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "host",
		Short: "Manage hub site host inventory",
	}
	cmd.AddCommand(siteHostListCmd())
	cmd.AddCommand(siteHostGetCmd())
	cmd.AddCommand(siteHostAddCmd())
	cmd.AddCommand(siteHostPatchCmd())
	cmd.AddCommand(siteHostDeleteCmd())
	cmd.AddCommand(siteHostProvisionCmd())
	cmd.AddCommand(siteHostClusterApplyCmd())
	return cmd
}

func siteHostClusterIntentFlags(cmd *cobra.Command) {
	cmd.Flags().String("cluster-mode", "", "enroll | add_worker | replace_control")
	cmd.Flags().String("cluster-name", "", "New cluster name (enroll)")
	cmd.Flags().String("target-cluster", "", "Existing cluster (add_worker / replace_control)")
	cmd.Flags().String("cluster-topology", "", "single_node | ha_3_node (enroll)")
	cmd.Flags().String("ha-group", "", "HA group id for ha_3_node enroll")
	cmd.Flags().String("replace-node", "", "Control-plane node to replace")
	cmd.Flags().String("control-plane-vip", "", "kube-vip address for ha_3_node")
	cmd.Flags().Bool("provision-now", false, "Start provision immediately after add")
}

func mergeSiteHostClusterIntent(cmd *cobra.Command, body map[string]interface{}) {
	if cmd.Flags().Changed("provision-now") {
		if v, err := cmd.Flags().GetBool("provision-now"); err == nil {
			body["provisionNow"] = v
		}
	}
	if !cmd.Flags().Changed("cluster-mode") {
		return
	}
	intent := map[string]interface{}{"mode": cmd.Flag("cluster-mode").Value.String(), "autoApply": true}
	if v := cmd.Flag("cluster-name").Value.String(); v != "" {
		intent["clusterName"] = v
	}
	if v := cmd.Flag("target-cluster").Value.String(); v != "" {
		intent["targetCluster"] = v
	}
	if v := cmd.Flag("cluster-topology").Value.String(); v != "" {
		intent["topology"] = v
	}
	if v := cmd.Flag("ha-group").Value.String(); v != "" {
		intent["haGroup"] = v
	}
	if v := cmd.Flag("replace-node").Value.String(); v != "" {
		intent["replaceNode"] = v
	}
	if v := cmd.Flag("control-plane-vip").Value.String(); v != "" {
		intent["controlPlaneVip"] = v
	}
	body["clusterIntent"] = intent
}

func siteHostListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list [site]",
		Short: "List site hosts",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/sites/" + args[0] + "/hosts")
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
			return output.WriteSiteHosts(raw, format)
		},
	}
}

func siteHostGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get [site] [name]",
		Short: "Get a site host",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/sites/" + args[0] + "/hosts/" + args[1])
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

func siteHostAddCmd() *cobra.Command {
	c := &cobra.Command{
		Use:        "add [site]",
		Short:      "Add a site host (deprecated)",
		Deprecated: "use `umkim site inventory files put` for guests.yml and host_vars/*.yml",
		Args:       cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("site host add is deprecated; use `umkim site inventory files put` for guests.yml, host_vars/*.yml, {cluster}.yml, and group_vars/{cluster}.yml")
		},
	}
	return c
}

func siteHostPatchCmd() *cobra.Command {
	c := &cobra.Command{
		Use:        "patch [site] [name]",
		Short:      "Patch a site host (deprecated)",
		Deprecated: "use `umkim site inventory files put`",
		Args:       cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("site host patch is deprecated; edit guests.yml or host_vars/*.yml via `umkim site inventory files put`")
		},
	}
	return c
}

func siteHostDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:        "delete [site] [name]",
		Short:      "Delete a site host (deprecated)",
		Deprecated: "use `umkim site inventory files delete` for host_vars and edit guests.yml",
		Args:       cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("site host delete is deprecated; remove entries from guests.yml and delete host_vars/*.yml via inventory files API")
		},
	}
}

func siteHostProvisionCmd() *cobra.Command {
	var action string
	var confirm, wait bool
	c := &cobra.Command{
		Use:   "provision [site] [name]",
		Short: "Run provision action for a site host",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]interface{}{"confirm": confirm}
			if action != "" {
				body["action"] = action
			}
			raw, code, err := api.Post("/sites/"+args[0]+"/hosts/"+args[1]+"/provision", body)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, 200, 202); err != nil {
				return err
			}
			if !wait {
				printJSON(raw)
				return nil
			}
			return waitProvisionActionFromHostResponse(raw, args[0])
		},
	}
	c.Flags().StringVar(&action, "action", "", "install_bm | create_vm | plan_vm | destroy | discover | discover_pxe | status (default from kind)")
	c.Flags().BoolVar(&confirm, "confirm", false, "Required for install_bm / discover_pxe / destroy")
	c.Flags().BoolVar(&wait, "wait", false, "Poll until Succeeded or Failed")
	return c
}

func waitProvisionActionFromHostResponse(raw []byte, site string) error {
	var started map[string]interface{}
	if err := json.Unmarshal(raw, &started); err != nil {
		return err
	}
	id, _ := started["id"].(string)
	if id == "" {
		printJSON(raw)
		return fmt.Errorf("response missing action id")
	}
	return waitProvisionAction(site, id)
}

func siteHostClusterApplyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cluster-apply [site] [name]",
		Short: "Apply cluster intent for a Ready host",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Post("/sites/"+args[0]+"/hosts/"+args[1]+"/cluster", map[string]interface{}{})
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, 202); err != nil {
				return err
			}
			printJSON(raw)
			return nil
		},
	}
}
