package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors/yaidap"
	"github.com/rborso/umkim/controllers/fleet-site/internal/siteprovision"
	fleetssh "github.com/rborso/umkim/controllers/fleet-site/internal/ssh"
	sshlib "golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// verifyProbe is a read-only host check. It prints KEY=VALUE lines and changes
// nothing on the target: it only reads systemd state and asks libvirt what it
// knows (virsh version/list/pool-list/net-list).
const verifyProbe = `set -u
echo "host=$(hostname 2>/dev/null)"
echo "kernel=$(uname -r 2>/dev/null)"
echo "cpus=$(nproc 2>/dev/null)"
echo "mem_mb=$(awk '/MemTotal/{print int($2/1024)}' /proc/meminfo 2>/dev/null)"
svc=""
for s in libvirtd virtqemud; do
  if systemctl is-active --quiet "$s" 2>/dev/null; then svc="$s"; break; fi
done
echo "libvirt_service=$svc"
echo "virsh_version=$(sudo -n virsh version --daemon 2>/dev/null | awk '/Using library/{print $NF}' | head -1)"
echo "domains=$(sudo -n virsh list --all --name 2>/dev/null | grep -c .)"
echo "pools=$(sudo -n virsh pool-list --all --name 2>/dev/null | grep -c .)"
echo "networks=$(sudo -n virsh net-list --all --name 2>/dev/null | grep -c .)"`

// verifyWorkloadProbe is the read-only check for a workload node (a bare-metal
// cluster member). A workload runs no libvirt, so it reports systemd state and
// asks the local K3s supervisor what it knows about this node. Nothing is
// changed on the host. k3s is called by absolute path because sudo resets PATH.
const verifyWorkloadProbe = `set -u
echo "host=$(hostname 2>/dev/null)"
echo "kernel=$(uname -r 2>/dev/null)"
echo "cpus=$(nproc 2>/dev/null)"
echo "mem_mb=$(awk '/MemTotal/{print int($2/1024)}' /proc/meminfo 2>/dev/null)"
svc=""
for s in k3s k3s-agent; do
  if systemctl is-active --quiet "$s" 2>/dev/null; then svc="$s"; break; fi
done
echo "k3s_service=$svc"
kc="/usr/local/bin/k3s kubectl"
if [ ! -x /usr/local/bin/k3s ]; then kc="/usr/bin/kubectl --kubeconfig /etc/rancher/k3s/k3s.yaml"; fi
node="$(hostname 2>/dev/null)"
echo "k3s_version=$(sudo -n /usr/local/bin/k3s --version 2>/dev/null | awk '/^k3s version/{print $3}' | head -1)"
echo "node_ready=$(sudo -n $kc get node "$node" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null)"
echo "node_ip=$(sudo -n $kc get node "$node" -o jsonpath='{.status.addresses[?(@.type=="InternalIP")].address}' 2>/dev/null)"
echo "node_version=$(sudo -n $kc get node "$node" -o jsonpath='{.status.nodeInfo.kubeletVersion}' 2>/dev/null)"
echo "nodes=$(sudo -n $kc get nodes --no-headers 2>/dev/null | grep -c .)"`

// verifyHostResult is the parsed read-only probe output.
type verifyHostResult struct {
	Host           string `json:"host,omitempty"`
	Kernel         string `json:"kernel,omitempty"`
	CPUs           string `json:"cpus,omitempty"`
	MemMB          string `json:"memMb,omitempty"`
	LibvirtService string `json:"libvirtService,omitempty"`
	VirshVersion   string `json:"virshVersion,omitempty"`
	Domains        string `json:"domains,omitempty"`
	Pools          string `json:"pools,omitempty"`
	Networks       string `json:"networks,omitempty"`
	K3sService     string `json:"k3sService,omitempty"`
	K3sVersion     string `json:"k3sVersion,omitempty"`
	NodeReady      string `json:"nodeReady,omitempty"`
	NodeIP         string `json:"nodeIp,omitempty"`
	NodeVersion    string `json:"nodeVersion,omitempty"`
	Nodes          string `json:"nodes,omitempty"`
	Raw            string `json:"raw,omitempty"`
}

// ok reports whether the probe found the control plane the host's role needs:
// libvirt for a hypervisor, the K3s supervisor for a workload node.
func (v verifyHostResult) ok(kind string) bool {
	if isWorkloadKind(kind) {
		return strings.TrimSpace(v.K3sService) != "" &&
			strings.EqualFold(strings.TrimSpace(v.NodeReady), "True")
	}
	return strings.TrimSpace(v.LibvirtService) != "" && strings.TrimSpace(v.VirshVersion) != ""
}

