package cmd

import (
	"fmt"

	"github.com/rborso/umkim/cmd/umkim/internal/output"
	"github.com/spf13/cobra"
)

func infraCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "infra",
		Short: "Hub infrastructure summary",
	}
	cmd.AddCommand(infraGetHubCmd())
	return cmd
}

func infraGetHubCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get hub",
		Short: "Show hub host and platform summary (GET /infrastructure/hub)",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/infrastructure/hub")
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
			return output.WriteHubInfrastructure(raw, format)
		},
	}
}

func printPatchOK(resource string, raw []byte) error {
	format, err := outputFormat()
	if err != nil {
		return err
	}
	if format == output.FormatJSON {
		printJSON(raw)
		return nil
	}
	fmt.Printf("OK: %s updated\n", resource)
	return nil
}
