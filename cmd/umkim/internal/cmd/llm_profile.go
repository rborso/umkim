package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func llmProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Llama-cpp Helm values profiles (FleetLlamaProfile CRs)",
	}
	cmd.AddCommand(llmProfileListCmd())
	cmd.AddCommand(llmProfileGetCmd())
	cmd.AddCommand(llmProfileCreateCmd())
	cmd.AddCommand(llmProfileUpdateCmd())
	cmd.AddCommand(llmProfileDeleteCmd())
	cmd.AddCommand(llmProfileValuesCmd())
	cmd.AddCommand(llmProfileVerifyCmd())
	test := &cobra.Command{Use: "test", Short: "Profile verify sessions"}
	test.AddCommand(llmProfileTestListCmd())
	test.AddCommand(llmProfileTestGetCmd())
	test.AddCommand(llmProfileTestEndCmd())
	test.AddCommand(llmProfileTestChatCmd())
	cmd.AddCommand(test)
	return cmd
}

func llmProfileListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List llama profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/llama-profiles")
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

func llmProfileGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get NAME",
		Short: "Get a llama profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/llama-profiles/" + args[0])
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

func llmProfileCreateCmd() *cobra.Command {
	var id, description, profileFile, chartVersion string
	c := &cobra.Command{
		Use:   "create",
		Short: "Create a llama profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			id = strings.TrimSpace(id)
			if id == "" {
				return fmt.Errorf("--id is required")
			}
			body := map[string]interface{}{"id": id}
			if description != "" {
				body["description"] = description
			}
			if chartVersion != "" {
				body["chartVersion"] = chartVersion
			}
			if profileFile != "" {
				raw, err := os.ReadFile(profileFile)
				if err != nil {
					return err
				}
				body["profileYaml"] = string(raw)
			}
			resp, code, err := api.Post("/llama-profiles", body)
			if err != nil {
				return err
			}
			if err := mustOK(resp, code, 201); err != nil {
				return err
			}
			return printPatchOK("llama profile create", resp)
		},
	}
	c.Flags().StringVar(&id, "id", "", "Profile name (DNS-1123)")
	c.Flags().StringVar(&description, "description", "", "Description")
	c.Flags().StringVar(&profileFile, "profile-file", "", "Profile overlay YAML file")
	c.Flags().StringVar(&chartVersion, "chart-version", "", "llama-cpp-helm chart version pin")
	return c
}

func llmProfileUpdateCmd() *cobra.Command {
	var description, profileFile, chartVersion string
	c := &cobra.Command{
		Use:   "update NAME",
		Short: "Update a llama profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]interface{}{}
			if description != "" {
				body["description"] = description
			}
			if chartVersion != "" {
				body["chartVersion"] = chartVersion
			}
			if profileFile != "" {
				raw, err := os.ReadFile(profileFile)
				if err != nil {
					return err
				}
				body["profileYaml"] = string(raw)
			}
			if len(body) == 0 {
				return fmt.Errorf("nothing to update; pass --description, --profile-file, or --chart-version")
			}
			resp, code, err := api.Patch("/llama-profiles/"+args[0], body)
			if err != nil {
				return err
			}
			if err := mustOK(resp, code); err != nil {
				return err
			}
			return printPatchOK("llama profile update", resp)
		},
	}
	c.Flags().StringVar(&description, "description", "", "Description")
	c.Flags().StringVar(&profileFile, "profile-file", "", "Profile overlay YAML file")
	c.Flags().StringVar(&chartVersion, "chart-version", "", "Chart version pin")
	return c
}

func llmProfileDeleteCmd() *cobra.Command {
	var force bool
	c := &cobra.Command{
		Use:   "delete NAME",
		Short: "Delete a llama profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "/llama-profiles/" + args[0]
			if force {
				path += "?force=true"
			}
			raw, code, err := api.Delete(path)
			if err != nil {
				return err
			}
			if code != 204 && code != 200 {
				return mustOK(raw, code)
			}
			fmt.Printf("OK: deleted llama profile %q\n", args[0])
			return nil
		},
	}
	c.Flags().BoolVar(&force, "force", false, "Delete system profiles")
	return c
}

func llmProfileValuesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "values NAME",
		Short: "Merged Helm values preview",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/llama-profiles/" + args[0] + "/values")
			if err != nil {
				return err
			}
			if err := mustOK(raw, code); err != nil {
				return err
			}
			var doc struct {
				ValuesYAML string `json:"valuesYaml"`
			}
			if err := json.Unmarshal(raw, &doc); err != nil {
				return err
			}
			fmt.Print(doc.ValuesYAML)
			return nil
		},
	}
}

