package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/rborso/umkim/cmd/umkim/internal/output"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func vmCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vm",
		Short: "KubeVirt virtual machine operations on a cluster",
	}
	cmd.AddCommand(vmListCmd())
	cmd.AddCommand(vmGetCmd())
	cmd.AddCommand(vmPowerCmd("start", "Power on a VM"))
	cmd.AddCommand(vmPowerCmd("stop", "Power off a VM (soft ACPI; --force hard poweroff)"))
	cmd.AddCommand(vmPowerCmd("restart", "Reboot a VM (soft; --force hard reboot)"))
	cmd.AddCommand(vmConsoleCmd())
	return cmd
}

func vmListCmd() *cobra.Command {
	var cluster string
	c := &cobra.Command{
		Use:   "list",
		Short: "List VirtualMachines on a cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cluster == "" {
				return fmt.Errorf("--cluster required")
			}
			raw, code, err := api.Get("/clusters/" + cluster + "/vms")
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
			return output.WriteVMs(raw, format)
		},
	}
	c.Flags().StringVar(&cluster, "cluster", "", "Cluster name")
	_ = c.MarkFlagRequired("cluster")
	return c
}

func vmGetCmd() *cobra.Command {
	var cluster, namespace string
	c := &cobra.Command{
		Use:   "get NAME",
		Short: "Get a VirtualMachine",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cluster == "" {
				return fmt.Errorf("--cluster required")
			}
			ns := namespace
			if ns == "" {
				ns = "default"
			}
			raw, code, err := api.Get("/clusters/" + cluster + "/vms/" + ns + "/" + args[0])
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
	c.Flags().StringVar(&cluster, "cluster", "", "Cluster name")
	c.Flags().StringVarP(&namespace, "namespace", "n", "default", "VM namespace")
	_ = c.MarkFlagRequired("cluster")
	return c
}

func vmPowerCmd(action, short string) *cobra.Command {
	var cluster, namespace string
	var force bool
	c := &cobra.Command{
		Use:   action + " NAME",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cluster == "" {
				return fmt.Errorf("--cluster required")
			}
			ns := namespace
			if ns == "" {
				ns = "default"
			}
			body := map[string]interface{}{"action": action, "force": force}
			raw, code, err := api.Post("/clusters/"+cluster+"/vms/"+ns+"/"+args[0]+"/power", body)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, http.StatusAccepted, http.StatusOK); err != nil {
				return err
			}
			fmt.Printf("OK: %s %s/%s (force=%v)\n", action, ns, args[0], force)
			if flagOutput == "json" {
				printJSON(raw)
			}
			return nil
		},
	}
	c.Flags().StringVar(&cluster, "cluster", "", "Cluster name")
	c.Flags().StringVarP(&namespace, "namespace", "n", "default", "VM namespace")
	c.Flags().BoolVar(&force, "force", false, "Hard poweroff/reboot (skip guest ACPI)")
	_ = c.MarkFlagRequired("cluster")
	return c
}

func vmConsoleCmd() *cobra.Command {
	var cluster, namespace, kind string
	c := &cobra.Command{
		Use:   "console NAME",
		Short: "Attach to VM serial console (default) or VNC websocket",
		Long: `Opens a websocket to the hub VM console proxy.

Default is the serial console (interactive TTY). Use --type=vnc for the VNC
protocol stream (binary; typically consumed by the console UI).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cluster == "" {
				return fmt.Errorf("--cluster required")
			}
			ns := namespace
			if ns == "" {
				ns = "default"
			}
			kind = strings.ToLower(strings.TrimSpace(kind))
			if kind == "" {
				kind = "serial"
			}
			sub := "console"
			proto := "plain.kubevirt.io"
			if kind == "vnc" {
				sub = "vnc"
				proto = "vnc.kubevirt.io"
			} else if kind != "serial" && kind != "console" {
				return fmt.Errorf("--type must be serial or vnc")
			}
			wsURL, err := api.WebsocketURL("/clusters/" + cluster + "/vms/" + ns + "/" + args[0] + "/" + sub)
			if err != nil {
				return err
			}
			header := http.Header{}
			api.SetAuthHeaders(header)
			dialer := websocket.Dialer{Subprotocols: []string{proto}}
			conn, resp, err := dialer.Dial(wsURL, header)
			if err != nil {
				if resp != nil {
					b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
					_ = resp.Body.Close()
					return fmt.Errorf("console dial: %w (%s)", err, strings.TrimSpace(string(b)))
				}
				return fmt.Errorf("console dial: %w", err)
			}
			defer conn.Close()

			if kind == "vnc" {
				fmt.Fprintf(os.Stderr, "Connected to VNC websocket (binary). Ctrl-C to exit.\n")
				for {
					_, data, rerr := conn.ReadMessage()
					if len(data) > 0 {
						_, _ = os.Stdout.Write(data)
					}
					if rerr != nil {
						return rerr
					}
				}
			}

			if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
				return fmt.Errorf("serial console requires a TTY")
			}
			oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
			if err != nil {
				return err
			}
			defer term.Restore(int(os.Stdin.Fd()), oldState)
			fmt.Fprintf(os.Stderr, "Serial console %s/%s on %s — escape with Ctrl-]\n", ns, args[0], cluster)

			errc := make(chan error, 2)
			go func() {
				buf := make([]byte, 4096)
				for {
					n, rerr := os.Stdin.Read(buf)
					if n > 0 {
						if werr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
							errc <- werr
							return
						}
					}
					if rerr != nil {
						errc <- rerr
						return
					}
				}
			}()
			go func() {
				for {
					_, data, rerr := conn.ReadMessage()
					if len(data) > 0 {
						_, _ = os.Stdout.Write(data)
					}
					if rerr != nil {
						errc <- rerr
						return
					}
				}
			}()
			return <-errc
		},
	}
	c.Flags().StringVar(&cluster, "cluster", "", "Cluster name")
	c.Flags().StringVarP(&namespace, "namespace", "n", "default", "VM namespace")
	c.Flags().StringVar(&kind, "type", "serial", "Console type: serial|vnc")
	_ = c.MarkFlagRequired("cluster")
	return c
}
