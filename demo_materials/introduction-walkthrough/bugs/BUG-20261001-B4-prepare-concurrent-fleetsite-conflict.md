# BUG: concurrent `site host prepare` hits FleetSite update conflict

**Date:** 2026-10-01  
**Phase:** B4  
**Severity:** medium (CLI fails; Ansible job may still run)

## Summary
Starting `umkim site host prepare homelab virt01 --wait` and `... virt02 --wait` nearly simultaneously caused virt02 CLI to fail with:

```
HTTP 500: Operation cannot be fulfilled on fleetsites.fleet.umkim.com "homelab":
the object has been modified; please apply your changes to the latest version and try again
```

virt01 host status moved to Provisioning; virt02 stayed Declared. Despite the CLI error, a second `fleet-site-prov-*` job for `prepare_hypervisor` / subject virt02 was observed Running in `fleet-console`.

## Expected
Either serialize host provision/prepare mutations on FleetSite, or retry on conflict; host status should reflect in-flight jobs for each subject.

## Workaround
Run prepare sequentially; if CLI fails but job exists, wait for job completion and re-get host / re-issue prepare if status stuck Declared.

## Assets
- `assets/B4-virt02-prepare.nohup.log`
- `assets/B4-discover_pxe-skipped.txt`
