package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func siteInventoryImportPreviewCmd() *cobra.Command {
	var wait bool
	c := &cobra.Command{
		Use:   "preview [site]",
		Short: "Pull jumphost inventory and show per-file diff (no hub write)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Post("/sites/"+args[0]+"/provision/inventory/import/preview", map[string]interface{}{})
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, 202); err != nil {
				return err
			}
			printJSON(raw)
			if !wait {
				return nil
			}
			for i := 0; i < 60; i++ {
				prev, c2, err := api.Get("/sites/" + args[0] + "/provision/inventory/import/preview")
				if err != nil {
					return err
				}
				if err := mustOK(prev, c2); err != nil {
					return err
				}
				var out struct {
					Items []interface{} `json:"items"`
				}
				_ = json.Unmarshal(prev, &out)
				if len(out.Items) > 0 {
					printJSON(prev)
					return nil
				}
				time.Sleep(5 * time.Second)
			}
			return fmt.Errorf("timed out waiting for import preview")
		},
	}
	c.Flags().BoolVar(&wait, "wait", false, "Poll until preview items are available")
	return c
}

func siteInventoryImportApplyCmd() *cobra.Command {
	var paths string
	var confirm bool
	c := &cobra.Command{
		Use:   "apply [site]",
		Short: "Apply selected import preview paths to hub inventory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !confirm {
				return fmt.Errorf("--confirm required")
			}
			body := map[string]interface{}{"confirm": true}
			if strings.TrimSpace(paths) != "" {
				var list []string
				for _, p := range strings.Split(paths, ",") {
					if v := strings.TrimSpace(p); v != "" {
						list = append(list, v)
					}
				}
				body["paths"] = list
			}
			raw, code, err := api.Post("/sites/"+args[0]+"/provision/inventory/import/apply", body)
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
	c.Flags().StringVar(&paths, "paths", "", "Comma-separated paths to merge (default: all changed)")
	c.Flags().BoolVar(&confirm, "confirm", false, "Confirm merge")
	return c
}

func siteInventoryMigrateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "migrate [site]",
		Short: "Migrate legacy JSON/site-host inventory to YAML files",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Post("/sites/"+args[0]+"/provision/inventory/migrate", map[string]interface{}{})
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
