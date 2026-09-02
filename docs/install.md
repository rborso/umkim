# Install a UMKIM hub

This is the greenfield path: install one hub, then create workload clusters
through FleetClusterJoin. It does not create Ansible-managed spokes.

> [!WARNING]
> This v0.2 path is experimental and can make destructive changes. Review
> [../SECURITY.md](../SECURITY.md) and [../RELEASE_NOTES.md](../RELEASE_NOTES.md)
> before continuing.

## Prerequisites

- Rocky Linux 10 hub host, non-root operator account, passwordless `sudo`
- Roughly 4+ vCPU, 8+ GiB RAM, and 100+ GiB disk
- Public IPv4 with WAN TCP 22/80/443 and UDP 51820 (WireGuard)
- DNS and firewall control for intended public application endpoints
- Private management connectivity for cluster enrollment
- Workload nodes for join are Rocky Linux 10 as well

### Controller packages

Install packages, clone UMKIM, then install Ansible collections:

```bash
sudo dnf install -y ansible-core git python3-pip python3-libselinux \
  httpd-tools podman containernetworking-plugins
pip install netaddr

git clone https://github.com/rborso/umkim.git ~/umkim
cd ~/umkim
ansible-galaxy collection install -r requirements.yml
```

Harbor fleet image builds use this checkout. Sibling service-provider
repos are not required for hub install or cluster join.

## 1. Create inventory

Never publish or reuse a filled-in inventory. Start from the hub-only template:

```bash
cd ~/umkim
./scripts/bootstrap-hub-inventory.sh
```

Edit these generated files and replace every `CHANGE_ME` / example value before
continuing:

| File | What to set |
|------|-------------|
| `inventory/hosts.yml` | Hub inventory host key, public IPv4 (`ansible_host`), SSH user |
| `inventory/host_vars/umkim-hub.yml` | `fleet_hub_public_ipv4` (same WAN IPv4 as in `hosts.yml`) |
| `inventory/group_vars/all/site.yml` | DNS zone, ACME email, WireGuard hostname, LDAP/`domain`, management site ids, per-site `exposure_mode` |
| `inventory/group_vars/all/fleet_sites.yml` | Per-site exposure profiles — see [examples/site-profiles/](../examples/site-profiles/) |

Then run:

```bash
export INV="-i inventory/hosts.yml -i inventory/hosts-from-hub.yml"
./scripts/preflight-hub-inventory.sh
ansible $INV -m ping hub
```

The template is documented in
[examples/inventory/hub-only/README.md](../examples/inventory/hub-only/README.md).

## 2. Create encrypted secrets

Choose one path below. Store the Vault password and an encrypted Vault backup
outside the repository. See [secrets.md](secrets.md).

### Create passwords manually

Copy the example schema, replace every `REPLACE_ME` value, then encrypt:

```bash
cp docs/vault.example.yml inventory/group_vars/all/vault.yml
# Edit inventory/group_vars/all/vault.yml — set all required secrets
openssl rand -base64 32 > ~/.umkim-vault-pass
chmod 600 ~/.umkim-vault-pass
env -u ANSIBLE_VAULT_PASSWORD_FILE ansible-vault encrypt \
  inventory/group_vars/all/vault.yml \
  --vault-password-file ~/.umkim-vault-pass \
  --encrypt-vault-id default
```

Optional keys (per-cluster LLDAP passwords, Gitea chart admin when installing
Gitea via FleetApp) stay in the example for later use; leave them unset or
commented until needed.

### Generate passwords

The helper fills hub-required secrets with random values and encrypts the file.
It omits optional per-cluster LLDAP and Gitea chart keys (add those later from
[`vault.example.yml`](vault.example.yml) if you need them):

```bash
./scripts/generate-vault-secrets.sh --encrypt \
  --vault-pass-file ~/.umkim-vault-pass
```

Use `--force` only when intentionally overwriting an existing `vault.yml`.

### Preflight

```bash
export ANSIBLE_VAULT_PASSWORD_FILE=~/.umkim-vault-pass
./scripts/preflight-v1.sh
```

## 3. Install and verify the hub

```bash
export INV="-i inventory/hosts.yml -i inventory/hosts-from-hub.yml"
export ANSIBLE_VAULT_PASSWORD_FILE=~/.umkim-vault-pass
./scripts/install-hub.sh
./scripts/verify-hub-install.sh
```

The wrapper runs host preparation, WireGuard, OCM, Harbor, platform services,
console, hub ingress, firewall, FleetSite, and fleet-edge. Image builds can take a
long time. To resume a known failed phase, use a targeted tag, for example:

```bash
ansible-playbook $INV hub-install.yml -l hub --tags harbor_push_fleet
```

