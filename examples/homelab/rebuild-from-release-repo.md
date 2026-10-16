# Rebuild the lab from the release repo (v0.3.0 cut)

Runbook for reinstalling the hub from the clean release line, reconfiguring
YAIDAP, recreating the workload clusters, and re-running the full verification.

## Accepted decisions

| Decision | Choice |
|---|---|
| Teardown depth | Fleet teardown only — host OS, repo clone, vault and `~/.umkim-vault-pass` survive |
| Inventory | Reuse the private line's `inventory/` (same secrets, hub IP, domain, LDAP base DN) |
| Cluster VMs | Destroy and recreate through YAIDAP (exercises `create_vm`) |
| App chart versions | Latest published |
| App charts | Rebuilt from their own repos and pushed into the new Harbor |
| Scope | Hub + `lab-e2e-7` (3-node HA) + `rp-test` + KubeVirt/Multus. No LLM path |

## Target state to reproduce

`lab-e2e-7` — `siteId homelab`, `topology ha_3_node`, `autoProvision: true`,
`ingressId homelab-pf`, ingress auth on (own directory; OIDC clients `immich`,
`paperless`), control-plane VIP `172.16.10.100`.

| Node | Address | Cores | Memory | Root disk | Data disk |
|---|---|---|---|---|---|
| `lab-e2e-7-1` | 172.16.10.101 | 6 | 16384 | 50 Gi | 20 Gi |
| `lab-e2e-7-2` | 172.16.10.102 | 6 | 16384 | 50 Gi | 20 Gi |
| `lab-e2e-7-3` | 172.16.10.103 | 6 | 16384 | 50 Gi | 20 Gi |

Site `homelab` ingress: `homelab-pf` (port_forward, `172.16.10.100`, zone
`homelab.otthol.hu`, wildcard `*.homelab.otthol.hu`) assigned to `lab-e2e-7`;
`edge-rp` (reverse_proxy, zone `devtest.otthol.hu`) assigned to `rp-test`.

Provision binding: provider `yaidap`, jumphost `linux@jump.otthol.hu:30200`,
`inventoryPath /home/linux/yaidap/inventory/homelab`,
`yaidapRoot /home/linux/yaidap`, secret `fleet-site-homelab-provision-ssh`.

Apps on `lab-e2e-7`: `gitea`, `immich`, `nextcloud` (+collabora), `paperless`,
`petclinic`, `servarr` (+jellyfin/radarr/sonarr), `whoami`.

## Phase 0 — release line (both repos)

1. `devel` branch from the latest state; retire `publish-clean` / `v0.2` /
   `v0.2.1-devel` once `devel` is pushed.
2. Fold `[Unreleased]` + `[0.2.1]` into `[0.3.0]`; bump `umkim_version` to
   `0.3.0` in `inventory/` (private) and `examples/inventory/hub-only/` (both
   lines); update `VERSIONS.md` and `RELEASE_NOTES.md`.
3. Delete the `v0.2.1` / `umkim-0.2.1` tags (local and, when pushing, remote).
4. Fix `scripts/fleet-verify/presets/clusters.yaml` — it references
   `lab-sn01.yml`, `lab-3n01.yml`, `lab-ha01.yml`, none of which exist in either
   line, so the preset path of `checks/cluster-reinstall.sh` cannot run.

## Phase 1 — clean the machine

```bash
sudo podman system prune -a -f     # 93 GB reclaimable, 122 dangling images
sudo podman system df
rm -rf .cache/harbor-push
```

Stale cached images mislead verification (a cached runner image already did
once), and the hub disk is at 80%.

## Phase 2 — teardown

```bash
# free the cluster IPs first: destroy the lab-e2e-7 guests through YAIDAP
./scripts/teardown-hub.sh --confirm
# then verify nothing survives: no k3s, no wg0, no Harbor, no CRDs,
# no leftover PVCs, no NFS exports
```

## Phase 3 — reinstall from the release line

```bash
git clone <release-remote> -b devel /tmp/umkim-clean   # fresh clone, not the working tree
cp -a <private>/inventory ./inventory                  # reused values + vault
./scripts/preflight-hub-inventory.sh
./scripts/install-hub.sh
# push fleet images tagged 0.3.0 and the in-repo charts into the new Harbor
ansible-playbook harbor.yml -l hub --tags harbor_push_fleet \
  --vault-password-file ~/.umkim-vault-pass
```

## Phase 4 — YAIDAP again

1. Jumphost: `/home/linux/yaidap` checkout, `inventory/homelab` tree, SSH to
   hypervisors/guests/BM targets.
2. Hub: create Secret `fleet-site-homelab-provision-ssh` (private key whose
   public key is in the jumphost's `authorized_keys`), then
   `umkim site provision set homelab --provider yaidap --jumphost
   linux@jump.otthol.hu:30200 --inventory /home/linux/yaidap/inventory/homelab
   --yaidap-root /home/linux/yaidap`.
3. Seed the hub inventory store (hypervisors, guests, bare metals) — reference:
   the private line's `examples/homelab/`.
4. Verify the connector: `umkim site host discover` / `validate`, `plan_vm`.

## Phase 5 — recreate the clusters

```bash
umkim cluster create -f lab-e2e-7.json    # 3 controls, same IPs/resources
umkim cluster create -f rp-test.json      # single node, edge-rp
# wait Ready; check certs, VIP, spoke ingress
```

## Phase 6 — apps

```bash
# package + push the app charts from their own repos, then
umkim chart add --chart <tgz> --name <name> --force
umkim app create ...                      # gitea immich nextcloud paperless
                                          # petclinic servarr whoami
# wait all Ready + bootstraps Complete; labuser, tenants, invites
```

## Phase 7 — verify everything

- `scripts/verify-fleet.sh` — hub/fleet/auth/console/playwright registry checks
- storage, ingress-auth, console-auth, gitea, CLI checks
- app e2e: servarr chain (Seerr → Radarr → Prowlarr/nCore → qBittorrent →
  import → Jellyfin), Nextcloud/Collabora WOPI, Immich share
- KubeVirt/Multus: `cluster-enable-kubevirt-lan` flow + its verification
- lab scripts: `verify-homelab-ha-e2e.sh`, `verify-fleet-app-multicluster.sh`
- record every defect found; that is the point of the exercise

## Phase 8 — wrap

Decide where lab-only material lives now that the private repo is a reference:
`inventory/`, `examples/homelab/`, `scripts/lab/` and `demo_materials/` are not
part of the release line, so keeping them out of it needs a home of their own.
