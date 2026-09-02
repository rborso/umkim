# Hub DR test runbook — 2026-09-02 (umkim-hub)

Live disaster-recovery exercise on the management host `umkim-hub`: full backup,
complete hub teardown, restore from backup on the same machine.

## Pre-state (before backup)

| Item | Value |
|---|---|
| Host | `umkim-hub` (Rocky Linux 10.2) |
| K3s | v1.32.13+k3s1, **SQLite** datastore (`state.db`, not embedded etcd) |
| WireGuard | `wg0` @ `10.10.0.1/24`, UDP 51820 |
| ManagedCluster | `lab-single-0` — Accepted, Available |
| Harbor | 8 pods running |
| Fleet API | `lab-single-0` registered |

Captured in `/tmp/umkim-dr-prestore.log`.

## 1. Backup

```bash
cd ~/umkim
sudo ./scripts/backup-hub.sh --full
```

**Bundle:** `/var/backups/umkim/20260902T104537Z/`

| Component | File(s) | Notes |
|---|---|---|
| WireGuard | `wireguard/wg.tgz` | OK |
| ACME | `host/traefik-acme.tgz` | OK |
| Harbor PVCs | `harbor/pvc/*.tgz` (5 claims) | Scaled Harbor down during backup |
| K3s SQLite | `k3s-db/state.db`, `k3s-db/token` | Added manually after etcd path failed (see issues) |
| etcd | — | **Skipped** — this hub uses SQLite, not embedded etcd |

**Issues during backup:**

1. **`k3s not found` under sudo** — `sudo` PATH omits `/usr/local/bin`. Fixed in
   `scripts/lib/hub-backup-common.sh` via `hub_k3s_bin()`.
2. **etcd snapshot unavailable** — `k3s etcd-snapshot` returns `etcd datastore disabled`
   on SQLite hubs. Added `hub_backup_sqlite()` / `hub_restore_sqlite()` paths.
3. **`manifest.json` missing** — Python helper used `sys.argv` instead of `sys.argv[1:]`.
   Fixed; manifest written manually for this run.
4. **Harbor DB dump skipped** — database pod not found while Harbor was scaled down
   (PVC archives sufficient for this test).
5. **`sqlite3` not installed** — live `state.db` copy used (acceptable for lab; install
   `sqlite` for consistent online backup).

## 2. Hub teardown (complete wipe)

Spoke cluster **preserved** (`--skip-cluster-delete` — did not reset `lab-single-0` node K3s):

```bash
export ANSIBLE_VAULT_PASSWORD_FILE=~/.umkim-vault-pass
./scripts/teardown-hub.sh --confirm --skip-cluster-delete
```

**Removed:** K3s, Calico, Harbor, OCM namespaces, fleet controllers, WireGuard keys,
`/var/lib/umkim`, `/opt/fleet-src`, nft `fleet_hub` table.

**Kept:** `inventory/`, encrypted `vault.yml`, backup bundle under `/var/backups/umkim/`.

Log: `/tmp/umkim-dr-teardown.log` (~4 min).

Post-teardown checks:

```bash
test ! -f /etc/rancher/k3s/k3s.yaml && echo OK   # k3s gone
systemctl is-active k3s                            # inactive
```

## 3. Minimal prep before restore

Full `install-hub.sh` was **not** run (would create new WireGuard keys and empty cluster).

```bash
export INV="-i inventory/hosts.yml -i inventory/hosts-from-hub.yml"
export ANSIBLE_VAULT_PASSWORD_FILE=~/.umkim-vault-pass

ansible-playbook $INV host-prepare.yml -l hub
ansible-playbook $INV ocm.yml -l hub --tags k3s,calico
```

K3s install **failed to start** until WireGuard was restored — API binds `10.10.0.1:6443`.
Expected; restore script brings WG up first.

## 4. Restore

```bash
sudo ./scripts/restore-hub.sh --yes /var/backups/umkim/20260902T104537Z
```

**First attempt issues:**

1. K3s start failed: `bind: cannot assign requested address` (WG not up yet on first
   k3s start attempt inside sqlite restore — race before `wg-quick@wg0` completed).
2. K3s start failed: TLS files from fresh install newer than restored `state.db`.
   **Manual fix:**
   ```bash
   sudo rm -rf /var/lib/rancher/k3s/server/tls /var/lib/rancher/k3s/server/cred
   sudo rm -f /etc/rancher/k3s/k3s.yaml
   sudo systemctl start k3s
   ```
   **Script fix:** `hub_restore_sqlite()` now removes TLS/cred before start.

**Second pass** (ACME + Harbor only, after k3s healthy):

```bash
sudo ./scripts/restore-hub.sh --yes --skip-wireguard --skip-etcd \
  /var/backups/umkim/20260902T104537Z
```

## 5. Post-restore fixes

| Issue | Fix |
|---|---|
| `nft table fleet_hub missing` | `ansible-playbook $INV hub-firewall.yml -l hub` |
| Fleet controllers `ImagePullBackOff` (Harbor TLS) | `/etc/rancher/k3s/registries.yaml` missing after teardown; applied via `ansible-playbook $INV harbor.yml -l hub --tags harbor_registries` |
| Fleet API down | Resolved after registries + pod restart |

## 6. Final verification

```bash
./scripts/verify-hub-install.sh          # OK (all checks pass)
kubectl get managedclusters              # lab-single-0 Accepted, Available
curl http://127.0.0.1:18080/api/v1alpha1/clusters   # lab-single-0 present
```

Harbor: all 8 pods Running after PVC restore + registries fix.

## Script changes made during this test

- `hub_k3s_bin()` — find k3s under `/usr/local/bin` when sudo PATH is minimal
- SQLite backup/restore (`k3s-db/`) alongside etcd snapshot path
- `hub_restore_sqlite()` — remove stale TLS/cred after restore on fresh k3s install
- `hub_restore_wireguard()` — wait for `wg0` IPv4 before returning
- `hub_write_manifest()` — fix `sys.argv[1:]` parsing

## Recommended operator procedure (fresh hub DR)

```bash
# 1. Backup (on healthy hub)
sudo ./scripts/backup-hub.sh --full
rsync -a /var/backups/umkim/<stamp>/ off-host-backup:

# 2. Teardown (hub only; add --skip-cluster-delete to keep spoke K3s)
./scripts/teardown-hub.sh --confirm --skip-cluster-delete

# 3. Minimal prep
ansible-playbook $INV host-prepare.yml -l hub
ansible-playbook $INV ocm.yml -l hub --tags k3s,calico   # binary only; may not start until WG up

# 4. Restore
sudo ./scripts/restore-hub.sh --yes /var/backups/umkim/<stamp>

# 5. Post-restore Ansible (not in backup bundle)
ansible-playbook $INV hub-firewall.yml -l hub
ansible-playbook $INV harbor.yml -l hub --tags harbor_registries

# 6. Verify
./scripts/verify-hub-install.sh
```

## Lessons

- UMKIM hub on K3s **SQLite** needs `k3s-db/` backup, not `k3s etcd-snapshot`.
- Restore **before** full `install-hub.sh`; reuse original inventory + vault.
- After sqlite restore onto a host that ran a fresh `k3s` install, delete generated
  TLS/cred (now automated in script).
- `registries.yaml` and nft firewall are host/Ansible state — re-apply after restore.
- Use `--skip-cluster-delete` during hub DR tests to avoid wiping remote spokes.
