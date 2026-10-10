{{- define "fleet-ingress-auth.namespace" -}}
{{- .Values.namespace | default "fleet-ingress-auth" -}}
{{- end -}}

{{- define "fleet-ingress-auth.protectedIngressMiddlewares" -}}
{{- $t := .Values.traefik -}}
{{- printf "%s@file,%s@file" ($t.middlewareName | default "fleet-ingress-authelia") ($t.stripAuthorizationMiddlewareName | default "strip-authorization") -}}
{{- end -}}

{{- define "fleet-ingress-auth.image" -}}
{{- $repo := .repository -}}
{{- if .Values.registry.prefix -}}
{{- printf "%s/%s:%s" .Values.registry.prefix $repo .tag -}}
{{- else -}}
{{- printf "%s:%s" $repo .tag -}}
{{- end -}}
{{- end -}}

{{- define "fleet-ingress-auth.inviteSignupNginx" -}}
server {
  listen 8080;
  location = /invite/redeem {
    proxy_pass {{ .Values.signup.hubRedeemURL }};
    proxy_http_version 1.1;
    proxy_set_header Content-Type application/json;
  }
  location /invite {
    root /usr/share/nginx/html;
    try_files /index.html =404;
  }
  location / {
    return 302 /invite;
  }
}
{{- end -}}

{{- define "fleet-ingress-auth.inviteSignupIndex" -}}
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Accept invitation</title>
<style>
body{font-family:system-ui,sans-serif;max-width:28rem;margin:3rem auto;padding:0 1rem;color:#151515}
label{display:block;margin:.75rem 0 .25rem;font-weight:600}
input{width:100%;padding:.5rem;box-sizing:border-box}
button{margin-top:1rem;padding:.6rem 1rem;font-weight:600}
button:disabled{opacity:.6;cursor:not-allowed}
.msg{margin-top:1rem;padding:.75rem;border-radius:.25rem}
.err{background:#faeae8;color:#a30000}
.ok{background:#e7f5e8;color:#1e7b34}
</style>
</head>
<body>
<h1>Accept invitation</h1>
<p>Create your account for the shared directory.</p>
<form id="f">
  <label>Username</label><input id="username" required pattern="[a-z0-9-]+">
  <label>Display name</label><input id="displayName">
  <label>Email</label><input id="email" type="email">
  <label>Password (min 8 characters)</label><input id="password" type="password" required>
  <label>Confirm password</label><input id="confirm" type="password" required>
  <button id="submit" type="submit">Create account</button>
</form>
<div id="msg"></div>
<script>
const params = new URLSearchParams(location.search);
const token = params.get('token') || '';
const msg = document.getElementById('msg');
const btn = document.getElementById('submit');
let busy = false;
document.getElementById('f').addEventListener('submit', async (e) => {
  e.preventDefault();
  if (busy) return;
  msg.className = 'msg'; msg.textContent = '';
  const username = document.getElementById('username').value.trim().toLowerCase();
  const password = document.getElementById('password').value;
  const confirm = document.getElementById('confirm').value;
  if (password.length < 8) { msg.className='msg err'; msg.textContent='Password must be at least 8 characters.'; return; }
  if (password !== confirm) { msg.className='msg err'; msg.textContent='Passwords do not match.'; return; }
  busy = true; btn.disabled = true; btn.textContent = 'Creating account…';
  msg.className='msg'; msg.textContent='Creating your account, this can take a few seconds…';
  try {
    const r = await fetch('/invite/redeem', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({token,username,password,displayName:document.getElementById('displayName').value,email:document.getElementById('email').value})});
    const text = await r.text();
    if (!r.ok) throw new Error(text || ('HTTP '+r.status));
    msg.className='msg ok'; msg.textContent='Account created: '+username+'. You can now sign in.';
  } catch (err) {
    msg.className='msg err'; msg.textContent=String(err.message||err);
  } finally {
    busy = false; btn.disabled = false; btn.textContent = 'Create account';
  }
});
if (!token) { msg.className='msg err'; msg.textContent='This link is missing its invite token.'; }
</script>
</body>
</html>
{{- end -}}

{{- define "fleet-ingress-auth.autheliaConfig" -}}
{{- $ldapPass := .Values.authelia.ldapPassword | default .Values.lldap.adminPassword -}}
theme: light
server:
  address: 'tcp://0.0.0.0:{{ .Values.authelia.port }}'
log:
  level: info
identity_validation:
  reset_password:
    jwt_secret: {{ .Values.authelia.resetPasswordJWTSecret | quote }}
authentication_backend:
  ldap:
    address: {{ .Values.lldap.externalUrl | default (printf "ldap://lldap.%s.svc.cluster.local:%v" (include "fleet-ingress-auth.namespace" .) .Values.lldap.ldapPort) | quote }}
    implementation: lldap
    base_dn: {{ .Values.lldap.baseDN | quote }}
    user: {{ .Values.authelia.ldapUser | default (printf "uid=admin,ou=people,%s" .Values.lldap.baseDN) | quote }}
    password: {{ $ldapPass | quote }}
{{- if .Values.authelia.oidc.enabled }}
identity_providers:
  oidc:
    hmac_secret: {{ .Values.authelia.oidc.hmacSecret | quote }}
    issuer_private_key: |
{{ .Values.authelia.oidc.issuerPrivateKey | indent 6 }}
    clients:
{{- range .Values.authelia.oidc.clients }}
      - client_id: {{ .clientId | quote }}
        client_name: {{ .clientName | default .clientId | quote }}
        client_secret: {{ .clientSecret | quote }}
        public: false
        authorization_policy: {{ .authorizationPolicy | default "one_factor" }}
        require_pkce: {{ .requirePkce | default true }}
        pkce_challenge_method: S256
        redirect_uris:
{{- range .redirectUris }}
          - {{ . | quote }}
{{- end }}
        scopes:
{{- range (.scopes | default (list "openid" "profile" "email")) }}
          - {{ . | quote }}
{{- end }}
        response_types:
          - code
        grant_types:
          - authorization_code
        consent_mode: {{ .consentMode | default "implicit" }}
        token_endpoint_auth_method: {{ .tokenEndpointAuthMethod | default "client_secret_basic" }}
{{- end }}
{{- end }}
access_control:
  default_policy: deny
  rules:
    - domain: {{ .Values.authelia.portalHost | quote }}
      policy: bypass
{{- range .Values.authelia.accessRules }}
    - domain: {{ .domain | quote }}
      subject:
{{- range .groups }}
        - {{ printf "group:%s" . | quote }}
{{- end }}
      policy: one_factor
    # Deny authenticated users who are not in the allowed groups; Authelia
    # skips a subject rule when it does not match, so an explicit deny must
    # precede the zone wildcard rules.
    - domain: {{ .domain | quote }}
      policy: deny
{{- end }}
{{- range .Values.authelia.accessDomains }}
    - domain: {{ . | quote }}
      policy: one_factor
{{- end }}
session:
  secret: {{ .Values.authelia.sessionSecret | quote }}
  cookies:
    - domain: {{ .Values.authelia.cookieDomain | quote }}
      name: authelia_session
      same_site: lax
      expiration: 8h
      inactivity: 30m
      authelia_url: {{ .Values.authelia.publicURL | quote }}
      default_redirection_url: {{ printf "https://%s" .Values.authelia.cookieDomain | quote }}
storage:
  encryption_key: {{ .Values.authelia.storageEncryptionKey | quote }}
  local:
    path: /data/db.sqlite3
notifier:
  filesystem:
    filename: /data/notification.txt
{{- end -}}
