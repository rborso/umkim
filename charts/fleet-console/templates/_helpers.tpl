{{- define "fleet-console.namespace" -}}
{{- .Values.namespace | default "fleet-console" }}
{{- end }}

{{- define "fleet-console.authImage" -}}
{{- $p := .Values.registry.prefix -}}
{{- if $p -}}
{{ printf "%s/%s:%s" $p .Values.auth.image.repository .Values.auth.image.tag }}
{{- else -}}
{{ printf "%s:%s" .Values.auth.image.repository .Values.auth.image.tag }}
{{- end -}}
{{- end }}

{{- define "fleet-console.dashboardImage" -}}
{{- $p := .Values.registry.prefix -}}
{{- if $p -}}
{{ printf "%s/%s:%s" $p .Values.dashboard.image.repository .Values.dashboard.image.tag }}
{{- else -}}
{{ printf "%s:%s" .Values.dashboard.image.repository .Values.dashboard.image.tag }}
{{- end -}}
{{- end }}
