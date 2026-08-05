#!/usr/bin/env bash
# Resolve playwright CLI for fleet-verify (npx optional).
set -euo pipefail

fv_playwright_bin() {
  local ui_dir="$1"
  if command -v npx >/dev/null 2>&1; then
    echo "npx playwright"
    return 0
  fi
  if [[ -x "${ui_dir}/node_modules/.bin/playwright" ]]; then
    echo "${ui_dir}/node_modules/.bin/playwright"
    return 0
  fi
  return 1
}

fv_playwright_chromium_bin() {
  compgen -G "${HOME}/.cache/ms-playwright/chromium_headless_shell-"*/chrome-headless-shell-linux64/chrome-headless-shell 2>/dev/null \
    | head -1 || true
}

fv_playwright_ensure_chromium() {
  local ui_dir="$1"
  local bin
  bin="$(fv_playwright_bin "$ui_dir")" || return 127
  if compgen -G "${HOME}/.cache/ms-playwright/chromium"* >/dev/null 2>&1 \
    || compgen -G "${HOME}/.cache/ms-playwright/chromium_headless_shell"* >/dev/null 2>&1; then
    return 0
  fi
  echo "playwright: installing chromium (first run)..."
  (cd "$ui_dir" && $bin install chromium)
}

fv_playwright_runtime_ok() {
  local ui_dir="$1"
  local shell
  shell="$(fv_playwright_chromium_bin)"
  if [[ -z "$shell" ]]; then
    return 1
  fi
  if "$shell" --version >/dev/null 2>&1; then
    return 0
  fi
  echo "SKIP: chromium cannot launch (missing OS libs, e.g. libatk); install deps:" >&2
  echo "  cd ${ui_dir} && npx playwright install-deps chromium" >&2
  return 2
}

fv_playwright_test() {
  local ui_dir="$1"
  shift
  local bin rc=0
  bin="$(fv_playwright_bin "$ui_dir")" || {
    echo "FAIL: playwright not found (install ui deps: cd ui/dcm-fleet-console && npm ci)" >&2
    return 127
  }
  fv_playwright_ensure_chromium "$ui_dir" || {
    echo "SKIP: playwright chromium unavailable (run: cd ui/dcm-fleet-console && npx playwright install chromium)" >&2
    return 0
  }
  fv_playwright_runtime_ok "$ui_dir" || rc=$?
  if [[ "$rc" == 2 ]]; then
    return 0
  fi
  if [[ "$rc" != 0 ]]; then
    return "$rc"
  fi
  (cd "$ui_dir" && $bin test -c e2e/playwright.config.ts "$@") || rc=$?
  return "$rc"
}
