package cmd

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

func edgeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edge",
		Short: "Edge DNS and hostname helpers",
	}
	cmd.AddCommand(edgeDNSValidateCmd())
	cmd.AddCommand(edgeHostnameCheckCmd())
	return cmd
}

func edgeDNSValidateCmd() *cobra.Command {
	var host, expectedIP string
	c := &cobra.Command{
		Use:   "dns validate",
		Short: "Validate DNS for a hostname (GET /edge/dns/validate)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if host == "" {
				return fmt.Errorf("--host required")
			}
			q := url.Values{"host": {host}}
			if expectedIP != "" {
				q.Set("expected_ip", expectedIP)
			}
			raw, code, err := api.Get("/edge/dns/validate?" + q.Encode())
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
	c.Flags().StringVar(&host, "host", "", "Hostname to validate")
	c.Flags().StringVar(&expectedIP, "expected-ip", "", "Expected WAN IP")
	return c
}

func edgeHostnameCheckCmd() *cobra.Command {
	var host string
	c := &cobra.Command{
		Use:   "hostname check",
		Short: "Check hostname availability (GET /edge/hostnames/check)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if host == "" {
				return fmt.Errorf("--host required")
			}
			q := url.Values{"host": {host}}
			raw, code, err := api.Get("/edge/hostnames/check?" + q.Encode())
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
	c.Flags().StringVar(&host, "host", "", "Hostname to check")
	return c
}
