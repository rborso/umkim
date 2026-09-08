package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func siteProvisionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provision",
		Short: "Site metal/VM provisioner connector (YAIDAP / noop / tofu)",
	}
	cmd.AddCommand(siteProvisionGetCmd())
	cmd.AddCommand(siteProvisionSetCmd())
	cmd.AddCommand(siteProvisionRunCmd())
	cmd.AddCommand(siteProvisionStatusCmd())
	cmd.AddCommand(siteProvisionActionsCmd())
	return cmd
}

func siteProvisionGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get [site]",
		Short: "Show site provisioner binding",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/sites/" + args[0] + "/provision")
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

func siteProvisionSetCmd() *cobra.Command {
	var provider, jumphost, sshSecret, inventory, yaidapRoot, workspace string
	c := &cobra.Command{
		Use:   "set [site]",
		Short: "Set site provisioner binding (PUT /sites/{site}/provision)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]interface{}{}
			if provider == "" {
				return fmt.Errorf("--provider required")
			}
			body["provider"] = provider
			if cmd.Flags().Changed("jumphost") {
				body["jumphost"] = jumphost
			}
			if cmd.Flags().Changed("ssh-secret") {
				body["sshSecretRef"] = sshSecret
			}
			if cmd.Flags().Changed("inventory") {
				body["inventoryPath"] = inventory
			}
			if cmd.Flags().Changed("yaidap-root") {
				body["yaidapRoot"] = yaidapRoot
			}
			if cmd.Flags().Changed("workspace") {
				body["workspacePath"] = workspace
			}
			raw, code, err := api.Put("/sites/"+args[0]+"/provision", body)
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
	c.Flags().StringVar(&provider, "provider", "", "noop | yaidap | opentofu | terraform")
	c.Flags().StringVar(&jumphost, "jumphost", "", "SSH jumphost (user@host:port)")
	c.Flags().StringVar(&sshSecret, "ssh-secret", "", "Hub Secret name for jumphost SSH key")
	c.Flags().StringVar(&inventory, "inventory", "", "YAIDAP inventory path on jumphost")
	c.Flags().StringVar(&yaidapRoot, "yaidap-root", "", "YAIDAP checkout on jumphost")
	c.Flags().StringVar(&workspace, "workspace", "", "OpenTofu/Terraform workspace path")
	return c
}

func siteProvisionRunCmd() *cobra.Command {
	var action, subject, ansibleHost, ansibleUser, osFamily string
	var confirm, wait bool
	c := &cobra.Command{
		Use:   "run [site]",
		Short: "Start a provision action (POST /sites/{site}/provision/actions)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if action == "" || subject == "" {
				return fmt.Errorf("--action and --subject required")
			}
			body := map[string]interface{}{
				"action":  action,
				"subject": subject,
				"confirm": confirm,
			}
			if ansibleHost != "" {
				body["host"] = map[string]string{
					"hostname":     subject,
					"ansible_host": ansibleHost,
					"ansible_user": ansibleUser,
					"osFamily":     osFamily,
				}
			}
			raw, code, err := api.Post("/sites/"+args[0]+"/provision/actions", body)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, 200, 202); err != nil {
				return err
			}
			if !wait {
				printJSON(raw)
				return nil
			}
			var started map[string]interface{}
			if err := json.Unmarshal(raw, &started); err != nil {
				return err
			}
			id, _ := started["id"].(string)
			if id == "" {
				printJSON(raw)
				return fmt.Errorf("response missing action id")
			}
			return waitProvisionAction(args[0], id)
		},
	}
	c.Flags().StringVar(&action, "action", "", "install_bm | create_vm | plan_vm | destroy | discover | status")
	c.Flags().StringVar(&subject, "subject", "", "Host/guest name (--limit)")
	c.Flags().BoolVar(&confirm, "confirm", false, "Required for install_bm / destroy")
	c.Flags().BoolVar(&wait, "wait", false, "Poll until Succeeded or Failed")
	c.Flags().StringVar(&ansibleHost, "ansible-host", "", "noop handoff: ansible_host")
	c.Flags().StringVar(&ansibleUser, "ansible-user", "linux", "noop handoff: ansible_user")
	c.Flags().StringVar(&osFamily, "os-family", "rocky", "noop handoff: osFamily")
	return c
}

func siteProvisionStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [site] [action-id]",
		Short: "Get provision action status",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/sites/" + args[0] + "/provision/actions/" + args[1])
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

func siteProvisionActionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "actions [site]",
		Short: "List recent provision actions",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/sites/" + args[0] + "/provision/actions")
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

func waitProvisionAction(site, id string) error {
	deadline := time.Now().Add(90 * time.Minute)
	for time.Now().Before(deadline) {
		raw, code, err := api.Get("/sites/" + site + "/provision/actions/" + id)
		if err != nil {
			return err
		}
		if err := mustOK(raw, code); err != nil {
			return err
		}
		var out map[string]interface{}
		if err := json.Unmarshal(raw, &out); err != nil {
			return err
		}
		phase, _ := out["phase"].(string)
		if phase == "Succeeded" || phase == "Failed" {
			printJSON(raw)
			if phase == "Failed" {
				return fmt.Errorf("provision action failed")
			}
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("timed out waiting for provision action %s", id)
}
