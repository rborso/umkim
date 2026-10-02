# BUG: fleet-cluster-provision Traefik /ping probe SSH times out (port 65535)

## Context
Phase B5 `umkim cluster provision lab-single-0` after Validated bootstrap.
Provision Ansible job `fleet-cluster-provision-lab-single-0` failed twice on task
`fleet_site : Fleet site | Probe spoke Traefik /ping locally on cluster node`.

## Symptom
```
fatal: [umkim-hub -> lab-single-0]: UNREACHABLE!
Failed to connect to the host via ssh: Connection timed out during banner exchange
Connection to UNKNOWN port 65535 timed out
```
Job pods `...-6r28m` and `...-96v9n` Error; Job status Failed.

## Impact
Job reports Failed / unreachable, but FleetClusterJoin later reached **Ready**
(capability discovery Succeeded; `umkim cluster get` available=true). Misleading
ops signal; may block Ready on other clusters if reconcile does not recover.

## Evidence
- `assets/B5-provision-job-failed-logs.txt`
- `assets/B5-provision-job-current-logs.txt`
- Contrast: hub → spoke WireGuard SSH `linux@10.10.0.10` and
  `curl http://127.0.0.1:30080/ping` → `OK` (`assets/B5-traefik-ping-via-wg.txt`)

## Suggested fix
Probe Traefik via WireGuard / cluster kubeconfig instead of jumphost ProxyCommand
SSH (port 65535), or increase timeout / prefer direct WG when enrolled.
