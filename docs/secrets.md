# Secrets

> Status: v0.2 is an experimental proof of concept. Operators remain
> responsible for the confidentiality, rotation, and recovery of all secrets.

Hub install requires an encrypted Ansible Vault. Start from
[`vault.example.yml`](vault.example.yml), keep the resulting vault file outside
version control, and protect the vault password with an approved secret
manager.

## What belongs in the vault

Store control-plane credentials such as:

- directory and console session secrets;
- break-glass authentication credentials;
- Harbor credentials;
- ingress-auth material; and
- backup repository credentials.

Do not place live values in inventory defaults, Markdown, issue trackers, CI
logs, shell history, or image build arguments.

## Runtime application secrets

Helm values that contain passwords should be supplied through `FleetApp`
`valuesYaml` only over the authenticated console or API. Prefer binding
secrets on the spoke rather than leaving plaintext in the FleetApp object.

Do not handcraft API requests containing application passwords unless they are
sent by an approved secret-aware client over an authenticated channel. Never
copy a Kubernetes Secret value into a command, example, or ticket.

## Operations

1. Generate or obtain high-entropy values in an approved secret manager.
2. Encrypt the vault before use and restrict access to the vault password.
3. Run `./scripts/preflight-secrets.sh` (or `./scripts/preflight-v1.sh`, which
   includes it) before deployment. Preflight currently validates required vault
   keys such as `harbor_admin_password` and
   `fleet_console_auth_session_secret`; other keys are operator responsibility.
4. Rotate compromised, bootstrap, and break-glass credentials promptly.
5. Keep encrypted vault and recovery material in protected, off-repository
   backups.

The repository ignores the live vault file; only the example schema should be
tracked. Cluster inventory ConfigMaps do not carry the Harbor admin password.
Cluster provision Jobs read it from the hub Ansible secret
`fleet-site-hub-ansible-secrets` (mounted at `/secrets/hub/ansible_vars.yml`).
Spoke `registries.yaml` still contains that password because K3s uses it to
authenticate to Harbor. Spoke workload secrets remain on the cluster after hub
detach; hub backup does not include spoke etcd or workload secrets. See [install.md](install.md),
[backup-restore.md](backup-restore.md), and [fleet-console-auth.md](fleet-console-auth.md)
for related operational guidance.
