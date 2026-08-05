# Secrets

> Status: v0.1.0 is an experimental proof of concept. Operators remain
> responsible for the confidentiality, rotation, and recovery of all secrets.

Production deployments require an encrypted Ansible Vault. Start from
[`vault.example.yml`](vault.example.yml), keep the resulting vault file outside
version control, and protect the vault password with an approved secret
manager.

## What belongs in the vault

Store control-plane credentials such as:

- directory and console session secrets;
- break-glass authentication credentials;
- DCM database and Harbor credentials;
- ingress-auth material; and
- backup repository credentials.

Do not place live values in inventory defaults, Markdown, issue trackers, CI
logs, shell history, or image build arguments.

## Runtime application secrets

Catalog fields marked `x-sensitive` and `x-secret-bind` are designed to become
spoke Kubernetes Secrets. Use the console or the supported catalog deployment
tooling so the deployment request references the Secret instead of retaining
plaintext in DCM catalog values.

Do not handcraft API requests containing application passwords unless they are
sent by an approved secret-aware client over an authenticated channel. Never
copy a Kubernetes Secret value into a command, example, or ticket.

## Operations

1. Generate or obtain high-entropy values in an approved secret manager.
2. Encrypt the vault before use and restrict access to the vault password.
3. Run the secret preflight check before deployment.
4. Rotate compromised, bootstrap, and break-glass credentials promptly.
5. Keep encrypted vault and recovery material in protected, off-repository
   backups.

The repository ignores the live vault file; only the example schema should be
tracked. See [install.md](install.md), [backup-restore.md](backup-restore.md),
and [fleet-console-auth.md](fleet-console-auth.md) for related operational
guidance.
