# Installing Immich and Paperless-ngx

Both apps ship as UMKIM-tailored, self-contained Helm charts with a post-install
bootstrap: it creates the local admin, seeds defaults, and (optionally) wires SSO
against the hub directory — Authelia as the OIDC provider over LLDAP. UMKIM owns
routing, TLS, and the Authelia OIDC client; the chart only creates workloads and
Services.

| App | Chart | Source | Workloads | Route target |
| --- | --- | --- | --- | --- |
| Immich | `immich` | `rborso/immich-helm` | `immich-server` + vectorchord Postgres + valkey | `immich-server:2283` |
| Paperless-ngx | `paperless` | `rborso/paperless-helm` | `paperless-web` (web + consumer + scheduler) + Postgres + valkey | `paperless-web:8000` |

Both bootstraps are idempotent and re-run on Helm upgrade (post-install/upgrade
hooks), so changing values and re-applying is safe.

Everything below is driven by the **console** (Apps) or the **`umkim` CLI**. Use
placeholder values and substitute your own: `<cluster>` is a Ready cluster name,
`<hub-domain>` / `<site-domain>` are your hub/site DNS suffixes, and
`<namespace>` is the target namespace.

## 0. Prerequisites

- A **Ready** cluster: console **Clusters**, or `umkim cluster list`.
- The chart in the hub **library** (`umkim chart list`). Seed it once per chart
  from the private repo (console **Apps → Library → Add**, or
  `umkim chart add --origin oci://<chart-repo>`); see
  [fleet-apps.md](components/fleet-apps.md) and `examples/umkim/chart-add-oci.json`.
- For SSO: directory auth enabled for the cluster (Authelia + LLDAP). OIDC is
  convenient but optional — both apps also work with the local bootstrap admin.
- A route decision: **hub** (`<app>.<hub-domain>`, published by the hub gateway),
  **site** (`<app>.<site-domain>`, published by the spoke ingress), or **none**.
- If you want a tenant gate, create the directory tenant first — see
  [hub-tenant-app-deploy.md](hub-tenant-app-deploy.md) (`umkim tenant create …`).

## 1. Install from the console

### 1.1 Check the library

**Apps → Library** should list `immich` and `paperless` with a `Ready` phase.
If a chart is missing, use **Add chart** (OCI URL, HTTP `.tgz`, or upload) — the
same as `umkim chart add`.

### 1.2 Open the install dialog

**Apps → Installed → Install Helm chart**. Fill the form:

| Field | Immich | Paperless |
| --- | --- | --- |
| Cluster | `<cluster>` | `<cluster>` |
| Chart | `immich` (library pick) | `paperless` (library pick) |
| Version | chart version from the library | chart version from the library |
| Release name | `immich` | `paperless` |
| Namespace | `immich` | `paperless` |
| Route mode | `hub` or `site` | `hub` or `site` |
| Hostname | `immich.<domain>` | `paperless.<domain>` |
| Service name | `immich-server` | `paperless-web` |
| Service port | `2283` | `8000` |
| Require fleet ingress auth | on for the Authelia gate | on for the Authelia gate |
| Tenant groups | the directory group(s) allowed (optional) | same |

The service name/port must match the chart's Service — hub/site routing proxies
to it directly.

### 1.3 Values

Paste into **Values (YAML)**. `${…}` tokens are resolved by UMKIM (see §3).

Immich:

```yaml
bootstrap:
  oauth:
    enabled: true
    issuerUrl: ${OIDC_ISSUER_URL}
    clientId: ${OIDC_CLIENT_ID}
    clientSecret: ${OIDC_CLIENT_SECRET}
    tokenEndpointAuthMethod: client_secret_basic
    buttonText: "Sign in with SSO"
immich:
  externalUrl: https://${APP_HOST}
ingress:
  enabled: true
  host: ${APP_HOST}
  forwardAuth: ${FORWARD_AUTH}
```

Paperless-ngx:

```yaml
externalUrl: https://${APP_HOST}
oidc:
  enabled: true
  issuerUrl: ${OIDC_ISSUER_URL}/.well-known/openid-configuration
  clientId: ${OIDC_CLIENT_ID}
  clientSecret: ${OIDC_CLIENT_SECRET}
  tokenAuthMethod: client_secret_basic
ingress:
  enabled: true
  host: ${APP_HOST}
  forwardAuth: ${FORWARD_AUTH}
```

### 1.4 OIDC client