func (v verifyHostResult) summary(kind string) string {
	workload := isWorkloadKind(kind)
	if !v.ok(kind) {
		if workload {
			return "read-only check: k3s not responding"
		}
		return "read-only check: libvirt not responding"
	}
	var parts []string
	if workload {
		parts = append(parts, fmt.Sprintf("k3s %s (%s)", v.K3sVersion, v.K3sService))
		if v.NodeReady != "" {
			parts = append(parts, "node Ready="+v.NodeReady)
		}
		if v.NodeIP != "" {
			parts = append(parts, v.NodeIP)
		}
		if v.Nodes != "" {
			parts = append(parts, fmt.Sprintf("%s node(s)", v.Nodes))
		}
	} else {
		parts = append(parts, fmt.Sprintf("libvirt %s (%s)", v.VirshVersion, v.LibvirtService))
		if v.Domains != "" {
			parts = append(parts, fmt.Sprintf("%s domain(s)", v.Domains))
		}
	}
	if v.CPUs != "" {
		parts = append(parts, fmt.Sprintf("%s vCPU", v.CPUs))
	}
	if v.MemMB != "" {
		if mb, err := strconv.Atoi(v.MemMB); err == nil {
			parts = append(parts, fmt.Sprintf("%d GiB RAM", mb/1024))
		}
	}
	return "verified (read-only): " + strings.Join(parts, ", ")
}

// isWorkloadKind reports whether a host kind is a workload (no libvirt) rather
// than a virtualization host.
func isWorkloadKind(kind string) bool {
	return strings.TrimSpace(kind) == fleetv1alpha1.SiteHostKindBareMetal
}

// verifySiteHost runs a read-only check against a hypervisor/bare-metal host
// through the site jumphost and records the outcome as the guest status. A host
// that answers moves from Declared to Ready; nothing is changed on the host.
func (s *Server) verifySiteHost(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName, hostName string) {
	var fs fleetv1alpha1.FleetSite
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: siteName}, &fs); err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "site not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	prov := fs.Spec.Provision
	if prov == nil || strings.TrimSpace(prov.Jumphost) == "" {
		http.Error(w, "site has no provisioner jumphost configured", http.StatusConflict)
		return
	}
	jh, ok := fleetssh.ParseJumphost(prov.Jumphost, "linux")
	if !ok {
		http.Error(w, "site jumphost is invalid", http.StatusConflict)
		return
	}

	host, found, err := yaidap.LookupInventoryHost(ctx, s.Client, s.Cfg.Namespace, siteName, hostName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "host not found", http.StatusNotFound)
		return
	}
	target := strings.TrimSpace(host.AnsibleHost)
	if target == "" {
		http.Error(w, "host has no ansible_host", http.StatusConflict)
		return
	}

	key, err := s.siteProvisionSSHKey(ctx, prov.SSHSecretRef)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	// A hypervisor is checked for libvirt; a workload node has no libvirt, so it
	// is checked for the K3s supervisor and its cluster membership instead.
	kind := strings.TrimSpace(host.Kind)
	probe := verifyProbe
	if isWorkloadKind(kind) {
		probe = verifyWorkloadProbe
	}

	out, err := runReadOnlyProbe(ctx, jh, key, target, probe, 90*time.Second)
	res := parseVerifyOutput(out)
	// The first hop from the jumphost may still be learning the host key; retry
	// once before calling a host unhealthy.
	if !res.ok(kind) {
		if out2, err2 := runReadOnlyProbe(ctx, jh, key, target, probe, 90*time.Second); err2 == nil || strings.TrimSpace(out2) != "" {
			if r2 := parseVerifyOutput(out2); r2.ok(kind) {
				out, err, res = out2, err2, r2
			}
		}
	}
	res.Raw = out
	if err != nil && strings.TrimSpace(out) == "" {
		s.recordVerify(ctx, &fs, siteName, hostName, fleetv1alpha1.SiteHostPhaseFailed, "read-only check failed: "+err.Error())
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	phase := fleetv1alpha1.SiteHostPhaseFailed
	if res.ok(kind) {
		phase = fleetv1alpha1.SiteHostPhaseReady
	}
	msg := res.summary(kind)
	if err != nil {
		msg = msg + " (" + err.Error() + ")"
	}
	s.recordVerify(ctx, &fs, siteName, hostName, phase, msg)
	writeJSON(w, http.StatusOK, map[string]any{
		"name": hostName, "phase": phase, "message": msg, "checks": res,
	})
}

