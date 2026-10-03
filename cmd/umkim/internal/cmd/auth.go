package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/rborso/umkim/cmd/umkim/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func loginCmd() *cobra.Command {
	var username, password string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate and store session cookie",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgFile, err := config.Load()
			if err != nil {
				return err
			}
			cfgFile = config.Resolve(cfgFile, flagAPIURL, flagAuthURL, flagToken)
			if cfgFile.AuthURL == "" {
				return fmt.Errorf("set UMKIM_AUTH_URL or --auth-url (e.g. https://hub/fleet/auth/v1alpha1)")
			}
			user := username
			if user == "" {
				fmt.Fprint(os.Stderr, "Username: ")
				line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
				user = strings.TrimSpace(line)
			}
			pass := password
			if pass == "" {
				if v := strings.TrimSpace(os.Getenv("UMKIM_PASSWORD")); v != "" {
					pass = v
				}
			}
			if pass == "" {
				fmt.Fprint(os.Stderr, "Password: ")
				passBytes, err := term.ReadPassword(int(syscall.Stdin))
				fmt.Fprintln(os.Stderr)
				if err != nil {
					return err
				}
				pass = string(passBytes)
			}
			c, err := newClient(cfgFile)
			if err != nil {
				return err
			}
			me, err := c.Login(user, pass)
			if err != nil {
				return err
			}
			f, _ := config.Load()
			f = config.Resolve(f, flagAPIURL, flagAuthURL, flagToken)
			if f.APIURL == "" && cfgFile.APIURL != "" {
				f.APIURL = cfgFile.APIURL
			}
			if f.AuthURL == "" && cfgFile.AuthURL != "" {
				f.AuthURL = cfgFile.AuthURL
			}
			if err := config.Save(f); err != nil {
				return err
			}
			printJSON(mustMarshal(me))
			return nil
		},
	}
	cmd.Flags().StringVarP(&username, "username", "u", "", "Username")
	cmd.Flags().StringVarP(&password, "password", "p", "", "Password (or UMKIM_PASSWORD)")
	return cmd
}

func logoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Clear stored session",
		RunE: func(cmd *cobra.Command, args []string) error {
			return api.Logout()
		},
	}
}

func whoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show current authenticated identity",
		RunE: func(cmd *cobra.Command, args []string) error {
			me, err := api.Whoami()
			if err != nil {
				return err
			}
			printJSON(mustMarshal(me))
			return nil
		},
	}
}

func mustMarshal(v interface{}) []byte {
	raw, _ := json.Marshal(v)
	return raw
}
