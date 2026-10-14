package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rborso/umkim/cmd/umkim/internal/bootstrap"
	"github.com/rborso/umkim/cmd/umkim/internal/output"
	"github.com/spf13/cobra"
)

func clusterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cluster",
		Short: "Manage fleet clusters (FleetClusterJoin via API)",
	}
	cmd.AddCommand(clusterListCmd())
	cmd.AddCommand(clusterCandidatesCmd())
	cmd.AddCommand(clusterGetCmd())
	cmd.AddCommand(clusterCreateCmd())
	cmd.AddCommand(clusterPatchCmd())
	cmd.AddCommand(clusterDeleteCmd())
	cmd.AddCommand(clusterProvisionCmd())
	cmd.AddCommand(clusterWaitCmd())
	cmd.AddCommand(clusterBootstrapCmd())
	cmd.AddCommand(clusterStorageCmd())
	cmd.AddCommand(clusterPlatformCmd())
	cmd.AddCommand(clusterGpuCmd())
	cmd.AddCommand(clusterEdgeCmd())
	cmd.AddCommand(clusterAcmeCmd())
	cmd.AddCommand(clusterValidateCmd())
	cmd.AddCommand(clusterDiscoverCmd())
	return cmd
}

func clusterListCmd() *cobra.Command {
	var require []string
	c := &cobra.Command{
		Use:   "list",
		Short: "List clusters",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/clusters")
			if err != nil {
				return err
			}
			if err := mustOK(raw, code); err != nil {
				return err
			}
			reqs, err := output.ParseRequireFlags(require)
			if err != nil {
				return err
			}
			if len(reqs) > 0 {
				raw, err = output.FilterClustersJSON(raw, reqs, false)
				if err != nil {
					return err
				}
			}
			format, err := outputFormat()
			if err != nil {
				return err
			}
			if len(reqs) > 0 && format != output.FormatJSON {
				return output.WriteClusterCandidates(raw, format)
			}
			return output.WriteClusters(raw, format)
		},
	}
	c.Flags().StringArrayVar(&require, "require", nil, "Filter by capability (repeatable/comma): gpu, kvm, kubevirt, network.multus_l2, ha_topology, storage.rwx, storage.rwo")
	return c
}

func clusterCandidatesCmd() *cobra.Command {
	var require []string
	var site string
	c := &cobra.Command{
		Use:   "candidates",
		Short: "List Ready clusters matching capability requirements (placement helper)",
		Long: `Placement helper: show Ready clusters that have the requested capabilities.

Examples:
  umkim cluster candidates --require gpu
  umkim cluster candidates --require kvm,ha_topology
  umkim cluster candidates --require storage.rwx --site homelab`,
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/clusters")
			if err != nil {
				return err
			}
			if err := mustOK(raw, code); err != nil {
				return err
			}
			reqs, err := output.ParseRequireFlags(require)
			if err != nil {
				return err
			}
			raw, err = output.FilterClustersJSON(raw, reqs, true)
			if err != nil {
				return err
			}
			site = strings.TrimSpace(site)
			if site != "" {
				raw, err = filterClustersBySite(raw, site)
				if err != nil {
					return err
				}
			}
			format, err := outputFormat()
			if err != nil {
				return err
			}
			return output.WriteClusterCandidates(raw, format)
		},
	}
	c.Flags().StringArrayVar(&require, "require", nil, "Required capability (repeatable/comma): gpu, kvm, kubevirt, network.multus_l2, ha_topology, storage.rwx, storage.rwo")
	c.Flags().StringVar(&site, "site", "", "Limit to clusters on this site id")
	return c
}

