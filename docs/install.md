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

Greenfield installs pin K3s channel **`v1.36`** (stable **1.36.4+k3s1** at time of
release). Resolved platform versions are listed in [VERSIONS.md](../VERSIONS.md).

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
| `inventory/group_vars/all/site.yml` | `fleet_edge_domain`, DNS, LDAP base DN |
| `inventory/group_vars/all/fleet_sites.yml` | Management site seed (`hub_gateway: true`); workload sites are created in the console |
| `inventory/group_vars/all/vault.yml` | Encrypted secrets (see §2 below) |

`profile.yml`, `common.yml`, `hub/main.yml`, and other files copied by
`bootstrap-hub-inventory.sh` ship with template defaults; review comments
before install. Workload site ingress patterns:
[examples/site-profiles/](../examples/site-profiles/).

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

Copy the example schema, replace every `CHANGE_ME` value, then encrypt:

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
fleet-console-auth CRD, console auth, console, hub ingress, firewall, FleetSite,
and fleet-edge. Optional ingress-auth (Authelia) deploys when
`fleet_ingress_auth_enabled: true` in `inventory/group_vars/hub/main.yml`
(greenfield template defaults it off). Image builds can take a long time. To
resume a known failed phase, use a targeted tag, for example:

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

1. Sign in to the UMKIM console through the private management path.
2. Create a registry site other than the management site. Configure exposure
   to match how that location reaches the WAN:
   - optional **`hubGateway`** (inventory: `hub_gateway: true`) for hub-routed DNS;
   - optional **`ingress[]`** rows with `method` `port_forward` or `reverse_proxy`;
   - **private** when neither is set (no fleet-managed WAN path).
   The console create wizard offers `port_forward`, `hub_gateway`, or `custom`
   profiles; `reverse_proxy` is an ingress-row method under custom. See
   [examples/site-profiles/](../examples/site-profiles/) and [design.md](design.md).
3. Create a FleetClusterJoin cluster for that site.
4. Download its bootstrap bundle through the authenticated UMKIM console and treat
   the archive as a secret.
5. Copy the bundle to a Rocky Linux 10 control node, run `sudo ./bootstrap.sh`,
   wait for **Validated**, then select **Provision**. With the CLI:
   `umkim cluster bootstrap NAME --bootstrap-host MGMT_IP` (management address of
   node 1; see `bootstrapHost` in cluster JSON examples).
6. When the cluster is **Ready**, it joins the management overlay and receives
   platform ManifestWorks (OLM, cert-manager, spoke Traefik). The spoke remains
   an independent K3s cluster. Helm installs use FleetApp onto that Ready
   cluster; do not enable leftover provider playbooks.

**HA clusters** (`ha_3_node`): bootstrap on three nodes often takes ~20 minutes;
hub finalize can run up to **90 minutes** (Ansible Job deadline). Console and
CLI flows default to **30-minute** polls — use `umkim cluster wait --timeout 90m`
or raise `fleet_flow_poll_retries` in
[cluster-create.yml](../playbooks/flows/cluster-create.yml) (default 90 × 20s).
See [components/fleet-site.md](components/fleet-site.md) for hub inventory and
spoke ingress probe details.

To remove a cluster from the hub without wiping workloads, delete it with the
default **detach** mode. Full K3s reset is opt-in (`cleanupKubernetes=true`).
On a YAIDAP site, `cleanupVms=true` also destroys the cluster's guest VMs (and
`cleanupVmDeclarations=true` drops them from the site inventory) so no manual
hypervisor cleanup is needed.
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

- `hub` — site has `hubGateway` (inventory `hub_gateway: true`); DNS for
  `hubHost` must be ready.
- `site` — site has `port_forward` or `reverse_proxy` ingress; `siteHost` and
  site ingress must be configured.
- `none` — chart or workload owns ingress (or app is internal).

Hub-routed apps require a healthy hub. `site` and `none` apps can remain
reachable when the hub is down if site ingress is healthy. Details:
[components/fleet-apps.md](components/fleet-apps.md).

Optional GPU inference (`llama-cpp-helm`) and the hub LiteLLM proxy are **not**
installed by `install-hub.sh`. After a GPU-capable cluster is Ready, see
[components/gpu-host-prep.md](components/gpu-host-prep.md) and
[components/fleet-llm.md](components/fleet-llm.md) (`fleet-llm.yml` /
`umkim llm set --hub-enabled`).

For the model and lifecycle, see [design.md](design.md) and
[components/fleet-site.md](components/fleet-site.md).

## 7. Teardown and reinstall from zero

`./scripts/teardown-hub.sh --confirm` is destructive on the **hub** and, by
default, on **every console-provisioned workload cluster** (Fleet API delete with
`cleanupKubernetes=true`, then hub K3s, Harbor, controllers, Traefik, the
nftables `fleet_hub` table, and WireGuard). Use that path only when a full
greenfield reset is intended. Inventory and encrypted vault under `inventory/`
are kept.

To wipe the hub while leaving joined clusters running on their nodes (detach
without K3s reset), use `--skip-cluster-delete`:

```bash
export INV="-i inventory/hosts.yml -i inventory/hosts-from-hub.yml"
export ANSIBLE_VAULT_PASSWORD_FILE=~/.umkim-vault-pass
./scripts/teardown-hub.sh --confirm
# Hub-only: spokes keep independent K3s (no fleet API cluster delete)
# ./scripts/teardown-hub.sh --confirm --skip-cluster-delete
test ! -f /etc/rancher/k3s/k3s.yaml && echo OK
./scripts/install-hub.sh
./scripts/verify-hub-install.sh
```

## Migrating to the ingress entry model

v0.2 uses `FleetSite.spec.edge.ingress[]` and `FleetClusterJoin.spec.edge.ingressId`
instead of `domains`, `external_endpoints`, and per-cluster `domain_bindings`.

1. Patch each `FleetSite`: map each former domain id to an `ingress[]` row
   (`port_forward` with `address` + `zone`, or `reverse_proxy` with `id` only).
   Set `hubGateway` / `hubZone` where the site used `hub_gateway`.
2. Patch each `FleetClusterJoin`: set `spec.edge.ingressId` to the chosen ingress
   row (former `domainBindings[0]` is a common mapping).
3. Re-apply `fleet-edge.yml` or let the fleet-site controller reconcile
   `fleet-edge-clusters` and `fleet-edge-domains` ConfigMaps.
4. Verify no two `port_forward` clusters share the same `ingressId`.

Inventory snippets under [examples/site-profiles/](../examples/site-profiles/)
show the new shape.

## Safe troubleshooting

- Check hub readiness with `./scripts/verify-hub-install.sh`.
- Inspect controller and workload status before retrying a phase.
- Use targeted playbook tags rather than a full redeploy.
- Treat teardown as destructive; do not use it as normal troubleshooting.
  To reinstall from zero, use `./scripts/teardown-hub.sh --confirm`.

For non-destructive verification, see [fleet-verify.md](fleet-verify.md).
