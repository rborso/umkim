{{- define "pet-clinic-helm.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "pet-clinic-helm.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "pet-clinic-helm.labels" -}}
app.kubernetes.io/name: {{ include "pet-clinic-helm.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end -}}

{{- define "pet-clinic-helm.postgresHost" -}}
{{- printf "%s-postgres" .Release.Name -}}
{{- end -}}

{{- define "pet-clinic-helm.appHost" -}}
{{- printf "%s-app" .Release.Name -}}
{{- end -}}