## 4. Create a WireGuard client

Console and management APIs are reached over the hub overlay. After install,
generate a split-tunnel client on the hub (or a trusted Ansible controller):

```bash
export INV="-i inventory/hosts.yml -i inventory/hosts-from-hub.yml"
./scripts/wg-dev-client.sh <client-name>
# Example: ./scripts/wg-dev-client.sh laptop
```

Copy the generated config to your operator device and bring it up:

```bash
scp linux@<hub-public-ipv4>:~/umkim/output/wireguard-clients/<client-name>/wg-umkim.conf .
sudo wg-quick up ./wg-umkim.conf
ping -c1 10.10.0.1
```

Treat the config as a secret. Details and peer lifecycle are in
[wireguard-operator-clients.md](wireguard-operator-clients.md).

## 5. Create a workload cluster

After the hub is ready and your WireGuard client can reach the overlay:

1. Sign in to the console through the private management path.
2. Create a registry site other than the management site. Choose an exposure
   mode (`hub_gateway`, `port_forward`, `reverse_proxy`, or `none`) that
   matches how that location reaches the WAN. See
   [examples/site-profiles/](../examples/site-profiles/).
3. Create a FleetClusterJoin cluster for that site.
4. Download its bootstrap bundle through the authenticated console and treat
   the archive as a secret.
5. Copy the bundle to a Rocky Linux 10 control node, run `sudo ./bootstrap.sh`,
   wait for **Validated**, then select **Provision**.
6. When the cluster is **Ready**, it joins the management overlay and receives
   platform ManifestWorks (OLM, cert-manager, spoke Traefik). The spoke remains
   an independent K3s cluster. Helm installs use FleetApp onto that Ready
   cluster; do not enable leftover provider playbooks.

To remove a cluster from the hub without wiping workloads, delete it with the
default **detach** mode. Full K3s reset is opt-in (`cleanupKubernetes=true`).
See [playbooks/flows/cluster-delete.yml](../playbooks/flows/cluster-delete.yml).

## 6. Install a Helm chart on a selected cluster

There is no placement engine. Pick the Ready cluster, then install:

1. Console **Apps** → **Install chart** (library picker), or
2. Authenticated `POST /fleet/api/v1alpha1/apps` with `clusterName` and `chart`
   (controller path `/api/v1alpha1/apps`), or
3. `ansible-playbook $INV playbooks/flows/app-install.yml` with
   `fleet_flow_cluster_name` and `fleet_flow_chart`.

Publish additional charts with **Apps → Add chart** or
`./scripts/fleet-chart.sh publish`. Harbor already publishes `whoami` at
`oci://<hub-overlay>:30443/charts/whoami`. The library lists it after import.
`route.mode` defaults to `none`. Match routing to site exposure:

- `hub` — site uses `hub_gateway`; DNS for `hubHost` must be ready.
- `site` — site uses `port_forward` or `reverse_proxy`; `siteHost` and site
  ingress must be configured.
- `none` — chart or workload owns ingress (or app is internal).

Hub-routed apps require a healthy hub. `site` and `none` apps can remain
reachable when the hub is down if site ingress is healthy. Details:
[components/fleet-apps.md](components/fleet-apps.md).

For the model and lifecycle, see [design.md](design.md) and
[components/fleet-site.md](components/fleet-site.md).

## 7. Teardown and reinstall from zero

Hub teardown is destructive on the **hub host** only. It uninstalls hub K3s,
Harbor, controllers, Traefik, the nftables `fleet_hub` table, and WireGuard
so `./scripts/install-hub.sh` can run on a clean host. Detached workload
clusters keep running independently. Inventory and encrypted vault under
`inventory/` are kept.

```bash
export INV="-i inventory/hosts.yml -i inventory/hosts-from-hub.yml"
export ANSIBLE_VAULT_PASSWORD_FILE=~/.umkim-vault-pass
./scripts/teardown-hub.sh --confirm
# Optional: hub-only wipe if no workload clusters exist
# ./scripts/teardown-hub.sh --confirm --skip-cluster-delete
test ! -f /etc/rancher/k3s/k3s.yaml && echo OK
./scripts/install-hub.sh
./scripts/verify-hub-install.sh
```

## Safe troubleshooting

- Check hub readiness with `./scripts/verify-hub-install.sh`.
- Inspect controller and workload status before retrying a phase.
- Use targeted playbook tags rather than a full redeploy.
- Treat teardown as destructive; do not use it as normal troubleshooting.
  To reinstall from zero, use `./scripts/teardown-hub.sh --confirm`.

For non-destructive verification, see [fleet-verify.md](fleet-verify.md).
