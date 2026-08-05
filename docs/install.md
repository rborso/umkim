# Install a UMKIM hub

This is the greenfield path: install one hub, then create workload clusters
through FleetClusterJoin. It does not create Ansible-managed spokes.

> [!WARNING]
> This v0.1.0 path is experimental and can make destructive changes. Review
> [../SECURITY.md](../SECURITY.md) and [../RELEASE_NOTES.md](../RELEASE_NOTES.md)
> before continuing.

## Prerequisites

- Rocky Linux 9 hub host, non-root operator account, passwordless `sudo`
- Roughly 4+ vCPU, 8+ GiB RAM, and 100+ GiB disk
- Public IPv4 with WAN TCP 22/80/443 and UDP 51820 (WireGuard)
- DNS and firewall control for intended public application endpoints
- Private management connectivity for cluster enrollment

### Controller packages and repository layout

Harbor fleet image builds expect sibling checkouts beside `~/umkim`. Install
packages, clone UMKIM and the required siblings, then pin them to the release:

```bash
sudo dnf install -y ansible-core git python3-pip python3-libselinux \
  httpd-tools podman containernetworking-plugins
pip install netaddr

# HTTPS (or use matching git@github.com:rborso/... SSH URLs for every clone)
git clone https://github.com/rborso/umkim.git ~/umkim
cd ~/umkim
ansible-galaxy collection install -r requirements.yml

cd ~
git clone https://github.com/rborso/control-plane.git
git clone https://github.com/rborso/fleet-olm-subscription-sp.git
git clone https://github.com/rborso/fleet-helm-release-sp.git
git clone https://github.com/rborso/k8s-container-service-provider.git
git clone https://github.com/rborso/kubevirt-service-provider.git
```

Use one transport (HTTPS or SSH) for every clone. Harbor clone defaults use
HTTPS.

Align siblings to the release pins and verify before building images:

```bash
cd ~/umkim
./scripts/fleet-checkout-pins.sh
./scripts/fleet-verify-pins.sh --strict
```

Pin sources of truth are documented in [../VERSIONS.md](../VERSIONS.md).

**Optional:** [`umkim-catalog`](https://github.com/rborso/umkim-catalog) holds
application charts. It is not required for hub install; clone it later only if
you need the catalog add-on (see [Optional catalog items](#optional-catalog-items)).

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
| `inventory/group_vars/all/site.yml` | DNS zone, ACME email, WireGuard hostname, LDAP/`domain`, management site ids |

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

Optional keys (per-cluster LLDAP passwords, Gitea catalog admin) stay in the
example for later use; leave them unset or commented until needed.

### Generate passwords

The helper fills hub-required secrets with random values and encrypts the file.
It omits optional per-cluster LLDAP and Gitea catalog keys (add those later from
[`vault.example.yml`](vault.example.yml) if you need them):

```bash
./scripts/generate-vault-secrets.sh --encrypt \
  --vault-pass-file ~/.umkim-vault-pass
```

Use `--force` only when intentionally overwriting an existing `vault.yml`.

### Preflight

```bash
export ANSIBLE_VAULT_PASSWORD_FILE=~/.umkim-vault-pass
FLEET_PROFILE=production ./scripts/preflight-v1.sh
```

## 3. Install and verify the hub

```bash
export INV="-i inventory/hosts.yml -i inventory/hosts-from-hub.yml"
export ANSIBLE_VAULT_PASSWORD_FILE=~/.umkim-vault-pass
FLEET_PROFILE=production ./scripts/install-hub.sh
./scripts/verify-fleet.sh --layer 1 --suite hub
```

The wrapper runs host preparation, WireGuard, OCM, Harbor, platform services,
console, DCM, firewall, FleetSite, and fleet-edge. Image builds can take a
long time. To resume a known failed phase, use a targeted tag, for example:

```bash
ansible-playbook $INV hub-install.yml -l hub --tags dcm_hub
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
2. Create a registry site other than the management site.
3. Create a FleetClusterJoin cluster for that site.
4. Download its bootstrap bundle through the authenticated console and treat
   the archive as a secret.
5. Run `sudo ./bootstrap.sh` on the first control node, wait for
   **Validated**, then select **Provision**.
6. When the cluster is **Ready**, install the k8s provider; install Helm or
   KubeVirt only when needed.
7. Deploy a seeded catalog item or an approved OCI chart.

For the model and lifecycle, see [design.md](design.md) and
[components/fleet-site.md](components/fleet-site.md).

## Optional catalog items

Clone the catalog beside UMKIM when you need the packaged application charts:

```bash
cd ~
git clone https://github.com/rborso/umkim-catalog.git
# Or: git clone git@github.com:rborso/umkim-catalog.git
```

Configure that repository per its public README (`shared/config.env`). Set
`HARBOR_HOST` to the hub WireGuard address (for example `10.10.0.1`), not
`127.0.0.1` — Helm uses plain HTTP for loopback and Harbor rejects it. Then from
the UMKIM checkout push charts and seed DCM items:

```bash
cd ~/umkim
./scripts/seed-umkim-catalog.sh
```

Deploy an item only after a non-management FleetClusterJoin is **Ready** and
its providers are installed.

## Safe troubleshooting

- Check hub readiness with `./scripts/verify-hub-install.sh`.
- Inspect controller and workload status before retrying a phase.
- Use targeted playbook tags rather than a full redeploy.
- Treat teardown as destructive; do not use it as normal troubleshooting.

For non-destructive verification, see [fleet-verify.md](fleet-verify.md).
