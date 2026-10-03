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
