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
	var kind, desiredState, ansibleHost, ansibleUser, osFamily, vmLocation string
	var file string
	c := &cobra.Command{
		Use:   "add [site]",
		Short: "Add a site host (POST /sites/{site}/hosts)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]interface{}{}
			if file != "" {
				if err := readFileJSON(file, &body); err != nil {
					return err
				}
			}
			if cmd.Flags().Changed("name") {
				body["name"] = cmd.Flag("name").Value.String()
			}
			if kind != "" {
				body["kind"] = kind
			}
			if cmd.Flags().Changed("desired-state") {
				body["desiredState"] = desiredState
			}
			if cmd.Flags().Changed("ansible-host") {
				body["ansible_host"] = ansibleHost
			}
			if cmd.Flags().Changed("ansible-user") {
				body["ansible_user"] = ansibleUser
			}
			if cmd.Flags().Changed("os-family") {
				body["osFamily"] = osFamily
			}
			if cmd.Flags().Changed("vm-location") {
				body["vm_location"] = vmLocation
			}
			mergeSiteHostClusterIntent(cmd, body)
			if body["name"] == nil || body["name"] == "" {
				return fmt.Errorf("--name required")
			}
			if body["kind"] == nil || body["kind"] == "" {
				return fmt.Errorf("--kind required (vm or bare_metal)")
			}
			raw, code, err := api.Post("/sites/"+args[0]+"/hosts", body)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, 201); err != nil {
				return err
			}
			printJSON(raw)
			return nil
		},
	}
	c.Flags().String("name", "", "Host name (DNS label)")
	c.Flags().StringVar(&kind, "kind", "", "vm or bare_metal")
	c.Flags().StringVar(&desiredState, "desired-state", "", "Desired state label")
	c.Flags().StringVar(&ansibleHost, "ansible-host", "", "Known ansible_host")
	c.Flags().StringVar(&ansibleUser, "ansible-user", "", "SSH user")
	c.Flags().StringVar(&osFamily, "os-family", "", "OS family (rocky, …)")
	c.Flags().StringVar(&vmLocation, "vm-location", "", "VM hypervisor/location")
	c.Flags().StringVarP(&file, "file", "f", "", "Host JSON body")
	siteHostClusterIntentFlags(c)
	return c
}

func siteHostPatchCmd() *cobra.Command {
	var file string
	var kind, desiredState, ansibleHost, ansibleUser, osFamily, vmLocation string
	c := &cobra.Command{
		Use:   "patch [site] [name]",
		Short: "Patch a site host",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]interface{}{}
			if file != "" {
				if err := readFileJSON(file, &body); err != nil {
					return err
				}
			}
			if cmd.Flags().Changed("kind") {
				body["kind"] = kind
			}
			if cmd.Flags().Changed("desired-state") {
				body["desiredState"] = desiredState
			}
			if cmd.Flags().Changed("ansible-host") {
				body["ansible_host"] = ansibleHost
			}
			if cmd.Flags().Changed("ansible-user") {
				body["ansible_user"] = ansibleUser
			}
			if cmd.Flags().Changed("os-family") {
				body["osFamily"] = osFamily
			}
			if cmd.Flags().Changed("vm-location") {
				body["vm_location"] = vmLocation
			}
			if len(body) == 0 {
				return fmt.Errorf("no patch fields: use --file or flags")
			}
			raw, code, err := api.Patch("/sites/"+args[0]+"/hosts/"+args[1], body)
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
	c.Flags().StringVarP(&file, "file", "f", "", "Patch JSON body")
	c.Flags().StringVar(&kind, "kind", "", "vm or bare_metal")
	c.Flags().StringVar(&desiredState, "desired-state", "", "Desired state label")
	c.Flags().StringVar(&ansibleHost, "ansible-host", "", "ansible_host")
	c.Flags().StringVar(&ansibleUser, "ansible-user", "", "SSH user")
	c.Flags().StringVar(&osFamily, "os-family", "", "OS family")
	c.Flags().StringVar(&vmLocation, "vm-location", "", "VM location")
	return c
}

func siteHostDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete [site] [name]",
		Short: "Delete a site host",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Delete("/sites/" + args[0] + "/hosts/" + args[1])
			if err != nil {
				return err
			}
			if code != 204 {
				return mustOK(raw, code)
			}
			return nil
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
	c.Flags().StringVar(&action, "action", "", "install_bm | create_vm | plan_vm | destroy | discover | status (default from kind)")
	c.Flags().BoolVar(&confirm, "confirm", false, "Required for install_bm / destroy")
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
