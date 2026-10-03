package bundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestBuildIncludesCleanupNodeLocal(t *testing.T) {
	archive, err := Build(Params{
		ClusterName:     "lab-int04",
		SiteID:          "homelab-shared",
		InventoryINI:    "[fleet_cluster_control]\nlab-int04 ansible_host=192.168.1.50\n",
		EnrollmentToken: "token",
		BootstrapScript: "#!/bin/bash\necho ok\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	ok, err := ArchiveContains(archive, "cleanup_node_local.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("bootstrap archive missing cleanup_node_local.sh")
	}

	readme, err := tarGzipFileContents(archive, "README.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readme, "cleanup_node_local.sh") {
		t.Fatalf("README.txt missing cleanup_node_local.sh docs:\n%s", readme)
	}
	if !strings.Contains(readme, "--mode detach") {
		t.Fatalf("README.txt missing detach mode docs:\n%s", readme)
	}

	script, err := tarGzipFileContents(archive, "cleanup_node_local.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(script, "#!/") {
		t.Fatalf("cleanup_node_local.sh missing shebang: %q", script[:min(40, len(script))])
	}
	if !strings.Contains(script, "--mode") {
		t.Fatal("cleanup_node_local.sh missing --mode handling")
	}
}

func tarGzipFileContents(archive []byte, name string) (string, error) {
	gr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return "", err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if hdr.Name != name {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	return "", fmt.Errorf("%s not found in archive", name)
}
