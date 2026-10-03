# WireGuard clients for operators

> Status: v0.2 is an experimental proof of concept. Overlay access is
> privileged; treat client configs as secrets.

WireGuard operator clients provide split-tunnel access to the fleet management
overlay. They are for trusted operator devices, not a general remote-access
service.

## Create a client

After the hub WireGuard server is configured, run from the hub or another host
with inventory access and `ansible-playbook` (see [install.md](install.md) §4):

```bash
./scripts/wg-dev-client.sh <operator-device>
```

The helper records the client in
`inventory/group_vars/all/wg_operator_clients.yml`, runs `wireguard-clients.yml`,
and writes an importable configuration under `output/wireguard-clients/`.
Existing operator and FleetClusterJoin peers are added without rewriting the
hub interface configuration.

Import the generated configuration with the approved WireGuard client on the
operator device. Confirm that only the intended overlay routes are installed
and that the management interface is reachable over the tunnel.

## Handle client material safely

- Treat the generated configuration and private key as credentials.
- Do not commit files under `secrets/` or `output/`.
- Transfer configurations only over an approved secure channel.
- Remove the peer from inventory and the hub when a device is retired or lost.
- Rotate the peer when compromise is suspected.

## Network boundary

WireGuard does not configure public firewall policy for you. Keep management
services restricted to the overlay or another reviewed administrative network,
and independently verify firewall and cloud security-group rules before
exposure.

See [install.md](install.md) for hub setup and [secrets.md](secrets.md) for
credential handling.
