package cmd

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func siteInventoryFilesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "files",
		Short: "Manage YAIDAP inventory YAML files on the hub (native Ansible tree)",
	}
	cmd.AddCommand(siteInventoryFilesListCmd())
	cmd.AddCommand(siteInventoryFilesGetCmd())
	cmd.AddCommand(siteInventoryFilesPutCmd())
	cmd.AddCommand(siteInventoryFilesDeleteCmd())
	return cmd
}

func siteInventoryFilesListCmd() *cobra.Command {
	var asTree bool
	c := &cobra.Command{
		Use:   "list [site]",
		Short: "List inventory file paths",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/sites/" + args[0] + "/provision/inventory/files")
			if err != nil {
				return err
			}
			if err := mustOK(raw, code); err != nil {
				return err
			}
			if !asTree {
				printJSON(raw)
				return nil
			}
			var out struct {
				Items []struct {
					Path   string `json:"path"`
					Synced bool   `json:"synced"`
				} `json:"items"`
			}
			if err := decodeJSON(raw, &out); err != nil {
				return err
			}
			for _, item := range out.Items {
				depth := strings.Count(item.Path, "/")
				prefix := strings.Repeat("  ", depth)
				hub := ""
				if !item.Synced {
					hub = " (hub)"
				}
				fmt.Printf("%s%s%s\n", prefix, item.Path, hub)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&asTree, "tree", false, "Print paths indented by directory depth")
	return c
}

func siteInventoryFilesGetCmd() *cobra.Command {
	var path string
	var missingOK bool
	c := &cobra.Command{
		Use:   "get [site]",
		Short: "Fetch one inventory file body",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(path) == "" {
				return fmt.Errorf("--path required")
			}
			apiPath := "/sites/" + args[0] + "/provision/inventory/files/" + escapeInventoryPath(path)
			raw, code, err := api.Get(apiPath)
			if err != nil {
				return err
			}
			if code == 404 && missingOK {
				return nil
			}
			if err := mustOK(raw, code); err != nil {
				return err
			}
			fmt.Print(string(raw))
			return nil
		},
	}
	c.Flags().StringVar(&path, "path", "", "Inventory path (e.g. guests.yml, lab-ha01.yml, group_vars/lab-ha01.yml, host_vars/foo.yml)")
	c.Flags().BoolVar(&missingOK, "missing-ok", false, "Exit 0 with empty output when file is absent")
	return c
}

func siteInventoryFilesPutCmd() *cobra.Command {
	var path, file string
	c := &cobra.Command{
		Use:   "put [site]",
		Short: "Replace one inventory file (PUT /provision/inventory/files/{path})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(path) == "" {
				return fmt.Errorf("--path required")
			}
			var body []byte
			var err error
			if file == "" || file == "-" {
				body, err = io.ReadAll(os.Stdin)
			} else {
				body, err = os.ReadFile(file)
			}
			if err != nil {
				return err
			}
			apiPath := "/sites/" + args[0] + "/provision/inventory/files/" + escapeInventoryPath(path)
			raw, code, err := api.PutBytes(apiPath, "text/yaml; charset=utf-8", body)
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
	c.Flags().StringVar(&path, "path", "", "Inventory path")
	c.Flags().StringVarP(&file, "file", "f", "", "File to upload (- for stdin)")
	return c
}

func siteInventoryFilesDeleteCmd() *cobra.Command {
	var path string
	c := &cobra.Command{
		Use:   "delete [site]",
		Short: "Delete one inventory file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(path) == "" {
				return fmt.Errorf("--path required")
			}
			apiPath := "/sites/" + args[0] + "/provision/inventory/files/" + escapeInventoryPath(path)
			raw, code, err := api.Delete(apiPath)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, 204); err != nil {
				return err
			}
			return nil
		},
	}
	c.Flags().StringVar(&path, "path", "", "Inventory path")
	return c
}

func escapeInventoryPath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}
