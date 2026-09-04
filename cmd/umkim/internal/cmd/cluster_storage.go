package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rborso/umkim/cmd/umkim/internal/output"
	"github.com/spf13/cobra"
)

func clusterStorageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "storage",
		Short: "Configure cluster storage backends (NFS RWX)",
	}
	cmd.AddCommand(clusterStorageEnableLocalCmd())
	cmd.AddCommand(clusterStorageStatusCmd())
	return cmd
}

func clusterStorageEnableLocalCmd() *cobra.Command {
	var force bool
	var wait time.Duration
	var exportBase, clusterPath, storageClass string
	c := &cobra.Command{
		Use:   "enable-local [cluster]",
		Short: "Enable nfs_local RWX (cluster operation: POST /configure-storage)",
		Long: `Cluster operation: install nfs-server on the single-node cluster, export a local
directory, and deploy StorageClass nfs-client for ReadWriteMany.

Runs as an Ansible job on the hub (same operation queue as node lifecycle —
cannot run while a node operation is in progress). Completed runs appear in
operation history (umkim node-op history).

Default export directory is /srv/fleet-nfs (override with --dir).
The per-cluster subdirectory is <dir>/<cluster> unless --cluster-path is set.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.TrimSpace(args[0])
			exportBase = strings.TrimSpace(exportBase)
			if exportBase == "" {
				exportBase = "/srv/fleet-nfs"
			}
			if !strings.HasPrefix(exportBase, "/") {
				return fmt.Errorf("--dir must be an absolute path (got %q)", exportBase)
			}
			exportBase = strings.TrimRight(exportBase, "/")
			if exportBase == "" {
				return fmt.Errorf("--dir cannot be root /")
			}
			clusterPath = strings.TrimSpace(clusterPath)
			if clusterPath == "" {
				clusterPath = name
			}
			storageClass = strings.TrimSpace(storageClass)
			if storageClass == "" {
				storageClass = "nfs-client"
			}
			body := map[string]interface{}{
				"backend": "nfs_local",
				"rwo":     true,
				"rwx":     true,
				"nfs": map[string]interface{}{
					"exportBase":   exportBase,
					"clusterPath":  clusterPath,
					"storageClass": storageClass,
				},
			}
			if force {
				body["force"] = true
			}
			raw, code, err := api.Post("/clusters/"+name+"/configure-storage", body)
			if err != nil {
				return err
			}
			if err := mustOK(raw, code, http.StatusAccepted, http.StatusOK); err != nil {
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
			share := exportBase + "/" + clusterPath
			phase := ""
			var resp map[string]interface{}
			if json.Unmarshal(raw, &resp) == nil {
				if p, ok := resp["phase"].(string); ok {
					phase = p
				}
				if msg, ok := resp["message"].(string); ok && msg != "" {
					fmt.Printf("OK: cluster %q local RWX (%s); share %s\n", name, msg, share)
					return nil
				}
			}
			if phase == "" {
				phase = "Pending"
			}
			fmt.Printf("OK: cluster %q local RWX configure requested (phase %s); share %s\n", name, phase, share)
			if wait <= 0 {
				return nil
			}
			deadline := time.Now().Add(wait)
			for time.Now().Before(deadline) {
				st, err := fetchClusterStorageStatus(name)
				if err != nil {
					return err
				}
				p := strings.TrimSpace(st.Phase)
				switch p {
				case "Applied":
					fmt.Printf("OK: cluster %q storage Applied (share %s)\n", name, share)
					if st.Message != "" {
						fmt.Println(st.Message)
					}
					return nil
				case "Failed":
					if st.Message != "" {
						return fmt.Errorf("storage configure failed: %s", st.Message)
					}
					return fmt.Errorf("storage configure failed")
				}
				time.Sleep(3 * time.Second)
			}
			return fmt.Errorf("timed out waiting for storage Applied (use: umkim cluster storage status %s)", name)
		},
	}
	c.Flags().StringVar(&exportBase, "dir", "/srv/fleet-nfs", "Absolute path exported by nfs-server on the node (NFS share root)")
	c.Flags().StringVar(&clusterPath, "cluster-path", "", "Subdirectory under --dir (default: cluster name)")
	c.Flags().StringVar(&storageClass, "storage-class", "nfs-client", "Kubernetes StorageClass name")
	c.Flags().BoolVar(&force, "force", false, "Re-run configure even when already Applied")
	c.Flags().DurationVar(&wait, "wait", 0, "Wait for Applied/Failed (e.g. 10m); 0 = return after request accepted")
	return c
}

func clusterStorageStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [cluster]",
		Short: "Show cluster storage backend and status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.TrimSpace(args[0])
			raw, code, err := api.Get("/clusters/" + name)
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
			var doc map[string]interface{}
			if err := json.Unmarshal(raw, &doc); err != nil {
				return err
			}
			storage, _ := doc["storage"].(map[string]interface{})
			status, _ := doc["storageStatus"].(map[string]interface{})
			payload := map[string]interface{}{
				"cluster": name,
				"storage": storage,
				"status":  status,
			}
			if format == output.FormatJSON {
				b, err := json.MarshalIndent(payload, "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(b))
				return nil
			}
			backend := "—"
			if storage != nil {
				if b, ok := storage["backend"].(string); ok && b != "" {
					backend = b
				}
			}
			phase := "—"
			msg := ""
			if status != nil {
				if p, ok := status["phase"].(string); ok && p != "" {
					phase = p
				}
				if m, ok := status["message"].(string); ok {
					msg = m
				}
			}
			fmt.Printf("CLUSTER\t%s\nBACKEND\t%s\nPHASE\t%s\n", name, backend, phase)
			if msg != "" {
				fmt.Printf("MESSAGE\t%s\n", msg)
			}
			if storage != nil {
				if nfs, ok := storage["nfs"].(map[string]interface{}); ok && nfs != nil {
					if v, ok := nfs["server"].(string); ok && v != "" {
						fmt.Printf("NFS_SERVER\t%s\n", v)
					}
					if v, ok := nfs["exportBase"].(string); ok && v != "" {
						fmt.Printf("EXPORT_BASE\t%s\n", v)
					}
					if v, ok := nfs["clusterPath"].(string); ok && v != "" {
						fmt.Printf("CLUSTER_PATH\t%s\n", v)
					}
					if v, ok := nfs["storageClass"].(string); ok && v != "" {
						fmt.Printf("STORAGE_CLASS\t%s\n", v)
					}
				}
			}
			return nil
		},
	}
}

type clusterStorageStatusView struct {
	Phase   string
	Message string
}

func fetchClusterStorageStatus(name string) (*clusterStorageStatusView, error) {
	raw, code, err := api.Get("/clusters/" + name)
	if err != nil {
		return nil, err
	}
	if err := mustOK(raw, code); err != nil {
		return nil, err
	}
	var doc struct {
		StorageStatus *struct {
			Phase   string `json:"phase"`
			Message string `json:"message"`
		} `json:"storageStatus"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	out := &clusterStorageStatusView{}
	if doc.StorageStatus != nil {
		out.Phase = doc.StorageStatus.Phase
		out.Message = doc.StorageStatus.Message
	}
	return out, nil
}
