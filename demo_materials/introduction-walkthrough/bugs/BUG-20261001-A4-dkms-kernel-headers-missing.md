# BUG-20261001-A4-dkms-kernel-headers-missing

## Stage
A4 — cluster bootstrap on virt03 (Rocky 10)

## Summary
During fleet-cluster-bundle host prep, a kernel upgrade installed `6.12.0-211.61.1.el10_2` and a DKMS step printed:

`Error! Your kernel headers for kernel 6.12.0-211.61.1.el10_2.x86_64 cannot be found at /lib/modules/.../build`

## Impact
Bootstrap continued and completed (Validated → Ready). Warning may leave WireGuard/DKMS modules incomplete until reboot onto new kernel with headers installed.

## Evidence
`assets/A4-bootstrap-from-hub.txt`, `assets/A4-bootstrap-dkms-note.txt`

## Suggested fix
Install matching `kernel-devel` / headers before DKMS, or reboot after kernel upgrade before building modules; make the bundle fail closed if required modules are missing.
