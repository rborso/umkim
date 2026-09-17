package cmd

import (
	"github.com/spf13/cobra"
)

func llmCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "llm",
		Short: "Hub LiteLLM proxy settings",
	}
	cmd.AddCommand(llmGetCmd())
	cmd.AddCommand(llmSetCmd())
	return cmd
}

func llmGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Get hub LLM settings (GET /llm)",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/llm")
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

func llmSetCmd() *cobra.Command {
	var hubEnabled bool
	var publicHost string
	var hubEnabledSet bool
	c := &cobra.Command{
		Use:   "set",
		Short: "Patch hub LLM settings (PATCH /llm)",
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]interface{}{}
			if hubEnabledSet {
				body["hubEnabled"] = hubEnabled
			}
			if cmd.Flags().Changed("public-host") {
				body["publicHost"] = publicHost
			}
			if len(body) == 0 {
				return errNoPatchFields
			}
			raw, code, err := api.Patch("/llm", body)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code); err != nil {
				return err
			}
			return printPatchOK("llm", raw)
		},
	}
	c.Flags().BoolVar(&hubEnabled, "hub-enabled", false, "Enable hub LiteLLM proxy")
	c.Flags().StringVar(&publicHost, "public-host", "", "Public host for LLM API")
	c.Flags().Lookup("hub-enabled").NoOptDefVal = "true"
	c.PreRunE = func(cmd *cobra.Command, args []string) error {
		hubEnabledSet = cmd.Flags().Changed("hub-enabled")
		return nil
	}
	return c
}
