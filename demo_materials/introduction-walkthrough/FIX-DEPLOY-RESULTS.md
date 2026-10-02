# Fix deploy results (operational rollout)

**When:** 2026-10-02 ~07:54–08:20 Europe/Budapest (UTC+2)  
**Hub:** `wg.otthol.hu` / umkim HEAD `3e2e8d0`  
**Constraint:** no hub teardown; secrets scrubbed from assets

## What was deployed

| Component | Action | Result |
| --- | --- | --- |
| Harbor fleet images | `ansible-playbook hub-install.yml --tags harbor_push_fleet -e harbor_fleet_push_force=true` | Pushed `fleet-site-controller`, `fleet-ansible-runner`, `fleet-edge-controller`, `fleet-failover-controller`, `umkim-console`, `fleet-console-auth` tag `0.2` |
| fleet-site-controller | Redeploy + `kubectl rollout restart` | New pod ~08:14; digest `sha256:6d8364eca0f2…` |
| fleet-edge-controller | Redeploy with fixed gateway env | New pod ~08:14; digest `sha256:30e1346d948f…` |
| umkim CLI | `go build` in `cmd/umkim` | `~/.local/bin/umkim` mtime **2026-10-02 08:10** |
| YAIDAP | Local commit on jumphost (push creds missing) | **`63bd7c1`** — cloud-init + kickstart accept `fleet_ansible_ssh_pubkey` / `fleet_provision_fleet_ansible_ssh_pubkey` |
| VERSIONS.md | Pin YAIDAP SHA | `63bd7c1` (jumphost local; push pending) |

**Skipped on purpose**

- Full hub teardown / wipe
- `umkim site provision yaidap-sync` (would risk resetting jumphost to origin without `63bd7c1`)
- HA `autoApply: false` (no HA teardown this pass)
- New create_vm soak / B8c 4-CP replace / install_bm UEFI

## Edge gateway URLs (A6)

| Env | Before | After |
| --- | --- | --- |
| `HUB_GATEWAY_URL` | `https://127.0.0.1` | **`https://10.10.0.1`** (WG node IP; hostNetwork Traefik) |
| `HUB_GATEWAY_HTTP_URL` | `http://127.0.0.1:80` / mixed | **`http://10.10.0.1:80`** |

Inventory `inventory/group_vars/hub/main.yml` had hardcoded `127.0.0.1` overrides that defeated role defaults. Fixed to `wireguard_address`. Removed bogus NodePort `10.10.0.1:30909` override from `detect_ingress_auth.yml`. Hub HTTP `/ping` → 200 from node IP.

Note: no ClusterIP Service for hub gateway (hostNetwork only); WG node IP is the correct probe target for controller pods.

## YAIDAP

- Jumphost `/home/linux/yaidap` HEAD **`63bd7c1ceb19c86d503e6fe3dc106d53716baed2`** (ahead of `origin/main` by 1; HTTPS push failed: no credentials).
- Claimed Cursor pin `52bd22f` still does **not** exist.
- Template render with real fleet pubkey includes inventory key + fleet key (scrubbed evidence).
- SSH with hub `fleet-site-ansible-ssh` key to existing `lab-ha-0-cp1` (**172.16.10.101**) succeeded (key already present / usable on that guest). New guests will get the key via create_vm cloud-init after this YAIDAP commit.

## Re-test results

| Check | Result | Evidence |
| --- | --- | --- |
| Clusters Ready | lab-single-0, lab-ha-0, lab-gpu-pt-0 Ready | `POST-FIX-cluster-status-*.txt` |
| Layer-1 | **24 passed, 0 failed, 12 skipped** (hub-only) | `POST-FIX-layer1-summary-20261002-081853.json`, `logs/verify-l1-postfix-20261002-081853/` |
| B7 whoami site | VIP `--resolve :30443:10.10.0.12` → **HTTP 200** whoami; public DNS :443 → hub Traefik **404** | `POST-FIX-B7-whoami-site-*.txt` |
| B8 fleet SSH | Key fingerprint + template render OK; SSH to existing CP OK | `POST-FIX-B8-*.txt` |
| B9 VM smoke | lab-ha-0 CPs/workers **running** on virt01/virt02 | `POST-FIX-B9-vm-smoke-*.txt` |
| B12 LiteLLM | phase Ready; `verify-fleet-llm.sh` OK | `POST-FIX-B12-litellm-*.txt` |
| B13 llama | apps Ready; chat completion returned content | `POST-FIX-B13-llama-inference-*.txt` |

## Remaining blockers / known gaps

1. **DNS `*.homelab.otthol.hu`** still resolves to hub WAN (`84.1.215.254`) → hub Traefik 404. Workaround: `--resolve` / VIP `10.10.0.12:30443` (or site VIP). Not fixed in this pass.
2. **YAIDAP push** to GitHub pending credentials; hub `yaidap-sync` deferred until remote has `63bd7c1`.
3. **install_bm UEFI** / TempXE fail-fast not re-validated.
4. **B8c 4-CP replace** not re-run.
5. **B8 create_vm** end-to-end for a brand-new guest not soak-tested (template+SSH evidence only).
6. Pre-existing dirty trees: umkim `wg_operator_clients.yml`; YAIDAP inventory/TempXE WIP files.

## Rebuild recipe used (no teardown)

```bash
export INV="-i inventory/hosts.yml -i inventory/hosts-from-hub.yml"
export ANSIBLE_VAULT_PASSWORD_FILE=~/.umkim-vault-pass
ansible-playbook $INV hub-install.yml -l hub --tags harbor_push_fleet -e harbor_fleet_push_force=true
ansible-playbook $INV hub-install.yml -l hub --tags fleet_site_controller,fleet_edge_controller
# after inventory gateway fix:
ansible-playbook $INV hub-install.yml -l hub --tags fleet_edge_controller
kubectl -n fleet-console rollout restart deploy/fleet-site-controller
(cd cmd/umkim && go build -o ~/.local/bin/umkim .)
```

Logs: `logs/rebuild-fleet-images-20261002-075534.log`, `logs/redeploy-controllers-20261002-081022.log`, `logs/redeploy-edge-gateway-20261002-081407.log`.
