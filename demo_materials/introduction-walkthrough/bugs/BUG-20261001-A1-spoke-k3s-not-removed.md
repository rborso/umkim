# BUG-20261001-A1-spoke-k3s-not-removed

| Field | Content |
|-------|---------|
| **Stage** | A1 teardown / A4 prep |
| **Environment** | virt03 172.16.10.4 after hub teardown with interrupted delete jobs |
| **Expected** | `cleanupKubernetes=true` removes K3s on spokes |
| **Actual** | virt03 still had active k3s + calico + wg-umkim after hub wipe |
| **Reproduce** | Teardown with stuck delete-job loop; hub wiped via `--skip-cluster-delete` |
| **Evidence** | `assets/A4-virt03-precheck.txt`, `A4-virt03-k3s-cleanup.txt` |
| **Workaround** | SSH jumphost and run `/usr/local/bin/k3s-uninstall.sh` before re-bootstrap |
