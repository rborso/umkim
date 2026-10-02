# BUG: create_vm guests lack fleet Ansible SSH pubkey; post-Ready node add fails UNREACHABLE

**Date:** 2026-10-02  
**Phase:** B8a  
**Severity:** high (blocks worker add after YAIDAP create_vm)

## Summary
Fresh guests provisioned via `umkim site host provision … create_vm` only receive the YAIDAP/jumphost cloud-init SSH pubkey (`linux@bck.otthol.local`). Control-plane nodes that previously joined via bootstrap also have the hub fleet key (`linux@umkim-hub` / `~/.ssh/id_ed25519.pub` on umkim-hub).

`umkim node add` Ansible jobs SSH via jumphost using the fleet key and fail immediately:

```
TASK [Install wireguard-tools]
fatal: [lab-ha-0-w2]: UNREACHABLE!
  Permission denied (publickey,gssapi-keyex,gssapi-with-mic).
```

Same failure mode for leftover `lab-ha-0-w1` autoApply addWorker storm (prior to disabling `autoApply`).

## Expected
create_vm / cloud-init should inject the same fleet Ansible SSH public key used by hub node-op jobs (or node-op should authenticate with the YAIDAP key already on the guest).

## Actual
Only YAIDAP key present; node-op UNREACHABLE until operator manually appends hub pubkey via jumphost.

## Workaround
From jumphost, append hub `id_ed25519.pub` to guest `~/.ssh/authorized_keys`, then retry `umkim node add`. Prefer jumphost YAIDAP key for SSH readiness checks (B6 lesson).

## Evidence
- `assets/B8a-w2-add-fail-logs.txt`
- `assets/B8a-ssh-key-compare.txt`
- `assets/B8-w1-add-fail-logs.txt`
