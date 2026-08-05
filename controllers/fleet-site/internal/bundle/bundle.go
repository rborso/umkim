package bundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"

	"github.com/rborso/umkim/controllers/fleet-site/internal/wg"
	"golang.org/x/crypto/ssh"
)

//go:embed static/*
var staticFS embed.FS

const (
	// BootstrapArchiveKey is the Secret data key for the bootstrap archive.
	BootstrapArchiveKey = "bootstrap.tar.gz"
	AnnotationBundleMode = "fleet.umkim.com/bundle-mode"
)

// ArchiveContains reports whether name exists in a bootstrap tar.gz archive.
func ArchiveContains(archiveBytes []byte, name string) (bool, error) {
	gr, err := gzip.NewReader(bytes.NewReader(archiveBytes))
	if err != nil {
		return false, err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return false, err
		}
		if hdr.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// WireguardConfig is a per-node overlay WireGuard config embedded in the bootstrap archive.
type WireguardConfig struct {
	Hostname string
	Content  []byte
}

type Params struct {
	ClusterName       string
	SiteID            string
	InventoryINI      string
	EnrollmentToken   string
	BootstrapScript   string
	// HostBridgeScript is configure-kubevirt-bridge.sh (empty = omit from archive).
	HostBridgeScript  string
	SSHPrivateKey     []byte
	EnrollmentBaseURL string
	WireguardConfigs  []WireguardConfig
}

// Build creates a bootstrap tar.gz for local provision.
func Build(p Params) ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	write := func(name string, data []byte, mode int64) error {
		return writeTarFile(tw, name, data, mode)
	}

	if err := addStaticTree(write); err != nil {
		return nil, err
	}
	if err := write("bootstrap.sh", []byte(p.BootstrapScript), 0755); err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.HostBridgeScript) != "" {
		if err := write("configure-kubevirt-bridge.sh", []byte(p.HostBridgeScript), 0755); err != nil {
			return nil, err
		}
	}
	if err := write("inventory.ini", []byte(p.InventoryINI), 0644); err != nil {
		return nil, err
	}
	if err := write("enrollment-token", []byte(p.EnrollmentToken), 0600); err != nil {
		return nil, err
	}
	if len(p.SSHPrivateKey) > 0 {
		if err := write("ssh/id_rsa", p.SSHPrivateKey, 0600); err != nil {
			return nil, err
		}
		pub, err := sshPublicKeyAuthorized(p.SSHPrivateKey)
		if err != nil {
			return nil, fmt.Errorf("derive ssh public key: %w", err)
		}
		if err := write("ssh/id_rsa.pub", pub, 0644); err != nil {
			return nil, err
		}
	}
	for _, wgCfg := range p.WireguardConfigs {
		name := fmt.Sprintf("wireguard/%s/%s", wgCfg.Hostname, wg.OverlayConfigBasename)
		if err := write(name, wgCfg.Content, 0600); err != nil {
			return nil, err
		}
	}
	readme := fmt.Sprintf(`UMKIM fleet bootstrap bundle — cluster %s (site %s)

1. Copy this archive to the bootstrap node (first control plane).
2. tar -xzf bootstrap-%s.tar.gz && cd bootstrap-%s
3. sudo ./bootstrap.sh
   (WireGuard on all nodes via Ansible SSH; K3s + Calico follow locally.)
4. When phase is Validated, use Fleet Console → Provision for hub finalize.

Optional local cleanup (on any node; does not delete hub FleetClusterJoin):
  sudo ./cleanup_node_local.sh --cluster %s --mode detach   # keep K3s + apps
  sudo ./cleanup_node_local.sh --cluster %s --mode reset    # uninstall K3s too

Nodes include control-plane and optional worker roles. Treat enrollment-token,
ssh/id_rsa, and wireguard/*/`+wg.OverlayConfigBasename+` as secrets.
`, p.ClusterName, p.SiteID, p.ClusterName, p.ClusterName, p.ClusterName, p.ClusterName)
	if err := write("README.txt", []byte(readme), 0644); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func sshPublicKeyAuthorized(privateKey []byte) ([]byte, error) {
	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		return nil, err
	}
	return ssh.MarshalAuthorizedKey(signer.PublicKey()), nil
}

func addStaticTree(write func(name string, data []byte, mode int64) error) error {
	return fs.WalkDir(staticFS, "static", func(entryPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(entryPath, "static/")
		if rel == "" {
			return nil
		}
		data, err := staticFS.ReadFile(entryPath)
		if err != nil {
			return err
		}
		mode := int64(0644)
		if strings.HasSuffix(rel, ".sh") {
			mode = 0755
		}
		return write(rel, data, mode)
	})
}

func writeTarFile(tw *tar.Writer, name string, data []byte, mode int64) error {
	hdr := &tar.Header{
		Name: path.Clean(name),
		Mode: mode,
		Size: int64(len(data)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}
