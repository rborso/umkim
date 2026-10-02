# BUG-20261002-B13-llama-gpu-pt-insufficient-memory

**Stage:** B13 — llama.cpp on lab-gpu-pt-0
**Environment:** introduction-run2; lab-gpu-pt-0 guest 172.16.10.141 (~3.6Gi allocatable)
**Expected:** Profile `qwen35-2b-ud-4g` schedules on GPU-capable nodes
**Actual:** Pod Pending: `Insufficient memory` (profile requests 4Gi / limits 6Gi)
**Reproduce:** `umkim app create ... --llama-profile qwen35-2b-ud-4g` on lab-gpu-pt-0
**Evidence:** `cli-logs/B13-gpu-pt-node-memory.txt`; spoke events Insufficient memory
**Workaround / Fix applied:** `umkim app update llama-gpu-pt --values-file` with requests 1500Mi / limits 2800Mi (same as run1). Profile default too large for 4Gi passthrough guest.
**Follow-up:** Consider a smaller profile or auto-size for small GPU nodes.