func filterClustersBySite(raw []byte, site string) ([]byte, error) {
	var body struct {
		Clusters []map[string]interface{} `json:"clusters"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	out := make([]map[string]interface{}, 0, len(body.Clusters))
	for _, c := range body.Clusters {
		if strings.TrimSpace(fmt.Sprint(c["site"])) == site {
			out = append(out, c)
		}
	}
	body.Clusters = out
	return json.Marshal(body)
}

func clusterGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get [name]",
		Short: "Get cluster details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Get("/clusters/" + args[0])
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

func clusterCreateCmd() *cobra.Command {
	var (
		file            string
		name            string
		site            string
		bootstrapHost   string
		topology        string
		controlPlaneVip string
		minRootDisk     int
		jumphost        string
		ingressID       string
		autoProvision   bool
		createVMs       bool
		controlAddrs    string
		workerAddrs     string
		controlCores    int
		controlMemory   int
		controlDisk     int
		controlDataDisk int
		workerCores     int
		workerMemory    int
		workerDisk      int
		workerDataDisk  int
		hypervisor      string
	)
	c := &cobra.Command{
		Use:   "create",
		Short: "Create cluster intent on the hub (POST /clusters)",
		Long: `Create a cluster from a JSON body (-f) or from flags.

With --create-vms the node VMs are generated from the address lists and given to
the bring-up pipeline, which declares the YAIDAP guests (named after the cluster)
and creates them — no separate "site host add" step. Without it, nodes are used
as given (existing inventory hosts or external/bare-metal nodes).

  umkim cluster create \
    --name lab-ha-1 --site homelab --topology ha_3_node \
    --control-addresses 172.16.10.101,172.16.10.102,172.16.10.103 \
    --control-plane-vip 172.16.10.111 --create-vms \
    --control-cores 4 --control-memory 6144 --control-disk 50 \
    --auto-provision

Flags override the same fields in -f; --create-vms replaces nodes.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]interface{}{}
			if file != "" {
				if err := readFileJSON(file, &body); err != nil {
					return err
				}
			}
			changed := func(flag string) bool { return cmd.Flags().Changed(flag) }
			if changed("name") {
				body["clusterName"] = name
			}
			if changed("site") {
				body["siteId"] = site
			}
			if changed("bootstrap-host") {
				body["bootstrapHost"] = bootstrapHost
			}
			if changed("topology") {
				body["topology"] = topology
			}
			if changed("control-plane-vip") {
				body["controlPlaneVip"] = controlPlaneVip
			}
			if changed("min-root-disk") {
				body["minRootDiskGi"] = minRootDisk
			}
			if changed("jumphost") {
				body["jumphost"] = jumphost
			}
			if changed("ingress-id") {
				edge, _ := body["edge"].(map[string]interface{})
				if edge == nil {
					edge = map[string]interface{}{}
				}
				edge["ingressId"] = ingressID
				body["edge"] = edge
			}
			if changed("auto-provision") {
				body["autoProvision"] = autoProvision
			}
			if createVMs {
				clusterName, _ := body["clusterName"].(string)
				if strings.TrimSpace(clusterName) == "" {
					return fmt.Errorf("--create-vms needs a cluster name (--name or -f)")
				}
				topo, _ := body["topology"].(string)
				nodes, err := buildVMNodes(
					topo, controlAddrs, workerAddrs,
					nodeVMFlags{cores: controlCores, memory: controlMemory, diskGi: controlDisk, dataDiskGi: controlDataDisk},
					nodeVMFlags{cores: workerCores, memory: workerMemory, diskGi: workerDisk, dataDiskGi: workerDataDisk},
					hypervisor,
				)
				if err != nil {
					return err
				}
				body["nodes"] = nodes
				// The first control node is the bootstrap host.
				if _, ok := body["bootstrapHost"]; !ok {
					body["bootstrapHost"] = nodes[0]["address"]
				}
			}
			if v, _ := body["clusterName"].(string); strings.TrimSpace(v) == "" {
				return fmt.Errorf("cluster name required (--name or -f)")
			}
			if v, _ := body["siteId"].(string); strings.TrimSpace(v) == "" {
				return fmt.Errorf("site required (--site or -f)")
			}
			if v, _ := body["bootstrapHost"].(string); strings.TrimSpace(v) == "" {
				return fmt.Errorf("bootstrap host required (--bootstrap-host or -f)")
			}
			raw, code, err := api.Post("/clusters", body)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, http.StatusCreated, http.StatusConflict); err != nil {
				return err
			}
			printJSON(raw)
			return nil
		},
	}
	c.Flags().StringVarP(&file, "file", "f", "", "Cluster create JSON body (flags override its fields)")
	c.Flags().StringVar(&name, "name", "", "Cluster name")
	c.Flags().StringVar(&site, "site", "", "Site id")
	c.Flags().StringVar(&bootstrapHost, "bootstrap-host", "", "Bootstrap host address (defaults to the first control address with --create-vms)")
	c.Flags().StringVar(&topology, "topology", "", "single_node | ha_3_node")
	c.Flags().StringVar(&controlPlaneVip, "control-plane-vip", "", "Control-plane VIP (ha_3_node)")
	c.Flags().IntVar(&minRootDisk, "min-root-disk", 0, "Minimum root filesystem size in GiB validated by the bootstrap")
	c.Flags().StringVar(&jumphost, "jumphost", "", "Site jumphost override (user@host:port)")
	c.Flags().StringVar(&ingressID, "ingress-id", "", "Site ingress entry id")
	c.Flags().BoolVar(&autoProvision, "auto-provision", false, "Let the controller create the VMs, bootstrap, and provision")
	c.Flags().BoolVar(&createVMs, "create-vms", false, "Declare and create the node VMs (needs --control-addresses)")
	c.Flags().StringVar(&controlAddrs, "control-addresses", "", "Comma-separated control node addresses")
	c.Flags().StringVar(&workerAddrs, "worker-addresses", "", "Comma-separated worker node addresses")
	c.Flags().IntVar(&controlCores, "control-cores", 0, "Control node vCPU (0 = catalog default)")
	c.Flags().IntVar(&controlMemory, "control-memory", 0, "Control node memory MiB (0 = catalog default)")
	c.Flags().IntVar(&controlDisk, "control-disk", 0, "Control node boot disk GiB (0 = catalog default)")
	c.Flags().IntVar(&controlDataDisk, "control-data-disk", 0, "Control node data disk GiB (0 = derived from storage.longhorn)")
	c.Flags().IntVar(&workerCores, "worker-cores", 0, "Worker node vCPU (0 = catalog default)")
	c.Flags().IntVar(&workerMemory, "worker-memory", 0, "Worker node memory MiB (0 = catalog default)")
	c.Flags().IntVar(&workerDisk, "worker-disk", 0, "Worker node boot disk GiB (0 = catalog default)")
	c.Flags().IntVar(&workerDataDisk, "worker-data-disk", 0, "Worker node data disk GiB (0 = derived from storage.longhorn)")
	c.Flags().StringVar(&hypervisor, "hypervisor", "", "Hypervisor for the node VMs (empty = least-utilized)")
	return c
}

