{{/*
Expand the name of the chart.
*/}}
{{- define "odh-observability.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "odh-observability.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "odh-observability.labels" -}}
helm.sh/chart: {{ include "odh-observability.chart" . }}
{{ include "odh-observability.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "odh-observability.selectorLabels" -}}
app.kubernetes.io/name: {{ include "odh-observability.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Chart label
*/}}
{{- define "odh-observability.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Namespace where Monitoring operands (Tempo, collector) are deployed.
Tempo write ACL treats this namespace name as the RBAC resource.
Falls back to operatorNamespace when monitoringNamespace is unset.
*/}}
{{- define "odh-observability.monitoringNamespace" -}}
{{- default .Values.operatorNamespace .Values.monitoringNamespace }}
{{- end }}
