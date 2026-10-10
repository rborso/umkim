package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/rborso/umkim/cmd/umkim/internal/output"
	"github.com/spf13/cobra"
)

func chartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chart",
		Short: "Manage the FleetChart library (Apps → Library)",
	}
	cmd.AddCommand(chartListCmd())
	cmd.AddCommand(chartGetCmd())
	cmd.AddCommand(chartAddCmd())
	cmd.AddCommand(chartDeleteCmd())
	return cmd
}

func chartListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List charts in the hub library",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/charts")
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
			return output.WriteCharts(raw, format)
		},
	}
}

func chartGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get [name]",
		Short: "Get a library chart",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/charts/" + args[0])
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

func chartAddCmd() *cobra.Command {
	var (
		origin, name, version, imagePolicy, valuesFile, jsonFile, tgzPath string
		force                                                              bool
	)
	c := &cobra.Command{
		Use:   "add",
		Short: "Publish a chart to the library (POST /charts)",
		Long: `Publish a Helm chart into the hub FleetChart library (same as console Apps → Library).

Sources (pick one):
  --origin oci://… or https://…/.tgz   remote chart
  --chart package.tgz                  upload a local Helm package
  -f chart.json                        JSON body {origin,version,imagePolicy,…}

Image policy: external (default), proxy, or copy.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			imagePolicy = strings.TrimSpace(imagePolicy)
			if imagePolicy == "" {
				imagePolicy = "external"
			}
			switch imagePolicy {
			case "external", "proxy", "copy":
			default:
				return fmt.Errorf("image-policy must be external, proxy, or copy")
			}

			var valuesYAML string
			if valuesFile != "" {
				raw, err := os.ReadFile(valuesFile)
				if err != nil {
					return err
				}
				valuesYAML = string(raw)
			}

			modes := 0
			if jsonFile != "" {
				modes++
			}
			if tgzPath != "" {
				modes++
			}
			if strings.TrimSpace(origin) != "" {
				modes++
			}
			if modes != 1 {
				return fmt.Errorf("specify exactly one of --origin, --chart, or --file")
			}

			if tgzPath != "" {
				fields := map[string]string{
					"imagePolicy": imagePolicy,
				}
				if name != "" {
					fields["name"] = name
				}
				if version != "" {
					fields["version"] = version
				}
				if valuesYAML != "" {
					fields["valuesYaml"] = valuesYAML
				}
				if force {
					fields["force"] = "true"
				}
				raw, code, err := api.PostMultipart("/charts", "chart", tgzPath, fields)
				if err != nil {
					return err
				}
				if err := mustOK(raw, code, 201, 200); err != nil {
					return err
				}
				printJSON(raw)
				return nil
			}

			body := map[string]interface{}{}
			if jsonFile != "" {
				if err := readFileJSON(jsonFile, &body); err != nil {
					return err
				}
			} else {
				body["origin"] = strings.TrimSpace(origin)
			}
			if name != "" {
				body["name"] = name
			}
			if version != "" {
				body["version"] = version
			}
			if cmd.Flags().Changed("image-policy") || body["imagePolicy"] == nil {
				body["imagePolicy"] = imagePolicy
			}
			if valuesYAML != "" {
				body["valuesYaml"] = valuesYAML
			}
			if force {
				body["force"] = true
			}
			if _, ok := body["origin"]; !ok || strings.TrimSpace(fmt.Sprint(body["origin"])) == "" {
				return fmt.Errorf("origin required (oci:// or https:// chart URL)")
			}

			raw, code, err := api.Post("/charts", body)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, 201, 200); err != nil {
				return err
			}
			printJSON(raw)
			return nil
		},
	}
	c.Flags().StringVar(&origin, "origin", "", "Chart origin (oci://… or https://….tgz)")
	c.Flags().StringVar(&tgzPath, "chart", "", "Local Helm chart .tgz to upload")
	c.Flags().StringVarP(&jsonFile, "file", "f", "", "JSON create body (origin, version, imagePolicy, …)")
	c.Flags().StringVar(&name, "name", "", "Library name (DNS-1123); default inferred from origin/package")
	c.Flags().StringVar(&version, "version", "", "Chart version (optional for upload; required for some OCI refs)")
	c.Flags().StringVar(&imagePolicy, "image-policy", "external", "external | proxy | copy")
	c.Flags().StringVar(&valuesFile, "values-file", "", "Optional default values YAML stored on FleetChart")
	c.Flags().BoolVar(&force, "force", false, "Replace an existing Ready chart with a different source/origin")
	return c
}

func chartDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete [name]",
		Short: "Remove a chart from the library (FleetChart only; Harbor artifacts remain)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Delete("/charts/" + args[0])
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, 202, 200); err != nil {
				return err
			}
			printJSON(raw)
			return nil
		},
	}
}
