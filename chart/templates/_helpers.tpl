{{- define "games-manager.name" -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "games-manager.selectorLabels" -}}
app.kubernetes.io/name: games-manager
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "games-manager.labels" -}}
{{ include "games-manager.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}
