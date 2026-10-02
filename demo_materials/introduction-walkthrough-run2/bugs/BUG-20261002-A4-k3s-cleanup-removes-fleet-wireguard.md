# BUG-20261002-A4-k3s-cleanup-removes-fleet-wireguard

**Stage:** A4 — cluster bootstrap on virt03 (bundle mode)
**Environment:** run2; intro-single-0; Rocky 10.2 virt03 172.16.10.4

## Expected
Bootstrap order: wireguard-install → enroll → validate → cluster-provision-local → post_progress Validated.
Fleet overlay (wg-umkim) remains up so progress POST and hub provision succeed.

## Actual
- wireguard-install brought up wg-umkim (hub ping OK; Enrollment OK HTTP 200).
- cluster-provision-local.yml imports k3s/tasks/cleanup.yml which **stops wg-quick@wg-umkim and deletes /etc/wireguard/wg-umkim.conf**.
- Journal 08:35:50: ansible systemd enabled=False state=stopped; file state=absent on wg-umkim.conf.
- post_progress Validated failed (CLI exit status 28 / curl timeout); phase stuck **Enrolled**.
- Spoke k3s itself installed and node Ready, but overlay gone → hub cannot reach 10.10.0.10.

## Reproduce
1. umkim cluster create/bootstrap single_node wireguard on clean Rocky 10.
2. Observe WG up after wireguard play, then down after provision-local cleanup play.
3. umkim cluster get shows Enrolled; ping 10.10.0.10 fails.

## Evidence
- cli-logs/A4-bootstrap-from-hub.txt
- cli-logs/A4-wg-torn-down-journal.txt (redacted)

## Workaround (temporary)
Restore conf from /tmp/bootstrap-*/wireguard/... as root:root 0600; systemctl enable --now wg-quick@wg-umkim; then umkim cluster provision.

## Follow-up
Gate WireGuard removal in k3s cleanup behind k3s_cleanup_remove_wireguard (default false for provision-local; true for full teardown).


## Follow-up (FIXED in tree + workaround applied)
- Code: k3s cleanup gates WG removal on k3s_cleanup_remove_wireguard (default false); teardown-hub sets true. Commit d8bf63c.
- Live: restored wg-umkim.conf from bootstrap tmp as root:root 0600; overlay OK; umkim cluster provision → Ready; ManagedCluster Available.
- Evidence: cli-logs/A4-provision-start.txt, A4-wait-ready.txt
- Note: fleet-site-controller image still embeds old static cleanup until rebuilt; static source under controllers/fleet-site/internal/bundle/static/ is fixed for next image push.
