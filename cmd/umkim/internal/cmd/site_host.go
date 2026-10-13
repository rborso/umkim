package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rborso/umkim/cmd/umkim/internal/output"
	"github.com/spf13/cobra"
)

func siteHostCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "host",
		Short: "Manage hub site host inventory",
	}
	cmd.AddCommand(siteHostListCmd())
	cmd.AddCommand(siteHostGetCmd())
	cmd.AddCommand(siteHostAddCmd())
	cmd.AddCommand(siteHostProvisionCmd())
	cmd.AddCommand(siteHostDestroyCmd())
	cmd.AddCommand(siteHostVerifyCmd())
	cmd.AddCommand(siteHostRemoveCmd())
	cmd.AddCommand(siteHostPrepareCmd())
	cmd.AddCommand(siteHostClusterApplyCmd())
	return cmd
}

func siteHostListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list [site]",
		Short: "List site hosts",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/sites/" + args[0] + "/hosts")
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
			return output.WriteSiteHosts(raw, format)
		},
	}
}

func siteHostGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get [site] [name]",
		Short: "Get a site host",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/sites/" + args[0] + "/hosts/" + args[1])
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

func siteHostAddCmd() *cobra.Command {
	var kind, name, ip, mac, hypervisor, network, zone, storage, osFamily, cluster string
	var cores, memory, disk int
	var gpu, pxe, discover bool
	c := &cobra.Command{
		Use:   "add [site]",
		Short: "Add a host to site inventory (minimal fields; catalog defaults; optional discovery)",
		Long: `Add a host to a site's YAIDAP inventory from a minimal, kind-aware request.

Only a name plus an address (IP for SSH, or MAC for PXE) is required; everything
else is filled from the site catalog. Use --discover to chain discovery (SSH by
default, PXE with --pxe).`,
		Example: `  umkim site host add homelab --kind bare_metal --name bm01 --ip 172.16.10.50 --discover
  umkim site host add homelab --kind bare_metal --name bm02 --mac aa:bb:cc:dd:ee:ff --pxe
  umkim site host add homelab --kind vm --name vm01 --hypervisor virt04 --cluster lab-ha-0`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			site := args[0]
			body := map[string]interface{}{"kind": kind, "name": name}
			set := func(k, v string) {
				if strings.TrimSpace(v) != "" {
					body[k] = v
				}
			}
			set("ansibleHost", ip)
			set("macAddress", mac)
			set("hypervisor", hypervisor)
			set("networkName", network)
			set("zone", zone)
			set("storage", storage)
			set("os", osFamily)
			set("cluster", cluster)
			if cores > 0 {
				body["cores"] = cores
			}
			if memory > 0 {
				body["memory"] = memory
			}
			if disk > 0 {
				body["diskSizeGi"] = disk
			}
			if gpu {
				body["gpuPassthrough"] = true
			}
			if pxe {
				body["pxe"] = true
			}
			raw, code, err := api.Post("/sites/"+site+"/provision/inventory/hosts", body)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, 200); err != nil {
				return err
			}
			printJSON(raw)
			if discover {
				action := "discover"
				if pxe {
					action = "discover_pxe"
				}
				praw, pcode, perr := retryOnHTTP409(func() ([]byte, int, error) {
					return api.Post("/sites/"+site+"/hosts/"+name+"/provision", map[string]interface{}{
						"action":  action,
						"confirm": pxe,
					})
				})
				if perr != nil {
					return perr
				}
				if err := mustOK(praw, pcode, 200, 202); err != nil {
					return err
				}
				printJSON(praw)
			}
			return nil
		},
	}
	c.Flags().StringVar(&kind, "kind", "bare_metal", "vm | bare_metal | hypervisor")
	c.Flags().StringVar(&name, "name", "", "host name (required)")
	c.Flags().StringVar(&ip, "ip", "", "ansible_host / management IP (SSH path)")
	c.Flags().StringVar(&mac, "mac", "", "MAC address (PXE path)")
	c.Flags().StringVar(&hypervisor, "hypervisor", "", "hypervisor for VM guests (default least-utilized)")
	c.Flags().StringVar(&network, "network", "", "network_name from the site catalog")
	c.Flags().StringVar(&zone, "zone", "", "zone from the site catalog")
	c.Flags().StringVar(&storage, "storage", "", "storage from the site catalog")
	c.Flags().StringVar(&osFamily, "os", "", "OS family (default from catalog, e.g. rocky-10)")
	c.Flags().StringVar(&cluster, "cluster", "", "optional cluster group to join")
	c.Flags().IntVar(&cores, "cores", 0, "vCPU cores (VM)")
	c.Flags().IntVar(&memory, "memory", 0, "memory in MiB (VM)")
	c.Flags().IntVar(&disk, "disk", 0, "boot disk in GiB (VM)")
	c.Flags().BoolVar(&gpu, "gpu", false, "GPU passthrough guest (VM)")
	c.Flags().BoolVar(&pxe, "pxe", false, "PXE host (sets tempxe_discovery; discover_pxe with --discover)")
	c.Flags().BoolVar(&discover, "discover", false, "chain discovery after adding (SSH, or PXE with --pxe)")
	return c
}