// nodeVMFlags are the per-role VM resources for --create-vms.
type nodeVMFlags struct {
	cores      int
	memory     int
	diskGi     int
	dataDiskGi int
}

// buildVMNodes expands address lists into cluster nodes carrying a vm block. Node
// hostnames are left to the server, which names guests after the cluster.
func buildVMNodes(topology, controlAddrs, workerAddrs string, control, worker nodeVMFlags, hypervisor string) ([]map[string]interface{}, error) {
	topology = strings.TrimSpace(topology)
	if topology == "" {
		topology = "single_node"
	}
	controls := splitAddresses(controlAddrs)
	workers := splitAddresses(workerAddrs)
	switch topology {
	case "single_node":
		if len(controls) != 1 {
			return nil, fmt.Errorf("single_node needs exactly one --control-addresses entry (got %d)", len(controls))
		}
		if len(workers) > 0 {
			return nil, fmt.Errorf("single_node does not support workers")
		}
	case "ha_3_node":
		if len(controls) != 3 {
			return nil, fmt.Errorf("ha_3_node needs exactly three --control-addresses entries (got %d)", len(controls))
		}
	default:
		return nil, fmt.Errorf("--topology must be single_node or ha_3_node")
	}
	seen := map[string]bool{}
	nodes := make([]map[string]interface{}, 0, len(controls)+len(workers))
	add := func(role, address string, res nodeVMFlags) error {
		address = strings.TrimSpace(address)
		if address == "" {
			return fmt.Errorf("empty address for %s node", role)
		}
		if seen[address] {
			return fmt.Errorf("duplicate node address %s", address)
		}
		seen[address] = true
		vm := map[string]interface{}{}
		if res.cores > 0 {
			vm["cores"] = res.cores
		}
		if res.memory > 0 {
			vm["memory"] = res.memory
		}
		if res.diskGi > 0 {
			vm["diskGi"] = res.diskGi
		}
		if res.dataDiskGi > 0 {
			vm["dataDiskGi"] = res.dataDiskGi
		}
		if h := strings.TrimSpace(hypervisor); h != "" {
			vm["hypervisor"] = h
		}
		nodes = append(nodes, map[string]interface{}{"address": address, "role": role, "vm": vm})
		return nil
	}
	for _, a := range controls {
		if err := add("control", a, control); err != nil {
			return nil, err
		}
	}
	for _, a := range workers {
		if err := add("worker", a, worker); err != nil {
			return nil, err
		}
	}
	return nodes, nil
}

