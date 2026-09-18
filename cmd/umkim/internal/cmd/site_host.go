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
	cmd.AddCommand(siteHostProvisionCmd())
	cmd.AddCommand(siteHostDestroyCmd())
	cmd.AddCommand(siteHostClusterApplyCmd())
	return cmd
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
			raw, code, err := retryOnHTTP409(func() ([]byte, int, error) {
				return api.Post("/sites/"+args[0]+"/hosts/"+args[1]+"/provision", body)
			})
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

func siteHostDestroyCmd() *cobra.Command {
	var confirm, wait bool
	c := &cobra.Command{
		Use:   "destroy [site] [name]",
		Short: "Destroy libvirt VM for a site host (keeps inventory for reprovision)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !confirm {
				return fmt.Errorf("--confirm is required for destroy")
			}
			body := map[string]interface{}{
				"action":  "destroy",
				"confirm": true,
			}
			raw, code, err := retryOnHTTP409(func() ([]byte, int, error) {
				return api.Post("/sites/"+args[0]+"/hosts/"+args[1]+"/provision", body)
			})
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
	c.Flags().BoolVar(&confirm, "confirm", false, "Required: tear down libvirt VM while keeping guest in inventory")
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
