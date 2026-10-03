#!/usr/bin/env bash
# Generate inventory/group_vars/all/vault.yml from docs/vault.example.yml with random secrets.
# Omits optional per-cluster LLDAP and Gitea keys (add later from vault.example.yml).
#
# Usage:
#   ./scripts/generate-vault-secrets.sh
#   ./scripts/generate-vault-secrets.sh --encrypt
#   ./scripts/generate-vault-secrets.sh --encrypt --vault-pass-file ~/.umkim-vault-pass
#   ./scripts/generate-vault-secrets.sh --force --encrypt
#
# Does not commit vault.yml (gitignored). Store ~/.umkim-vault-pass offline after --encrypt.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TEMPLATE="${REPO_ROOT}/docs/vault.example.yml"
VAULT_FILE="${VAULT_FILE:-${REPO_ROOT}/inventory/group_vars/all/vault.yml}"
VAULT_PASS_FILE="${VAULT_PASS_FILE:-${HOME}/.umkim-vault-pass}"
FORCE=0
ENCRYPT=0

usage() {
  sed -n '2,11p' "$0" | sed 's/^# \{0,1\}//'
  exit "${1:-0}"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help) usage 0 ;;
    -f|--force) FORCE=1; shift ;;
    -e|--encrypt) ENCRYPT=1; shift ;;
    --vault-pass-file)
      [[ $# -ge 2 ]] || { echo "missing value for --vault-pass-file" >&2; exit 1; }
      VAULT_PASS_FILE="$2"
      shift 2
      ;;
    --vault-file)
      [[ $# -ge 2 ]] || { echo "missing value for --vault-file" >&2; exit 1; }
      VAULT_FILE="$2"
      shift 2
      ;;
    *) echo "unknown option: $1" >&2; usage 1 ;;
  esac
done

if [[ ! -f "$TEMPLATE" ]]; then
  echo "[generate-vault-secrets] missing template: $TEMPLATE" >&2
  exit 1
fi

if [[ -f "$VAULT_FILE" && "$FORCE" -ne 1 ]]; then
  echo "[generate-vault-secrets] $VAULT_FILE already exists (use --force to overwrite)" >&2
  exit 1
fi

rand_alnum() {
  local n="${1:?}" out
  # head closes the pipe early; ignore SIGPIPE from tr under pipefail.
  out="$(set +o pipefail
    LC_ALL=C tr -dc 'A-Za-z0-9' < /dev/urandom | head -c "$n")"
  [[ ${#out} -eq "$n" ]] || {
    echo "[generate-vault-secrets] FAIL: could not generate ${n}-char secret" >&2
    return 1
  }
  printf '%s' "$out"
}

rand_hex() {
  local nbytes="${1:?}"
  openssl rand -hex "$nbytes"
}

# Authelia storage encryption key: exactly 32 characters (see vault.example.yml).
rand_exact32() {
  rand_alnum 32
}

set_yaml() {
  local key="$1" value="$2"
  if grep -q "^${key}:" "$VAULT_FILE"; then
    sed -i "s|^${key}:.*|${key}: \"${value}\"|" "$VAULT_FILE"
  else
    echo "[generate-vault-secrets] WARN key not in template: $key" >&2
  fi
}

mkdir -p "$(dirname "$VAULT_FILE")"
cp "$TEMPLATE" "$VAULT_FILE"

# Hub greenfield secrets only — drop optional catalog / per-cluster examples.
sed -i \
  -e '/^# Per-cluster LLDAP/,/^$/d' \
  -e '/^# Gitea catalog/,/^fleet_gitea_admin_password_vault:/d' \
  "$VAULT_FILE"

set_yaml fleet_lldap_jwt_secret_vault "$(rand_hex 32)"
set_yaml fleet_lldap_admin_password_vault "$(rand_alnum 24)"
set_yaml fleet_console_auth_session_secret_vault "$(rand_alnum 48)"
set_yaml fleet_console_auth_admin_password_vault "$(rand_alnum 24)"
set_yaml fleet_ingress_auth_bootstrap_password_vault "$(rand_alnum 24)"
set_yaml fleet_ingress_auth_session_secret_vault "$(rand_alnum 48)"
set_yaml fleet_ingress_auth_storage_key_vault "$(rand_exact32)"
set_yaml fleet_ingress_auth_reset_jwt_secret_vault "$(rand_hex 32)"
set_yaml fleet_ingress_auth_lldap_jwt_secret_vault "$(rand_hex 32)"
set_yaml harbor_admin_password_vault "$(rand_alnum 24)"
set_yaml fleet_velero_minio_access_key_vault "$(rand_alnum 20)"
set_yaml fleet_velero_minio_secret_key_vault "$(rand_alnum 40)"
set_yaml fleet_velero_repo_password_vault "$(rand_alnum 32)"

if grep -qE 'REPLACE_ME|change-me|ChangeMe' "$VAULT_FILE"; then
  echo "[generate-vault-secrets] FAIL: placeholders remain in $VAULT_FILE" >&2
  exit 1
fi

echo "[generate-vault-secrets] wrote $VAULT_FILE (plaintext)"

if [[ "$ENCRYPT" -eq 1 ]]; then
  if ! command -v ansible-vault >/dev/null 2>&1; then
    echo "[generate-vault-secrets] FAIL: ansible-vault not found" >&2
    exit 1
  fi
  if [[ ! -f "$VAULT_PASS_FILE" ]]; then
    rand_alnum 32 > "$VAULT_PASS_FILE"
    chmod 600 "$VAULT_PASS_FILE"
    echo "[generate-vault-secrets] created vault password file: $VAULT_PASS_FILE"
  fi
  if head -1 "$VAULT_FILE" | grep -q '^\$ANSIBLE_VAULT'; then
    echo "[generate-vault-secrets] already encrypted: $VAULT_FILE"
  else
    # Avoid "vault-ids default,default" when ANSIBLE_VAULT_PASSWORD_FILE is also set.
    env -u ANSIBLE_VAULT_PASSWORD_FILE -u ANSIBLE_VAULT_IDENTITY_LIST \
      ansible-vault encrypt "$VAULT_FILE" \
      --vault-password-file "$VAULT_PASS_FILE" \
      --encrypt-vault-id default
    echo "[generate-vault-secrets] encrypted $VAULT_FILE"
  fi
  echo "[generate-vault-secrets] export ANSIBLE_VAULT_PASSWORD_FILE=$VAULT_PASS_FILE"
else
  echo "[generate-vault-secrets] next: ansible-vault encrypt $VAULT_FILE --vault-password-file $VAULT_PASS_FILE --encrypt-vault-id default"
  echo "[generate-vault-secrets] or re-run with --encrypt"
fi

echo "[generate-vault-secrets] done"
