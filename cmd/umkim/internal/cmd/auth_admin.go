package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/rborso/umkim/cmd/umkim/internal/output"
	"github.com/spf13/cobra"
)

func authAdminCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Access control admin (requires dcAdmin role)",
	}
	cmd.AddCommand(authUsersCmd())
	cmd.AddCommand(authGroupsCmd())
	cmd.AddCommand(authRoleBindingsCmd())
	cmd.AddCommand(authServiceAccountsCmd())
	cmd.AddCommand(authAuditCmd())
	cmd.AddCommand(authLLMApiKeysCmd())
	return cmd
}

func authUsersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "users",
		Short: "Fleet auth users",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List fleet auth users",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.AuthGet("/users")
			if err != nil {
				return err
			}
			if err := authMustOK(raw, code); err != nil {
				return err
			}
			format, ferr := outputFormat()
			if ferr != nil {
				return ferr
			}
			return output.WriteAuthUsers(raw, format)
		},
	})
	return cmd
}

func authGroupsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "groups",
		Short: "Fleet auth groups",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List fleet auth groups",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.AuthGet("/groups")
			if err != nil {
				return err
			}
			if err := authMustOK(raw, code); err != nil {
				return err
			}
			format, ferr := outputFormat()
			if ferr != nil {
				return ferr
			}
			return output.WriteAuthGroups(raw, format)
		},
	})
	return cmd
}

func authRoleBindingsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "role-bindings",
		Short: "Manage role bindings",
	}
	cmd.AddCommand(authRoleBindingsListCmd())
	cmd.AddCommand(authRoleBindingsGetCmd())
	cmd.AddCommand(authRoleBindingsCreateCmd())
	cmd.AddCommand(authRoleBindingsDeleteCmd())
	return cmd
}

func authRoleBindingsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List role bindings",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.AuthGet("/role-bindings")
			if err != nil {
				return err
			}
			if err := authMustOK(raw, code); err != nil {
				return err
			}
			format, ferr := outputFormat()
			if ferr != nil {
				return ferr
			}
			return output.WriteAuthRoleBindings(raw, format)
		},
	}
}

func authRoleBindingsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get [name]",
		Short: "Get a role binding",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.AuthGet("/role-bindings/" + args[0])
			if err != nil {
				return err
			}
			if err := authMustOK(raw, code); err != nil {
				return err
			}
			printJSON(raw)
			return nil
		},
	}
}

func authRoleBindingsCreateCmd() *cobra.Command {
	var jsonFile string
	c := &cobra.Command{
		Use:   "create",
		Short: "Create a role binding (POST /role-bindings)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonFile == "" {
				return fmt.Errorf("--file required")
			}
			var body map[string]interface{}
			if err := readFileJSON(jsonFile, &body); err != nil {
				return err
			}
			raw, code, err := api.AuthPost("/role-bindings", body)
			if err != nil {
				return err
			}
			if err := authMustOK(raw, code, 201); err != nil {
				return err
			}
			printJSON(raw)
			return nil
		},
	}
	c.Flags().StringVarP(&jsonFile, "file", "f", "", "FleetRoleBinding JSON")
	return c
}

func authRoleBindingsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete [name]",
		Short: "Delete a role binding",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.AuthDelete("/role-bindings/" + args[0])
			if err != nil {
				return err
			}
			if err := authMustOK(raw, code, 204, 200); err != nil {
				return err
			}
			fmt.Printf("OK: role binding %q deleted\n", args[0])
			return nil
		},
	}
}

func authServiceAccountsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "service-accounts",
		Aliases: []string{"sa"},
		Short:   "Manage service accounts",
	}
	cmd.AddCommand(authServiceAccountsListCmd())
	cmd.AddCommand(authServiceAccountsCreateCmd())
	return cmd
}

func authServiceAccountsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List service accounts",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.AuthGet("/service-accounts")
			if err != nil {
				return err
			}
			if err := authMustOK(raw, code); err != nil {
				return err
			}
			format, ferr := outputFormat()
			if ferr != nil {
				return ferr
			}
			return output.WriteAuthServiceAccounts(raw, format)
		},
	}
}

func authServiceAccountsCreateCmd() *cobra.Command {
	var jsonFile string
	var quietToken bool
	c := &cobra.Command{
		Use:   "create",
		Short: "Create a service account (POST /service-accounts)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonFile == "" {
				return fmt.Errorf("--file required")
			}
			var body map[string]interface{}
			if err := readFileJSON(jsonFile, &body); err != nil {
				return err
			}
			raw, code, err := api.AuthPost("/service-accounts", body)
			if err != nil {
				return err
			}
			if err := authMustOK(raw, code, 201); err != nil {
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
			var resp struct {
				Token string `json:"token"`
			}
			_ = json.Unmarshal(raw, &resp)
			if resp.Token != "" && !quietToken {
				fmt.Fprintf(os.Stderr, "Service account token (save now; shown once):\n%s\n", resp.Token)
			}
			fmt.Println("OK: service account created")
			return nil
		},
	}
	c.Flags().StringVarP(&jsonFile, "file", "f", "", "FleetServiceAccount JSON")
	c.Flags().BoolVar(&quietToken, "quiet-token", false, "Do not print token to stderr")
	return c
}

