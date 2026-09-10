#!/bin/sh
set -eu

export FLEET_AUTH_HOST="${FLEET_AUTH_HOST:-fleet-console-auth.fleet-console.svc.cluster.local:18090}"
export FLEET_SITE_HOST="${FLEET_SITE_HOST:-fleet-site-controller.fleet-console.svc.cluster.local:18080}"

envsubst '${FLEET_AUTH_HOST} ${FLEET_SITE_HOST}' \
  < /etc/nginx/templates/default.conf.template \
  > /etc/nginx/conf.d/default.conf

exec nginx -g 'daemon off;'
