# BUG / ops: create_vm recreated existing lab-ha-0 guests; hub user SSH key no longer works

**Date:** 2026-10-01  
**Phase:** B6

## Summary
After pinned create_vm Succeeded ("VM created"), hub→jumphost→guest SSH with hub `linux` key got `Permission denied (publickey)`; host keys rotated. Jumphost YAIDAP key still works. Bootstrap used fleet/YAIDAP path successfully.

## Impact
Manual hub-user SSH verification broken until keys redistributed; automation OK via YAIDAP cloudinit pubkey.
