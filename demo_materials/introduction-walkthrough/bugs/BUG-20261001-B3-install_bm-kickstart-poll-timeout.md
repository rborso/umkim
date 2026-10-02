# BUG: install_bm fails — kickstart post-install poll times out (no reinstall)

## Context
Phase B3: `umkim site host provision homelab virt03 --action install_bm --confirm --wait -o json`

- jobName: `fleet-site-prov-9d39b09b38b9`
- startedAt: 2026-10-01T20:18:16Z
- endedAt: 2026-10-01T20:38:30Z
- umkim phase: **Failed**
- message: YAIDAP connector job failed
- Evidence: `assets/B3-install_bm.json`, `assets/B3-install_bm-kubectl-logs.txt`, `assets/B3-virt03-ssh-verify.txt`, `assets/B3-host-get-after-install.json`

## Observed failure (from YAIDAP playbook stdout in connector job)
1. TempXE arming report printed: `REINSTALL on next PXE reboot (destructive): none` (seen twice: after build and before start).
2. Play still ran `Reboot target into PXE install` (`select uefi ipv4 network boot entry and reboot`); SSH drop wait succeeded.
3. Task `configure-tempxe : poll until kickstart post-install completes` failed after **45** attempts.
   - Poll command: SSH to `linux@172.16.10.4` running `sudo test -f /root/ks-post.log && cat /proc/sys/kernel/random/boot_id`.
   - Last attempt returned `rc: 0` with boot_id `e055eb9e-9c89-44a7-9a40-93ede60a15e7` (condition for success still not met by the role until-logic).
4. Follow-on: `fail bare-metal install after recording integration result` → connector task `Run YAIDAP ansible-playbook command on jumphost` fatal (rc 2, ~20m).
5. PLAY RECAP virt03: `ok=107 changed=9 failed=1 skipped=57 rescued=1`.

## Post-failure SSH verify (via jumphost)
- `/etc/redhat-release`: Rocky Linux release 10.2 (Red Quartz)
- hostname: virt03.lab.local
- uptime at verify: still multi-day (boot since `2026-09-29 16:31:29`); `NO_KS_POST` (`/root/ks-post.log` absent)
- Same boot_id as poll stdout: `e055eb9e-9c89-44a7-9a40-93ede60a15e7`
- leftover `tunl0` interface still present (see prior B3 tunl0 bug)

## Host API status after
```json
{"phase":"Failed","message":"YAIDAP connector job failed"}
```

## Note
Kubernetes Job `fleet-site-prov-9d39b09b38b9` had backoff: first pod Error, second pod still Running with another `ansible-playbook ... install-bare-metal.yml` on jumphost when evidence was captured. umkim `--wait` already returned Failed from the first completion.

## Impact
Bare-metal OS reinstall via fleet `install_bm` does not complete for virt03; host remains on pre-existing OS without ks-post marker.

## Investigation

Facts gathered 2026-10-01 ~22:47–22:51 CEST (hub + jumphost + virt03).

### Stopped stale work
- Deleted Running Job `fleet-site-prov-9d39b09b38b9` in `fleet-console` (pods: prior Error `…-k4pgk`, Running backoff `…-kg9vx`).
- Killed jumphost ansible-playbook PIDs `316192` / `318762`: `playbooks/install-bare-metal.yml --limit virt03 --tags install_bm -e tempxe_confirm=true -e install_os=true`.
- Removed leftover jumphost `pxe-server` podman container left from that run.
- Evidence: `assets/B3-install_bm-stop-stale.txt`.

### Why arming printed `REINSTALL … none`
Prior connector log (`assets/B3-install_bm-kubectl-logs.txt`) showed:
- `REINSTALL MACs on the wire: none`
- `DISCOVERY MACs on the wire: 88:d7:f6:2c:17:3b`

Inventory at failure time had both `install_os: true` and `tempxe_discovery.enabled: true` in `host_vars/virt03.yml`.

Role logic (`roles/configure-tempxe/tasks/tempxe_arming_report.yml`):
- `install_enabled` from `-e install_os` / `hv.install_os`
- `discovery_enabled` from `(hv.tempxe_discovery).enabled`
- When `install_enabled` and `discovery_enabled`: `mode=discovery`, `risk=discovery_pxe` (not `reinstall_pxe`)
- `tempxe_armed_reinstall_*` only selects `risk == reinstall_pxe`
- PXE MAC extraction from `uplinks[].networks[].pxe` + `uplink.mac_address` worked (MAC listed under DISCOVERY)

Same precedence in `roles/configure-tempxe/templates/default.j2`: `is_discovery` branch before install kickstart → would serve discovery-ks, not `ks/virt03.cfg`.

`tempxe_confirm` was already true (playbook `-e` and `install_bm_arm_tempxe.yml`); missing confirm was not the cause of empty REINSTALL list.

### Disk by-id vs missing_disks
- host_vars disk: `disk/by-id/ata-TOSHIBA_THNSNJ256G8NU_45KB3082KFJX` (no `/dev/` prefix — YAIDAP convention).
- Live symlink exists: `/dev/disk/by-id/ata-TOSHIBA_THNSNJ256G8NU_45KB3082KFJX` → `../../sda`.
- Filters `disk_dev_path` / `disk_kickstart_ref` (`roles/configure-common/filter_plugins/yaidap_disk.py`) prepend `/dev/` for shell and strip it for kickstart.
- `check_host_details` compares `disks[].disk` (by-id string) to `lsblk` `item.name` (`sda`) → informational `missing_disks` false positive; not evidence the by-id path is absent.

### virt03 in inventory tree
- Hub: `bare_metals.yml` hosts.virt03 (hub-only by design; not synced as that filename).
- Jumphost `guests.yml` has `external.hosts.virt03` (connector projection); MAC/uplinks live in `host_vars/virt03.yml`.

### EFI / network boot
- `efibootmgr`: only `Boot0000* rocky` and `Boot0001* UEFI OS`; **no** Boot#### matching IPv4/Network/PXE/HTTP.
- `install_bm_pxe_reboot.yml` selects such an entry then `efibootmgr -n` + reboot; with none present the shell exits 1.
- `install_bm_wait_complete.yml` SSH-drop wait uses `failed_when: false` (180s) — prior run continued into kickstart poll without a real PXE boot.
- Poll requires `/root/ks-post.log` **and** `boot_id !=` pre-capture; host still has prior `ks-post.log` and unchanged boot_id `e055eb9e-9c89-44a7-9a40-93ede60a15e7` (uptime still multi-day since 2026-09-29).

### Inventory fix applied
- Hub `umkim site inventory files put` `host_vars/virt03.yml`: set `tempxe_discovery.enabled: false` (kept `install_os: true`).
- `umkim site inventory sync homelab --wait` → synced; jumphost file confirms `enabled: false`.
- After this fix, arming would classify virt03 as `reinstall_pxe` with MAC `88:d7:f6:2c:17:3b` **if** install_bm were re-run.

### Retry decision
- **Did not** run `install_bm` retry: no UEFI network-boot entry → automated PXE reboot cannot succeed; same boot_id poll failure expected.
- Skip note: `assets/B3-install_bm-retry-skipped.txt`.
- Host remains SSH-usable (Rocky 10.2) for continuing walkthrough with existing OS (`assets/B3-host-get-after-diag.json` phase still `Failed`).
