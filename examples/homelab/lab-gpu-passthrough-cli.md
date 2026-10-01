# lab-gpu-0 → GPU passthrough VM → lab-gpu-pt-0 (CLI)

Promote bare-metal **`virt04`** (lab-gpu-0 site, 172.16.10.6) to a libvirt hypervisor with VFIO,
create a GPU passthrough guest, and enroll cluster **`lab-gpu-pt-0`**. Canonical inventory snippets
live under [inventory/](inventory/) and [cluster-lab-gpu-pt-0.json](cluster-lab-gpu-pt-0.json).

Prerequisites match [rebuild-homelab-cli.md](rebuild-homelab-cli.md) (hub, `umkim login`,
YAIDAP on jumphost, homelab site provision binding).

## 1. Remove K3s cluster on bare metal

```bash
umkim cluster delete lab-gpu-0 --cleanup-kubernetes --wait --timeout 120m
```

Disable old cluster automation: set `autoApply: false` on `group_vars/lab-gpu-0.yml` and
clear `lab-gpu-0.yml` hosts on the hub (`umkim site inventory files put`).

## 2. Promote virt04 to hypervisor

- Remove the GPU bare-metal entry from `bare_metals.yml` (if present)
- Add **`virt04`** to `hypervisors.yml` with `ansible_host: 172.16.10.6`
- Merge [inventory/host_vars/virt04-hypervisor.yml.example](inventory/host_vars/virt04-hypervisor.yml.example) into `host_vars/virt04.yml` (adjust PCI ids after discover)

```bash
umkim site inventory sync homelab --wait
```

## 3. Discover and host prepare

```bash
umkim site host provision homelab virt04 --action discover --wait
# import preview/apply if needed; fix gpu_passthrough_host PCI in host_vars

umkim site host prepare homelab virt04 --wait
umkim site host prepare homelab virt04 --gpu-passthrough --confirm --wait
```

Equivalent granular actions:

```bash
umkim site host provision homelab virt04 --action prepare_hypervisor --wait
umkim site host provision homelab virt04 --action prepare_gpu_passthrough --confirm --wait
```

## 4. GPU passthrough VM guest

Add `lab-gpu-pt-0-cp1` to `guests.yml` and `host_vars/lab-gpu-pt-0-cp1.yml`, plus
`lab-gpu-pt-0.yml` / `group_vars/lab-gpu-pt-0.yml` (see examples in this directory).

```bash
umkim site inventory sync homelab --wait
umkim site host provision homelab lab-gpu-pt-0-cp1 --action create_vm --wait
```

## 5. Cluster lab-gpu-pt-0

Update `bootstrapHost` in `cluster-lab-gpu-pt-0.json` to the guest `ansible_host`.
Point site ingress `homelab-gpu-pf` at the VM if WAN port-forwards still target the old BM IP.

```bash
umkim cluster create -f examples/homelab/cluster-lab-gpu-pt-0.json
umkim cluster wait lab-gpu-pt-0 --phase ScriptIssued --timeout 15m
umkim cluster bootstrap lab-gpu-pt-0 --bootstrap-host 172.16.10.141 \
  --jumphost linux@jump.otthol.hu:30200 --ssh-user linux
umkim cluster wait lab-gpu-pt-0 --phase Validated --timeout 60m
umkim cluster provision lab-gpu-pt-0
umkim cluster wait lab-gpu-pt-0 --phase Ready --timeout 120m
umkim cluster gpu host-prep lab-gpu-pt-0 --wait 60m
umkim cluster gpu enable lab-gpu-pt-0 --wait 30m
umkim cluster gpu discover lab-gpu-pt-0
```