func splitAddresses(raw string) []string {
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func clusterDeleteCmd() *cobra.Command {
	var cleanupK8s, cleanupVms, cleanupVmDecls, cleanupApps, cleanupStorage, force, wait bool
	var waitTimeout time.Duration
	var waitInterval time.Duration
	c := &cobra.Command{
		Use:   "delete [name]",
		Short: "Delete a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			path := "/clusters/" + name
			q := map[string]string{}
			if cleanupK8s {
				q["cleanupKubernetes"] = "true"
			}
			if cleanupVms || cleanupVmDecls {
				// Destroying the VMs implies resetting Kubernetes first.
				q["cleanupKubernetes"] = "true"
				q["cleanupVms"] = "true"
			}
			if cleanupVmDecls {
				q["cleanupVmDeclarations"] = "true"
			}
			if cleanupApps {
				q["cleanupApps"] = "true"
			}
			if cleanupStorage {
				q["cleanupStorage"] = "true"
			}
			if force {
				q["force"] = "true"
			}
			raw, code, err := api.DeleteQuery(path, q)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, http.StatusAccepted, http.StatusOK); err != nil {
				return err
			}
			if !wait {
				printJSON(raw)
				return nil
			}
			deadline := time.Now().Add(waitTimeout)
			for {
				_, getCode, err := api.Get(path)
				if err != nil {
					return err
				}
				if getCode == http.StatusNotFound {
					printJSON(raw)
					return nil
				}
				if time.Now().After(deadline) {
					return fmt.Errorf("timeout waiting for cluster %q to be removed", name)
				}
				time.Sleep(waitInterval)
			}
		},
	}
	c.Flags().BoolVar(&cleanupK8s, "cleanup-kubernetes", false, "Run Kubernetes cleanup on spoke nodes")
	c.Flags().BoolVar(&cleanupVms, "cleanup-vms", false, "Also destroy the site VMs backing the cluster (implies --cleanup-kubernetes)")
	c.Flags().BoolVar(&cleanupVmDecls, "cleanup-vm-declarations", false, "Also remove the destroyed VMs from the site inventory (implies --cleanup-vms)")
	c.Flags().BoolVar(&cleanupApps, "cleanup-apps", false, "Uninstall the cluster's applications first (needs the spoke reachable)")
	c.Flags().BoolVar(&cleanupStorage, "cleanup-storage", false, "Also delete the cluster's NFS export directory on the storage host (data)")
	c.Flags().BoolVar(&force, "force", false, "Force delete while provisioning")
	c.Flags().BoolVar(&wait, "wait", false, "Poll until the cluster is removed from the API")
	c.Flags().DurationVar(&waitTimeout, "timeout", 90*time.Minute, "With --wait, overall timeout")
	c.Flags().DurationVar(&waitInterval, "interval", 3*time.Minute, "With --wait, poll interval")
	return c
}

func clusterProvisionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "provision [name]",
		Short: "Trigger cluster provisioning (POST /clusters/{name}/provision)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, code, err := api.Post("/clusters/"+args[0]+"/provision", nil)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, http.StatusAccepted); err != nil {
				return err
			}
			printJSON(raw)
			return nil
		},
	}
}

func clusterWaitCmd() *cobra.Command {
	var phase string
	var interval time.Duration
	var timeout time.Duration
	c := &cobra.Command{
		Use:   "wait [name]",
		Short: "Poll cluster until provisioning_phase matches",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			deadline := time.Now().Add(timeout)
			for {
				raw, code, err := api.Get("/clusters/" + name)
				if err != nil {
					return err
				}
				if err := mustOK(raw, code); err != nil {
					return err
				}
				var summary map[string]interface{}
				if err := json.Unmarshal(raw, &summary); err != nil {
					return err
				}
				got := clusterPhase(summary)
				if strings.EqualFold(got, phase) {
					printJSON(raw)
					return nil
				}
				if time.Now().After(deadline) {
					return fmt.Errorf("timeout waiting for phase %q (last=%q)", phase, got)
				}
				time.Sleep(interval)
			}
		},
	}
	c.Flags().StringVar(&phase, "phase", "Ready", "Target provisioning_phase")
	c.Flags().DurationVar(&interval, "interval", 20*time.Second, "Poll interval")
	c.Flags().DurationVar(&timeout, "timeout", 30*time.Minute, "Overall timeout")
	return c
}

func clusterBootstrapCmd() *cobra.Command {
	var sshUser, bootstrapHost, jumphost string
	var pollInterval time.Duration
	var pollTimeout time.Duration
	c := &cobra.Command{
		Use:   "bootstrap [name]",
		Short: "Download bootstrap bundle and run bootstrap.sh on the first control node",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster := args[0]
			host := strings.TrimSpace(bootstrapHost)
			if host == "" {
				return fmt.Errorf("--bootstrap-host required (mgmt address of first control node)")
			}
			jump := strings.TrimSpace(jumphost)
			if jump == "" {
				var err error
				jump, err = resolveJumphost(cluster)
				if err != nil {
					return err
				}
			}
			tarPath, err := waitBootstrapBundle(cluster, pollInterval, pollTimeout)
			if err != nil {
				return err
			}
			return bootstrap.Run(bootstrap.Options{
				ClusterName: cluster,
				BundleTar:   tarPath,
				TargetHost:  host,
				SSHUser:     sshUser,
				Jumphost:    jump,
			})
		},
	}
	c.Flags().StringVar(&sshUser, "ssh-user", "linux", "SSH user on bootstrap host")
	c.Flags().StringVar(&bootstrapHost, "bootstrap-host", "", "Bootstrap host address (required)")
	c.Flags().StringVar(&jumphost, "jumphost", "", "Override site/cluster jumphost (user@host:port)")
	c.Flags().DurationVar(&pollInterval, "interval", 20*time.Second, "Bundle poll interval")
	c.Flags().DurationVar(&pollTimeout, "timeout", 30*time.Minute, "Bundle wait timeout")
	return c
}

func clusterPhase(summary map[string]interface{}) string {
	if v, ok := summary["provisioning_phase"].(string); ok && v != "" {
		return v
	}
	if v, ok := summary["phase"].(string); ok {
		return v
	}
	return ""
}

