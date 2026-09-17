package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"

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
	cmd.AddCommand(nodeOpValidateCmd())
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

func nodeOpValidateCmd() *cobra.Command {
	var cluster, jsonFile, phase, opType string
	c := &cobra.Command{
		Use:   "validate",
		Short: "Validate a node operation before or after run",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cluster == "" {
				return fmt.Errorf("--cluster required")
			}
			var body map[string]interface{}
			if jsonFile != "" {
				if err := readFileJSON(jsonFile, &body); err != nil {
					return err
				}
			} else if opType != "" {
				body = map[string]interface{}{"type": opType}
			} else {
				return fmt.Errorf("--file or --type required")
			}
			path := "/clusters/" + cluster + "/node-operations/validate"
			if phase == "post" {
				path += "?phase=post"
			}
			raw, code, err := api.Post(path, body)
			if err != nil {
				return err
			}
			if code != http.StatusOK && code != http.StatusUnprocessableEntity {
				if err := mustOK(raw, code); err != nil {
					return err
				}
			}
			var result struct {
				OK     bool                     `json:"ok"`
				Checks []map[string]interface{} `json:"checks"`
			}
			if err := json.Unmarshal(raw, &result); err != nil {
				printJSON(raw)
				return nil
			}
			format, ferr := outputFormat()
			if ferr != nil {
				return ferr
			}
			if format == output.FormatJSON {
				printJSON(raw)
			} else {
				for _, chk := range result.Checks {
					id := fmt.Sprint(chk["id"])
					if id == "" || id == "<nil>" {
						id = fmt.Sprint(chk["name"])
					}
					status := fmt.Sprint(chk["status"])
					if status == "" || status == "<nil>" {
						if pass, _ := chk["ok"].(bool); pass {
							status = "pass"
						} else if _, exists := chk["ok"].(bool); exists {
							status = "fail"
						}
					}
					fmt.Printf("%s\t%s\t%s\n", id, status, chk["message"])
				}
			}
			if !result.OK {
				return fmt.Errorf("node operation validation failed")
			}
			return nil
		},
	}
	c.Flags().StringVar(&cluster, "cluster", "", "Cluster name")
	c.Flags().StringVarP(&jsonFile, "file", "f", "", "Validate request JSON body")
	c.Flags().StringVar(&phase, "phase", "pre", "Validation phase: pre|post")
	c.Flags().StringVar(&opType, "type", "", "Operation type when not using --file")
	return c
}
