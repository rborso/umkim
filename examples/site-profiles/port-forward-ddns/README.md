# Port-forward with dynamic WAN (DDNS)

Use when the site WAN address rotates and DNS (A/AAAA) tracks the current public IP.

- Set `wan_address_mode: dns` on the port_forward ingress row (or enable `wan_egress` without `expected_wan_ip` for auto inference).
- Do **not** store a stale public IP in `address`; document the DDNS target in `wan_egress.dns_target_note`.
- Router/cloud operator still configures port-forwards to cluster NodePorts.

See [../port-forward/](../port-forward/) for static WAN (`wan_address_mode: fixed`).

Snippet: [fleet_sites.yml.snippet](fleet_sites.yml.snippet)
