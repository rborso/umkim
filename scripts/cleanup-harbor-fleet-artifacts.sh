#!/usr/bin/env bash
# Delete untagged Harbor artifacts in fleet project (keep tagged :main digests).
set -euo pipefail

HARBOR="${HARBOR_URL:-https://10.10.0.1:30443}"
USER="${HARBOR_USER:-admin}"
PASS="${HARBOR_PASS:-CHANGE_ME}"
PROJECT="${HARBOR_PROJECT:-fleet}"

export HARBOR_URL="$HARBOR" HARBOR_USER="$USER" HARBOR_PASS="$PASS" HARBOR_PROJECT="$PROJECT"

repos="${*:-fleet-site-controller umkim-console fleet-console-auth fleet-ansible-runner}"

for repo in $repos; do
  echo "[harbor-cleanup] $PROJECT/$repo"
  curl -sk -u "$USER:$PASS" \
    "$HARBOR/api/v2.0/projects/$PROJECT/repositories/$repo/artifacts?page_size=100" \
    | python3 -c "
import json, sys, urllib.request, base64, os, ssl
repo = sys.argv[1]
arts = json.load(sys.stdin)
harbor = os.environ['HARBOR_URL']
user = os.environ['HARBOR_USER']
passwd = os.environ['HARBOR_PASS']
project = os.environ['HARBOR_PROJECT']
ctx = ssl.create_default_context()
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE
auth = base64.b64encode(f'{user}:{passwd}'.encode()).decode()
for a in arts:
    tags = a.get('tags') or []
    if tags:
        continue
    ref = a['digest']
    url = f'{harbor}/api/v2.0/projects/{project}/repositories/{repo}/artifacts/{ref}'
    req = urllib.request.Request(url, method='DELETE')
    req.add_header('Authorization', f'Basic {auth}')
    try:
        urllib.request.urlopen(req, context=ctx)
        print(f'  deleted untagged {ref[:24]}')
    except Exception as e:
        print(f'  skip {ref[:24]}: {e}')
" "$repo"
done
echo "[harbor-cleanup] done"
