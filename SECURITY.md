# Security

UMKIM v0.2 is an experimental small-fleet reference stack. It is not
security-certified or production-supported. Operators are responsible for
reviewing every image, playbook, network rule, and secret before deployment.

## Security model

- Hub operators are trusted and hold SSH plus Ansible Vault access.
- The public network is untrusted. Use the hub firewall role and provider
  security groups to expose only the services you intentionally operate.
- The WireGuard overlay carries management traffic. Treat every overlay peer as
  privileged.
- A compromised hub can affect the fleet control plane.

The intended public ingress is SSH, WireGuard, and optionally HTTP/HTTPS for
public applications. Management APIs, Kubernetes, registry, and console
NodePorts should be reachable only through the management network. Verify the
rendered firewall configuration on every release: the current experimental LLM
path needs an explicit exposure review and must not be assumed WAN-blocked.

## Required operator controls

- Use an encrypted Ansible Vault for hub install; see
  [docs/secrets.md](docs/secrets.md).
- Use unique credentials and remove every `REPLACE_ME` value before deployment.
- Restrict SSH access and enable host-key checking.
- Keep the management plane behind WireGuard or an equivalent private network.
- Enable TLS and authenticated access before exposing an application.
- Test backup and restore independently; they are not a production
  data-protection guarantee.

## Secrets that must stay out of Git

- `inventory/group_vars/all/vault.yml` and its password file
- WireGuard private keys
- Harbor, session, directory, and application credentials
- live inventory values, public addresses, and personal DNS names

Commit only public templates such as [docs/vault.example.yml](docs/vault.example.yml).

## Reporting a vulnerability

Use a private GitHub Security Advisory for this repository:
<https://github.com/rborso/umkim/security/advisories/new>. Do not include
exploit details in a public issue. The link must be verified after the
repository is made publicly accessible.

## Release checks

Run these from a clean, sanitized worktree before a release:

```bash
./scripts/preflight-secrets.sh
./scripts/security-scan.sh --skip-images
```

Review [NOTICE](NOTICE) for distribution and third-party obligations.
