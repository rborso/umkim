package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func tenantCmd() *cobra.Command {
	c := &cobra.Command{
		Use:     "tenant",
		Aliases: []string{"tenants"},
		Short:   "Manage directory tenants (hub LLDAP groups) and their members",
		Long: `Tenants are hub directory (LLDAP) groups. Route access is restricted to a
tenant with FleetApp requireAuthGroups, and directory users join a tenant either
by an existing account (add-member) or a new account with a password (add-user).`,
	}
	c.AddCommand(tenantListCmd())
	c.AddCommand(tenantCreateCmd())
	c.AddCommand(tenantDeleteCmd())
	c.AddCommand(tenantAddMemberCmd())
	c.AddCommand(tenantRemoveMemberCmd())
	c.AddCommand(tenantAddUserCmd())
	return c
}

func tenantListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List tenant groups and directory users (GET /tenants)",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/tenants")
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

func tenantCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create NAME",
		Short: "Create a tenant group (POST /tenants)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.TrimSpace(args[0])
			raw, code, err := api.Post("/tenants", map[string]interface{}{"name": name})
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, 201, 200, 204); err != nil {
				return err
			}
			fmt.Printf("OK: tenant %q created\n", name)
			return nil
		},
	}
}

func tenantDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete NAME",
		Short: "Delete a tenant group (DELETE /tenants/{name})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Delete("/tenants/" + args[0])
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, 204, 200); err != nil {
				return err
			}
			fmt.Printf("OK: tenant %q deleted\n", args[0])
			return nil
		},
	}
}

func tenantAddMemberCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add-member TENANT USERNAME",
		Short: "Add an existing directory user to a tenant (POST /tenants/{name}/members)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Post("/tenants/"+args[0]+"/members", map[string]interface{}{"username": args[1]})
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, 204, 200, 201); err != nil {
				return err
			}
			fmt.Printf("OK: %s added to tenant %s\n", args[1], args[0])
			return nil
		},
	}
}

func tenantRemoveMemberCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove-member TENANT USERNAME",
		Short: "Remove a directory user from a tenant (DELETE /tenants/{name}/members/{username})",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Delete("/tenants/" + args[0] + "/members/" + args[1])
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, 204, 200); err != nil {
				return err
			}
			fmt.Printf("OK: %s removed from tenant %s\n", args[1], args[0])
			return nil
		},
	}
}

// tenantAddUserCmd creates a new directory account with an initial password (via
// a one-time invite) and adds it to the tenant group.
func tenantAddUserCmd() *cobra.Command {
	var password, displayName, email string
	c := &cobra.Command{
		Use:   "add-user TENANT USERNAME --password P",
		Short: "Create a directory user with a password and add it to a tenant (invite + redeem)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			tenant := strings.TrimSpace(args[0])
			username := strings.ToLower(strings.TrimSpace(args[1]))
			if password == "" {
				return fmt.Errorf("--password is required")
			}
			createBody := map[string]interface{}{"groups": []string{tenant}}
			if displayName != "" {
				createBody["displayNameHint"] = displayName
			}
			if email != "" {
				createBody["emailHint"] = email
			}
			raw, code, err := api.Post("/invites", createBody)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, 201, 200); err != nil {
				return err
			}
			var inv struct {
				ID    string `json:"id"`
				Token string `json:"token"`
			}
			if err := json.Unmarshal(raw, &inv); err != nil {
				return err
			}
			if inv.Token == "" {
				return fmt.Errorf("invite response did not include a token")
			}
			redeemBody := map[string]interface{}{
				"token":    inv.Token,
				"username": username,
				"password": password,
			}
			if displayName != "" {
				redeemBody["displayName"] = displayName
			}
			if email != "" {
				redeemBody["email"] = email
			}
			raw, code, err = api.Post("/invites/redeem", redeemBody)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, 200, 201, 204); err != nil {
				return err
			}
			// The one-time invite is consumed; drop it so the list stays clean.
			_, _, _ = api.Delete("/invites/" + inv.ID)
			fmt.Printf("OK: user %q created and added to tenant %q\n", username, tenant)
			return nil
		},
	}
	c.Flags().StringVar(&password, "password", "", "Initial password for the new user (required)")
	c.Flags().StringVar(&displayName, "display-name", "", "Display name for the new user")
	c.Flags().StringVar(&email, "email", "", "Email address for the new user")
	return c
}

func inviteCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "invite",
		Short: "Manage directory sign-up invites (tenant-scoped)",
	}
	c.AddCommand(inviteListCmd())
	c.AddCommand(inviteCreateCmd())
	c.AddCommand(inviteDeleteCmd())
	c.AddCommand(inviteRedeemCmd())
	return c
}

func inviteListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List invites (GET /invites)",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/invites")
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

func inviteCreateCmd() *cobra.Command {
	var groups, displayName, email, note string
	var ttlHours int
	c := &cobra.Command{
		Use:   "create --groups TENANT[,TENANT...]",
		Short: "Create a tenant-scoped invite (POST /invites)",
		RunE: func(cmd *cobra.Command, args []string) error {
			var gs []string
			for _, g := range strings.Split(groups, ",") {
				if g = strings.TrimSpace(g); g != "" {
					gs = append(gs, g)
				}
			}
			if len(gs) == 0 {
				return fmt.Errorf("--groups is required (comma-separated tenant names)")
			}
			body := map[string]interface{}{"groups": gs}
			if ttlHours > 0 {
				body["ttlHours"] = ttlHours
			}
			if displayName != "" {
				body["displayNameHint"] = displayName
			}
			if email != "" {
				body["emailHint"] = email
			}
			if note != "" {
				body["note"] = note
			}
			raw, code, err := api.Post("/invites", body)
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
	c.Flags().StringVar(&groups, "groups", "", "Comma-separated tenant groups (required)")
	c.Flags().IntVar(&ttlHours, "ttl-hours", 0, "Invite lifetime in hours (default 72)")
	c.Flags().StringVar(&displayName, "display-name", "", "Display name hint")
	c.Flags().StringVar(&email, "email", "", "Email hint")
	c.Flags().StringVar(&note, "note", "", "Free-form note")
	return c
}

func inviteDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete ID",
		Short: "Delete an invite (DELETE /invites/{id})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Delete("/invites/" + args[0])
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, 204, 200); err != nil {
				return err
			}
			fmt.Printf("OK: invite %q deleted\n", args[0])
			return nil
		},
	}
}

func inviteRedeemCmd() *cobra.Command {
	var token, username, password, displayName, email string
	c := &cobra.Command{
		Use:   "redeem --token T --username U --password P",
		Short: "Redeem an invite: create the directory user and set its password (POST /invites/redeem)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if token == "" || username == "" || password == "" {
				return fmt.Errorf("--token, --username, and --password are required")
			}
			body := map[string]interface{}{
				"token":    token,
				"username": strings.ToLower(strings.TrimSpace(username)),
				"password": password,
			}
			if displayName != "" {
				body["displayName"] = displayName
			}
			if email != "" {
				body["email"] = email
			}
			raw, code, err := api.Post("/invites/redeem", body)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, 200, 201, 204); err != nil {
				return err
			}
			printJSON(raw)
			return nil
		},
	}
	c.Flags().StringVar(&token, "token", "", "Invite token (required)")
	c.Flags().StringVar(&username, "username", "", "Directory username (required)")
	c.Flags().StringVar(&password, "password", "", "Initial password, min 8 chars (required)")
	c.Flags().StringVar(&displayName, "display-name", "", "Display name")
	c.Flags().StringVar(&email, "email", "", "Email address")
	return c
}
