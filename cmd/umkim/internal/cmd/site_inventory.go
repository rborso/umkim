package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func siteInventoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inventory",
		Short: "Connector-owned provisioner inventory (YAIDAP / tofu)",
	}
	cmd.AddCommand(siteInventoryGetCmd())
	cmd.AddCommand(siteInventoryRevisionsCmd())
	cmd.AddCommand(siteInventorySyncCmd())
	cmd.AddCommand(siteInventoryFilesCmd())
	cmd.AddCommand(siteInventoryMigrateCmd())
	cmd.AddCommand(siteInventoryImportCmd())
	return cmd
}

func siteInventoryImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import YAIDAP inventory from the site jumphost",
	}
	cmd.AddCommand(siteInventoryImportPreviewCmd())
	cmd.AddCommand(siteInventoryImportApplyCmd())
	return cmd
}

func siteInventoryGetCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "get [site]",
		Short: "Get provisioner inventory envelope",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/sites/" + args[0] + "/provision/inventory")
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
	return c
}

func siteInventoryRevisionsCmd() *cobra.Command {
	var limit int
	var id string
	c := &cobra.Command{
		Use:   "revisions [site]",
		Short: "List inventory revisions or fetch one diff",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "/sites/" + args[0] + "/provision/inventory/revisions"
			if id != "" {
				path += "/" + id
			} else if limit > 0 {
				path += fmt.Sprintf("?limit=%d", limit)
			}
			raw, code, err := api.Get(path)
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
	c.Flags().IntVar(&limit, "limit", 20, "Revision list limit")
	c.Flags().StringVar(&id, "id", "", "Fetch one revision diff by id")
	return c
}

func siteInventorySyncCmd() *cobra.Command {
	var wait bool
	c := &cobra.Command{
		Use:   "sync [site]",
		Short: "Push connector inventory to jumphost",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := retryOnHTTP409(func() ([]byte, int, error) {
				return api.Post("/sites/"+args[0]+"/provision/inventory/sync", map[string]interface{}{})
			})
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, 202); err != nil {
				return err
			}
			printJSON(raw)
			if wait {
				for i := 0; i < 60; i++ {
					env, c2, err := api.Get("/sites/" + args[0] + "/provision/inventory")
					if err != nil {
						return err
					}
					if err := mustOK(env, c2); err != nil {
						return err
					}
					var meta struct {
						Meta struct {
							SyncPhase string `json:"syncPhase"`
						} `json:"meta"`
					}
					_ = json.Unmarshal([]byte(env), &meta)
					if meta.Meta.SyncPhase == "synced" {
						printJSON(env)
						return nil
					}
					time.Sleep(5 * time.Second)
				}
				return fmt.Errorf("timed out waiting for inventory sync")
			}
			return nil
		},
	}
	c.Flags().BoolVar(&wait, "wait", false, "Poll until syncPhase=synced")
	return c
}
