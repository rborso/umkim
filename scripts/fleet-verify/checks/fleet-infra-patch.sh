#!/usr/bin/env bash
set -euo pipefail
API="${FLEET_API:-http://127.0.0.1:18080/api/v1alpha1}"

curl -sf "${API}/ingress-auth" >/dev/null || { echo "FAIL GET ingress-auth"; exit 1; }
curl -sf "${API}/acme" >/dev/null || { echo "FAIL GET acme"; exit 1; }
curl -sf "${API}/llm" >/dev/null || { echo "FAIL GET llm"; exit 1; }
echo "OK fleet-infra-patch (singleton GET smoke)"
