# BUG-20261001-A1-teardown-stuck-delete-jobs

| Field | Content |
|-------|---------|
| **Stage** | A1 — hub teardown before reinstall |
| **Environment** | umkim-hub, UMKIM 0.2, clusters lab-gpu-pt-0 / lab-ha-0 / lab-single-0 Ready→Deleting |
| **Expected** | `teardown-hub.sh --confirm` waits for delete Jobs to complete (cleanupKubernetes) then Ansible hub wipe |
| **Actual** | Script repeatedly classified `lab-gpu-pt-0` delete Job as stuck within ~15–60s, deleted the Job, re-issued DELETE, looping while Jobs were still Running and progressing (ManifestWorks / klusterlet cleanup) |
| **Reproduce** | 1. Hub with ≥1 Ready FCJ. 2. `./scripts/teardown-hub.sh --confirm`. 3. Observe log lines `Replacing stuck delete job(s)` while Job age &lt; few minutes. |
| **Evidence** | `assets/A1-teardown-hub.txt` |
| **Workaround** | Stop teardown parent; allow `fleet-cluster-delete-*` Jobs to finish; re-run teardown when FCJs gone (or force-remove FCJ only if nodes unreachable). |
| **Follow-up** | Review `force_remove_stuck_cluster` timing thresholds in `scripts/teardown-hub.sh` |