Tick **Create an Authelia OIDC client for this app** and fill the defaults the
chart publishes (they pre-fill from the chart's `umkim.oidc` block):

| | Immich | Paperless |
| --- | --- | --- |
| Client id | `immich` | `paperless` |
| Redirect paths | `/auth/login`, `/user-settings` | `/accounts/oidc/authelia/login/callback/` |
| Extra redirect URIs | `app.immich:///oauth-callback` | — |
| Require PKCE | on | off (django-allauth sends no `code_challenge`) |

Click **Install**. The app appears under **Apps → Installed** and moves to
`Ready` when the Helm release and the bootstrap Job have both finished.

## 2. Install from the CLI

```bash
umkim login -u <admin> -p <password>
umkim whoami
umkim cluster list          # confirm <cluster> is Ready
umkim chart list            # confirm immich / paperless are in the library
```

### 2.1 Immich

```bash
umkim app create --name immich --cluster <cluster> \
  --chart immich --version <chart-version> \
  --release immich --namespace immich \
  --route-mode hub --route-host immich.<hub-domain> \
  --require-auth \
  --service-name immich-server --service-port 2283 \
  --values-file immich-values.yaml \
  --oidc-client '{"clientId":"immich","clientName":"Immich","redirectPaths":["/auth/login","/user-settings"],"extraRedirectUris":["app.immich:///oauth-callback"]}'
```

### 2.2 Paperless-ngx

```bash
umkim app create --name paperless --cluster <cluster> \
  --chart paperless --version <chart-version> \
  --release paperless --namespace paperless \
  --route-mode hub --route-host paperless.<hub-domain> \
  --require-auth \
  --service-name paperless-web --service-port 8000 \
  --values-file paperless-values.yaml \
  --oidc-client '{"clientId":"paperless","clientName":"Paperless","redirectPaths":["/accounts/oidc/authelia/login/callback/"],"requirePkce":false}'
```

`immich-values.yaml` / `paperless-values.yaml` are the YAML from §1.3.
`--chart` also accepts a full OCI reference:
`--chart oci://<harbor>/charts/immich`.

### 2.3 Equivalent JSON

`umkim app create -f <file>` takes the same fields as one object. See
[examples/umkim/app-create-immich.json](../examples/umkim/app-create-immich.json)
and
[examples/umkim/app-create-paperless.json](../examples/umkim/app-create-paperless.json).
`valuesYaml` is an embedded YAML string.

## 3. What UMKIM resolves for you

The FleetApp controller substitutes these tokens in the app values before the
Helm install, so no secret is written to the app spec:

| Token | Resolved to |
| --- | --- |
| `${APP_HOST}` | host of the app's first route |
| `${FORWARD_AUTH}` | `true` when any route sets `requireAuth` |
| `${FORWARD_AUTH_MIDDLEWARE}` | the Authelia forward-auth middleware name |
| `${OIDC_ISSUER_URL}` | the **cluster** Authelia OIDC issuer URL |
| `${OIDC_CLIENT_ID}` | the app's registered client id |
| `${OIDC_CLIENT_SECRET}` | the generated client secret for that id |

An `oidcClient` in the app spec registers the Authelia client (redirect URIs are
derived from the route host) and stores the secret for the substitution above.
With `requireAuth`, the route is additionally chained through Authelia
forward-auth as a "first line defense" ahead of the app's own login.

## 4. Verify

```bash
umkim app list
umkim app get immich
umkim app get paperless
```

Then, on the target cluster:

```bash
# Workloads and the bootstrap Job
kubectl -n immich get deploy,svc,ingress,jobs
kubectl -n paperless get deploy,svc,ingress,jobs

# Bootstrap result (admin created, SSO wired)
kubectl -n immich logs job/immich-bootstrap --tail=20
kubectl -n paperless logs job/paperless-bootstrap --tail=20

# App health
kubectl -n immich exec deploy/immich-server -- curl -s localhost:2283/api/server/ping   # {"res":"pong"}
kubectl -n paperless exec deploy/paperless-web -- curl -s -o /dev/null -w '%{http_code}\n' localhost:8000/
```

Expected:

- Both FleetApps report `Ready` with `Helm release <name> installed on <cluster>`.
- The bootstrap Job is `Complete`; its log ends with `bootstrap complete`.
- The app host redirects unauthenticated browsers:
  `curl -sk -o /dev/null -w '%{http_code} %{redirect_url}\n' https://<app-host>/`
  → `302 https://<auth-host>/?rd=…` when `requireAuth` is on.
- With SSO on, a directory user can complete the OIDC login; Immich/Paperless
  also accept the bootstrap admin.

### Credentials

The charts generate credentials on first install and store them in a release
Secret, so they survive upgrades:

```bash
kubectl -n immich get secret immich-secrets -o jsonpath='{.data.admin-password}' | base64 -d
kubectl -n paperless get secret paperless-secrets -o jsonpath='{.data.admin-password}' | base64 -d
```

## 5. Update

Change the version and/or values, then re-apply — UMKIM runs `helm upgrade
--install`, and the bootstrap hook re-runs after the upgrade:

```bash
umkim app update immich --version <new-version> --values-file immich-values.yaml
```

The console equivalent: **Apps → Installed →** row actions **→ Edit**.

## 6. Uninstall

```bash
umkim app delete immich paperless
```

Console: **Apps → Installed →** row actions **→ Uninstall**. This removes the
Helm release; PVCs and the namespace are left behind, so delete them explicitly
when you want the storage back.

## 7. Troubleshooting

- **App stuck `Installing`.** Check the FleetApp status and the release:
  `umkim app get <name>` and `kubectl -n fleet-console get fleetapp <name> -o yaml`.
  The most common cause is a chart the library cannot resolve, or a Pending PVC.
- **Bootstrap Job failed.** Read its log (above). It waits up to its
  `activeDeadlineSeconds` for the app's health endpoint; a CrashLooping app or a
  slow image pull shows up here first. Fix the app, then re-run the hook with an
  `umkim app update` (or delete/re-create the Job).
- **OIDC login fails.** Confirm the client id and redirect paths match the app
  values, and that `${OIDC_ISSUER_URL}` resolved to a host the app pod can reach.
  Immich auto-adds a `hostAliases` entry mapping the issuer host to the
  in-cluster ingress; if you set `hostAliases` yourself, the explicit value wins.
- **403 after SSO.** The directory user is not in the route's tenant group —
  add them (`umkim tenant add-member <tenant> <user>`) or clear the group
  restriction on the route.
- **PKCE.** Immich requires PKCE (default on); Paperless via django-allauth does
  not — set `requirePkce: false` or its login callback fails.

## Related

- [fleet-apps.md](components/fleet-apps.md) — FleetApp/FleetChart mechanics.
- [hub-tenant-app-deploy.md](hub-tenant-app-deploy.md) — directory tenant and
  hub-routed, tenant-gated apps end to end.
- [umkim-cli.md](components/umkim-cli.md) — full CLI reference.
- Chart sources: `rborso/immich-helm`, `rborso/paperless-helm`.
