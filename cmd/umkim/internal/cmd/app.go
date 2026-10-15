package cmd

import (
	"encoding/json"
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
	cmd.AddCommand(appUpdateCmd())
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

func fetchLlamaProfileYAML(profileID string) (string, error) {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return "", fmt.Errorf("empty llama profile id")
	}
	raw, code, err := api.Get("/llama-profiles/" + profileID + "/values")
	if err != nil {
		return "", err
	}
	if err := mustOK(raw, code); err != nil {
		return "", err
	}
	var doc struct {
		ValuesYAML string `json:"valuesYaml"`
		Name       string `json:"name"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", err
	}
	if strings.TrimSpace(doc.ValuesYAML) == "" {
		return "", fmt.Errorf("profile %q returned empty valuesYaml", profileID)
	}
	return doc.ValuesYAML, nil
}

func applyOptionalLLMBackend(body map[string]interface{}, opt string) error {
	opt = strings.TrimSpace(strings.ToLower(opt))
	if opt == "" {
		return nil
	}
	switch opt {
	case "true", "1", "yes":
		body["llmBackend"] = true
	case "false", "0", "no":
		body["llmBackend"] = false
	default:
		return fmt.Errorf("--llm-backend must be true or false")
	}
	return nil
}

func appCreateCmd() *cobra.Command {
	var (
		jsonFile, cluster, chart, version, release, namespace, valuesFile, appName string
		routeMode, routeHost                                                       string
		llamaProfile, llmBackendOpt, oidcClient, requireAuthGroups                 string
		routeServiceName                                                           string
		routeServicePort                                                           int
		routeProtocol                                                              string
		routeForwardPort                                                           int
		requireAuth                                                                bool
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
				if appName != "" {
					body["name"] = strings.TrimSpace(appName)
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
				if requireAuth {
					route["requireAuth"] = true
				}
				if routeServiceName != "" {
					route["serviceName"] = routeServiceName
				}
				if routeServicePort > 0 {
					route["servicePort"] = routeServicePort
				}
				if strings.TrimSpace(routeProtocol) != "" {
					route["protocol"] = strings.TrimSpace(routeProtocol)
				}
				if routeForwardPort > 0 {
					route["forwardPort"] = routeForwardPort
				}
				if strings.TrimSpace(requireAuthGroups) != "" {
					var groups []string
					for _, g := range strings.Split(requireAuthGroups, ",") {
						if g = strings.TrimSpace(g); g != "" {
							groups = append(groups, g)
						}
					}
					if len(groups) > 0 {
						route["requireAuthGroups"] = groups
					}
				} else if cmd.Flags().Changed("require-auth-groups") {
					// An explicit empty value clears the restriction; omitting the
					// field would leave it in place with no way to remove it.
					route["requireAuthGroups"] = []string{}
				}
				body["route"] = route
			}
			if llamaProfile != "" && valuesFile != "" {
				return fmt.Errorf("use either --llama-profile or --values-file, not both")
			}
			if llamaProfile != "" {
				yaml, err := fetchLlamaProfileYAML(llamaProfile)
				if err != nil {
					return err
				}
				body["valuesYaml"] = yaml
			}
			if valuesFile != "" {
				raw, err := os.ReadFile(valuesFile)
				if err != nil {
					return err
				}
				body["valuesYaml"] = string(raw)
			}
			if err := applyOptionalLLMBackend(body, llmBackendOpt); err != nil {
				return err
			}
			if strings.TrimSpace(oidcClient) != "" {
				var oc map[string]interface{}
				if err := json.Unmarshal([]byte(oidcClient), &oc); err != nil {
					return fmt.Errorf("--oidc-client must be JSON: %w", err)
				}
				body["oidcClient"] = oc
				if cid, _ := oc["clientId"].(string); cid != "" {
					body["oidcClientId"] = cid
				}
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
	c.Flags().StringVar(&appName, "name", "", "FleetApp resource name (defaults to release)")
	c.Flags().StringVar(&namespace, "namespace", "default", "Target namespace")
	c.Flags().StringVar(&valuesFile, "values-file", "", "Helm values YAML file")
	c.Flags().StringVar(&oidcClient, "oidc-client", "", "Register an Authelia OIDC client (JSON: {clientId,redirectPaths,...})")
	c.Flags().StringVar(&llamaProfile, "llama-profile", "", "Built-in llama-cpp-helm values profile id (hub API)")
	c.Flags().StringVar(&llmBackendOpt, "llm-backend", "", "Llama-cpp only: register hub LiteLLM backend when Ready (true|false)")
	c.Flags().StringVar(&routeMode, "route-mode", "none", "Route mode: none|hub|site")
	c.Flags().StringVar(&routeHost, "route-host", "", "Route hostname (hubHost or siteHost per mode)")
	c.Flags().BoolVar(&requireAuth, "require-auth", false, "Gate the route behind Authelia forward-auth")
	c.Flags().StringVar(&requireAuthGroups, "require-auth-groups", "", "Comma-separated directory groups allowed through the route")
	c.Flags().StringVar(&routeServiceName, "service-name", "", "Backend Service name for the route (defaults to the release name)")
	c.Flags().IntVar(&routeServicePort, "service-port", 0, "Backend Service port for the route (defaults to 80)")
	c.Flags().StringVar(&routeProtocol, "route-protocol", "", "Route protocol: http (default) or tcp (hub L4 port forward)")
	c.Flags().IntVar(&routeForwardPort, "route-forward-port", 0, "Hub port to listen on for a tcp route")
	return c
}

func appUpdateCmd() *cobra.Command {
	var version, valuesFile, namespace, routeMode, routeHost, llmBackendOpt, requireAuthGroups, routeServiceName, routeProtocol string
	var requireAuth bool
	var routeForwardPort int
	var routeServicePort int
	c := &cobra.Command{
		Use:   "update [name]",
		Short: "Update a FleetApp install (PATCH /apps/{name})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]interface{}{}
			if version != "" {
				body["version"] = version
			}
			if namespace != "" {
				body["namespace"] = namespace
			}
			if valuesFile != "" {
				raw, err := os.ReadFile(valuesFile)
				if err != nil {
					return err
				}
				body["valuesYaml"] = string(raw)
			}
			if routeMode != "" || routeHost != "" || requireAuth || strings.TrimSpace(requireAuthGroups) != "" || cmd.Flags().Changed("require-auth-groups") || routeServiceName != "" || routeServicePort > 0 || strings.TrimSpace(routeProtocol) != "" || routeForwardPort > 0 {
				route := map[string]interface{}{}
				if routeMode != "" {
					switch routeMode {
					case "none", "hub", "site":
					default:
						return fmt.Errorf("--route-mode must be none, hub, or site")
					}
					route["mode"] = routeMode
				}
				if routeHost != "" {
					switch routeMode {
					case "hub", "":
						if routeMode == "" {
							route["hubHost"] = routeHost
						} else if routeMode == "hub" {
							route["hubHost"] = routeHost
						}
					case "site":
						route["siteHost"] = routeHost
					}
				}
				if requireAuth {
					route["requireAuth"] = true
				}
				if routeServiceName != "" {
					route["serviceName"] = routeServiceName
				}
				if routeServicePort > 0 {
					route["servicePort"] = routeServicePort
				}
				if strings.TrimSpace(routeProtocol) != "" {
					route["protocol"] = strings.TrimSpace(routeProtocol)
				}
				if routeForwardPort > 0 {
					route["forwardPort"] = routeForwardPort
				}
				if strings.TrimSpace(requireAuthGroups) != "" {
					var groups []string
					for _, g := range strings.Split(requireAuthGroups, ",") {
						if g = strings.TrimSpace(g); g != "" {
							groups = append(groups, g)
						}
					}
					if len(groups) > 0 {
						route["requireAuthGroups"] = groups
					}
				} else if cmd.Flags().Changed("require-auth-groups") {
					// An explicit empty value clears the restriction; omitting the
					// field would leave it in place with no way to remove it.
					route["requireAuthGroups"] = []string{}
				}
				body["route"] = route
			}
			if err := applyOptionalLLMBackend(body, llmBackendOpt); err != nil {
				return err
			}
			// An empty body is an explicit re-apply: the API asks the controller to
			// retry a Failed app, and returns the app unchanged when it is Ready.
			raw, code, err := api.Patch("/apps/"+args[0], body)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, 200); err != nil {
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
			fmt.Printf("OK: app %q update requested\n", args[0])
			return nil
		},
	}
	c.Flags().StringVar(&version, "version", "", "Chart version")
	c.Flags().StringVar(&valuesFile, "values-file", "", "Helm values YAML file")
	c.Flags().StringVar(&namespace, "namespace", "", "Target namespace")
	c.Flags().StringVar(&routeMode, "route-mode", "", "Route mode: none|hub|site")
	c.Flags().StringVar(&routeHost, "route-host", "", "Route hostname")
	c.Flags().BoolVar(&requireAuth, "require-auth", false, "Gate the route behind Authelia forward-auth")
	c.Flags().StringVar(&requireAuthGroups, "require-auth-groups", "", "Comma-separated directory groups allowed through the route")
	c.Flags().StringVar(&routeServiceName, "service-name", "", "Backend Service name for the route")
	c.Flags().IntVar(&routeServicePort, "service-port", 0, "Backend Service port for the route")
	c.Flags().StringVar(&routeProtocol, "route-protocol", "", "Route protocol: http (default) or tcp (hub L4 port forward)")
	c.Flags().IntVar(&routeForwardPort, "route-forward-port", 0, "Hub port to listen on for a tcp route")
	c.Flags().StringVar(&llmBackendOpt, "llm-backend", "", "Llama-cpp only: hub LiteLLM backend registration (true|false)")
	return c
}

func appDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete [name...]",
		Short: "Uninstall one or more FleetApps (DELETE /apps/{name})",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, ferr := outputFormat()
			if ferr != nil {
				return ferr
			}
			for _, name := range args {
				raw, code, err := api.Delete("/apps/" + name)
				if err != nil {
					return err
				}
				if err := mustOK(raw, code, 202, 200, 204); err != nil {
					return err
				}
				if format == output.FormatJSON {
					if len(raw) > 0 {
						printJSON(raw)
					}
					continue
				}
				fmt.Printf("OK: app %q delete requested\n", name)
			}
			return nil
		},
	}
}