func llmProfileVerifyCmd() *cobra.Command {
	var cluster, chartVersion string
	var extended, wait, verbose bool
	var timeout time.Duration
	c := &cobra.Command{
		Use:   "verify NAME",
		Short: "Verify profile on a GPU cluster (quick or extended)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster = strings.TrimSpace(cluster)
			if cluster == "" {
				return fmt.Errorf("--cluster is required")
			}
			mode := "quick"
			if extended {
				mode = "extended"
			}
			body := map[string]interface{}{
				"clusterName": cluster,
				"mode":        mode,
				// Server-side wait holds the HTTP connection; CLI polls test status instead.
				"wait": false,
			}
			if chartVersion != "" {
				body["chartVersion"] = chartVersion
			}
			raw, code, err := api.Post("/llama-profiles/"+args[0]+"/verify", body)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, 202, 200); err != nil {
				return err
			}
			var doc struct {
				ID      string `json:"id"`
				Phase   string `json:"phase"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(raw, &doc); err != nil {
				return err
			}
			if !wait {
				printJSON(raw)
				return nil
			}
			if doc.ID == "" {
				printJSON(raw)
				return fmt.Errorf("verify response missing test id")
			}
			want := "Succeeded"
			if extended {
				want = "Ready"
			}
			deadline := time.Now().Add(timeout)
			for time.Now().Before(deadline) {
				poll, pcode, perr := api.Get("/llama-profiles/tests/" + doc.ID)
				if perr != nil {
					return perr
				}
				if err := mustOK(poll, pcode); err != nil {
					return err
				}
				if err := json.Unmarshal(poll, &doc); err != nil {
					return err
				}
				if verbose {
					var detail struct {
						FleetAppName string `json:"fleetAppName"`
						APIBase      string `json:"apiBase"`
						Model        string `json:"model"`
						Message      string `json:"message"`
						Phase        string `json:"phase"`
					}
					_ = json.Unmarshal(poll, &detail)
					fmt.Printf("test %s: phase=%s", doc.ID, doc.Phase)
					if detail.FleetAppName != "" {
						fmt.Printf(" app=%s", detail.FleetAppName)
					}
					if detail.APIBase != "" {
						fmt.Printf(" apiBase=%s", detail.APIBase)
					}
					if detail.Message != "" {
						fmt.Printf(" — %s", detail.Message)
					}
					fmt.Println()
				} else {
					fmt.Printf("test %s: phase=%s", doc.ID, doc.Phase)
					if doc.Message != "" {
						fmt.Printf(" (%s)", doc.Message)
					}
					fmt.Println()
				}
				switch doc.Phase {
				case "Succeeded", "Ready":
					if doc.Phase == want {
						printJSON(poll)
						return nil
					}
				case "Failed":
					printJSON(poll)
					return fmt.Errorf("verify failed: %s", doc.Message)
				}
				time.Sleep(15 * time.Second)
			}
			return fmt.Errorf("timeout waiting for verify phase %s", want)
		},
	}
	c.Flags().StringVar(&cluster, "cluster", "", "Target Ready GPU cluster")
	c.Flags().BoolVar(&extended, "extended", false, "Keep FleetApp running for hub-proxied chat")
	c.Flags().BoolVar(&wait, "wait", false, "Block until quick verify finishes or extended test is Ready")
	c.Flags().DurationVar(&timeout, "timeout", 45*time.Minute, "Wait timeout")
	c.Flags().StringVar(&chartVersion, "chart-version", "", "Chart version override")
	c.Flags().BoolVar(&verbose, "verbose", false, "Print fleetAppName, apiBase, and status message each poll")
	return c
}

func llmProfileTestListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List profile verify tests",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/llama-profiles/tests")
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

func llmProfileTestGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get ID",
		Short: "Get verify test status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/llama-profiles/tests/" + args[0])
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

func llmProfileTestEndCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "end ID",
		Short: "End extended test (delete FleetApp and test CR)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Delete("/llama-profiles/tests/" + args[0])
			if err != nil {
				return err
			}
			if code != 204 && code != 200 {
				return mustOK(raw, code)
			}
			fmt.Printf("OK: ended profile test %q\n", args[0])
			return nil
		},
	}
}

func llmProfileTestChatCmd() *cobra.Command {
	var message string
	var maxTokens int
	c := &cobra.Command{
		Use:   "chat ID",
		Short: "Send one chat completion via hub proxy",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			message = strings.TrimSpace(message)
			if message == "" {
				return fmt.Errorf("--message is required")
			}
			body := map[string]interface{}{
				"messages": []map[string]string{
					{"role": "user", "content": message},
				},
				"max_tokens": maxTokens,
			}
			if maxTokens <= 0 {
				body["max_tokens"] = 64
			}
			raw, code, err := api.Post("/llama-profiles/tests/"+args[0]+"/chat/completions", body)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, 200); err != nil {
				return err
			}
			printJSON(raw)
			return nil
		},
	}
	c.Flags().StringVar(&message, "message", "", "User message")
	c.Flags().IntVar(&maxTokens, "max-tokens", 64, "Max completion tokens")
	return c
}
