# BUG: kubevirt_host_bridge CNI install lacks mkdir -p /opt/cni/bin

## Summary
Role `kubevirt_host_bridge` installs bridge CNI with `install -m 0755 … /opt/cni/bin/bridge`
without creating `/opt/cni/bin`. On hosts without k3s CNI dir (stale worker VM
`lab-ha-0-w1`) the task fails: `No such file or directory`, failing the whole
`enable-kubevirt` Ansible run (exit 2) even when CP nodes succeeded.

## Fix
`mkdir -p /opt/cni/bin` before `install`.
