package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/rborso/umkim/cmd/umkim/internal/output"
	"github.com/spf13/cobra"
)

func appCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "app",
		Short: "Manage FleetApp Helm installs (console Apps page)",
	}
	cmd.AddCommand(appListCmd())
	cmd.AddCommand(appGetCmd())
	cmd.AddCommand(appCreateCmd())
	cmd.AddCommand(appDeleteCmd())
	return cmd
}

func appListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List Helm app installs",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/apps")
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
			return output.WriteApps(raw, format)
		},
	}
}

func appGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get [name]",
		Short: "Get a FleetApp install",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/apps/" + args[0])
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

func appCreateCmd() *cobra.Command {
	var (
		jsonFile, cluster, chart, version, release, namespace, valuesFile string
		routeMode, routeHost                                                string
	)
	c := &cobra.Command{
		Use:   "create",
		Short: "Install a Helm chart on a cluster (POST /apps)",
		Long: `Install a FleetApp on a Ready cluster (same as console Apps → Install).

Use -f app.json or flags. Route mode: none (default), hub, or site.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]interface{}{}
			if jsonFile != "" {
				if err := readFileJSON(jsonFile, &body); err != nil {
					return err
				}
			} else {
				if strings.TrimSpace(cluster) == "" || strings.TrimSpace(chart) == "" {
					return fmt.Errorf("--cluster and --chart required (or use -f app.json)")
				}
				body["clusterName"] = strings.TrimSpace(cluster)
				body["chart"] = strings.TrimSpace(chart)
				if version != "" {
					body["version"] = version
				}
				if release != "" {
					body["releaseName"] = release
				}
				if namespace != "" {
					body["namespace"] = namespace
				}
				if routeMode == "" {
					routeMode = "none"
				}
				switch routeMode {
				case "none", "hub", "site":
				default:
					return fmt.Errorf("--route-mode must be none, hub, or site")
				}
				route := map[string]interface{}{"mode": routeMode}
				if routeHost != "" {
					switch routeMode {
					case "hub":
						route["hubHost"] = routeHost
					case "site":
						route["siteHost"] = routeHost
					}
				}
				body["route"] = route
			}
			if valuesFile != "" {
				raw, err := os.ReadFile(valuesFile)
				if err != nil {
					return err
				}
				body["valuesYaml"] = string(raw)
			}
			raw, code, err := api.Post("/apps", body)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, 201, 200); err != nil {
				return err
			}
			format, ferr := outputFormat()
			if ferr != nil {
				return ferr
			}
			if format == output.FormatJSON {
				printJSON(raw)
				return nil
			}
			name := strings.TrimSpace(fmt.Sprint(body["releaseName"]))
			if name == "" {
				name = strings.TrimSpace(fmt.Sprint(body["name"]))
			}
			fmt.Printf("OK: app %q install requested on cluster %q\n", name, body["clusterName"])
			return nil
		},
	}
	c.Flags().StringVarP(&jsonFile, "file", "f", "", "JSON create body")
	c.Flags().StringVar(&cluster, "cluster", "", "Target cluster name")
	c.Flags().StringVar(&chart, "chart", "", "Chart name or oci:// URL")
	c.Flags().StringVar(&version, "version", "", "Chart version")
	c.Flags().StringVar(&release, "release", "", "Helm release name")
	c.Flags().StringVar(&namespace, "namespace", "default", "Target namespace")
	c.Flags().StringVar(&valuesFile, "values-file", "", "Helm values YAML file")
	c.Flags().StringVar(&routeMode, "route-mode", "none", "Route mode: none|hub|site")
	c.Flags().StringVar(&routeHost, "route-host", "", "Route hostname (hubHost or siteHost per mode)")
	return c
}

func appDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete [name]",
		Short: "Uninstall a FleetApp (DELETE /apps/{name})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Delete("/apps/" + args[0])
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, 202, 200, 204); err != nil {
				return err
			}
			format, ferr := outputFormat()
			if ferr != nil {
				return ferr
			}
			if format == output.FormatJSON {
				if len(raw) > 0 {
					printJSON(raw)
				}
				return nil
			}
			fmt.Printf("OK: app %q delete requested\n", args[0])
			return nil
		},
	}
}