func siteHostProvisionCmd() *cobra.Command {
	var action string
	var confirm, wait bool
	c := &cobra.Command{
		Use:   "provision [site] [name]",
		Short: "Run provision action for a site host",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]interface{}{"confirm": confirm}
			if action != "" {
				body["action"] = action
			}
			raw, code, err := retryOnHTTP409(func() ([]byte, int, error) {
				return api.Post("/sites/"+args[0]+"/hosts/"+args[1]+"/provision", body)
			})
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
			return waitProvisionActionFromHostResponse(raw, args[0])
		},
	}
	c.Flags().StringVar(&action, "action", "", "install_bm | create_vm | plan_vm | destroy | discover | discover_pxe | status | prepare_hypervisor | prepare_gpu_passthrough | prepare_hypervisor_gpu (default from kind)")
	c.Flags().BoolVar(&confirm, "confirm", false, "Required for install_bm / discover_pxe / destroy / prepare_gpu_passthrough / prepare_hypervisor_gpu")
	c.Flags().BoolVar(&wait, "wait", false, "Poll until Succeeded or Failed")
	return c
}

func siteHostPrepareCmd() *cobra.Command {
	var gpuPassthrough, confirm, wait bool
	c := &cobra.Command{
		Use:   "prepare [site] [name]",
		Short: "Prepare a hypervisor (libvirt; optional GPU passthrough / VFIO)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			action := "prepare_hypervisor"
			if gpuPassthrough {
				action = "prepare_hypervisor_gpu"
				if !confirm {
					return fmt.Errorf("--confirm is required when --gpu-passthrough is set (reboot and host NVIDIA removal)")
				}
			}
			body := map[string]interface{}{
				"action":  action,
				"confirm": confirm,
			}
			raw, code, err := retryOnHTTP409(func() ([]byte, int, error) {
				return api.Post("/sites/"+args[0]+"/hosts/"+args[1]+"/provision", body)
			})
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
			return waitProvisionActionFromHostResponse(raw, args[0])
		},
	}
	c.Flags().BoolVar(&gpuPassthrough, "gpu-passthrough", false, "Run prepare + prepare_gpu_passthrough (requires --confirm)")
	c.Flags().BoolVar(&confirm, "confirm", false, "Required with --gpu-passthrough")
	c.Flags().BoolVar(&wait, "wait", false, "Poll until Succeeded or Failed")
	return c
}

func siteHostDestroyCmd() *cobra.Command {
	var confirm, wait bool
	c := &cobra.Command{
		Use:   "destroy [site] [name]",
		Short: "Destroy libvirt VM for a site host (keeps inventory for reprovision)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !confirm {
				return fmt.Errorf("--confirm is required for destroy")
			}
			body := map[string]interface{}{
				"action":  "destroy",
				"confirm": true,
			}
			raw, code, err := retryOnHTTP409(func() ([]byte, int, error) {
				return api.Post("/sites/"+args[0]+"/hosts/"+args[1]+"/provision", body)
			})
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
			return waitProvisionActionFromHostResponse(raw, args[0])
		},
	}
	c.Flags().BoolVar(&confirm, "confirm", false, "Required: tear down libvirt VM while keeping guest in inventory")
	c.Flags().BoolVar(&wait, "wait", false, "Poll until Succeeded or Failed")
	return c
}

func waitProvisionActionFromHostResponse(raw []byte, site string) error {
	var started map[string]interface{}
	if err := json.Unmarshal(raw, &started); err != nil {
		return err
	}
	id, _ := started["id"].(string)
	if id == "" {
		printJSON(raw)
		return fmt.Errorf("response missing action id")
	}
	return waitProvisionAction(site, id)
}

func siteHostClusterApplyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cluster-apply [site] [name]",
		Short: "Apply cluster intent for a Ready host",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Post("/sites/"+args[0]+"/hosts/"+args[1]+"/cluster", map[string]interface{}{})
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, 202); err != nil {
				return err
			}
			printJSON(raw)
			return nil
		},
	}
}

func siteHostVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify [site] [name]",
		Short: "Read-only check that a hypervisor / bare-metal host is configured and working",
		Long: `Run read-only checks against a site host through the site jumphost and record
the outcome on the host. Hypervisors are checked for libvirt (service state, virsh
version, domain/pool/network counts); bare-metal workload nodes run no libvirt, so
they are checked for the K3s supervisor and their cluster membership via kubectl.
Basic host facts are reported in both cases. Nothing is changed on the target. A
host that answers moves from Declared to Ready.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Post("/sites/"+args[0]+"/hosts/"+args[1]+"/verify", map[string]interface{}{})
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
}

func siteHostRemoveCmd() *cobra.Command {
	var force bool
	c := &cobra.Command{
		Use:   "remove [site] [name]",
		Short: "Remove a host from the site inventory (after its VM is gone)",
		Long: `Drop a host's inventory records: its entry in guests.yml / bare_metals.yml, its
host_vars / group_vars files and its status record. Use this for hosts whose VM has
already been destroyed. A host that is still Ready/Provisioning is refused unless
--force is given.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "/sites/" + args[0] + "/hosts/" + args[1]
			if force {
				path += "?force=true"
			}
			raw, code, err := api.Delete(path)
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
	c.Flags().BoolVar(&force, "force", false, "Remove even when the host is Ready/Provisioning")
	return c
}