// recordVerify stores the outcome on the guest and appends it to the site
// provision history.
func (s *Server) recordVerify(ctx context.Context, fs *fleetv1alpha1.FleetSite, siteName, hostName, phase, message string) {
	_ = yaidap.UpdateGuestStatus(ctx, s.Client, s.Cfg.Namespace, siteName, hostName, yaidap.GuestStatus{
		Phase:   phase,
		Message: message,
	})
	now := metav1.NewTime(time.Now().UTC())
	rec := fleetv1alpha1.SiteProvisionActionRecord{
		ID:        "verify-" + strconv.FormatInt(now.UnixNano(), 36),
		Action:    fleetv1alpha1.ProvisionActionVerify,
		Subject:   hostName,
		Provider:  fs.Spec.EffectiveProvisionProvider(),
		Phase:     phaseToProvisionPhase(phase),
		ActorKind: "User",
		Message:   message,
		EndedAt:   &now,
	}
	_ = siteprovision.CommitAction(ctx, s.Client, fs, rec, true)
}

func phaseToProvisionPhase(phase string) string {
	if phase == fleetv1alpha1.SiteHostPhaseReady {
		return fleetv1alpha1.ProvisionPhaseSucceeded
	}
	return fleetv1alpha1.ProvisionPhaseFailed
}

func parseVerifyOutput(out string) verifyHostResult {
	res := verifyHostResult{}
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch k {
		case "host":
			res.Host = v
		case "kernel":
			res.Kernel = v
		case "cpus":
			res.CPUs = v
		case "mem_mb":
			res.MemMB = v
		case "libvirt_service":
			res.LibvirtService = v
		case "virsh_version":
			res.VirshVersion = v
		case "domains":
			res.Domains = v
		case "pools":
			res.Pools = v
		case "networks":
			res.Networks = v
		case "k3s_service":
			res.K3sService = v
		case "k3s_version":
			res.K3sVersion = v
		case "node_ready":
			res.NodeReady = v
		case "node_ip":
			res.NodeIP = v
		case "node_version":
			res.NodeVersion = v
		case "nodes":
			res.Nodes = v
		}
	}
	return res
}

// siteProvisionSSHKey reads the jumphost private key from the site's secret.
func (s *Server) siteProvisionSSHKey(ctx context.Context, ref string) ([]byte, error) {
	name := strings.TrimSpace(ref)
	if name == "" {
		return nil, fmt.Errorf("site has no provision sshSecretRef")
	}
	var sec corev1.Secret
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: name}, &sec); err != nil {
		return nil, fmt.Errorf("read provision ssh secret %q: %w", name, err)
	}
	for _, k := range []string{"id_ed25519", "id_rsa", "ssh-privatekey", "privateKey"} {
		if v, ok := sec.Data[k]; ok && len(v) > 0 {
			return v, nil
		}
	}
	return nil, fmt.Errorf("provision ssh secret %q has no private key", name)
}

// runReadOnlyProbe dials the jumphost and runs the probe against target over a
// nested ssh, so the hop uses the jumphost's own keys exactly like the Ansible
// path does.
func runReadOnlyProbe(ctx context.Context, jh fleetssh.Jumphost, key []byte, target, script string, timeout time.Duration) (string, error) {
	signer, err := sshlib.ParsePrivateKey(key)
	if err != nil {
		return "", fmt.Errorf("parse jumphost key: %w", err)
	}
	cfg := &sshlib.ClientConfig{
		User:            jh.User,
		Auth:            []sshlib.AuthMethod{sshlib.PublicKeys(signer)},
		HostKeyCallback: sshlib.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}
	addr := net.JoinHostPort(jh.Host, jh.Port)
	client, err := sshlib.Dial("tcp", addr, cfg)
	if err != nil {
		return "", fmt.Errorf("connect jumphost %s: %w", addr, err)
	}
	defer client.Close()

	remoteUser := "linux"
	nested := fmt.Sprintf(
		"ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=10 %s@%s %s",
		remoteUser, shellQuote(target), shellQuote(script),
	)
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	type result struct {
		out []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := session.CombinedOutput(nested)
		done <- result{out, err}
	}()
	select {
	case <-ctx.Done():
		_ = session.Signal(sshlib.SIGTERM)
		return "", ctx.Err()
	case <-time.After(timeout):
		_ = session.Signal(sshlib.SIGTERM)
		return "", fmt.Errorf("read-only check timed out")
	case r := <-done:
		return string(r.out), r.err
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// jsonBody is used by tests to decode the verify response.
func decodeVerifyBody(raw []byte) (verifyHostResult, error) {
	var body struct {
		Checks verifyHostResult `json:"checks"`
	}
	err := json.Unmarshal(raw, &body)
	return body.Checks, err
}
