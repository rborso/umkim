package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/rborso/umkim/cmd/umkim/internal/config"
	"github.com/spf13/cobra"
)

func configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show or update CLI configuration",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Print effective configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := config.Load()
			if err != nil {
				return err
			}
			f = config.Resolve(f, flagAPIURL, flagAuthURL, flagToken)
			raw, _ := json.MarshalIndent(f, "", "  ")
			fmt.Println(string(raw))
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "set [key] [value]",
		Short: "Set apiUrl, authUrl, or token in config file",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := config.Load()
			if err != nil {
				return err
			}
			switch args[0] {
			case "apiUrl", "api-url", "api_url":
				f.APIURL = args[1]
			case "authUrl", "auth-url", "auth_url":
				f.AuthURL = args[1]
			case "token":
				f.Token = args[1]
			case "devMode", "dev":
				f.DevMode = args[1] == "true" || args[1] == "1"
			default:
				return fmt.Errorf("unknown key %q (use apiUrl, authUrl, token, devMode)", args[0])
			}
			return config.Save(f)
		},
	})
	return cmd
}

func decodeJSON(raw []byte, dest interface{}) error {
	return json.Unmarshal(raw, dest)
}
