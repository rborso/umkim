package cmd

import (
	"github.com/spf13/cobra"
)

func acmeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "acme",
		Short: "Hub Let's Encrypt (ACME) settings",
	}
	cmd.AddCommand(acmeGetCmd())
	cmd.AddCommand(acmeSetCmd())
	return cmd
}

func acmeGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Get hub ACME settings (GET /acme)",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/acme")
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

func acmeSetCmd() *cobra.Command {
	var hubEnabled, staging bool
	var email string
	var hubEnabledSet bool
	c := &cobra.Command{
		Use:   "set",
		Short: "Patch hub ACME settings (PATCH /acme)",
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]interface{}{}
			if hubEnabledSet {
				body["hubEnabled"] = hubEnabled
			}
			if cmd.Flags().Changed("staging") {
				body["staging"] = staging
			}
			if cmd.Flags().Changed("email") {
				body["email"] = email
			}
			if len(body) == 0 {
				return errNoPatchFields
			}
			raw, code, err := api.Patch("/acme", body)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code); err != nil {
				return err
			}
			return printPatchOK("acme", raw)
		},
	}
	c.Flags().BoolVar(&hubEnabled, "hub-enabled", false, "Enable Let's Encrypt for hub apps")
	c.Flags().BoolVar(&staging, "staging", false, "Use ACME staging")
	c.Flags().StringVar(&email, "email", "", "ACME contact email")
	c.Flags().Lookup("hub-enabled").NoOptDefVal = "true"
	c.PreRunE = func(cmd *cobra.Command, args []string) error {
		hubEnabledSet = cmd.Flags().Changed("hub-enabled")
		return nil
	}
	return c
}