func authAuditCmd() *cobra.Command {
	var limit int
	list := &cobra.Command{
		Use:   "list",
		Short: "List audit events",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.AuthGet("/audit-events")
			if err != nil {
				return err
			}
			if err := authMustOK(raw, code); err != nil {
				return err
			}
			if limit > 0 {
				var body struct {
					AuditEvents []json.RawMessage `json:"auditEvents"`
				}
				if err := json.Unmarshal(raw, &body); err == nil && len(body.AuditEvents) > limit {
					body.AuditEvents = body.AuditEvents[:limit]
					raw, _ = json.Marshal(body)
				}
			}
			format, ferr := outputFormat()
			if ferr != nil {
				return ferr
			}
			return output.WriteAuthAudit(raw, format)
		},
	}
	list.Flags().IntVar(&limit, "limit", 200, "Max events to show")
	audit := &cobra.Command{Use: "audit", Short: "Audit log"}
	audit.AddCommand(list)
	return audit
}

func authLLMApiKeysCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "llm-api-keys",
		Short: "Manage LLM API keys",
	}
	cmd.AddCommand(authLLMApiKeysListCmd())
	cmd.AddCommand(authLLMApiKeysCreateCmd())
	cmd.AddCommand(authLLMApiKeysDeleteCmd())
	cmd.AddCommand(authLLMApiKeysRotateCmd())
	cmd.AddCommand(authLLMApiKeysRevokeCmd())
	cmd.AddCommand(authLLMApiKeysActivateCmd())
	return cmd
}

func authLLMApiKeysListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List LLM API keys",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.AuthGet("/llm-api-keys")
			if err != nil {
				return err
			}
			if err := authMustOK(raw, code); err != nil {
				return err
			}
			format, ferr := outputFormat()
			if ferr != nil {
				return ferr
			}
			return output.WriteAuthLLMApiKeys(raw, format)
		},
	}
}

func authLLMApiKeysCreateCmd() *cobra.Command {
	var name, displayName, models string
	var quietToken bool
	c := &cobra.Command{
		Use:   "create",
		Short: "Create an LLM API key",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("--name required")
			}
			body := map[string]interface{}{
				"name": strings.TrimSpace(name),
			}
			if displayName != "" {
				body["displayName"] = displayName
			}
			if models != "" {
				parts := strings.Split(models, ",")
				list := make([]string, 0, len(parts))
				for _, p := range parts {
					if s := strings.TrimSpace(p); s != "" {
						list = append(list, s)
					}
				}
				if len(list) > 0 {
					body["models"] = list
				}
			}
			raw, code, err := api.AuthPost("/llm-api-keys", body)
			if err != nil {
				return err
			}
			if err := authMustOK(raw, code, 201); err != nil {
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
			var resp struct {
				Token string `json:"token"`
			}
			_ = json.Unmarshal(raw, &resp)
			if resp.Token != "" && !quietToken {
				fmt.Fprintf(os.Stderr, "LLM API key token (save now; shown once):\n%s\n", resp.Token)
			}
			fmt.Printf("OK: LLM API key %q created\n", name)
			return nil
		},
	}
	c.Flags().StringVar(&name, "name", "", "Key name (DNS-1123)")
	c.Flags().StringVar(&displayName, "display-name", "", "Display name")
	c.Flags().StringVar(&models, "models", "", "Comma-separated allowed models")
	c.Flags().BoolVar(&quietToken, "quiet-token", false, "Do not print token to stderr")
	return c
}

func authLLMApiKeysDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete [name]",
		Short: "Delete an LLM API key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.AuthDelete("/llm-api-keys/" + args[0])
			if err != nil {
				return err
			}
			if err := authMustOK(raw, code, 204, 200); err != nil {
				return err
			}
			fmt.Printf("OK: LLM API key %q deleted\n", args[0])
			return nil
		},
	}
}

func authLLMApiKeysRotateCmd() *cobra.Command {
	var quietToken bool
	c := &cobra.Command{
		Use:   "rotate [name]",
		Short: "Rotate an LLM API key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.AuthPost("/llm-api-keys/"+args[0]+"/rotate", nil)
			if err != nil {
				return err
			}
			if err := authMustOK(raw, code); err != nil {
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
			var resp struct {
				Token string `json:"token"`
			}
			_ = json.Unmarshal(raw, &resp)
			if resp.Token != "" && !quietToken {
				fmt.Fprintf(os.Stderr, "Rotated LLM API key token (save now; shown once):\n%s\n", resp.Token)
			}
			fmt.Printf("OK: LLM API key %q rotated\n", args[0])
			return nil
		},
	}
	c.Flags().BoolVar(&quietToken, "quiet-token", false, "Do not print token to stderr")
	return c
}

func authLLMApiKeysRevokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke [name]",
		Short: "Revoke an LLM API key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.AuthPost("/llm-api-keys/"+args[0]+"/revoke", nil)
			if err != nil {
				return err
			}
			if err := authMustOK(raw, code); err != nil {
				return err
			}
			printJSON(raw)
			return nil
		},
	}
}

func authLLMApiKeysActivateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "activate [name]",
		Short: "Activate an LLM API key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.AuthPost("/llm-api-keys/"+args[0]+"/activate", nil)
			if err != nil {
				return err
			}
			if err := authMustOK(raw, code); err != nil {
				return err
			}
			printJSON(raw)
			return nil
		},
	}
}

func authMustOK(raw []byte, code int, allowed ...int) error {
	if code == 403 {
		return fmt.Errorf("requires dcAdmin role: HTTP 403: %s", strings.TrimSpace(string(raw)))
	}
	return mustOK(raw, code, allowed...)
}
