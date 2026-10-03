# Contributing

Keep changes small, source-backed, and safe to review.

## Before opening a pull request

- Never commit Vault files, private keys, live inventory, WAN addresses, or
  personal domains.
- Use `example.com` and RFC 5737 addresses in public examples.
- Update public documentation when behavior changes; keep one canonical
  explanation rather than duplicating a runbook. Operator docs must match v0.2
  (Rocky Linux 10, FleetApp, no DCM catalog, per-site exposure modes).
- When changing lifecycle, ingress, or routing, update [docs/design.md](docs/design.md)
  and the relevant component guide. Do not document `route.mode: dual`.
- Prefer scoped Ansible tags to a full deployment when validating a narrow
  change.
- Review [SECURITY.md](SECURITY.md) for release-sensitive changes.

For hub install preflight, use:

```bash
./scripts/preflight-v1.sh
```

Repository-specific contributor and agent workflow guidance is in
[AGENTS.md](AGENTS.md).

## Developer Certificate of Origin

By contributing, you certify that you have the right to submit the work under
the [Developer Certificate of Origin 1.1](https://developercertificate.org/).
Sign commits with:

```bash
git commit -s -m "Your message"
```

Contributions are licensed under [Apache License 2.0](LICENSE).
