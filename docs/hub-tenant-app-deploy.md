# Hub tenant + hub-routed apps walkthrough

End-to-end walkthrough: create a hub directory tenant, add a user, and deploy
apps hub-routed, gated by Authelia and restricted to that tenant. Every step is
driven by the `umkim` CLI.

In this example the tenant is `newtenant`, the user is `newtest`, and the apps
are `immich`, `paperless`, and `servarr`.

## 0. Authenticate

```bash
umkim login -u <admin> -p <password>
umkim whoami
```

## 1. Remove any existing installs of the apps

```bash
umkim app delete immich paperless servarr
umkim app list
```

## 2. Create the tenant and its user

A tenant is a hub directory (LLDAP) group. `add-user` creates the directory
account with an initial password (through a one-time invite) and adds it to the
group.

```bash
umkim tenant create newtenant
umkim tenant add-user newtenant newtest --password 'Almafa12.' \
  --display-name 'New Test' --email 'newtest@fleet.local'
umkim tenant list
```

Add an account that already exists in the directory with `tenant add-member`:

```bash
umkim tenant add-member newtenant some-existing-user
```

## 3. Deploy the apps (hub-routed, tenant-scoped, Authelia-gated)

`--route-mode hub` publishes the app through the hub gateway
(`<host>.demo.otthol.hu`). `--require-auth` puts Authelia in front of it, and
`--require-auth-groups newtenant` restricts it to the tenant. `--service-name` /
`--service-port` point the hub route at the app's Service. `${APP_HOST}`,
`${FORWARD_AUTH}`, and `${OIDC_*}` placeholders are filled in by the controller.

### immich

```bash
umkim app create --name immich --cluster lab-ha-0 \
  --chart oci://10.10.0.1:30443/charts/immich --version 0.1.1 \
  --release immich --namespace immich \
  --route-mode hub --route-host immich.demo.otthol.hu \
  --require-auth --require-auth-groups newtenant \
  --service-name immich-server --service-port 2283 \
  --values-file immich-values.yaml \
  --oidc-client '{"clientId":"immich","clientName":"Immich","redirectPaths":["/auth/login","/user-settings"],"extraRedirectUris":["app.immich:///oauth-callback"]}'
```

`immich-values.yaml`:

```yaml
bootstrap:
  oauth:
    enabled: true
    issuerUrl: ${OIDC_ISSUER_URL}
    clientId: ${OIDC_CLIENT_ID}
    clientSecret: ${OIDC_CLIENT_SECRET}
    tokenEndpointAuthMethod: client_secret_basic
    buttonText: "Sign in with LLDAP"
immich:
  externalUrl: https://${APP_HOST}
ingress:
  enabled: true
  host: ${APP_HOST}
  forwardAuth: ${FORWARD_AUTH}
```

### paperless-ngx

```bash
umkim app create --name paperless --cluster lab-ha-0 \
  --chart oci://10.10.0.1:30443/charts/paperless --version 0.1.4 \
  --release paperless --namespace paperless \
  --route-mode hub --route-host paperless.demo.otthol.hu \
  --require-auth --require-auth-groups newtenant \
  --service-name paperless-web --service-port 8000 \
  --values-file paperless-values.yaml \
  --oidc-client '{"clientId":"paperless","clientName":"Paperless","redirectPaths":["/accounts/oidc/authelia/login/callback/"],"requirePkce":false}'
```

`paperless-values.yaml`:

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

### servarr

servarr is a multi-service release, so use a JSON body with one hub route per
sub-app (`routes[]` takes precedence over `route`).

```bash
umkim app create -f servarr-hub.json
```

`servarr-hub.json` (abridged — one route per sub-app, all identical in shape):

```json
{
  "name": "servarr",
  "clusterName": "rp-test",
  "chart": "servarr",
  "version": "0.2.9",
  "releaseName": "servarr",
  "namespace": "servarr",
  "valuesYaml": "bootstrap:\n  enabled: true\n  image: 10.10.0.1:30443/fleet/servarr-bootstrap:0.2\napps:\n  jellyfin:\n    ldap:\n      enabled: true\n      server: 10.10.0.1\n      port: 30389\n      bindUser: uid=svc-bind,ou=people,dc=otthol,dc=local\n      existingSecret: servarr-lldap-bind\n",
  "routes": [
    {"key": "sonarr", "mode": "hub", "hubHost": "sonarr.demo.otthol.hu", "requireAuth": true, "requireAuthGroups": ["newtenant"], "serviceName": "sonarr", "servicePort": 80}
  ]
}
```

Repeat the route object for `radarr`, `prowlarr`, `qbt` (service `qbittorrent`),
`jellyfin`, `seerr`, `lidarr`, `bazarr`, and `cleanuparr`.

The servarr Jellyfin LDAP plugin points at the **hub** LLDAP (`10.10.0.1:30389`,
the hub directory's LDAP NodePort over the overlay) so directory users can sign
in to Jellyfin. Create the bind secret first (from the hub service-account
secret) and reference it as `apps.jellyfin.ldap.existingSecret`:

```bash
HUB_BIND=$(kubectl -n fleet-console get secret lldap-service-accounts \
  -o jsonpath='{.data.bind-password}' | base64 -d)
kubectl -n servarr create secret generic servarr-lldap-bind \
  --from-literal=LLDAP_LDAP_USER_PASS="$HUB_BIND" \
  --dry-run=client -o yaml | kubectl apply -f -
```

## 4. Verify

```bash
umkim app list
umkim tenant list
```

- **Routing**: each app host returns `302` to the hub Authelia portal when
  unauthenticated.
- **Tenant gate**: signing in as `newtest` reaches the app; signing in as a user
  who is not in `newtenant` gets `403`.
- **Bootstrap**: each chart's post-install bootstrap creates the admin and wires
  SSO.
- **SSO**: immich and paperless sign in through Authelia OIDC over the hub
  directory; servarr's Jellyfin signs in through the LDAP plugin.

## Notes

- Hub-routed apps are proxied by the hub gateway to the spoke's `web-hub`
  entrypoint, so `--service-name` / `--service-port` must match the app's Service
  (the default `<release>:80` rarely does).
- The tenant group becomes an Authelia `subject: group:<tenant>` rule per app
  host, with an explicit deny so authenticated non-members are rejected.
- Removing the tenant: `umkim tenant delete newtenant` (fails while apps still
  reference it through access rules until the routes are removed).
