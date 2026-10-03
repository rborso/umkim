#!/usr/bin/env bash
# Session cookies for fleet-verify (admin + optional customer).
set -euo pipefail

FV_AUTH_ADMIN_JAR="${FV_AUTH_ADMIN_JAR:-${TMPDIR:-/tmp}/fleet-verify-admin.jar}"
FV_AUTH_CUSTOMER_JAR="${FV_AUTH_CUSTOMER_JAR:-${TMPDIR:-/tmp}/fleet-verify-customer.jar}"
FV_CONSOLE_URL="${FLEET_CONSOLE_URL:-http://127.0.0.1:30912}"
FV_AUTH_USER="${FLEET_AUTH_USER:-localadmin}"

fv_auth_load_password() {
  if [[ -n "${FLEET_AUTH_PASSWORD:-}" ]]; then
    return 0
  fi
  local vault="${FV_ROOT:-.}/inventory/group_vars/all/vault.yml"
  local pass_file="${ANSIBLE_VAULT_PASSWORD_FILE:-${HOME}/.umkim-vault-pass}"
  if [[ -f "${pass_file}" && -f "${vault}" ]]; then
    FLEET_AUTH_PASSWORD="$(ansible-vault view "${vault}" --vault-password-file "${pass_file}" 2>/dev/null \
      | awk -F'"' '/fleet_console_auth_admin_password_vault/{print $2; exit}' || true)"
  fi
  FLEET_AUTH_PASSWORD="${FLEET_AUTH_PASSWORD:-CHANGE_ME}"
  export FLEET_AUTH_PASSWORD
}

fv_auth_admin_login() {
  fv_auth_load_password
  rm -f "${FV_AUTH_ADMIN_JAR}"
  curl -sf -c "${FV_AUTH_ADMIN_JAR}" -X POST "${FV_CONSOLE_URL}/fleet/auth/v1alpha1/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"${FV_AUTH_USER}\",\"password\":\"${FLEET_AUTH_PASSWORD}\"}" >/dev/null
  export FLEET_AUTH_COOKIE_JAR="${FV_AUTH_ADMIN_JAR}"
  export FLEET_SITE_COOKIE_JAR="${FV_AUTH_ADMIN_JAR}"
}

fv_auth_customer_login() {
  local user="${FLEET_CUSTOMER_USER:-}"
  local pass="${FLEET_CUSTOMER_PASSWORD:-}"
  [[ -n "$user" && -n "$pass" ]] || return 1
  rm -f "${FV_AUTH_CUSTOMER_JAR}"
  curl -sf -c "${FV_AUTH_CUSTOMER_JAR}" -X POST "${FV_CONSOLE_URL}/fleet/auth/v1alpha1/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"${user}\",\"password\":\"${pass}\"}" >/dev/null
}
