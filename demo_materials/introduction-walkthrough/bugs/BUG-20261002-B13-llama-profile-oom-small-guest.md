# BUG / gap: qwen35-2b-ud-4g profile requests 4Gi; lab-gpu-pt-0 guest ~3.7Gi allocatable

**Phase:** B13  
**Date:** 2026-10-02 (CEST)  
**Cluster:** lab-gpu-pt-0 (VM 172.16.10.141, ~4Gi RAM)

## Symptom
`umkim app create ... --llama-profile qwen35-2b-ud-4g` produced values with
`resources.requests.memory: 4Gi` / `limits.memory: 6Gi`.

Pod stayed Pending:

```
0/1 nodes are available: 1 Insufficient memory
```

Node allocatable memory ~3740276Ki (~3.56Gi).

## Workaround
`umkim app update llama-gpu-pt --values-file ...` with requests 1500Mi / limits 2800Mi.
Pod then Scheduled and became Ready (chart uses `useGpuResource: false` + runtimeClass nvidia).

## Expected
Profile docs / picker should warn for <4Gi+ nodes, or offer a smaller profile;
strategy guest sizing for lab-gpu-pt-0 should leave headroom above profile requests
(or profile should request less for 4G VRAM / small-RAM VMs).
