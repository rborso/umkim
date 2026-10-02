# NOTE (not a product bug): bare_metals.yml is hub-canonical

## Clarification
`docs/components/fleet-site.md`: `bare_metals.yml` is **hub only**; sync composes those hosts into jumphost `guests.yml` → `external.hosts`. Hub `inventory files list` correctly shows `bare_metals.yml` with `synced: false`.

## Walkthrough confusion
After sync, jumphost had no `bare_metals.yml` file; `virt03` appeared under `guests.yml` `external.hosts` instead. Expected.

## Action
Do not treat missing jumphost `bare_metals.yml` as a sync failure. Import apply of `bare_metals.yml` still applies on the hub.
