package cmd

import (
	"github.com/rborso/umkim/cmd/umkim/internal/client"
	"github.com/rborso/umkim/cmd/umkim/internal/config"
	"github.com/rborso/umkim/cmd/umkim/internal/output"
	"github.com/spf13/cobra"
)

var (
	flagAPIURL   string
	flagAuthURL  string
	flagToken    string
	flagOutput   string
	cfgFile      config.File
	api          *client.Client
)

func Execute() error {
	return rootCmd.Execute()
}

var rootCmd = &cobra.Command{
	Use:   "umkim",
	Short: "UMKIM fleet CLI — hub API for sites, clusters, charts, and node operations",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if shouldSkipAPIInit(cmd) {
			return nil
		}
		var err error
		cfgFile, err = config.Load()
		if err != nil {
			return err
		}
		cfgFile = config.Resolve(cfgFile, flagAPIURL, flagAuthURL, flagToken)
		api, err = client.New(cfgFile)
		return err
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagAPIURL, "api-url", "", "Fleet API base URL (or UMKIM_API_URL)")
	rootCmd.PersistentFlags().StringVar(&flagAuthURL, "auth-url", "", "Auth API base URL (or UMKIM_AUTH_URL)")
	rootCmd.PersistentFlags().StringVar(&flagToken, "token", "", "Service account token (or UMKIM_TOKEN)")
	rootCmd.PersistentFlags().StringVarP(&flagOutput, "output", "o", "table", "Output format: table, wide, json")

	rootCmd.AddCommand(configCmd())
	rootCmd.AddCommand(loginCmd())
	rootCmd.AddCommand(logoutCmd())
	rootCmd.AddCommand(whoamiCmd())
	rootCmd.AddCommand(authAdminCmd())
	rootCmd.AddCommand(siteCmd())
	rootCmd.AddCommand(clusterCmd())
	rootCmd.AddCommand(appCmd())
	rootCmd.AddCommand(chartCmd())
	rootCmd.AddCommand(infraCmd())
	rootCmd.AddCommand(ingressAuthCmd())
	rootCmd.AddCommand(acmeCmd())
	rootCmd.AddCommand(llmCmd())
	rootCmd.AddCommand(edgeCmd())
	rootCmd.AddCommand(nodeCmd())
	rootCmd.AddCommand(nodeOpCmd())
	rootCmd.AddCommand(vmCmd())
	rootCmd.AddCommand(tenantCmd())
	rootCmd.AddCommand(inviteCmd())
}

func printJSON(raw []byte) {
	output.WriteJSON(raw)
}

func outputFormat() (output.Format, error) {
	return output.ParseFormat(flagOutput)
}

func mustOK(raw []byte, code int, allowed ...int) error {
	return client.ExpectOK(raw, code, allowed...)
}

func newClient(cfg config.File) (*client.Client, error) {
	return client.New(cfg)
}

func shouldSkipAPIInit(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		switch c.Name() {
		case "login", "config", "completion", "help":
			return true
		}
	}
	return false
}
