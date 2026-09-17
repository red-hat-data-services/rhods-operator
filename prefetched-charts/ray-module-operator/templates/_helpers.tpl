{{- define "ray-module-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "ray-module-operator.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name (include "ray-module-operator.name" .) | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{- define "ray-module-operator.namespace" -}}
{{- default "ray-module-operator-system" .Values.operatorNamespace }}
{{- end }}

{{- define "ray-module-operator.labels" -}}
app.kubernetes.io/name: {{ include "ray-module-operator.name" . }}
app.kubernetes.io/managed-by: Helm
app.kubernetes.io/part-of: ray-module-operator
{{- end }}
