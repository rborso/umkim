package cmd

import (
	"fmt"

	"github.com/rborso/umkim/cmd/umkim/internal/output"
	"github.com/spf13/cobra"
)

func siteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "site",
		Short: "Manage FleetSite records on the hub",
	}
	var file string
	create := &cobra.Command{
		Use:   "create",
		Short: "Create a registry site (POST /sites)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return fmt.Errorf("--file required")
			}
			var body map[string]interface{}
			if err := readFileJSON(file, &body); err != nil {
				return err
			}
			raw, code, err := api.Post("/sites", body)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, 201, 409); err != nil {
				return err
			}
			printJSON(raw)
			return nil
		},
	}
	create.Flags().StringVarP(&file, "file", "f", "", "FleetSite JSON (apiVersion/kind/metadata/spec)")
	cmd.AddCommand(create)
	cmd.AddCommand(sitePatchCmd())
	cmd.AddCommand(siteIngressCmd())
	cmd.AddCommand(siteHostCmd())
	cmd.AddCommand(siteProvisionCmd())
	cmd.AddCommand(siteInventoryCmd())
	cmd.AddCommand(siteDeleteCmd())
	cmd.AddCommand(siteValidateCmd())
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List sites",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/sites")
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
			return output.WriteSites(raw, format)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "get [name]",
		Short: "Get a site",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/sites/" + args[0])
			if err != nil {
				return err
			}
			if err := mustOK(raw, code); err != nil {
				return err
			}
			printJSON(raw)
			return nil
		},
	})
	return cmd
}

func sitePatchCmd() *cobra.Command {
	var file, jumphost, region, failureDomain, displayName string
	c := &cobra.Command{
		Use:   "patch [name]",
		Short: "Patch site metadata (PATCH /sites/{name})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]interface{}{}
			if file != "" {
				if err := readFileJSON(file, &body); err != nil {
					return err
				}
			}
			if cmd.Flags().Changed("jumphost") {
				body["jumphost"] = jumphost
			}
			if cmd.Flags().Changed("region") {
				body["region"] = region
			}
			if cmd.Flags().Changed("failure-domain") {
				body["failureDomain"] = failureDomain
			}
			if cmd.Flags().Changed("display-name") {
				body["displayName"] = displayName
			}
			if len(body) == 0 {
				return fmt.Errorf("no patch fields: use --file or flags")
			}
			raw, code, err := api.Patch("/sites/"+args[0], body)
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
	c.Flags().StringVarP(&file, "file", "f", "", "JSON patch body")
	c.Flags().StringVar(&jumphost, "jumphost", "", "SSH jumphost (user@host:port); empty clears")
	c.Flags().StringVar(&region, "region", "", "Site region label")
	c.Flags().StringVar(&failureDomain, "failure-domain", "", "Failure domain label")
	c.Flags().StringVar(&displayName, "display-name", "", "Display name")
	return c
}

func siteDeleteCmd() *cobra.Command {
	var confirm bool
	c := &cobra.Command{
		Use:   "delete [name]",
		Short: "Delete a registry site (DELETE /sites/{name})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !confirm {
				return fmt.Errorf("--confirm required")
			}
			raw, code, err := api.Delete("/sites/" + args[0])
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, 200, 202, 204); err != nil {
				return err
			}
			fmt.Printf("OK: site %q deleted\n", args[0])
			return nil
		},
	}
	c.Flags().BoolVar(&confirm, "confirm", false, "Confirm site deletion")
	return c
}

func siteValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate [name]",
		Short: "Validate site intent (POST /sites/{name}/validate)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Post("/sites/"+args[0]+"/validate", nil)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code); err != nil {
				return err
			}
			if len(raw) > 0 {
				printJSON(raw)
			} else {
				fmt.Printf("OK: site %q validation passed\n", args[0])
			}
			return nil
		},
	}
}
