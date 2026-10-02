# BUG: virt04 bridge-nf-call-iptables=1 blocks jumphost→guest after leftover k3s / prepare

**Phase:** B10  
**Date:** 2026-10-02 (CEST)

## Symptom
After orphaned k3s on virt04 (old lab-gpu-0) and `prepare_hypervisor_gpu`,
jumphost saw `From 172.16.10.6: Packet filtered` / `No route to host` to guest
`172.16.10.141`, while virt04 could still ping the guest locally.

`/proc/sys/net/bridge/bridge-nf-call-iptables` was `1` (k3s leftover). Working
hypervisors (virt01) leave bridge-nf unset/0 so firewalld does not filter
bridged VM traffic.

## Impact
Bootstrap/SSH via jumphost to GPU passthrough guest fails until sysctl fixed.

## Workaround
```
sysctl -w net.bridge.bridge-nf-call-iptables=0
sysctl -w net.bridge.bridge-nf-call-ip6tables=0
sysctl -w net.bridge.bridge-nf-call-arptables=0
# persist /etc/sysctl.d/99-umkim-bridge-nf.conf
```
Also uninstall orphaned k3s/wg on promoted hypervisors before prepare.
