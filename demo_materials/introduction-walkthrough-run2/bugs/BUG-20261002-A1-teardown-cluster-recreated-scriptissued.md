# BUG-20261002-A1-teardown-cluster-recreated-scriptissued

- **Stage:** A1 teardown-hub Phase 1
- **Environment:** hub wg.otthol.hu run2; clusters lab-gpu-pt-0, lab-ha-0, lab-single-0 Ready
- **Expected:** DELETE with cleanupKubernetes=true removes FCJ; wait sees 404
- **Actual:** lab-gpu-pt-0 and lab-single-0 removed. lab-ha-0 delete Job completed (~30s), wg-rm jobs completed, then a **new** FleetClusterJoin lab-ha-0 appeared (manager fleet-site-controller, creationTimestamp ~teardown start+40s) in phase **ScriptIssued** ("Bootstrap bundle ready"), jobs=none, no ManagedCluster. Teardown wait loop stalled until FORCE_STUCK_AFTER_SEC force-removes never-joined ScriptIssued FCJ.
- **Reproduce:** Full hub teardown while FleetSite homelab Ready with YAIDAP provision bound; delete lab-ha-0 among others.
- **Evidence:** cli-logs/A1-teardown-hub.txt; kubectl events show delete Complete then fleet-cluster-wg-lab-ha-0 recreate; FCJ age ~minutes with ScriptIssued
- **Workaround:** TEARDOWN_FORCE_STUCK_AFTER_SEC=600 (used); force_remove deletes ScriptIssued without ManagedCluster. Optionally delete FleetSite before cluster wipe to avoid recreate.
- **Follow-up:** Fix fleet-site-controller so cluster delete does not re-issue ScriptIssued / recreate FCJ during teardown; consider deleting sites before clusters in teardown-hub.sh

## Update during run2 teardown

Force-remove deleted FCJ but **fleet-site-controller immediately recreated** lab-ha-0 (loop; ages reset to seconds). Broke loop by: scale controller to 0 → delete FleetApps (finalizers) → delete FleetSite homelab → delete FCJ → scale controller back. Teardown then saw 404 and entered Phase 3 Ansible hub wipe.

Also: cluster delete Jobs reported Complete while guest **k3s remained active** (cp1–cp3, gpu-pt). Manual uninstall via jumphost — see cli-logs/A1-orphan-spoke-k3s-uninstall.txt.
