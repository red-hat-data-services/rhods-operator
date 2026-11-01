{{/*
Expand the name of the chart.
*/}}
{{- define "xks-gateway.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "xks-gateway.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels applied to all resources.
*/}}
{{- define "xks-gateway.labels" -}}
helm.sh/chart: {{ include "xks-gateway.chart" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Resolve the gateway namespace.
*/}}
{{- define "xks-gateway.namespace" -}}
{{- .Values.gateway.namespace | default "rh-ai-gateway" }}
{{- end }}

{{/*
Resolve the namespace for the OIDC client secret (defaults to gateway.namespace).
*/}}
{{- define "xks-gateway.oidcSecretNamespace" -}}
{{- $gw := .Values.gateway -}}
{{- $gw.oidc.secretNamespace | default (include "xks-gateway.namespace" .) }}
{{- end }}

{{/*
Resolve the OIDC client secret name referenced by GatewayConfig (defaults when chart-managed).
*/}}
{{- define "xks-gateway.oidcClientSecretRefName" -}}
{{- $gw := .Values.gateway -}}
{{- if $gw.oidc.oidcClientSecret -}}
{{- $gw.oidc.clientSecretRef.name | default "oidc-client-secret" -}}
{{- else -}}
{{- $gw.oidc.clientSecretRef.name -}}
{{- end -}}
{{- end -}}

{{/*
True when the chart is enabled AND gateway configuration is provided (domain is set).
When used as a subchart with default empty values, all resource templates are skipped
and only the GatewayConfig CRD is installed.
*/}}
{{- define "xks-gateway.configured" -}}
{{- if and .Values.enabled .Values.gateway.domain -}}true{{- end -}}
{{- end -}}

{{/*
True when this chart creates the OIDC client secret (oidcClientSecret set and target namespace is gateway.namespace).
*/}}
{{- define "xks-gateway.oidcSecretManaged" -}}
{{- $gw := .Values.gateway -}}
{{- if $gw.oidc.oidcClientSecret -}}
{{- $secretNs := include "xks-gateway.oidcSecretNamespace" . -}}
{{- $gatewayNs := include "xks-gateway.namespace" . -}}
{{- if eq $secretNs $gatewayNs -}}true{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Render the GatewayConfig used by the lifecycle hook. The custom resource is
hook-managed so Helm never has to map it before the CRD becomes Established.
*/}}
{{- define "xks-gateway.gatewayConfigManifest" -}}
{{- $gw := .Values.gateway -}}
apiVersion: services.platform.opendatahub.io/v1alpha1
kind: GatewayConfig
metadata:
  # Name is enforced by CRD CEL validation: must be "default-gateway"
  name: default-gateway
  annotations:
    platform.opendatahub.io/gateway-config-owner: {{ printf "%s/%s" .Release.Namespace .Release.Name | quote }}
  labels:
    {{- include "xks-gateway.labels" . | nindent 4 }}
    app.kubernetes.io/instance: {{ .Release.Name }}
spec:
  domain: {{ $gw.domain | quote }}
  {{- with $gw.subdomain }}
  subdomain: {{ . | quote }}
  {{- end }}
  ingressMode: {{ $gw.ingressMode | default "LoadBalancer" }}
  certificate:
    type: {{ $gw.certificate.type | default "SelfSigned" }}
    {{- with $gw.certificate.secretName }}
    secretName: {{ . | quote }}
    {{- end }}
  oidc:
    issuerURL: {{ $gw.oidc.issuerURL | quote }}
    clientID: {{ $gw.oidc.clientID | quote }}
    clientSecretRef:
      name: {{ include "xks-gateway.oidcClientSecretRefName" . | quote }}
      key: {{ $gw.oidc.clientSecretRef.key | default "client-secret" | quote }}
    secretNamespace: {{ include "xks-gateway.oidcSecretNamespace" . | quote }}
  {{- if or $gw.cookie.expire $gw.cookie.refresh }}
  cookie:
    {{- with $gw.cookie.expire }}
    expire: {{ . | quote }}
    {{- end }}
    {{- with $gw.cookie.refresh }}
    refresh: {{ . | quote }}
    {{- end }}
  {{- end }}
  {{- with $gw.providerCASecretName }}
  providerCASecretName: {{ . | quote }}
  {{- end }}
  {{- if not (kindIs "invalid" $gw.verifyProviderCertificate) }}
  verifyProviderCertificate: {{ $gw.verifyProviderCertificate }}
  {{- end }}
  {{- if and $gw.networkPolicy $gw.networkPolicy.ingress }}
  networkPolicy:
    ingress:
      enabled: {{ $gw.networkPolicy.ingress.enabled }}
  {{- end }}
{{- end -}}
