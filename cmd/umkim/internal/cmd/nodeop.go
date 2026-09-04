package cmd

import (
	"fmt"

	"github.com/rborso/umkim/cmd/umkim/internal/output"
	"github.com/spf13/cobra"
)

func nodeOpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "node-op",
		Aliases: []string{"nodeop"},
		Short:   "Node operation history and logs",
	}
	cmd.AddCommand(nodeOpHistoryCmd())
	cmd.AddCommand(nodeOpLogsCmd())
	return cmd
}

func nodeOpHistoryCmd() *cobra.Command {
	var cluster string
	c := &cobra.Command{
		Use:   "history",
		Short: "List node operation history",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cluster == "" {
				return fmt.Errorf("--cluster required")
			}
			raw, code, err := api.Get("/clusters/" + cluster + "/node-operations")
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
			return output.WriteNodeOps(raw, format)
		},
	}
	c.Flags().StringVar(&cluster, "cluster", "", "Cluster name")
	return c
}

func nodeOpLogsCmd() *cobra.Command {
	var cluster, id string
	c := &cobra.Command{
		Use:   "logs",
		Short: "Fetch logs for a node operation",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cluster == "" || id == "" {
				return fmt.Errorf("--cluster and --id required")
			}
			raw, code, err := api.Get("/clusters/" + cluster + "/node-operations/" + id + "/logs")
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
	c.Flags().StringVar(&cluster, "cluster", "", "Cluster name")
	c.Flags().StringVar(&id, "id", "", "Operation ID")
	return c
}
