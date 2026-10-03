package cmd

import (
	"strings"

	"github.com/spf13/cobra"
)

func ingressAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ingress-auth",
		Aliases: []string{"ingressauth"},
		Short:   "Hub ingress authentication settings",
	}
	cmd.AddCommand(ingressAuthGetCmd())
	cmd.AddCommand(ingressAuthSetCmd())
	return cmd
}

func ingressAuthGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Get hub ingress-auth settings (GET /ingress-auth)",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/ingress-auth")
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

func ingressAuthSetCmd() *cobra.Command {
	var (
		hubEnabled, hubLldapWebUI                       bool
		portalHost, cookieDomain, accessDomains         string
		hubEnabledSet, hubLldapWebUISet                   bool
	)
	c := &cobra.Command{
		Use:   "set",
		Short: "Patch hub ingress-auth settings (PATCH /ingress-auth)",
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]interface{}{}
			if hubEnabledSet {
				body["hubEnabled"] = hubEnabled
			}
			if hubLldapWebUISet {
				body["hubExposeLldapWebUI"] = hubLldapWebUI
			}
			if cmd.Flags().Changed("portal-host") {
				body["portalHost"] = portalHost
			}
			if cmd.Flags().Changed("cookie-domain") {
				body["cookieDomain"] = cookieDomain
			}
			if cmd.Flags().Changed("access-domains") {
				parts := strings.Split(accessDomains, ",")
				domains := make([]string, 0, len(parts))
				for _, p := range parts {
					if s := strings.TrimSpace(p); s != "" {
						domains = append(domains, s)
					}
				}
				body["accessDomains"] = domains
			}
			if len(body) == 0 {
				return errNoPatchFields
			}
			raw, code, err := api.Patch("/ingress-auth", body)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code); err != nil {
				return err
			}
			return printPatchOK("ingress-auth", raw)
		},
	}
	c.Flags().BoolVar(&hubEnabled, "hub-enabled", false, "Enable hub ingress auth")
	c.Flags().BoolVar(&hubLldapWebUI, "hub-lldap-webui", false, "Expose LLDAP Web UI on hub")
	c.Flags().StringVar(&portalHost, "portal-host", "", "Portal hostname")
	c.Flags().StringVar(&cookieDomain, "cookie-domain", "", "Auth cookie domain")
	c.Flags().StringVar(&accessDomains, "access-domains", "", "Comma-separated access domains")
	c.Flags().Lookup("hub-enabled").NoOptDefVal = "true"
	c.Flags().Lookup("hub-lldap-webui").NoOptDefVal = "true"
	c.PreRunE = func(cmd *cobra.Command, args []string) error {
		hubEnabledSet = cmd.Flags().Changed("hub-enabled")
		hubLldapWebUISet = cmd.Flags().Changed("hub-lldap-webui")
		return nil
	}
	return c
}
