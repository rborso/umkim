# Hub-only inventory template

Use this template for the supported greenfield management installation. It
contains non-operational placeholders and must be completed locally before use.

## Start

From the repository root, run:

```bash
./scripts/bootstrap-hub-inventory.sh
```

The script copies this template into `inventory/`. Run it only when replacing
that inventory is intended, then follow the supported sequence in the
[installation guide](../../../docs/install.md).

## Review before installation

- Set the inventory host key and connection values.
- Set site, domain, endpoint, and DNS inputs for the deployment.
- Review group-level defaults and optional workload configuration.
- Supply encrypted vault data through the approved secret-handling process.
- Validate inventory before running an installation playbook.

Use the variable definitions and comments in the copied files as the current
contract. Do not infer values from this README or other examples.

## Safety

- Do not commit live addresses, domains, credentials, or generated vault files.
- Keep deployment-specific overrides outside shared templates unless they are
  intended to become a documented configuration contract.
- This template does not create external infrastructure or attach workload
  clusters; use the installation and component documentation for those paths.

## References

- [Installation guide](../../../docs/install.md)
- [Design guide](../../../docs/design.md)
- [Secrets guide](../../../docs/secrets.md)
- [Fleet site component](../../../docs/components/fleet-site.md)