func resolveJumphost(cluster string) (string, error) {
	raw, code, err := api.Get("/clusters/" + cluster)
	if err != nil {
		return "", err
	}
	if err := mustOK(raw, code); err != nil {
		return "", err
	}
	var summary map[string]interface{}
	if err := json.Unmarshal(raw, &summary); err != nil {
		return "", err
	}
	if jh, _ := summary["jumphost"].(string); strings.TrimSpace(jh) != "" {
		return strings.TrimSpace(jh), nil
	}
	site, _ := summary["site"].(string)
	if site == "" {
		return "", nil
	}
	siteRaw, code, err := api.Get("/sites/" + site)
	if err != nil {
		return "", err
	}
	if code != http.StatusOK {
		return "", nil
	}
	var siteDoc map[string]interface{}
	if err := json.Unmarshal(siteRaw, &siteDoc); err != nil {
		return "", err
	}
	spec, _ := siteDoc["spec"].(map[string]interface{})
	if spec == nil {
		return "", nil
	}
	jh, _ := spec["jumphost"].(string)
	return strings.TrimSpace(jh), nil
}

func clusterPatchCmd() *cobra.Command {
	var file, jumphost, ingressID string
	c := &cobra.Command{
		Use:   "patch [name]",
		Short: "Patch cluster settings (PATCH /clusters/{name})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]interface{}{}
			if file != "" {
				if err := readFileJSON(file, &body); err != nil {
					return err
				}
			}
			if cmd.Flags().Changed("jumphost") {
				body["jumphost"] = jumphost
			}
			if cmd.Flags().Changed("ingress-id") {
				body["edge"] = map[string]string{"ingress_id": ingressID}
			}
			if len(body) == 0 {
				return fmt.Errorf("no patch fields: use --file or flags")
			}
			raw, code, err := api.Patch("/clusters/"+args[0], body)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code); err != nil {
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
			msg := fmt.Sprintf("cluster %q patched", args[0])
			if cmd.Flags().Changed("ingress-id") {
				if ingressID == "" {
					msg = fmt.Sprintf("cluster %q ingress binding cleared", args[0])
				} else {
					msg = fmt.Sprintf("cluster %q bound to ingress %q", args[0], ingressID)
				}
			}
			if cmd.Flags().Changed("jumphost") {
				if jumphost == "" {
					msg += "; jumphost cleared"
				} else {
					msg += "; jumphost=" + jumphost
				}
			}
			fmt.Printf("OK: %s\n", msg)
			if cmd.Flags().Changed("ingress-id") {
				rawGet, codeGet, gerr := api.Get("/clusters/" + args[0])
				if gerr != nil {
					return fmt.Errorf("patch accepted but failed to re-read cluster: %w", gerr)
				}
				if err := mustOK(rawGet, codeGet); err != nil {
					return fmt.Errorf("patch accepted but failed to re-read cluster: %w", err)
				}
				var got struct {
					IngressID string `json:"ingress_id"`
					Edge      *struct {
						IngressID string `json:"ingress_id"`
					} `json:"edge"`
				}
				_ = json.Unmarshal(rawGet, &got)
				have := strings.TrimSpace(got.IngressID)
				if have == "" && got.Edge != nil {
					have = strings.TrimSpace(got.Edge.IngressID)
				}
				if have != strings.TrimSpace(ingressID) {
					return fmt.Errorf("patch reported OK but cluster ingress_id is %q (want %q)", have, ingressID)
				}
			}
			return nil
		},
	}
	c.Flags().StringVarP(&file, "file", "f", "", "JSON patch body")
	c.Flags().StringVar(&jumphost, "jumphost", "", "SSH jumphost override (user@host:port); empty clears")
	c.Flags().StringVar(&ingressID, "ingress-id", "", "Site ingress row ID")
	return c
}

func waitBootstrapBundle(cluster string, interval, timeout time.Duration) (string, error) {
	dir, err := bootstrap.DefaultBundleDir(cluster)
	if err != nil {
		return "", err
	}
	tarPath := filepath.Join(filepath.Dir(dir), cluster+".tar.gz")
	if err := os.MkdirAll(filepath.Dir(tarPath), 0o755); err != nil {
		return "", err
	}
	deadline := time.Now().Add(timeout)
	for {
		err := api.DownloadGET("/clusters/"+cluster+"/bootstrap-bundle", tarPath)
		if err == nil {
			return tarPath, nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("bootstrap bundle not ready: %w", err)
		}
		time.Sleep(interval)
	}
}
